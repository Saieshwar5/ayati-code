package workspace

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

func TestBuildEnvironmentSpecReadsMiseTools(t *testing.T) {
	root := t.TempDir()
	writeEnvironmentFile(t, root, ".mise.toml", `[tools]
go = "1.25"
node = "22.12"
python = "3.12.4"
`)
	writeEnvironmentFile(t, root, "go.mod", "module example.com/project\n\ngo 1.25\n")
	writeEnvironmentFile(t, root, "package.json", `{"scripts":{"build":"npm run build"}}`)

	spec, err := BuildEnvironmentSpec(root)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	versions := make(map[string]string)
	for _, tool := range spec.Toolchains {
		versions[tool.Name] = tool.Version
	}
	if versions["Go"] != "1.25" || versions["Node"] != "22.12" || versions["Python"] != "3.12.4" {
		t.Fatalf("toolchains = %#v", spec.Toolchains)
	}
	if !containsString(spec.SetupCommands, "mise install") {
		t.Fatalf("setup commands = %#v", spec.SetupCommands)
	}
}

func TestBuildEnvironmentSpecReadsToolVersions(t *testing.T) {
	root := t.TempDir()
	writeEnvironmentFile(t, root, ".tool-versions", "node 22.12.0\npython 3.12.4\n")
	spec, err := BuildEnvironmentSpec(root)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	versions := make(map[string]string)
	for _, tool := range spec.Toolchains {
		versions[tool.Name] = tool.Version
	}
	if versions["Node"] != "22.12.0" || versions["Python"] != "3.12.4" {
		t.Fatalf("toolchains = %#v", spec.Toolchains)
	}
	if spec.Toolchains[0].Source != ".tool-versions" {
		t.Fatalf("toolchain source = %#v", spec.Toolchains[0])
	}
}

func TestBuildEnvironmentSpecPrefersMiseVersion(t *testing.T) {
	root := t.TempDir()
	writeEnvironmentFile(t, root, ".mise.toml", `[tools]
node = "22"
`)
	writeEnvironmentFile(t, root, ".node-version", "20\n")
	writeEnvironmentFile(t, root, "package.json", `{}`)
	spec, err := BuildEnvironmentSpec(root)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	for _, tool := range spec.Toolchains {
		if tool.Name == "Node" && tool.Version != "22" {
			t.Fatalf("node toolchain = %#v", tool)
		}
	}
}

func TestBuildEnvironmentSpecReadsDevcontainer(t *testing.T) {
	root := t.TempDir()
	writeEnvironmentFile(t, root, "devcontainer.json", `{
  "postCreateCommand": "make setup",
  "features": {
    "ghcr.io/devcontainers/features/postgresql:1": {},
    "ghcr.io/devcontainers/features/redis:1": {}
  }
}`)
	spec, err := BuildEnvironmentSpec(root)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	if !containsString(spec.SetupCommands, "make setup") {
		t.Fatalf("setup commands = %#v", spec.SetupCommands)
	}
	serviceNames := make([]string, 0, len(spec.Services))
	for _, service := range spec.Services {
		serviceNames = append(serviceNames, service.Name)
	}
	if !containsString(serviceNames, "PostgreSQL") || !containsString(serviceNames, "Redis") {
		t.Fatalf("services = %#v", spec.Services)
	}
	if !containsString(spec.SourceFiles, "devcontainer.json") {
		t.Fatalf("source files = %#v", spec.SourceFiles)
	}
}

func TestBuildEnvironmentSpecFingerprintChangesWithManifest(t *testing.T) {
	root := t.TempDir()
	writeEnvironmentFile(t, root, "package.json", `{"name":"app","version":"1.0.0"}`)
	first, err := BuildEnvironmentSpec(root)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	writeEnvironmentFile(t, root, "package.json", `{"name":"app","version":"1.0.1"}`)
	second, err := BuildEnvironmentSpec(root)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	if first.Fingerprint == "" || first.Fingerprint == second.Fingerprint {
		t.Fatalf("fingerprints = %q and %q", first.Fingerprint, second.Fingerprint)
	}
}

func TestEnvironmentSpecPersistsWithProfile(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "perpetual.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	value, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	root := t.TempDir()
	writeEnvironmentFile(t, root, ".tool-versions", "node 22\n")
	spec, err := BuildEnvironmentSpec(root)
	if err != nil {
		t.Fatalf("BuildEnvironmentSpec: %v", err)
	}
	profile := ProjectProfile{
		ProjectRoot: ".", Languages: []string{"Node.js"},
		EnvironmentSpec: &spec,
	}
	if err := store.SaveProfile(context.Background(), value.ID, profile); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	loaded, err := store.ProjectProfile(context.Background(), value.ID)
	if err != nil {
		t.Fatalf("ProjectProfile: %v", err)
	}
	if loaded.EnvironmentSpec == nil ||
		!reflect.DeepEqual(*loaded.EnvironmentSpec, spec) {
		t.Fatalf("environment spec = %#v, want %#v", loaded.EnvironmentSpec, spec)
	}
}

func writeEnvironmentFile(t *testing.T, root, name, content string) {
	t.Helper()
	path := filepath.Join(root, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("WriteFile %s: %v", name, err)
	}
}

func containsString(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
