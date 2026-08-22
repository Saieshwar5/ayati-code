package database

import (
	"context"
	"strings"
	"testing"
)

func TestOpenSQLiteMemory(t *testing.T) {
	database, err := Open(":memory:")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer database.Close()
	if database.Provider() != ProviderSQLite {
		t.Fatalf("provider = %q", database.Provider())
	}
	version, err := database.SchemaVersion(context.Background())
	if err != nil {
		t.Fatalf("SchemaVersion: %v", err)
	}
	if version != 0 {
		t.Fatalf("version = %d", version)
	}
}

func TestOpenPostgresValidatesDSN(t *testing.T) {
	if _, err := OpenPostgres(context.Background(), "  "); err == nil ||
		!strings.Contains(err.Error(), "connection string") {
		t.Fatalf("OpenPostgres error = %v", err)
	}
}

func TestOpenConfiguredDefaultsToSQLite(t *testing.T) {
	database, err := OpenConfigured(context.Background(), Config{URL: ":memory:"})
	if err != nil {
		t.Fatalf("OpenConfigured: %v", err)
	}
	defer database.Close()
	if database.Dialect() != ProviderSQLite {
		t.Fatalf("dialect = %q", database.Dialect())
	}
}

func TestOpenConfiguredRejectsUnknownProvider(t *testing.T) {
	if _, err := OpenConfigured(context.Background(), Config{
		Provider: "oracle",
		URL:      "x",
	}); err == nil || !strings.Contains(err.Error(), "unsupported") {
		t.Fatalf("OpenConfigured error = %v", err)
	}
}
