package webapp

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Saieshwar5/ayati-code/internal/githubapp"
)

type fakeGitHub struct {
	repositories     []githubapp.Repository
	pull             githubapp.PullRequest
	created          []githubapp.CreateRepositoryInput
	createError      error
	currentUserError error
	repositoryError  error
}

func (f *fakeGitHub) AuthorizeURL(state string) string {
	return "https://github.test/authorize?state=" + state
}
func (f *fakeGitHub) LoginURL() string                                 { return "http://127.0.0.1:8080/auth/github" }
func (f *fakeGitHub) Exchange(context.Context, string) (string, error) { return "token", nil }
func (f *fakeGitHub) CurrentUser(context.Context, string) (githubapp.User, error) {
	if f.currentUserError != nil {
		return githubapp.User{}, f.currentUserError
	}
	return githubapp.User{ID: 1, Login: "octocat"}, nil
}
func (f *fakeGitHub) Repositories(context.Context, string) ([]githubapp.Repository, error) {
	return f.repositories, f.repositoryError
}
func (f *fakeGitHub) Branches(context.Context, string, string) ([]githubapp.Branch, error) {
	return []githubapp.Branch{{Name: "main"}}, nil
}
func (f *fakeGitHub) CreatePullRequest(
	_ context.Context, _, _, _, _, _, _ string,
) (githubapp.PullRequest, error) {
	return f.pull, nil
}

func TestHandlerRequiresReconnectForExpiredGitHubAuthorization(t *testing.T) {
	handler, _, _, github := testHandler(t)
	github.currentUserError = githubapp.APIError{
		StatusCode: http.StatusUnauthorized,
		Status:     "401 Unauthorized",
	}
	response := serve(handler, http.MethodGet, "/api/session", "", false)
	if response.Code != http.StatusOK ||
		response.Body.String() != "{\"github_configured\":true,\"authenticated\":false}\n" {
		t.Fatalf("session status = %d, body = %s", response.Code, response.Body.String())
	}

	github.repositoryError = github.currentUserError
	response = serve(handler, http.MethodGet, "/api/repositories", "", false)
	if response.Code != http.StatusUnauthorized ||
		response.Body.String() != "{\"error\":\"GitHub authorization expired; reconnect GitHub\"}\n" {
		t.Fatalf("repositories status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestGitHubLoginUsesCallbackOriginForStateCookie(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	response := serve(handler, http.MethodGet, "/auth/github", "", false)
	if response.Code != http.StatusFound ||
		response.Header().Get("Location") != "http://127.0.0.1:8080/auth/github" {
		t.Fatalf("canonical login status = %d, location = %q",
			response.Code, response.Header().Get("Location"))
	}

	request, err := http.NewRequest(http.MethodGet,
		"http://127.0.0.1:8080/auth/github", nil)
	if err != nil {
		t.Fatalf("create canonical request: %v", err)
	}
	response = httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound ||
		!strings.HasPrefix(response.Header().Get("Location"), "https://github.test/authorize?state=") {
		t.Fatalf("GitHub authorization status = %d, location = %q",
			response.Code, response.Header().Get("Location"))
	}
	if cookie := response.Header().Get("Set-Cookie"); !strings.Contains(cookie, "ayati_github_state=") {
		t.Fatalf("state cookie = %q", cookie)
	}
}
