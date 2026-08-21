package webapp

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"math/big"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Saieshwar5/perpetual/internal/accounts"
	appdatabase "github.com/Saieshwar5/perpetual/internal/database"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func TestResolvePathsUsesPerpetualDirectories(t *testing.T) {
	configRoot := t.TempDir()
	home := t.TempDir()
	t.Setenv("XDG_CONFIG_HOME", configRoot)
	t.Setenv("HOME", home)

	paths, err := resolvePaths("", "")
	if err != nil {
		t.Fatalf("resolvePaths: %v", err)
	}
	if paths.database != filepath.Join(configRoot, "perpetual", "perpetual.db") {
		t.Fatalf("database = %q", paths.database)
	}
	if paths.workspaces != filepath.Join(home, ".local", "share", "perpetual", "workspaces") {
		t.Fatalf("workspaces = %q", paths.workspaces)
	}
}

func TestOptionalGitHubSkipsWhenCredentialsMissing(t *testing.T) {
	client, err := optionalGitHub("", "", "", "127.0.0.1:8080", "")
	if err != nil {
		t.Fatalf("optionalGitHub: %v", err)
	}
	if client != nil {
		t.Fatalf("client = %v, want nil", client)
	}
}

func TestOptionalGitHubDefaultsLoopbackCallback(t *testing.T) {
	client, err := optionalGitHub("id", "secret", "", "127.0.0.1:8080", "")
	if err != nil {
		t.Fatalf("optionalGitHub: %v", err)
	}
	if got := client.LoginURL(); got != "http://127.0.0.1:8080/auth/github" {
		t.Fatalf("LoginURL = %q", got)
	}
	if got := client.AuthorizeURL("state"); !strings.Contains(got, "redirect_uri=http%3A%2F%2F127.0.0.1%3A8080%2Fauth%2Fgithub%2Fcallback") {
		t.Fatalf("AuthorizeURL = %q", got)
	}
}

func TestOptionalGitHubUsesPublicURLForRemoteAddress(t *testing.T) {
	client, err := optionalGitHub("id", "secret", "", "0.0.0.0:8080", "https://perpetual.example.com")
	if err != nil {
		t.Fatalf("optionalGitHub: %v", err)
	}
	if got := client.LoginURL(); got != "https://perpetual.example.com/auth/github" {
		t.Fatalf("LoginURL = %q", got)
	}
	if got := client.AuthorizeURL("state"); !strings.Contains(got, "redirect_uri=https%3A%2F%2Fperpetual.example.com%2Fauth%2Fgithub%2Fcallback") {
		t.Fatalf("AuthorizeURL = %q", got)
	}
}

func TestOptionalGitHubUsesExplicitCallbackOverPublicURL(t *testing.T) {
	client, err := optionalGitHub("id", "secret", "https://app.example.com/cb", "0.0.0.0:8080", "https://perpetual.example.com")
	if err != nil {
		t.Fatalf("optionalGitHub: %v", err)
	}
	if got := client.LoginURL(); got != "https://app.example.com/auth/github" {
		t.Fatalf("LoginURL = %q", got)
	}
}

func TestOptionalGitHubRequiresPublicCallbackForWildcardAddress(t *testing.T) {
	_, err := optionalGitHub("id", "secret", "", "0.0.0.0:8080", "")
	if err == nil {
		t.Fatal("optionalGitHub: want error for wildcard address without callback")
	}
	if !strings.Contains(err.Error(), "PERPETUAL_PUBLIC_URL") {
		t.Fatalf("error = %q", err)
	}
}

func TestLoopbackAddress(t *testing.T) {
	loopback := []string{"127.0.0.1:8080", "localhost:8080", "[::1]:8080"}
	for _, address := range loopback {
		if !loopbackAddress(address) {
			t.Fatalf("loopbackAddress(%q) = false, want true", address)
		}
	}
	notLoopback := []string{"0.0.0.0:8080", ":8080", "[::]:8080", "192.168.1.10:8080", "perpetual.example.com:8080", ""}
	for _, address := range notLoopback {
		if loopbackAddress(address) {
			t.Fatalf("loopbackAddress(%q) = true, want false", address)
		}
	}
}

func TestTLSCredentialsRequireBothFiles(t *testing.T) {
	cert, key, err := tlsCredentials("cert.pem", "")
	if err == nil || !strings.Contains(err.Error(), "both TLS certificate and key") {
		t.Fatalf("tlsCredentials = %q, %q, %v", cert, key, err)
	}
	cert, key, err = tlsCredentials("", "key.pem")
	if err == nil || !strings.Contains(err.Error(), "both TLS certificate and key") {
		t.Fatalf("tlsCredentials = %q, %q, %v", cert, key, err)
	}
	cert, key, err = tlsCredentials("", "")
	if err != nil || cert != "" || key != "" {
		t.Fatalf("tlsCredentials = %q, %q, %v", cert, key, err)
	}
}

func TestTLSCredentialsLoadValidPair(t *testing.T) {
	certFile, keyFile := writeTestKeyPair(t)
	cert, key, err := tlsCredentials(certFile, keyFile)
	if err != nil {
		t.Fatalf("tlsCredentials: %v", err)
	}
	if cert == "" || key == "" {
		t.Fatalf("tlsCredentials returned empty paths")
	}
}

func TestTLSCredentialsRejectsMissingFiles(t *testing.T) {
	dir := t.TempDir()
	_, _, err := tlsCredentials(filepath.Join(dir, "missing.pem"), filepath.Join(dir, "missing-key.pem"))
	if err == nil || !strings.Contains(err.Error(), "load TLS certificate and key") {
		t.Fatalf("tlsCredentials: %v", err)
	}
}

func TestRunningURL(t *testing.T) {
	if got := runningURL("0.0.0.0:8080", "", false); got != "http://0.0.0.0:8080" {
		t.Fatalf("runningURL = %q", got)
	}
	if got := runningURL("0.0.0.0:8443", "", true); got != "https://0.0.0.0:8443" {
		t.Fatalf("runningURL = %q", got)
	}
	if got := runningURL("127.0.0.1:8080", "https://perpetual.example.com/", false); got != "https://perpetual.example.com" {
		t.Fatalf("runningURL = %q", got)
	}
}

func TestServerServesTLSWithValidPair(t *testing.T) {
	certFile, keyFile := writeTestKeyPair(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	address := freeAddress(t)
	output := &lockedBuffer{}
	finished := make(chan int, 1)
	go func() {
		finished <- Run(ctx, []string{"-address", address, "-tls-cert", certFile, "-tls-key", keyFile, "-data-root", t.TempDir(), "-database", filepath.Join(t.TempDir(), "perpetual.db")}, output, output)
	}()
	t.Cleanup(func() { cancel(); <-finished })
	waitForOutput(t, output, "https://")
	if !strings.Contains(output.String(), "https://"+address) {
		t.Fatalf("output = %q", output.String())
	}
}

func TestServerRejectsTLSWithOnlyCertificate(t *testing.T) {
	certFile, _ := writeTestKeyPair(t)
	var output, errorOutput strings.Builder
	code := Run(context.Background(), []string{"-tls-cert", certFile, "-data-root", t.TempDir(), "-database", filepath.Join(t.TempDir(), "perpetual.db")}, &output, &errorOutput)
	if code != 1 || !strings.Contains(errorOutput.String(), "both TLS certificate and key") {
		t.Fatalf("Run = %d, stderr = %q", code, errorOutput.String())
	}
}

func writeTestKeyPair(t *testing.T) (string, string) {
	t.Helper()
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	template := x509.Certificate{
		SerialNumber: big.NewInt(1),
		Subject:      pkix.Name{CommonName: "perpetual-test"},
		NotBefore:    time.Now().Add(-time.Hour),
		NotAfter:     time.Now().Add(time.Hour),
	}
	der, err := x509.CreateCertificate(rand.Reader, &template, &template, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create certificate: %v", err)
	}
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		t.Fatalf("marshal key: %v", err)
	}
	dir := t.TempDir()
	certFile := filepath.Join(dir, "cert.pem")
	keyFile := filepath.Join(dir, "key.pem")
	if err := os.WriteFile(certFile, pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der}), 0o600); err != nil {
		t.Fatalf("write cert: %v", err)
	}
	if err := os.WriteFile(keyFile, pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER}), 0o600); err != nil {
		t.Fatalf("write key: %v", err)
	}
	return certFile, keyFile
}

func freeAddress(t *testing.T) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	address := listener.Addr().String()
	_ = listener.Close()
	return address
}

func waitForOutput(t *testing.T, output *lockedBuffer, want string) {
	t.Helper()
	deadline := time.Now().Add(10 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Contains(output.String(), want) {
			return
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("output never contained %q: %q", want, output.String())
}

// lockedBuffer serializes concurrent writes from the server goroutine and
// reads from the test goroutine so the race detector stays quiet.
type lockedBuffer struct {
	mu  sync.Mutex
	buf strings.Builder
}

func (b *lockedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *lockedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func TestSessionReportsGitHubUnconfiguredWhenCredentialsMissing(t *testing.T) {
	root := t.TempDir()
	store, err := workspace.Open(filepath.Join(root, "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	github, err := optionalGitHub("", "", "", "127.0.0.1:8080", "")
	if err != nil {
		t.Fatalf("optionalGitHub: %v", err)
	}
	accountDatabase, accountErr := appdatabase.Open(filepath.Join(root, "accounts.db"))
	if accountErr != nil {
		t.Fatalf("Open account database: %v", accountErr)
	}
	defer func() { _ = accountDatabase.Close() }()
	accountStore, accountErr := accounts.NewStore(accountDatabase)
	if accountErr != nil {
		t.Fatalf("New account store: %v", accountErr)
	}
	server, err := New(Options{
		Store: store, Accounts: accountStore,
		Workspaces:    &fakeWorkspaceService{store: store, initialized: make(chan string, 1)},
		GitHub:        github,
		WorkspaceRoot: filepath.Join(root, "workspaces"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	response := serve(server.Handler(), http.MethodGet, "/api/session", "", false)
	if response.Code != http.StatusOK {
		t.Fatalf("status = %d", response.Code)
	}
	if !strings.Contains(response.Body.String(), `"github_configured":false`) {
		t.Fatalf("session body = %s", response.Body.String())
	}
}
