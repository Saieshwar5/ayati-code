package workspace

import (
	"encoding/json"
	"errors"
	"fmt"
	"path/filepath"
	"sort"
	"strings"
)

// Toolchain is one language runtime requirement resolved from repository
// metadata such as a mise manifest or a package manifest.
type Toolchain struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`
}

// Service is one external development service, such as a database, that the
// environment should make available to the workspace.
type DevService struct {
	Name    string `json:"name"`
	Version string `json:"version,omitempty"`
	Source  string `json:"source,omitempty"`
}

// EnvironmentSpec is the deterministic, searchable definition of how to build
// a reusable workspace environment for a project root. It must never contain
// secret values.
type EnvironmentSpec struct {
	ProjectRoot      string       `json:"project_root"`
	Toolchains       []Toolchain  `json:"toolchains"`
	PackageManagers  []string     `json:"package_managers"`
	Lockfiles        []string     `json:"lockfiles"`
	SetupCommands    []string     `json:"setup_commands"`
	VerifyCommands   []string     `json:"verify_commands"`
	BuildCommands    []string     `json:"build_commands"`
	TestCommands     []string     `json:"test_commands"`
	Services         []DevService `json:"services"`
	SourceFiles      []string     `json:"source_files"`
	Fingerprint      string       `json:"fingerprint"`
	InstructionsFile string       `json:"instructions_file,omitempty"`
}

// environmentMetadataFiles are the files the deterministic engine reads when
// building an environment spec. The list augments the workspace profile's
// project metadata with toolchain and container development signals.
var environmentMetadataFiles = append([]string{
	".mise.toml", ".tool-versions", "devcontainer.json", "uv.lock",
	"rust-toolchain.toml", "rust-toolchain",
}, projectMetadataFiles...)

// BuildEnvironmentSpec discovers the project root and builds an environment
// spec for the single selected project root.
func BuildEnvironmentSpec(root string) (EnvironmentSpec, error) {
	projectRoot, err := resolveProjectRoot(root)
	if err != nil {
		return EnvironmentSpec{}, err
	}
	return BuildEnvironmentSpecAt(root, projectRoot)
}

// BuildEnvironmentSpecAt builds the deterministic environment definition for
// one project root inside the repository. It never guesses across multiple
// nested roots; callers that need selection use DiscoverProjectCandidates.
func BuildEnvironmentSpecAt(root, projectRoot string) (EnvironmentSpec, error) {
	projectRoot = filepath.ToSlash(filepath.Clean(strings.TrimSpace(projectRoot)))
	if projectRoot == "" || projectRoot == ".." || strings.HasPrefix(projectRoot, "../") ||
		filepath.IsAbs(projectRoot) {
		return EnvironmentSpec{}, errors.New("project root must stay inside the repository")
	}
	analysisRoot := filepath.Join(root, filepath.FromSlash(projectRoot))
	files := make(map[string][]byte)
	for _, name := range environmentMetadataFiles {
		value, found, err := readProjectMetadata(analysisRoot, name)
		if err != nil {
			return EnvironmentSpec{}, err
		}
		if found {
			files[name] = value
		}
	}
	toolVersions := parseToolVersions(files[".tool-versions"])
	miseTools := parseMiseTools(files[".mise.toml"])
	container := parseDevcontainer(files["devcontainer.json"])

	spec := EnvironmentSpec{
		ProjectRoot: projectRoot, Toolchains: []Toolchain{}, PackageManagers: []string{},
		Lockfiles: []string{}, SetupCommands: []string{}, VerifyCommands: []string{},
		BuildCommands: []string{}, TestCommands: []string{}, Services: []DevService{},
		SourceFiles: []string{},
	}

	if len(toolVersions) != 0 || len(miseTools) != 0 {
		spec.SetupCommands = appendIf(spec.SetupCommands, "mise install")
		for _, name := range sortedMergeToolNames(toolVersions, miseTools) {
			version, source := mergedToolVersion(toolVersions, miseTools, name)
			spec.Toolchains = append(spec.Toolchains, Toolchain{
				Name: canonicalToolName(name), Version: version, Source: source,
			})
		}
	}

	if goMod, ok := files["go.mod"]; ok {
		spec.PackageManagers = appendIf(spec.PackageManagers, "Go modules")
		spec.Lockfiles = appendIf(spec.Lockfiles, sumLockfile(files))
		spec.SetupCommands = appendIf(spec.SetupCommands, "go mod download")
		spec.BuildCommands = appendIf(spec.BuildCommands, "go build ./...")
		spec.TestCommands = appendIf(spec.TestCommands, "go test ./...")
		spec.Toolchains = appendToolchain(spec.Toolchains, "Go",
			toolchainVersion(goRuntime(goMod)), "go.mod")
	}

	if packageJSON, ok := files["package.json"]; ok {
		var manifest nodeManifest
		if err := json.Unmarshal(packageJSON, &manifest); err != nil {
			return EnvironmentSpec{}, fmt.Errorf("analyze package.json for environment spec: %w", err)
		}
		manager, lockfile, install := nodeSetup(files, manifest.PackageManager)
		spec.PackageManagers = appendIf(spec.PackageManagers, manager)
		spec.Lockfiles = appendIf(spec.Lockfiles, lockfile)
		spec.SetupCommands = appendIf(spec.SetupCommands, install)
		if script := nodeScript(manager, manifest.Scripts, "test"); script != "" {
			spec.TestCommands = appendIf(spec.TestCommands, script)
		}
		if script := nodeScript(manager, manifest.Scripts, "build"); script != "" {
			spec.BuildCommands = appendIf(spec.BuildCommands, script)
		}
		if script := nodeScript(manager, manifest.Scripts, "lint"); script != "" {
			spec.VerifyCommands = appendIf(spec.VerifyCommands, script)
		}
		version, source := nodeToolchain(files, manifest)
		if version != "" || source != "" {
			spec.Toolchains = appendToolchain(spec.Toolchains, "Node", version, source)
		}
	}

	if _, ok := files["uv.lock"]; ok {
		spec.PackageManagers = appendIf(spec.PackageManagers, "uv")
		spec.Lockfiles = appendIf(spec.Lockfiles, "uv.lock")
		spec.SetupCommands = appendIf(spec.SetupCommands, "uv sync")
	} else if _, ok := files["pyproject.toml"]; ok {
		spec.PackageManagers = appendIf(spec.PackageManagers, "pip")
		spec.SetupCommands = appendIf(spec.SetupCommands,
			"python3 -m venv .venv && .venv/bin/pip install .")
	} else if _, ok := files["requirements.txt"]; ok {
		spec.PackageManagers = appendIf(spec.PackageManagers, "pip")
		spec.Lockfiles = appendIf(spec.Lockfiles, "requirements.txt")
		spec.SetupCommands = appendIf(spec.SetupCommands,
			"python3 -m venv .venv && .venv/bin/pip install -r requirements.txt")
	}
	if _, ok := files["pyproject.toml"]; ok || len(files["requirements.txt"]) != 0 {
		version := toolchainVersion(pythonRuntime(files))
		if version != "" || len(files["pyproject.toml"]) != 0 {
			spec.Toolchains = appendToolchain(spec.Toolchains, "Python",
				version, pythonToolchainSource(files))
		}
	}

	spec.Toolchains = dedupeToolchains(spec.Toolchains)
	if command := devcontainerSetupCommand(container.PostCreateCommand); command != "" {
		spec.SetupCommands = appendIf(spec.SetupCommands, command)
	}
	spec.Services = servicesFromDevcontainer(container.Features)

	if len(spec.VerifyCommands) == 0 {
		spec.VerifyCommands = append(spec.VerifyCommands, spec.TestCommands...)
	}

	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	spec.SourceFiles = names
	spec.InstructionsFile = instructionsFile(files)
	spec.Fingerprint = fingerprint(projectRoot, files)
	return spec, nil
}
