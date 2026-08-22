// Package vmagent is the in-VM command server that runs inside Perpetual
// microVM images. The controller talks to the data plane (8080); Lambda
// invokes lifecycle hooks (9000). It deliberately uses the same bounded shell
// semantics as the local runtime.
package vmagent

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/exec"
)

// Handler owns the vmagent data plane.
type Handler struct {
	// Root is the working-directory root (default /workspace).
	Root string
	// Env are the only environment values passed to shell commands.
	Env map[string]string
}

// DataHandler returns the controller-facing HTTP mux.
func (h *Handler) DataHandler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", h.health)
	mux.HandleFunc("POST /v1/exec", h.exec)
	mux.HandleFunc("POST /v1/bootstrap", h.bootstrap)
	mux.HandleFunc("GET /v1/tar", h.tar)
	return mux
}

// HookHandler returns the Lambda lifecycle-hook mux. Hooks are fast no-ops;
// real per-VM initialization is owned by the controller bootstrap.
func (h *Handler) HookHandler() http.Handler {
	mux := http.NewServeMux()
	for _, name := range []string{"ready", "validate", "run", "resume", "suspend", "terminate"} {
		path := "/aws/lambda-microvms/runtime/v1/" + name
		mux.HandleFunc("POST "+path, func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
	}
	return mux
}

func (h *Handler) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

func (h *Handler) exec(w http.ResponseWriter, r *http.Request) {
	var request exec.ShellRequest
	if err := json.NewDecoder(io.LimitReader(r.Body, 1<<20)).Decode(&request); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": "invalid shell request"})
		return
	}
	root := strings.TrimSpace(h.Root)
	if root == "" {
		root = "/workspace"
	}
	shell, err := exec.New(h.Env, root)
	if err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	result := shell.Execute(r.Context(), request)
	writeJSON(w, http.StatusOK, result)
}

// bootstrap receives a gzip tar stream and extracts it into Root.
func (h *Handler) bootstrap(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(h.Root)
	if root == "" {
		root = "/workspace"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	gzipReader, err := newTarGzipReader(r.Body)
	if err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	defer gzipReader.Close()
	if err := extractTar(context.Background(), gzipReader, root); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]any{"error": err.Error()})
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"ok": true})
}

// tar streams the working tree as gzip tar. .git is excluded by the tar step.
func (h *Handler) tar(w http.ResponseWriter, r *http.Request) {
	root := strings.TrimSpace(h.Root)
	if root == "" {
		root = "/workspace"
	}
	w.Header().Set("Content-Type", "application/gzip")
	tw := newTarGzipWriter(w)
	defer tw.Close()
	if err := tarDirectory(r.Context(), tw, root); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
	if err := tw.Close(); err != nil {
		writeJSON(w, http.StatusInternalServerError, map[string]any{"error": err.Error()})
		return
	}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}

var errNoRoot = errors.New("workspace root does not exist")

func rootExists(root string) error {
	info, err := os.Stat(root)
	if err != nil {
		return fmt.Errorf("%w: %v", errNoRoot, err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace root is not a directory: %s", root)
	}
	return nil
}

var _ = filepath.Clean
