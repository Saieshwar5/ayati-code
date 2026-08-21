package workspace

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

const maxProjectMetadataBytes = 1 << 20

type ProjectProfile struct {
	ProjectRoot         string           `json:"project_root"`
	Languages           []string         `json:"languages"`
	RuntimeVersions     []string         `json:"runtime_versions"`
	PackageManagers     []string         `json:"package_managers"`
	Lockfiles           []string         `json:"lockfiles"`
	SetupCommand        string           `json:"setup_command"`
	TestCommand         string           `json:"test_command,omitempty"`
	LintCommand         string           `json:"lint_command,omitempty"`
	TypecheckCommand    string           `json:"typecheck_command,omitempty"`
	BuildCommand        string           `json:"build_command,omitempty"`
	InstructionsFile    string           `json:"instructions_file,omitempty"`
	ManifestFingerprint string           `json:"manifest_fingerprint"`
	BaselineCommit      string           `json:"baseline_commit,omitempty"`
	SetupResult         string           `json:"setup_result"`
	BaselineResult      string           `json:"baseline_result"`
	CachePath           string           `json:"cache_path"`
	EnvironmentSpec     *EnvironmentSpec `json:"environment_spec,omitempty"`
	PreparedAt          *time.Time       `json:"prepared_at,omitempty"`
}

type nodeManifest struct {
	PackageManager string            `json:"packageManager"`
	Scripts        map[string]string `json:"scripts"`
	Engines        map[string]string `json:"engines"`
}

func AnalyzeProject(root string) (ProjectProfile, error) {
	projectRoot, err := resolveProjectRoot(root)
	if err != nil {
		return ProjectProfile{}, err
	}
	return AnalyzeProjectAt(root, projectRoot)
}

func AnalyzeProjectAt(root, projectRoot string) (ProjectProfile, error) {
	projectRoot = filepath.ToSlash(filepath.Clean(strings.TrimSpace(projectRoot)))
	if projectRoot == "" || projectRoot == ".." || strings.HasPrefix(projectRoot, "../") ||
		filepath.IsAbs(projectRoot) {
		return ProjectProfile{}, errors.New("project root must stay inside the repository")
	}
	analysisRoot := filepath.Join(root, filepath.FromSlash(projectRoot))
	profile := ProjectProfile{
		ProjectRoot: projectRoot, Languages: []string{}, RuntimeVersions: []string{},
		PackageManagers: []string{}, Lockfiles: []string{},
		SetupResult: "pending", BaselineResult: "pending",
	}
	files := make(map[string][]byte)
	for _, name := range projectMetadataFiles {
		value, found, err := readProjectMetadata(analysisRoot, name)
		if err != nil {
			return ProjectProfile{}, err
		}
		if found {
			files[name] = value
		}
	}
	if _, unsupported := files["Cargo.toml"]; unsupported {
		return ProjectProfile{}, errors.New("Rust workspace preparation is not supported")
	}
	var setup, tests, lint, typecheck, build []string
	if goMod, ok := files["go.mod"]; ok {
		profile.Languages = append(profile.Languages, "Go")
		profile.PackageManagers = append(profile.PackageManagers, "Go modules")
		profile.RuntimeVersions = appendIf(profile.RuntimeVersions, goRuntime(goMod))
		setup = append(setup, "go mod download")
		tests = append(tests, "go test ./...")
		build = append(build, "go build ./...")
		if _, ok := files["go.sum"]; ok {
			profile.Lockfiles = append(profile.Lockfiles, "go.sum")
		}
	}
	if packageJSON, ok := files["package.json"]; ok {
		var manifest nodeManifest
		if err := json.Unmarshal(packageJSON, &manifest); err != nil {
			return ProjectProfile{}, fmt.Errorf("analyze package.json: %w", err)
		}
		manager, lockfile, install := nodeSetup(files, manifest.PackageManager)
		profile.Languages = append(profile.Languages, "Node.js")
		profile.PackageManagers = append(profile.PackageManagers, manager)
		profile.Lockfiles = appendIf(profile.Lockfiles, lockfile)
		profile.RuntimeVersions = appendIf(profile.RuntimeVersions, nodeRuntime(files, manifest))
		setup = appendIf(setup, install)
		tests = appendIf(tests, nodeScript(manager, manifest.Scripts, "test"))
		lint = appendIf(lint, nodeScript(manager, manifest.Scripts, "lint"))
		typecheck = appendIf(typecheck, nodeScript(manager, manifest.Scripts, "typecheck"))
		build = appendIf(build, nodeScript(manager, manifest.Scripts, "build"))
	}
	if _, requirements := files["requirements.txt"]; requirements {
		profile.Languages = append(profile.Languages, "Python")
		profile.PackageManagers = append(profile.PackageManagers, "pip")
		profile.Lockfiles = append(profile.Lockfiles, "requirements.txt")
		profile.RuntimeVersions = appendIf(profile.RuntimeVersions, pythonRuntime(files))
		setup = append(setup, "python3 -m venv .venv && .venv/bin/pip install -r requirements.txt")
	} else if _, pyproject := files["pyproject.toml"]; pyproject {
		profile.Languages = append(profile.Languages, "Python")
		profile.PackageManagers = append(profile.PackageManagers, "pip")
		profile.RuntimeVersions = appendIf(profile.RuntimeVersions, pythonRuntime(files))
		setup = append(setup, "python3 -m venv .venv && .venv/bin/pip install .")
	}
	if _, ok := files["AGENTS.md"]; ok {
		profile.InstructionsFile = "AGENTS.md"
	}
	profile.SetupCommand = projectCommand(projectRoot, joinCommands(setup))
	profile.TestCommand = projectCommand(projectRoot, joinCommands(tests))
	profile.LintCommand = projectCommand(projectRoot, joinCommands(lint))
	profile.TypecheckCommand = projectCommand(projectRoot, joinCommands(typecheck))
	profile.BuildCommand = projectCommand(projectRoot, joinCommands(build))
	profile.ManifestFingerprint = fingerprint(projectRoot, files)
	return profile, nil
}

var projectMetadataFiles = []string{
	"AGENTS.md", "Cargo.toml", ".node-version", ".nvmrc", ".python-version",
	"go.mod", "go.sum", "package.json", "package-lock.json", "pnpm-lock.yaml",
	"yarn.lock", "pyproject.toml", "requirements.txt",
}

func readProjectMetadata(root, name string) ([]byte, bool, error) {
	path := filepath.Join(root, name)
	info, err := os.Lstat(path)
	if errors.Is(err, os.ErrNotExist) {
		return nil, false, nil
	}
	if err != nil {
		return nil, false, fmt.Errorf("inspect %s: %w", name, err)
	}
	if !info.Mode().IsRegular() {
		return nil, false, fmt.Errorf("project metadata %s must be a regular file", name)
	}
	if info.Size() > maxProjectMetadataBytes {
		return nil, false, fmt.Errorf("project metadata %s exceeds 1 MiB", name)
	}
	value, err := os.ReadFile(path)
	if err != nil {
		return nil, false, fmt.Errorf("read %s: %w", name, err)
	}
	return value, true, nil
}

func nodeSetup(files map[string][]byte, declared string) (string, string, string) {
	switch {
	case files["pnpm-lock.yaml"] != nil || strings.HasPrefix(declared, "pnpm@"):
		return "pnpm", present(files, "pnpm-lock.yaml"), "corepack pnpm install --frozen-lockfile"
	case files["yarn.lock"] != nil || strings.HasPrefix(declared, "yarn@"):
		return "Yarn", present(files, "yarn.lock"), "corepack yarn install --immutable"
	case files["package-lock.json"] != nil:
		return "npm", "package-lock.json", "npm ci"
	default:
		return "npm", "", "npm install"
	}
}

func nodeScript(manager string, scripts map[string]string, name string) string {
	if strings.TrimSpace(scripts[name]) == "" {
		return ""
	}
	switch manager {
	case "pnpm":
		return "corepack pnpm run " + name
	case "Yarn":
		return "corepack yarn run " + name
	default:
		return "npm run " + name
	}
}

func goRuntime(value []byte) string {
	for _, line := range strings.Split(string(value), "\n") {
		fields := strings.Fields(line)
		if len(fields) == 2 && fields[0] == "go" {
			return "Go " + fields[1]
		}
	}
	return ""
}

func nodeRuntime(files map[string][]byte, manifest nodeManifest) string {
	for _, name := range []string{".nvmrc", ".node-version"} {
		if value := strings.TrimSpace(string(files[name])); value != "" {
			return "Node " + value
		}
	}
	if value := strings.TrimSpace(manifest.Engines["node"]); value != "" {
		return "Node " + value
	}
	return "Node (default)"
}

func pythonRuntime(files map[string][]byte) string {
	if value := strings.TrimSpace(string(files[".python-version"])); value != "" {
		return "Python " + value
	}
	return "Python (default)"
}

func fingerprint(projectRoot string, files map[string][]byte) string {
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	hash := sha256.New()
	hash.Write([]byte(projectRoot))
	hash.Write([]byte{0})
	for _, name := range names {
		hash.Write([]byte(name))
		hash.Write([]byte{0})
		hash.Write(files[name])
		hash.Write([]byte{0})
	}
	return hex.EncodeToString(hash.Sum(nil))
}

func appendIf(values []string, value string) []string {
	if strings.TrimSpace(value) != "" {
		return append(values, value)
	}
	return values
}

func present(files map[string][]byte, name string) string {
	if files[name] != nil {
		return name
	}
	return ""
}

func joinCommands(values []string) string { return strings.Join(values, " && ") }
