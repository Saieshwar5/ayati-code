package webapp

import (
	"net/http"
	"strings"
	"testing"
)

func TestHandlerConfiguresTestsAndRemovesProviderWithoutExposingKey(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	input := `{"api_key":"private-openai-key","default_model":"gpt-test"}`
	response := serve(handler, http.MethodPut, "/api/providers/openai", input, true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"configured":true`) ||
		!strings.Contains(response.Body.String(), `"default_model":"gpt-test"`) ||
		strings.Contains(response.Body.String(), "private-openai-key") {
		t.Fatalf("configure status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodPost, "/api/providers/openai/test", `{}`, true)
	if response.Code != http.StatusOK || !strings.Contains(response.Body.String(), `"verified":true`) {
		t.Fatalf("test status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodDelete, "/api/providers/openai", "", true)
	if response.Code != http.StatusNoContent {
		t.Fatalf("remove status = %d, body = %s", response.Code, response.Body.String())
	}
	response = serve(handler, http.MethodGet, "/api/providers", "", false)
	if response.Code != http.StatusOK || strings.Contains(response.Body.String(), "private-openai-key") ||
		!strings.Contains(response.Body.String(), `"id":"openai"`) ||
		!strings.Contains(response.Body.String(), `"configured":false`) {
		t.Fatalf("list status = %d, body = %s", response.Code, response.Body.String())
	}
}

func TestHandlerProtectsProviderMutationsAndRejectsUnknownProvider(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	input := `{"api_key":"private-key","default_model":"test"}`
	response := serve(handler, http.MethodPut, "/api/providers/openai", input, false)
	if response.Code != http.StatusForbidden {
		t.Fatalf("unprotected mutation status = %d", response.Code)
	}
	response = serve(handler, http.MethodPut, "/api/providers/missing", input, true)
	if response.Code != http.StatusBadRequest || !strings.Contains(response.Body.String(), "not available") {
		t.Fatalf("unknown provider status = %d, body = %s", response.Code, response.Body.String())
	}
}
