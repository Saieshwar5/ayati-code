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

	"github.com/Saieshwar5/perpetual/internal/accounts"
	appdatabase "github.com/Saieshwar5/perpetual/internal/database"
	"github.com/Saieshwar5/perpetual/internal/environments"
	"github.com/Saieshwar5/perpetual/internal/execution"
	"github.com/Saieshwar5/perpetual/internal/githubapp"
	"github.com/Saieshwar5/perpetual/internal/lambdaruntime"
	"github.com/Saieshwar5/perpetual/internal/model"
	"github.com/Saieshwar5/perpetual/internal/workspace"
	"github.com/Saieshwar5/perpetual/internal/workspaceruntime"
)

const version = "dev"

func Run(ctx context.Context, args []string, output, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("perpetual", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	address := flags.String("address", envOr("PERPETUAL_ADDRESS", "127.0.0.1:8080"), "web listen address")
	publicURL := flags.String("public-url", os.Getenv("PERPETUAL_PUBLIC_URL"), "public base URL for remote access")
	databasePath := flags.String("database", "", "database path or DSN (see -database-provider)")
	databaseProvider := flags.String("database-provider", envOr("PERPETUAL_DATABASE_PROVIDER", "sqlite"), "database provider (sqlite or postgres)")
	databaseURL := flags.String("database-url", os.Getenv("PERPETUAL_DATABASE_URL"), "Postgres connection string when -database-provider is postgres")
	dataRoot := flags.String("data-root", "", "workspace data directory")
	clientID := flags.String("github-client-id", os.Getenv("PERPETUAL_GITHUB_CLIENT_ID"), "GitHub App client ID")
	clientSecret := flags.String("github-client-secret", os.Getenv("PERPETUAL_GITHUB_CLIENT_SECRET"), "GitHub App client secret")
	callback := flags.String("github-callback-url", os.Getenv("PERPETUAL_GITHUB_CALLBACK_URL"), "GitHub callback URL")
	tlsCert := flags.String("tls-cert", os.Getenv("PERPETUAL_TLS_CERT"), "TLS certificate file")
	tlsKey := flags.String("tls-key", os.Getenv("PERPETUAL_TLS_KEY"), "TLS private key file")
	accessPassword := flags.String("access-password", os.Getenv("PERPETUAL_ACCESS_PASSWORD"), "optional password gate for remote access")
	runtime := flags.String("runtime", os.Getenv("PERPETUAL_RUNTIME"), "workspace runtime (local or cloud)")
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
	database, err := appdatabase.OpenConfigured(ctx, appdatabase.Config{
		Provider: appdatabase.Provider(strings.ToLower(strings.TrimSpace(*databaseProvider))),
		URL:      strings.TrimSpace(*databaseURL),
		Path:     paths.database,
	})
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
	accountStore, err := accounts.NewStore(database)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	runtimeProvider, runtimeErr := selectWorkspaceRuntime(*runtime, store)
	if runtimeErr != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", runtimeErr)
		return 1
	}
	startLambdaReconcile(ctx, runtimeProvider)
	workspaces, err := workspace.NewService(store, accountStore, runtimeProvider, paths.workspaces)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	if strings.TrimSpace(*runtime) == "lambda" {
		if err := wireLambdaImageBuilder(workspaces); err != nil {
			log.Printf("perpetual: lambda image builder unavailable: %v", err)
		}
	}
	if strings.TrimSpace(*runtime) == "lambda" {
		wireLambdaRepoSync(runtimeProvider, workspaces)
	}
	if err := workspaces.Recover(ctx); err != nil {
		fmt.Fprintf(errorOutput, "perpetual: recover workspaces: %v\n", err)
		return 1
	}
	go workspaces.RunWorker(ctx)
	go startExecutionWorker(ctx, store)
	go runSessionCleanup(ctx, accountStore)
	github, err := optionalGitHub(*clientID, *clientSecret, *callback, *address, *publicURL)
	if err != nil {
		fmt.Fprintf(errorOutput, "perpetual: %v\n", err)
		return 1
	}
	events := NewEventBroker()
	logger := log.New(errorOutput, "perpetual: ", log.LstdFlags)
	application, err := New(Options{
		Context: ctx, Store: store, Accounts: accountStore, Workspaces: workspaces,
		GitHub: github, WorkspaceRoot: paths.workspaces, Logger: logger, Events: events,
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

// runSessionCleanup periodically removes expired login sessions so the
// auth_sessions table never grows without bound. It stops with context
// cancellation and treats cleanup failures as non-fatal.
func runSessionCleanup(ctx context.Context, store *accounts.Store) {
	const interval = time.Hour
	const retention = time.Hour
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if _, err := store.DeleteExpiredSessions(ctx, time.Now().Add(-retention)); err != nil {
				continue
			}
		}
	}
}

// workspaceRuntimeProvider resolves a workspace's persisted runtime provider
// name to the runtime implementation, falling back to local for empty values.
type workspaceRuntimeProvider struct {
	local  workspaceruntime.Runtime
	cloud  workspaceruntime.Runtime
	lambda workspaceruntime.Runtime
}

func (p *workspaceRuntimeProvider) RuntimeFor(provider string) (workspaceruntime.Runtime, error) {
	switch strings.TrimSpace(provider) {
	case "", "local":
		return p.local, nil
	case "cloud":
		if p.cloud == nil {
			return nil, errors.New("cloud workspace runtime is not configured")
		}
		return p.cloud, nil
	case "lambda":
		if p.lambda == nil {
			return nil, errors.New("lambda workspace runtime is not configured")
		}
		return p.lambda, nil
	default:
		return nil, fmt.Errorf("unknown workspace runtime %q", provider)
	}
}

// selectWorkspaceRuntime builds the runtime provider for the configured
// runtime name. Local is the default; cloud validates its config up front so
// misconfiguration fails at startup instead of inside a workspace job.
func selectWorkspaceRuntime(name string, stores ...*workspace.Store) (workspace.RuntimeProvider, error) {
	provider := &workspaceRuntimeProvider{local: workspaceruntime.NewLocal()}
	switch strings.TrimSpace(name) {
	case "", "local":
		return provider, nil
	case "cloud":
		cloud, err := workspaceruntime.NewCloud(workspaceruntime.CloudConfig{
			Endpoint: os.Getenv("PERPETUAL_CLOUD_RUNTIME_ENDPOINT"),
			Token:    os.Getenv("PERPETUAL_CLOUD_RUNTIME_TOKEN"),
			Pool:     os.Getenv("PERPETUAL_CLOUD_RUNTIME_POOL"),
		})
		if err != nil {
			return nil, err
		}
		provider.cloud = cloud
		return provider, nil
	case "lambda":
		var store *workspace.Store
		if len(stores) > 0 {
			store = stores[0]
		}
		lambda, err := newLambdaRuntime(store)
		if err != nil {
			return nil, err
		}
		provider.lambda = lambda
		return provider, nil
	default:
		return nil, fmt.Errorf("workspace runtime %q is not supported", name)
	}
}

// startExecutionWorker runs the execution-room worker with the stub provider.
// A real model provider will replace the stub once provider configuration
// lands; the loop, store, quotas, and shell factory are already in place.
func startExecutionWorker(ctx context.Context, store *workspace.Store) {
	provider, providerErr := model.NewFromConfig(model.LoadFromEnv())
	if providerErr != nil {
		log.Printf("perpetual: model provider unavailable (%v); using stub provider", providerErr)
		provider = execution.StubProvider{}
	}
	worker, err := execution.NewWorkerWithFactory(store, provider, func(run workspace.Run) (execution.ShellRunner, error) {
		ws, err := store.Get(context.Background(), run.WorkspaceID)
		if err != nil {
			return nil, err
		}
		return execution.NewRuntimeShell(
			workspaceruntime.NewLocal(),
			workspaceruntime.Ref{ID: ws.ID, Directory: ws.Path},
			map[string]string{"PATH": os.Getenv("PATH")},
		)
	})
	if err != nil {
		log.Printf("perpetual: execution worker unavailable: %v", err)
		return
	}
	worker.SetLimits(workspace.ClaimLimits{MaxPerUser: 2, MaxPerWorkspace: 1, MaxGlobal: 64})
	go execution.RunWorker(ctx, worker, 750*time.Millisecond)
}

// newLambdaRuntime wires the Lambda MicroVMs provider from the environment.
// It fails fast when required Lambda configuration is missing.
func newLambdaRuntime(store *workspace.Store) (workspaceruntime.Runtime, error) {
	config := environments.LoadLambdaConfig()
	api, err := environments.NewAWSLambdaAPI(config.Region)
	if err != nil {
		return nil, err
	}
	manager, err := environments.NewManager(config, api)
	if err != nil {
		return nil, err
	}
	return lambdaruntime.NewLambda(manager, store)
}

// startLambdaReconcile periodically reconciles durable Lambda microVM records
// when the lambda runtime is selected. Local mode has nothing to reconcile.
func startLambdaReconcile(ctx context.Context, provider workspace.RuntimeProvider) {
	if provider == nil {
		return
	}
	runtime, err := provider.RuntimeFor("lambda")
	if err != nil {
		return
	}
	reconciler, ok := runtime.(workspaceruntime.Reconciler)
	if !ok {
		return
	}
	go func() {
		for {
			if ctx.Err() != nil {
				return
			}
			_ = reconciler.Reconcile(ctx)
			select {
			case <-ctx.Done():
				return
			case <-time.After(5 * time.Minute):
			}
		}
	}()
}

// wireLambdaImageBuilder injects the production microVM image builder into the
// workspace service when the lambda runtime is selected. PERPETUAL_LAMBDA_AGENT_BINARY
// must point at the compiled vmagent binary.
func wireLambdaImageBuilder(workspaces *workspace.Service) error {
	config := environments.LoadLambdaConfig()
	if config.Region == "" {
		return fmt.Errorf("PERPETUAL_AWS_REGION is required for lambda image builds")
	}
	agentPath := os.Getenv("PERPETUAL_LAMBDA_AGENT_BINARY")
	if agentPath == "" {
		return fmt.Errorf("PERPETUAL_LAMBDA_AGENT_BINARY is required for lambda image builds")
	}
	agentBinary, err := os.ReadFile(agentPath)
	if err != nil {
		return fmt.Errorf("read vmagent binary: %w", err)
	}
	api, err := environments.NewAWSLambdaAPI(config.Region)
	if err != nil {
		return err
	}
	s3, err := environments.NewS3(config.Region)
	if err != nil {
		return err
	}
	builder := &environments.ImageBuilder{
		Name:         os.Getenv("PERPETUAL_LAMBDA_IMAGE_NAME"),
		Bucket:       config.S3Bucket,
		BuildRoleARN: config.BuildRoleARN,
		BaseImageARN: os.Getenv("PERPETUAL_AWS_BASE_IMAGE_ARN"),
		AgentBinary:  agentBinary,
		API:          api,
		S3:           s3,
	}
	workspaces.SetImageBuilder(builder)
	return nil
}

type lambdaRepoSyncer struct{ runtime *lambdaruntime.LambdaRuntime }

func (s lambdaRepoSyncer) Push(ctx context.Context, workspaceID, tree string) error {
	return s.runtime.PushRepo(ctx, workspaceID, tree)
}

// wireLambdaRepoSync exposes the lambda runtime to the workspace service so
// preparation can push the repo into microVMs. Local mode stays unaffected.
func wireLambdaRepoSync(provider workspace.RuntimeProvider, service *workspace.Service) {
	if provider == nil || service == nil {
		return
	}
	runtime, err := provider.RuntimeFor("lambda")
	if err != nil {
		return
	}
	lambda, ok := runtime.(*lambdaruntime.LambdaRuntime)
	if !ok {
		return
	}
	service.SetRepoSyncer(lambdaRepoSyncer{runtime: lambda})
}
