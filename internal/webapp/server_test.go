package webapp

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/githubapp"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

type fakeWorkspaceService struct {
	store       *workspace.Store
	initialized chan string
	mu          sync.Mutex
	published   []string
}

func (f *fakeWorkspaceService) Initialize(ctx context.Context, id string) error {
	_ = f.store.CompletePreparation(ctx, id)
	f.initialized <- id
	return nil
}

func (f *fakeWorkspaceService) ConfigureProjectRoot(ctx context.Context, id, root string) error {
	return f.store.SelectProjectRoot(ctx, id, root)
}

func (f *fakeWorkspaceService) Stop(ctx context.Context, id string) error {
	return f.store.UpdateStatus(ctx, id, workspace.StatusStopped, "")
}

func (f *fakeWorkspaceService) Delete(ctx context.Context, id string) error {
	return f.store.Delete(ctx, id)
}

func (f *fakeWorkspaceService) Changes(context.Context, string) (workspace.Changes, error) {
	return workspace.Changes{Status: " M main.go\n", Diff: "diff --git a/main.go b/main.go\n"}, nil
}

func (f *fakeWorkspaceService) Publish(_ context.Context, id, message, name, email string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.published = []string{id, message, name, email}
	return nil
}

type fakeGitHub struct {
	repositories []githubapp.Repository
	pull         githubapp.PullRequest
}

func (f *fakeGitHub) AuthorizeURL(state string) string {
	return "https://github.test/authorize?state=" + state
}
func (f *fakeGitHub) Exchange(context.Context, string) (string, error) { return "token", nil }
func (f *fakeGitHub) CurrentUser(context.Context, string) (githubapp.User, error) {
	return githubapp.User{ID: 1, Login: "octocat"}, nil
}
func (f *fakeGitHub) Repositories(context.Context, string) ([]githubapp.Repository, error) {
	return f.repositories, nil
}
func (f *fakeGitHub) Branches(context.Context, string, string) ([]githubapp.Branch, error) {
	return []githubapp.Branch{{Name: "main"}}, nil
}
func (f *fakeGitHub) CreatePullRequest(
	_ context.Context, _, _, _, _, _, _ string,
) (githubapp.PullRequest, error) {
	return f.pull, nil
}

func TestHandlerServesInterfaceAndGuardsMutations(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	for _, path := range []string{"/", "/api/health"} {
		response := serve(handler, http.MethodGet, path, "", false)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", path, response.Code)
		}
	}
	index := serve(handler, http.MethodGet, "/", "", false).Body.String()
	for _, marker := range []string{`id="root"`, `type="module"`, `/assets/`} {
		if !strings.Contains(index, marker) {
			t.Fatalf("interface does not contain %s", marker)
		}
	}
	assetPattern := regexp.MustCompile(`(?:src|href)="(/assets/[^"]+)"`)
	assets := assetPattern.FindAllStringSubmatch(index, -1)
	if len(assets) < 2 {
		t.Fatalf("interface assets = %#v", assets)
	}
	for _, match := range assets {
		response := serve(handler, http.MethodGet, match[1], "", false)
		if response.Code != http.StatusOK {
			t.Fatalf("GET %s status = %d", match[1], response.Code)
		}
	}
	response := serve(handler, http.MethodPost, "/api/workspaces", `{}`, false)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unguarded mutation status = %d", response.Code)
	}
}

func TestHandlerCreatesWorkspaceAndPublishesPullRequest(t *testing.T) {
	handler, store, workspaces, _ := testHandler(t)
	create := `{"repository":"owner/project","base_branch":"main","branch":"ayati/change","create_branch":true,"authority":"develop","setup_command":"go mod download","environment":[{"name":"NPM_TOKEN","value":"private-token","expose_during_setup":true}]}`
	response := serve(handler, http.MethodPost, "/api/workspaces", create, true)
	if response.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, body = %s", response.Code, response.Body.String())
	}
	var created workspace.Workspace
	if err := json.Unmarshal(response.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode workspace: %v", err)
	}
	if initialized := <-workspaces.initialized; initialized != created.ID {
		t.Fatalf("initialized workspace = %q", initialized)
	}
	response = serve(handler, http.MethodGet, "/api/workspaces/"+created.ID+"/environment", "", false)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "private-token") ||
		!strings.Contains(response.Body.String(), `"name":"NPM_TOKEN"`) {
		t.Fatalf("environment response status = %d, body = %s", response.Code, response.Body.String())
	}
	update := `{"name":"DATABASE_URL","value":"sqlite://private","expose_during_setup":false}`
	response = serve(handler, http.MethodPost, "/api/workspaces/"+created.ID+"/environment", update, true)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "sqlite://private") {
		t.Fatalf("update environment status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodDelete,
		"/api/workspaces/"+created.ID+"/environment/NPM_TOKEN", "", true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete environment status = %d, body = %s", response.Code, response.Body.String())
	}
	if created.Authority != workspace.AuthorityDevelop || !created.CreateBranch {
		t.Fatalf("workspace branch policy = %#v", created)
	}
	publish := `{"commit_message":"feat: change","title":"Change project","body":"Verified."}`
	response = serve(handler, http.MethodPost, "/api/workspaces/"+created.ID+"/publish", publish, true)
	if response.Code != http.StatusOK {
		t.Fatalf("publish status = %d, body = %s", response.Code, response.Body.String())
	}
	loaded, err := store.Get(context.Background(), created.ID)
	if err != nil || loaded.PullRequestNumber != 7 || loaded.Status != workspace.StatusReady {
		t.Fatalf("workspace = %#v, error = %v", loaded, err)
	}
	if workspaces.published[1] != "feat: change" || workspaces.published[2] != "octocat" {
		t.Fatalf("publish arguments = %#v", workspaces.published)
	}
}

func TestHandlerCreatesRenamesAndDeletesWorkspaceSessions(t *testing.T) {
	handler, store, _, _ := testHandler(t)
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/sessions", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	response := serve(handler, http.MethodPost, "/api/workspaces/"+value.ID+"/sessions", `{}`, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("create session status = %d, body = %s", response.Code, response.Body.String())
	}
	var session workspace.Session
	if err := json.Unmarshal(response.Body.Bytes(), &session); err != nil {
		t.Fatalf("decode session: %v", err)
	}
	path := "/api/workspaces/" + value.ID + "/sessions/" + session.ID
	response = serve(handler, http.MethodPatch, path, `{"title":"Follow-up changes"}`, true)
	if response.Code != http.StatusOK {
		t.Fatalf("rename session status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodGet, path+"/messages", "", false)
	if response.Code != http.StatusOK || response.Body.String() != "[]\n" {
		t.Fatalf("session messages status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodPost, path+"/messages", `{"text":"continue the work"}`, true)
	if response.Code != http.StatusOK {
		t.Fatalf("send session message status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodDelete, path, "", true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete session status = %d, body = %s", response.Code, response.Body.String())
	}
	sessions, err := store.ListSessions(context.Background(), value.ID)
	if err != nil || len(sessions) != 1 || sessions[0].Title != "Original session" {
		t.Fatalf("remaining sessions = %#v, error = %v", sessions, err)
	}
}

func TestHandlerDeletesWorkspace(t *testing.T) {
	handler, store, _, _ := testHandler(t)
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/delete", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	response := serve(handler, http.MethodDelete, "/api/workspaces/"+value.ID, "", true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("delete workspace status = %d, body = %s", response.Code, response.Body.String())
	}
	if _, err := store.Get(context.Background(), value.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("deleted workspace error = %v", err)
	}
}

func TestHandlerRejectsEnvironmentChangesDuringInitialization(t *testing.T) {
	handler, store, _, _ := testHandler(t)
	value, err := store.Create(context.Background(), workspace.Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/initializing", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	response := serve(handler, http.MethodPost, "/api/workspaces/"+value.ID+"/environment",
		`{"name":"TOKEN","value":"secret"}`, true)
	if response.Code != http.StatusConflict {
		t.Fatalf("environment mutation status = %d, body = %s", response.Code, response.Body.String())
	}
}

func testHandler(t *testing.T) (http.Handler, *workspace.Store, *fakeWorkspaceService, *fakeGitHub) {
	t.Helper()
	root := t.TempDir()
	store, err := workspace.Open(filepath.Join(root, "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	workspaces := &fakeWorkspaceService{store: store, initialized: make(chan string, 1)}
	github := &fakeGitHub{
		repositories: []githubapp.Repository{{ID: 1, FullName: "owner/project", CloneURL: "https://github.com/owner/project.git", DefaultBranch: "main"}},
		pull:         githubapp.PullRequest{Number: 7, HTMLURL: "https://github.com/owner/project/pull/7"},
	}
	credentials := filepath.Join(root, "github.json")
	if err := githubapp.SaveCredentials(credentials, githubapp.Credentials{
		AccessToken: "secret", User: githubapp.User{ID: 1, Login: "octocat"},
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	server, err := New(Options{
		Store: store, Workspaces: workspaces, Chat: fakeChat{}, GitHub: github,
		CredentialsPath: credentials, WorkspaceRoot: filepath.Join(root, "workspaces"),
	})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	return server.Handler(), store, workspaces, github
}

type fakeChat struct{}

func (fakeChat) Messages(context.Context, string, string) ([]agent.Message, error) { return nil, nil }
func (fakeChat) Cancel(string)                                                     {}
func (fakeChat) CancelAndWait(string)                                              {}
func (fakeChat) Send(context.Context, string, string, string) (agent.Completion, error) {
	return agent.Completion{Text: "done"}, nil
}

func serve(handler http.Handler, method, path, body string, mutation bool) *httptest.ResponseRecorder {
	request := httptest.NewRequest(method, path, strings.NewReader(body))
	if mutation {
		request.Header.Set("X-Ayati-Request", "1")
	}
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	return response
}
