package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestAnalyzeProjectResolvesGoProfile(t *testing.T) {
	root := writeProjectFiles(t, map[string]string{
		"go.mod":    "module example.test/project\n\ngo 1.25\n",
		"go.sum":    "example.test/module v1.0.0 h1:test\n",
		"AGENTS.md": "# Instructions\n",
	})
	profile, err := AnalyzeProject(root)
	if err != nil {
		t.Fatalf("AnalyzeProject: %v", err)
	}
	if !reflect.DeepEqual(profile.Languages, []string{"Go"}) ||
		!reflect.DeepEqual(profile.RuntimeVersions, []string{"Go 1.25"}) ||
		profile.SetupCommand != "go mod download" || profile.TestCommand != "go test ./..." ||
		profile.BuildCommand != "go build ./..." || profile.InstructionsFile != "AGENTS.md" ||
		len(profile.ManifestFingerprint) != 64 {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestAnalyzeProjectResolvesNodeProfile(t *testing.T) {
	root := writeProjectFiles(t, map[string]string{
		"package.json":   `{"scripts":{"test":"vitest","lint":"eslint .","typecheck":"tsc --noEmit","build":"vite build"}}`,
		"pnpm-lock.yaml": "lockfileVersion: '9.0'\n",
		".nvmrc":         "22\n",
	})
	profile, err := AnalyzeProject(root)
	if err != nil {
		t.Fatalf("AnalyzeProject: %v", err)
	}
	if !reflect.DeepEqual(profile.PackageManagers, []string{"pnpm"}) ||
		!reflect.DeepEqual(profile.Lockfiles, []string{"pnpm-lock.yaml"}) ||
		profile.RuntimeVersions[0] != "Node 22" ||
		profile.SetupCommand != "corepack pnpm install --frozen-lockfile" ||
		profile.TestCommand != "corepack pnpm run test" ||
		profile.LintCommand != "corepack pnpm run lint" ||
		profile.TypecheckCommand != "corepack pnpm run typecheck" ||
		profile.BuildCommand != "corepack pnpm run build" {
		t.Fatalf("profile = %#v", profile)
	}
}

func TestAnalyzeProjectResolvesPythonProfile(t *testing.T) {
	root := writeProjectFiles(t, map[string]string{
		"pyproject.toml":  "[project]\nname = \"example\"\n",
		".python-version": "3.13\n",
	})
	profile, err := AnalyzeProject(root)
	if err != nil || profile.SetupCommand != "python3 -m venv .venv && .venv/bin/pip install ." ||
		!reflect.DeepEqual(profile.RuntimeVersions, []string{"Python 3.13"}) {
		t.Fatalf("profile = %#v, error = %v", profile, err)
	}
}

func TestAnalyzeProjectCombinesSupportedRootProjects(t *testing.T) {
	root := writeProjectFiles(t, map[string]string{
		"go.mod":            "module example.test/project\n\ngo 1.25\n",
		"package.json":      `{"scripts":{"test":"vitest","build":"vite build"}}`,
		"package-lock.json": `{"lockfileVersion":3}`,
		"requirements.txt":  "pytest==8.0.0\n",
	})
	want := "go mod download && npm ci && python3 -m venv .venv && .venv/bin/pip install -r requirements.txt"
	profile, err := AnalyzeProject(root)
	if err != nil || profile.SetupCommand != want || profile.TestCommand != "go test ./... && npm run test" {
		t.Fatalf("AnalyzeProject = %#v, error = %v", profile, err)
	}
}

func TestAnalyzeProjectRefusesMetadataSymlink(t *testing.T) {
	root := t.TempDir()
	if err := os.Symlink("/etc/passwd", filepath.Join(root, "package.json")); err != nil {
		t.Fatalf("Symlink: %v", err)
	}
	if _, err := AnalyzeProject(root); err == nil || !strings.Contains(err.Error(), "regular file") {
		t.Fatalf("AnalyzeProject error = %v", err)
	}
}

func TestAnalyzeProjectSelectsOneNestedProject(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "apps", "web")
	if err := os.MkdirAll(project, 0o700); err != nil {
		t.Fatalf("MkdirAll: %v", err)
	}
	if err := os.WriteFile(filepath.Join(project, "package.json"), []byte(`{"scripts":{"test":"vitest"}}`), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	profile, err := AnalyzeProject(root)
	if err != nil || profile.ProjectRoot != "apps/web" ||
		profile.SetupCommand != "cd 'apps/web' && npm install" ||
		profile.TestCommand != "cd 'apps/web' && npm run test" {
		t.Fatalf("profile = %#v, error = %v", profile, err)
	}
}

func TestAnalyzeProjectRequiresSelectionForMultipleProjects(t *testing.T) {
	root := t.TempDir()
	for _, name := range []string{"apps/api", "apps/web"} {
		path := filepath.Join(root, filepath.FromSlash(name))
		if err := os.MkdirAll(path, 0o700); err != nil {
			t.Fatalf("MkdirAll: %v", err)
		}
		if err := os.WriteFile(filepath.Join(path, "package.json"), []byte(`{}`), 0o600); err != nil {
			t.Fatalf("WriteFile: %v", err)
		}
	}
	_, err := AnalyzeProject(root)
	if err == nil || !strings.Contains(err.Error(), "apps/api, apps/web") {
		t.Fatalf("AnalyzeProject error = %v", err)
	}
	var selection ProjectSelectionRequiredError
	if !errors.As(err, &selection) || len(selection.Candidates) != 2 ||
		selection.Candidates[1].ProjectRoot != "apps/web" ||
		!reflect.DeepEqual(selection.Candidates[1].Languages, []string{"Node.js"}) ||
		!reflect.DeepEqual(selection.Candidates[1].PackageManagers, []string{"npm"}) {
		t.Fatalf("selection = %#v", selection)
	}
}

func TestStorePersistsWorkspaceProjectProfile(t *testing.T) {
	store, err := Open(filepath.Join(t.TempDir(), "ayati.db"))
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	t.Cleanup(func() { _ = store.Close() })
	workspace, err := store.Create(context.Background(), Create{
		Repository: "owner/project", CloneURL: "https://github.com/owner/project.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	preparedAt := time.Now().UTC().Truncate(time.Nanosecond)
	want := ProjectProfile{
		ProjectRoot: ".", Languages: []string{"Go", "Node.js"},
		RuntimeVersions: []string{"Go 1.25", "Node 22"},
		PackageManagers: []string{"Go modules", "npm"}, Lockfiles: []string{"go.sum", "package-lock.json"},
		SetupCommand: "go mod download && npm ci", ManifestFingerprint: strings.Repeat("a", 64),
		BaselineCommit: "abc123", SetupResult: "passed", BaselineResult: "clean",
		CachePath: filepath.Join(t.TempDir(), "cache"), PreparedAt: &preparedAt,
	}
	if err := store.SaveProfile(context.Background(), workspace.ID, want); err != nil {
		t.Fatalf("SaveProfile: %v", err)
	}
	loaded, err := store.Get(context.Background(), workspace.ID)
	if err != nil || loaded.Profile == nil || !reflect.DeepEqual(*loaded.Profile, want) {
		t.Fatalf("workspace profile = %#v, error = %v", loaded.Profile, err)
	}
	listed, err := store.List(context.Background())
	if err != nil || len(listed) != 1 || listed[0].Profile == nil {
		t.Fatalf("listed workspaces = %#v, error = %v", listed, err)
	}
}

func writeProjectFiles(t *testing.T, files map[string]string) string {
	t.Helper()
	root := t.TempDir()
	for name, contents := range files {
		if err := os.WriteFile(filepath.Join(root, name), []byte(contents), 0o600); err != nil {
			t.Fatalf("WriteFile %s: %v", name, err)
		}
	}
	return root
}
