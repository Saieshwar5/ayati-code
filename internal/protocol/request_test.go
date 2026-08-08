package protocol

import (
	"strings"
	"testing"
)

func TestDecodeRequestGeneratesIDAndTrimsPrompt(t *testing.T) {
	request, err := DecodeRequest(strings.NewReader(`{"prompt":"  test the project  ","workspace":"/workspace"}`))
	if err != nil {
		t.Fatalf("DecodeRequest: %v", err)
	}
	if request.RunID == "" || request.Prompt != "test the project" || request.Workspace != "/workspace" {
		t.Fatalf("unexpected request: %+v", request)
	}
}

func TestDecodeRequestRejectsTrailingJSON(t *testing.T) {
	_, err := DecodeRequest(strings.NewReader(`{"prompt":"one","workspace":"/workspace"} {"prompt":"two"}`))
	if err == nil {
		t.Fatal("expected trailing JSON error")
	}
}

func TestDecodeRequestRejectsUnsupportedVersion(t *testing.T) {
	_, err := DecodeRequest(strings.NewReader(`{"version":2,"prompt":"one","workspace":"/workspace"}`))
	if err == nil || !strings.Contains(err.Error(), "unsupported request version") {
		t.Fatalf("expected version error, got %v", err)
	}
}

func TestDecodeRequestRejectsOversizedInput(t *testing.T) {
	_, err := DecodeRequest(strings.NewReader(strings.Repeat(" ", maxRequestBytes+1)))
	if err == nil || !strings.Contains(err.Error(), "exceeds") {
		t.Fatalf("expected size error, got %v", err)
	}
}
