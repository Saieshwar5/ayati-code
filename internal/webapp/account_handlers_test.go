package webapp

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireUserRejectsGuest(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	for _, path := range []string{"/api/me", "/api/workspaces"} {
		response := serveGuest(handler, http.MethodGet, path, "", false)
		if response.Code != http.StatusUnauthorized ||
			response.Body.String() != "{\"error\":\"authentication required\"}\n" {
			t.Fatalf("GET %s guest status = %d, body = %s", path,
				response.Code, response.Body.String())
		}
	}
}

func TestCurrentUserEndpoint(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	response := serve(handler, http.MethodGet, "/api/me", "", false)
	if response.Code != http.StatusOK ||
		!strings.Contains(response.Body.String(), `"login":"octocat"`) {
		t.Fatalf("GET /api/me status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestLogoutRevokesSession(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	response := serve(handler, http.MethodGet, "/api/me", "", false)
	if response.Code != http.StatusOK {
		t.Fatalf("GET /api/me before logout status = %d", response.Code)
	}
	response = serve(handler, http.MethodPost, "/api/logout", "", true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("POST /api/logout status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodGet, "/api/me", "", false)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("GET /api/me after logout status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestGitHubCallbackCreatesSessionCookie(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	request := httptest.NewRequest(http.MethodGet,
		"/auth/github/callback?code=code&state=state", nil)
	request.AddCookie(&http.Cookie{
		Name: "perpetual_github_state", Value: "state", Path: "/auth/github/callback",
	})
	response := httptest.NewRecorder()
	handler.ServeHTTP(response, request)
	if response.Code != http.StatusFound || response.Header().Get("Location") != "/" {
		t.Fatalf("callback status = %d, location = %q", response.Code,
			response.Header().Get("Location"))
	}
	if !responseHasCookie(response, accountSessionCookie) {
		t.Fatalf("callback cookies = %#v", response.Result().Cookies())
	}
}

func responseHasCookie(response *httptest.ResponseRecorder, name string) bool {
	for _, cookie := range response.Result().Cookies() {
		if cookie.Name == name && cookie.Value != "" {
			return true
		}
	}
	return false
}
