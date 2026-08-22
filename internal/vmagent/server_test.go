package vmagent

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestExecEndpointRunsCommand(t *testing.T) {
	root := t.TempDir()
	handler := &Handler{Root: root, Env: map[string]string{"PATH": os.Getenv("PATH")}}
	server := httptest.NewServer(handler.DataHandler())
	defer server.Close()

	response, err := http.Post(server.URL+"/v1/exec", "application/json",
		strings.NewReader(`{"command":"printf hello"}`))
	if err != nil {
		t.Fatalf("POST /v1/exec: %v", err)
	}
	defer response.Body.Close()
	var result struct {
		Stdout   string `json:"stdout"`
		ExitCode int    `json:"exit_code"`
	}
	if err := json.NewDecoder(response.Body).Decode(&result); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if result.ExitCode != 0 || result.Stdout != "hello" {
		t.Fatalf("result = %#v", result)
	}
}

func TestBootstrapAndTarRoundTrip(t *testing.T) {
	root := t.TempDir()
	handler := &Handler{Root: root}
	data := handler.DataHandler()

	// Build a source tree and stream it via /v1/bootstrap.
	source := t.TempDir()
	if err := os.WriteFile(filepath.Join(source, "hello.txt"), []byte("world"), 0o644); err != nil {
		t.Fatalf("write source: %v", err)
	}
	var archive bytesBuffer
	tw := newTarGzipWriter(&archive)
	if err := tarDirectory(context.Background(), tw, source); err != nil {
		t.Fatalf("tarDirectory: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar: %v", err)
	}

	bootstrapRequest := httptest.NewRequest(http.MethodPost, "/v1/bootstrap", strings.NewReader(archive.String()))
	bootstrapResponse := httptest.NewRecorder()
	data.ServeHTTP(bootstrapResponse, bootstrapRequest)
	if bootstrapResponse.Code != http.StatusOK {
		t.Fatalf("bootstrap status = %d, body = %s", bootstrapResponse.Code, bootstrapResponse.Body.String())
	}
	extracted := filepath.Join(root, "hello.txt")
	value, err := os.ReadFile(extracted)
	if err != nil || string(value) != "world" {
		t.Fatalf("extract = %q, %v", value, err)
	}

	tarRequest := httptest.NewRequest(http.MethodGet, "/v1/tar", nil)
	tarResponse := httptest.NewRecorder()
	data.ServeHTTP(tarResponse, tarRequest)
	if tarResponse.Code != http.StatusOK || tarResponse.Header().Get("Content-Type") != "application/gzip" {
		t.Fatalf("tar status = %d, type = %q", tarResponse.Code, tarResponse.Header().Get("Content-Type"))
	}
	reader, err := newTarGzipReader(strings.NewReader(tarResponse.Body.String()))
	if err != nil {
		t.Fatalf("open tar response: %v", err)
	}
	defer reader.Close()
	found := false
	for {
		header, err := reader.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("read tar: %v", err)
		}
		if header.Name == "hello.txt" {
			found = true
		}
	}
	if !found {
		t.Fatal("hello.txt missing from tar response")
	}
}

type bytesBuffer struct{ values []string }

func (b *bytesBuffer) Write(value []byte) (int, error) {
	b.values = append(b.values, string(value))
	return len(value), nil
}

func (b *bytesBuffer) String() string { return strings.Join(b.values, "") }
