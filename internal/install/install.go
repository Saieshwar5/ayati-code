package install

import (
	"fmt"
	"os"
	"path/filepath"
)

func Current() (string, error) {
	executable, err := os.Executable()
	if err != nil {
		return "", fmt.Errorf("find executable: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(executable)
	if err != nil {
		return "", fmt.Errorf("resolve executable: %w", err)
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("find home directory: %w", err)
	}
	return Link(resolved, filepath.Join(home, ".local", "bin"))
}

func Link(executable, binDir string) (string, error) {
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		return "", fmt.Errorf("create install directory: %w", err)
	}
	target := filepath.Join(binDir, "ayati-code")
	if info, err := os.Lstat(target); err == nil {
		if info.Mode()&os.ModeSymlink == 0 {
			return "", fmt.Errorf("refusing to replace non-symlink %s", target)
		}
		if err := os.Remove(target); err != nil {
			return "", fmt.Errorf("replace existing link: %w", err)
		}
	} else if !os.IsNotExist(err) {
		return "", fmt.Errorf("inspect install target: %w", err)
	}
	if err := os.Symlink(executable, target); err != nil {
		return "", fmt.Errorf("install command: %w", err)
	}
	return target, nil
}
