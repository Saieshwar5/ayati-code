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

	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
	"github.com/Saieshwar5/perpetual/internal/githubapp"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

const version = "dev"

func Run(ctx context.Context, args []string, output, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("perpetual", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	address := flags.String("address", envOr("PERPETUAL_ADDRESS", "127.0.0.1:8080"), "local web address")
	databasePath := flags.String("database", "", "SQLite database path")
	dataRoot := flags.String("data-root", "", "workspace data directory")
	clientID := flags.String("github-client-id", os.Getenv("PERPETUAL_GITHUB_CLIENT_ID"), "GitHub App client ID")
	clientSecret := flags.String("github-client-secret", os.Getenv("PERPETUAL_GITHUB_CLIENT_SECRET"), "GitHub App client secret")
	callback := flags.String("github-callback-url", os.Getenv("PERPETUAL_GITHUB_CALLBACK_URL"), "GitHub callback URL")
	showVersion := flags.Bool("version", false, "print the Perpetual version")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(errorOutput, "perpetual: unexpected arguments: %v\n", flags.Args())
		return 2
	}
	if *showVersion {
		fmt.Fprintf(output, "perpetual %s\n", version)
		return 0
	}
	paths, err := resolvePaths(*databasePath, *dataRoot)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	database, err := appdatabase.Open(paths.database)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	defer database.Close()
	store, err := workspace.NewStore(database)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	credentialPath, err := githubapp.DefaultCredentialsPath()
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	token := func() (string, error) {
		credentials, err := githubapp.LoadCredentials(credentialPath)
		return credentials.AccessToken, err
	}
	workspaces, err := workspace.NewService(store, token, paths.workspaces)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	if err := workspaces.Recover(ctx); err != nil {
		fmt.Fprintf(errorOutput, "perpetual: recover workspaces: %v\n", err)
		return 1
	}
	github, err := optionalGitHub(*clientID, *clientSecret, *callback, *address)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	events := NewEventBroker()
	logger := log.New(errorOutput, "perpetual: ", log.LstdFlags)
	application, err := New(Options{
		Context: ctx, Store: store, Workspaces: workspaces,
		GitHub:          github,
		CredentialsPath: credentialPath, WorkspaceRoot: paths.workspaces, Logger: logger,
		Events: events,
	})
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	listener, err := net.Listen("tcp", *address)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: listen on %s: %v\n", *address, err)
		return 1
	}
	server := &http.Server{
		Handler: application.Handler(), ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second,
	}
	finished := make(chan error, 1)
	go func() { finished <- server.Serve(listener) }()
	fmt.Fprintf(output, "Perpetual is running at http://%s\n", listener.Addr())
	select {
	case <-ctx.Done():
		shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			fmt.Fprintf(errorOutput, "perpetual: shutdown: %v\n", err)
			return 1
		}
		return 0
	case err := <-finished:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			fmt.Fprintf(errorOutput, "perpetual: serve: %v\n", err)
			return 1
		}
		return 0
	}
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
		root = filepath.Join(home, ".local", "share", "perpetual", "workspaces")
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
