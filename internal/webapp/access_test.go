package webapp

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestRequireAccessPasswordAcceptsCorrectPassword(t *testing.T) {
	var called bool
	handler := requireAccessPassword("hunter2", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		called = true
		writer.WriteHeader(http.StatusNoContent)
	}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", basicHeader("any-user", "hunter2"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("status = %d", recorder.Code)
	}
	if !called {
		t.Fatal("next handler was not called")
	}
}

func TestRequireAccessPasswordRejectsMissingHeader(t *testing.T) {
	handler := requireAccessPassword("hunter2", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
	if got := recorder.Header().Get("WWW-Authenticate"); !strings.Contains(got, "Basic realm=\"perpetual\"") {
		t.Fatalf("WWW-Authenticate = %q", got)
	}
}

func TestRequireAccessPasswordRejectsWrongPassword(t *testing.T) {
	handler := requireAccessPassword("hunter2", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	request := httptest.NewRequest(http.MethodGet, "/", nil)
	request.Header.Set("Authorization", basicHeader("any-user", "wrong"))
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnauthorized {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestRequireAccessPasswordRejectsMalformedHeader(t *testing.T) {
	handler := requireAccessPassword("hunter2", http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	for _, header := range []string{
		"Bearer hunter2",
		"Basic !!!not-base64!!!",
		"Basic " + base64.StdEncoding.EncodeToString([]byte("no-colon")),
	} {
		request := httptest.NewRequest(http.MethodGet, "/", nil)
		request.Header.Set("Authorization", header)
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("header %q: status = %d", header, recorder.Code)
		}
	}
}

func TestRequireAccessPasswordCoversEventStreamAndMutations(t *testing.T) {
	for _, path := range []string{"/api/events", "/api/workspaces"} {
		handler := requireAccessPassword("hunter2", http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
			writer.WriteHeader(http.StatusNoContent)
		}))
		recorder := httptest.NewRecorder()
		handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodPost, path, nil))
		if recorder.Code != http.StatusUnauthorized {
			t.Fatalf("%s without password: status = %d", path, recorder.Code)
		}
	}
}

func basicHeader(username, password string) string {
	return "Basic " + base64.StdEncoding.EncodeToString([]byte(username+":"+password))
}
