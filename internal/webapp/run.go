package webapp

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/chat"
	"github.com/Saieshwar5/ayati-code/internal/config"
	"github.com/Saieshwar5/ayati-code/internal/fireworks"
	"github.com/Saieshwar5/ayati-code/internal/githubapp"
	"github.com/Saieshwar5/ayati-code/internal/sandbox"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

const version = "dev"

func Run(ctx context.Context, args []string, output, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("ayati", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	address := flags.String("address", envOr("AYATI_ADDRESS", "127.0.0.1:8080"), "local web address")
	database := flags.String("database", "", "SQLite database path")
	dataRoot := flags.String("data-root", "", "workspace data directory")
	image := flags.String("sandbox-image", envOr("AYATI_SANDBOX_IMAGE", sandbox.DefaultImage), "workspace sandbox image")
	clientID := flags.String("github-client-id", os.Getenv("AYATI_GITHUB_CLIENT_ID"), "GitHub App client ID")
	clientSecret := flags.String("github-client-secret", os.Getenv("AYATI_GITHUB_CLIENT_SECRET"), "GitHub App client secret")
	callback := flags.String("github-callback-url", os.Getenv("AYATI_GITHUB_CALLBACK_URL"), "GitHub callback URL")
	showVersion := flags.Bool("version", false, "print the Ayati version")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(errorOutput, "ayati: unexpected arguments: %v\n", flags.Args())
		return 2
	}
	if *showVersion {
		fmt.Fprintf(output, "ayati %s\n", version)
		return 0
	}
	paths, err := resolvePaths(*database, *dataRoot)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	store, err := workspace.Open(paths.database)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	defer store.Close()
	environment, err := sandbox.New(*image)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	credentialPath, err := githubapp.DefaultCredentialsPath()
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	token := func() (string, error) {
		credentials, err := githubapp.LoadCredentials(credentialPath)
		return credentials.AccessToken, err
	}
	workspaces, err := workspace.NewService(store, environment, token, paths.workspaces)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	github, err := optionalGitHub(*clientID, *clientSecret, *callback, *address)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	conversation, err := optionalChat(store, workspaces)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	logger := log.New(errorOutput, "ayati: ", log.LstdFlags)
	application, err := New(Options{
		Context: ctx, Store: store, Workspaces: workspaces, Chat: conversation, GitHub: github,
		CredentialsPath: credentialPath, WorkspaceRoot: paths.workspaces, Logger: logger,
	})
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: listen on %s: %v\n", *address, err)
		return 1
	}
	server := &http.Server{
		Handler: application.Handler(), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second,
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	fmt.Fprintf(output, "Ayati is running at http://%s\n", listener.Addr())
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			fmt.Fprintf(errorOutput, "ayati: shutdown: %v\n", err)
			return 1
		}
		return 0
	case err := <-finished:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(errorOutput, "ayati: serve: %v\n", err)
			return 1
		}
		return 0
	}
}

func optionalChat(store *workspace.Store, runtime *workspace.Service) (*chat.Service, error) {
	path, err := config.DefaultPath()
	if err != nil {
		return nil, err
	}
	values, err := config.Load(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	provider, err := fireworks.New(values.FireworksAPIKey)
	if err != nil {
		return nil, err
	}
	return chat.New(store, runtime, provider, values.Model)
}

type paths struct{ database, workspaces string }

func resolvePaths(database, root string) (paths, error) {
	if strings.TrimSpace(database) == "" {
		value, err := workspace.DefaultPath()
		if err != nil {
			return paths{}, err
		}
		database = value
	}
	if strings.TrimSpace(root) == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return paths{}, fmt.Errorf("resolve home directory: %w", err)
		}
		root = filepath.Join(home, ".local", "share", "ayati", "workspaces")
	}
	return paths{database: database, workspaces: root}, nil
}

func optionalGitHub(clientID, secret, callback, address string) (*githubapp.Client, error) {
	clientID, secret = strings.TrimSpace(clientID), strings.TrimSpace(secret)
	if clientID == "" && secret == "" {
		return nil, nil
	}
	if strings.TrimSpace(callback) == "" {
		callback = "http://" + address + "/auth/github/callback"
	}
	return githubapp.New(clientID, secret, callback)
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
