package workspace

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestStoreEncryptsAndManagesWorkspaceEnvironment(t *testing.T) {
	root := t.TempDir()
	store, err := Open(filepath.Join(root, "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	keyInfo, err := os.Stat(filepath.Join(root, "environment.key"))
	if err != nil || keyInfo.Mode().Perm() != 0o600 {
		t.Fatalf("environment key permissions = %v, error = %v", keyInfo.Mode().Perm(), err)
	}
	created, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/environment", Path: filepath.Join(root, "repo"),
		Environment: []EnvironmentInput{
			{Name: "DATABASE_URL", Value: "postgres://private", ExposeDuringSetup: false},
			{Name: "NPM_TOKEN", Value: "npm-private", ExposeDuringSetup: true},
		},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	metadata, err := store.ListEnvironment(context.Background(), created.ID)
	if err != nil || len(metadata) != 2 || metadata[0].Name != "DATABASE_URL" || !metadata[1].Configured {
		t.Fatalf("environment metadata = %#v, error = %v", metadata, err)
	}
	setup, err := store.EnvironmentValues(context.Background(), created.ID, true)
	if err != nil || len(setup) != 1 || setup["NPM_TOKEN"] != "npm-private" {
		t.Fatalf("setup environment = %#v, error = %v", setup, err)
	}
	all, err := store.EnvironmentValues(context.Background(), created.ID, false)
	if err != nil || all["DATABASE_URL"] != "postgres://private" || all["NPM_TOKEN"] != "npm-private" {
		t.Fatalf("workspace environment = %#v, error = %v", all, err)
	}
	var ciphertext []byte
	if err := store.db.QueryRow(`SELECT ciphertext FROM workspace_environment
		WHERE workspace_id = ? AND name = 'DATABASE_URL'`, created.ID).Scan(&ciphertext); err != nil {
		t.Fatalf("load ciphertext: %v", err)
	}
	if bytes.Contains(ciphertext, []byte("postgres://private")) {
		t.Fatal("ciphertext contains the plaintext value")
	}
	if _, err := store.UpsertEnvironment(context.Background(), created.ID,
		EnvironmentInput{Name: "DATABASE_URL", Value: "postgres://replacement", ExposeDuringSetup: true}); err != nil {
		t.Fatalf("UpsertEnvironment: %v", err)
	}
	if err := store.DeleteEnvironment(context.Background(), created.ID, "NPM_TOKEN"); err != nil {
		t.Fatalf("DeleteEnvironment: %v", err)
	}
	all, err = store.EnvironmentValues(context.Background(), created.ID, false)
	if err != nil || len(all) != 1 || all["DATABASE_URL"] != "postgres://replacement" {
		t.Fatalf("updated environment = %#v, error = %v", all, err)
	}
}

func TestStoreRejectsUnsafeEnvironmentVariables(t *testing.T) {
	store, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	for _, input := range []EnvironmentInput{
		{Name: "INVALID-NAME", Value: "value"},
		{Name: "PATH", Value: "/tmp"},
		{Name: "AYATI_GITHUB_TOKEN", Value: "value"},
		{Name: "NULL_VALUE", Value: "bad\x00value"},
	} {
		if err := validateEnvironmentInput(&input); err == nil {
			t.Fatalf("accepted environment variable %q", input.Name)
		}
	}
	if err := store.DeleteEnvironment(context.Background(), "missing", "MISSING"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("DeleteEnvironment error = %v", err)
	}
}

func TestStoreReopensEncryptedWorkspaceEnvironment(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "ayati.db")
	store, err := Open(database)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	created, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "ayati/reopen", Path: filepath.Join(root, "repo"),
		Environment: []EnvironmentInput{{Name: "PROJECT_TOKEN", Value: "persistent-secret"}},
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	store, err = Open(database)
	if err != nil {
		t.Fatalf("Reopen: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	values, err := store.EnvironmentValues(context.Background(), created.ID, false)
	if err != nil || values["PROJECT_TOKEN"] != "persistent-secret" {
		t.Fatalf("reopened environment = %#v, error = %v", values, err)
	}
}

func TestStoreRefusesToReplaceMissingEnvironmentKey(t *testing.T) {
	root := t.TempDir()
	database := filepath.Join(root, "ayati.db")
	store, err := Open(database)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	if err := store.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if err := os.Remove(filepath.Join(root, "environment.key")); err != nil {
		t.Fatalf("Remove environment key: %v", err)
	}
	if _, err := Open(database); err == nil || !strings.Contains(err.Error(), "environment key is missing") {
		t.Fatalf("Open error = %v", err)
	}
}
