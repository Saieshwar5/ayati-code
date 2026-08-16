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

func resolveProjectRoot(repositoryRoot string) (string, error) {
	rootHasProject, err := directoryHasProjectMarker(repositoryRoot)
	if err != nil {
		return "", err
	}
	if rootHasProject {
		return ".", nil
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
		return "", fmt.Errorf("discover project root: %w", err)
	}
	sort.Strings(candidates)
	switch len(candidates) {
	case 0:
		return ".", nil
	case 1:
		return candidates[0], nil
	default:
		return "", fmt.Errorf("multiple project roots detected: %s; project selection is required",
			strings.Join(candidates, ", "))
	}
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
