package workspace

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

const maxProjectRootDepth = 3

var projectRootMarkers = map[string]bool{
	"Cargo.toml": true, "go.mod": true, "package.json": true,
	"pyproject.toml": true, "requirements.txt": true,
}

var ignoredProjectDirectories = map[string]bool{
	".git": true, ".venv": true, "node_modules": true, "vendor": true,
}

type ProjectSelectionRequiredError struct{ Candidates []ProjectCandidate }

func (e ProjectSelectionRequiredError) Error() string {
	var roots []string
	for _, candidate := range e.Candidates {
		roots = append(roots, candidate.ProjectRoot)
	}
	return fmt.Sprintf("multiple project roots detected: %s; project selection is required",
		strings.Join(roots, ", "))
}

func DiscoverProjectCandidates(repositoryRoot string) ([]ProjectCandidate, error) {
	roots, err := discoverProjectRootPaths(repositoryRoot)
	if err != nil {
		return nil, err
	}
	candidates := make([]ProjectCandidate, 0, len(roots))
	for _, root := range roots {
		candidate, err := describeProjectCandidate(repositoryRoot, root)
		if err != nil {
			return nil, err
		}
		candidates = append(candidates, candidate)
	}
	return candidates, nil
}

func resolveProjectRoot(repositoryRoot string) (string, error) {
	candidates, err := DiscoverProjectCandidates(repositoryRoot)
	if err != nil {
		return "", err
	}
	switch len(candidates) {
	case 0:
		return ".", nil
	case 1:
		return candidates[0].ProjectRoot, nil
	default:
		return "", ProjectSelectionRequiredError{Candidates: candidates}
	}
}

func discoverProjectRootPaths(repositoryRoot string) ([]string, error) {
	rootHasProject, err := directoryHasProjectMarker(repositoryRoot)
	if err != nil {
		return nil, err
	}
	if rootHasProject {
		return []string{"."}, nil
	}
	var candidates []string
	err = filepath.WalkDir(repositoryRoot, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if path == repositoryRoot || !entry.IsDir() {
			return nil
		}
		relative, err := filepath.Rel(repositoryRoot, path)
		if err != nil {
			return err
		}
		depth := len(strings.Split(relative, string(filepath.Separator)))
		if ignoredProjectDirectories[entry.Name()] || depth > maxProjectRootDepth {
			return filepath.SkipDir
		}
		hasProject, err := directoryHasProjectMarker(path)
		if err != nil {
			return err
		}
		if hasProject {
			candidates = append(candidates, filepath.ToSlash(relative))
			return filepath.SkipDir
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("discover project root: %w", err)
	}
	sort.Strings(candidates)
	return candidates, nil
}

func describeProjectCandidate(repositoryRoot, root string) (ProjectCandidate, error) {
	path := filepath.Join(repositoryRoot, filepath.FromSlash(root))
	entries, err := os.ReadDir(path)
	if err != nil {
		return ProjectCandidate{}, fmt.Errorf("describe project root %s: %w", root, err)
	}
	present := make(map[string]bool, len(entries))
	for _, entry := range entries {
		present[entry.Name()] = true
	}
	candidate := ProjectCandidate{ProjectRoot: root, Languages: []string{}, PackageManagers: []string{}}
	if present["go.mod"] {
		candidate.Languages = append(candidate.Languages, "Go")
		candidate.PackageManagers = append(candidate.PackageManagers, "Go modules")
	}
	if present["package.json"] {
		candidate.Languages = append(candidate.Languages, "Node.js")
		switch {
		case present["pnpm-lock.yaml"]:
			candidate.PackageManagers = append(candidate.PackageManagers, "pnpm")
		case present["yarn.lock"]:
			candidate.PackageManagers = append(candidate.PackageManagers, "Yarn")
		default:
			candidate.PackageManagers = append(candidate.PackageManagers, "npm")
		}
	}
	if present["requirements.txt"] || present["pyproject.toml"] {
		candidate.Languages = append(candidate.Languages, "Python")
		candidate.PackageManagers = append(candidate.PackageManagers, "pip")
	}
	if present["Cargo.toml"] {
		candidate.Languages = append(candidate.Languages, "Rust")
		candidate.PackageManagers = append(candidate.PackageManagers, "Cargo")
	}
	return candidate, nil
}

func directoryHasProjectMarker(path string) (bool, error) {
	entries, err := os.ReadDir(path)
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if projectRootMarkers[entry.Name()] {
			return true, nil
		}
	}
	return false, nil
}

func projectCommand(root, command string) string {
	if root == "." || command == "" {
		return command
	}
	return "cd " + shellQuote(root) + " && " + command
}
