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
	if got := client.LoginURL(); got != "http://127.0.0.1:8080/auth/github" {
		t.Fatalf("login URL = %q", got)
	}
	value, err := url.Parse(client.AuthorizeURL("state-value"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if value.Query().Get("state") != "state-value" || !strings.Contains(value.Query().Get("redirect_uri"), "/auth/github/callback") {
		t.Fatalf("authorize URL = %s", value)
	}
}

func TestClientCreatesDraftPullRequest(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assertBearer(t, request)
		body := ""
		switch request.Method + " " + request.URL.Path {
		case "POST /repos/owner/project/pulls":
			var input map[string]any
			if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
				t.Fatalf("decode pull request: %v", err)
			}
			if input["draft"] != true || input["base"] != "main" || input["head"] != "perpetual/change" {
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
	pull, err := client.CreatePullRequest(
		context.Background(), "token", "owner/project", "main", "perpetual/change", "Change", "Body",
	)
	if err != nil || pull.Number != 9 {
		t.Fatalf("pull = %#v, error = %v", pull, err)
	}
}

func TestClientCreatesInitializedPrivateRepository(t *testing.T) {
	transport := roundTripFunc(func(request *http.Request) (*http.Response, error) {
		assertBearer(t, request)
		if request.Method != http.MethodPost || request.URL.Path != "/user/repos" {
			t.Fatalf("unexpected request: %s %s", request.Method, request.URL.Path)
		}
		var input map[string]any
		if err := json.NewDecoder(request.Body).Decode(&input); err != nil {
			t.Fatalf("decode repository: %v", err)
		}
		if input["name"] != "new-project" || input["private"] != true || input["auto_init"] != true {
			t.Fatalf("repository input = %#v", input)
		}
		return &http.Response{
			StatusCode: http.StatusCreated, Status: "201 Created", Header: make(http.Header),
			Body: io.NopCloser(strings.NewReader(`{"id":4,"full_name":"octocat/new-project","clone_url":"https://github.com/octocat/new-project.git","default_branch":"trunk","private":true}`)),
		}, nil
	})
	client, _ := New("client", "secret", "http://127.0.0.1/callback")
	client.httpClient = &http.Client{Transport: transport}
	repository, err := client.CreateRepository(context.Background(), "token", CreateRepositoryInput{
		Name: "new-project", Description: "A new project", Private: true,
	})
	if err != nil || repository.FullName != "octocat/new-project" || repository.DefaultBranch != "trunk" {
		t.Fatalf("repository = %#v, error = %v", repository, err)
	}
}

func TestClientRejectsInvalidRepositoryNameBeforeRequest(t *testing.T) {
	client, _ := New("client", "secret", "http://127.0.0.1/callback")
	client.httpClient = &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		t.Fatalf("unexpected request: %s", request.URL)
		return nil, nil
	})}
	if _, err := client.CreateRepository(context.Background(), "token", CreateRepositoryInput{Name: "bad name"}); err == nil {
		t.Fatal("CreateRepository accepted an invalid name")
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
