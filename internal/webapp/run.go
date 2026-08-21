package webapp

import (
	"context"
	"crypto/tls"
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
	address := flags.String("address", envOr("PERPETUAL_ADDRESS", "127.0.0.1:8080"), "web listen address")
	publicURL := flags.String("public-url", os.Getenv("PERPETUAL_PUBLIC_URL"), "public base URL for remote access")
	databasePath := flags.String("database", "", "SQLite database path")
	dataRoot := flags.String("data-root", "", "workspace data directory")
	clientID := flags.String("github-client-id", os.Getenv("PERPETUAL_GITHUB_CLIENT_ID"), "GitHub App client ID")
	clientSecret := flags.String("github-client-secret", os.Getenv("PERPETUAL_GITHUB_CLIENT_SECRET"), "GitHub App client secret")
	callback := flags.String("github-callback-url", os.Getenv("PERPETUAL_GITHUB_CALLBACK_URL"), "GitHub callback URL")
	tlsCert := flags.String("tls-cert", os.Getenv("PERPETUAL_TLS_CERT"), "TLS certificate file")
	tlsKey := flags.String("tls-key", os.Getenv("PERPETUAL_TLS_KEY"), "TLS private key file")
	accessPassword := flags.String("access-password", os.Getenv("PERPETUAL_ACCESS_PASSWORD"), "optional password gate for remote access")
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
	cert, key, err := tlsCredentials(*tlsCert, *tlsKey)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
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
	go workspaces.RunWorker(ctx)
	github, err := optionalGitHub(*clientID, *clientSecret, *callback, *address, *publicURL)
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
	handler := http.Handler(application.Handler())
	if trimmed := strings.TrimSpace(*accessPassword); trimmed != "" {
		handler = requireAccessPassword(trimmed, handler)
	}
	server := &http.Server{
		Handler: handler, ReadHeaderTimeout: 5 * time.Second,
		IdleTimeout: 60 * time.Second,
	}
	finished := make(chan error, 1)
	go func() { finished <- serveListener(listener, server, cert, key) }()
	fmt.Fprintf(output, "Perpetual is running at %s\n", runningURL(*address, *publicURL, cert != ""))
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

// optionalGitHub builds the GitHub App client. The callback URL is the
// externally visible address GitHub redirects the browser to after sign-in,
// so it must be reachable from the browser, not from the listen address.
// A loopback listen address keeps the historical local default; any other
// address (for example 0.0.0.0 for remote access) requires an explicit
// callback URL or public URL because GitHub cannot redirect to a wildcard.
func optionalGitHub(clientID, secret, callback, address, publicURL string) (githubClient, error) {
	clientID, secret = strings.TrimSpace(clientID), strings.TrimSpace(secret)
	if clientID == "" && secret == "" {
		return nil, nil
	}
	callback = strings.TrimSpace(callback)
	if callback == "" {
		publicURL = strings.TrimRight(strings.TrimSpace(publicURL), "/")
		switch {
		case publicURL != "":
			callback = publicURL + "/auth/github/callback"
		case loopbackAddress(address):
			callback = "http://" + address + "/auth/github/callback"
		default:
			return nil, errors.New("GitHub callback URL is required when listening on a non-loopback address; set PERPETUAL_PUBLIC_URL to your public URL or PERPETUAL_GITHUB_CALLBACK_URL explicitly")
		}
	}
	return githubapp.New(clientID, secret, callback)
}

// loopbackAddress reports whether the listen address refers to this machine
// only. Empty and wildcard hosts bind every interface and are not loopback.
func loopbackAddress(address string) bool {
	host, _, err := net.SplitHostPort(address)
	if err != nil {
		return false
	}
	if host == "" {
		return false
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return strings.EqualFold(host, "localhost")
}

// tlsCredentials validates an optional TLS certificate/key pair. Both files
// must be supplied together and must load before the server starts.
func tlsCredentials(certFile, keyFile string) (string, string, error) {
	certFile, keyFile = strings.TrimSpace(certFile), strings.TrimSpace(keyFile)
	switch {
	case certFile == "" && keyFile == "":
		return "", "", nil
	case certFile == "" || keyFile == "":
		return "", "", errors.New("both TLS certificate and key are required (PERPETUAL_TLS_CERT and PERPETUAL_TLS_KEY)")
	}
	if _, err := tls.LoadX509KeyPair(certFile, keyFile); err != nil {
		return "", "", fmt.Errorf("load TLS certificate and key: %w", err)
	}
	return certFile, keyFile, nil
}

func serveListener(listener net.Listener, server *http.Server, cert, key string) error {
	if cert != "" {
		return server.ServeTLS(listener, cert, key)
	}
	return server.Serve(listener)
}

func runningURL(address, publicURL string, tlsEnabled bool) string {
	if value := strings.TrimRight(strings.TrimSpace(publicURL), "/"); value != "" {
		return value
	}
	scheme := "http"
	if tlsEnabled {
		scheme = "https"
	}
	return scheme + "://" + address
}

func envOr(name, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(name)); value != "" {
		return value
	}
	return fallback
}
