package githubapp

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientExchangesAuthorizationAndListsRepositories(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		body := ""
		switch request.URL.Path {
		case "/login/oauth/access_token":
			if request.Method != http.MethodPost {
				t.Fatalf("method = %s", request.Method)
			}
			body = `{"access_token":"token"}`
		case "/user":
			assertBearer(t, request)
			body = `{"id":7,"login":"octocat"}`
		case "/user/installations":
			assertBearer(t, request)
			body = `{"installations":[{"id":12}]}`
		case "/user/installations/12/repositories":
			body = `{"repositories":[{"id":2,"full_name":"owner/z"},{"id":1,"full_name":"owner/a"}]}`
		default:
			return &http.Response{StatusCode: http.StatusNotFound, Status: "404 Not Found", Body: http.NoBody}, nil
		}
		return &http.Response{
			StatusCode: http.StatusOK, Status: "200 OK",
			Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header),
		}, nil
	})
	client, err := New("client", "secret", "http://127.0.0.1/callback")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	client.httpClient = &http.Client{Transport: transport}
	token, err := client.Exchange(context.Background(), "code")
	if err != nil || token != "token" {
		t.Fatalf("token = %q, error = %v", token, err)
	}
	user, err := client.CurrentUser(context.Background(), token)
	if err != nil || user.Login != "octocat" {
		t.Fatalf("user = %#v, error = %v", user, err)
	}
	repositories, err := client.Repositories(context.Background(), token)
	if err != nil || len(repositories) != 2 || repositories[0].FullName != "owner/a" {
		t.Fatalf("repositories = %#v, error = %v", repositories, err)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func TestAuthorizeURLCarriesStateAndCallback(t *testing.T) {
	client, _ := New("client", "secret", "http://127.0.0.1:8080/auth/github/callback")
	value, err := url.Parse(client.AuthorizeURL("state-value"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if value.Query().Get("state") != "state-value" || !strings.Contains(value.Query().Get("redirect_uri"), "/auth/github/callback") {
		t.Fatalf("authorize URL = %s", value)
	}
}

func TestClientCreatesBranchAndDraftPullRequest(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assertBearer(t, request)
		body := ""
		switch request.Method + " " + request.URL.Path {
		case "GET /repos/owner/project/branches/main":
			body = `{"name":"main","commit":{"sha":"abc123"}}`
		case "POST /repos/owner/project/git/refs":
			var input map[string]string
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatalf("decode ref request: %v", err)
			}
			if input["ref"] != "refs/heads/ayati/change" || input["sha"] != "abc123" {
				t.Fatalf("ref request = %#v", input)
			}
			body = `{}`
		case "POST /repos/owner/project/pulls":
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatalf("decode pull request: %v", err)
			}
			if input["draft"] != true || input["base"] != "main" || input["head"] != "ayati/change" {
				t.Fatalf("pull request = %#v", input)
			}
			body = `{"number":9,"html_url":"https://github.com/owner/project/pull/9"}`
		default:
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		return &http.Response{
			StatusCode: http.StatusOK, Status: "200 OK",
			Body: io.NopCloser(strings.NewReader(body)), Header: make(http.Header),
		}, nil
	})
	client, err := New("client", "secret", "http://127.0.0.1/callback")
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	client.httpClient = &http.Client{Transport: transport}
	if err := client.CreateBranch(context.Background(), "token", "owner/project", "main", "ayati/change"); err != nil {
		t.Fatalf("CreateBranch: %v", err)
	}
	pull, err := client.CreatePullRequest(
		context.Background(), "token", "owner/project", "main", "ayati/change", "Change", "Body",
	)
	if err != nil || pull.Number != 9 {
		t.Fatalf("pull = %#v, error = %v", pull, err)
	}
}

func TestCredentialsUsePrivateFile(t *testing.T) {
	path := t.TempDir() + "/config/github.json"
	want := Credentials{AccessToken: "secret", User: User{ID: 7, Login: "octocat"}}
	if err := SaveCredentials(path, want); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil || info.Mode().Perm() != 0o600 {
		t.Fatalf("mode = %v, error = %v", info.Mode().Perm(), err)
	}
	directory, err := os.Stat(filepath.Dir(path))
	if err != nil || directory.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode = %v, error = %v", directory.Mode().Perm(), err)
	}
	got, err := LoadCredentials(path)
	if err != nil || got.AccessToken != want.AccessToken || got.User.Login != want.User.Login {
		t.Fatalf("credentials = %#v, error = %v", got, err)
	}
}

func assertBearer(t *testing.T, request *http.Request) {
	t.Helper()
	if request.Header.Get("Authorization") != "Bearer token" {
		t.Fatalf("authorization = %q", request.Header.Get("Authorization"))
	}
}
