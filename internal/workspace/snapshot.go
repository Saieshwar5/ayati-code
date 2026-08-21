package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	maxSnapshotBytes      = 1 << 30 // 1 GiB safety bound
	snapshotDirectoryName = "environment-snapshots"
)

// EnvironmentSnapshot describes a captured reusable prepared state.
type EnvironmentSnapshot struct {
	Type      string
	Ref       string
	Manifest  []string
	Bytes     int64
	CreatedAt time.Time
}

// usableEnvironmentSnapshot reports whether a ready version has a snapshot
// that is safe and meaningful enough to restore instead of running setup.
func usableEnvironmentSnapshot(version EnvironmentVersion) bool {
	return version.SnapshotType == SnapshotTypeLocalCopy &&
		strings.TrimSpace(version.SnapshotRef) != "" && len(version.SnapshotManifest) > 0
}

// captureWorkspaceSnapshot copies ignored and untracked setup outputs from the
// workspace repository into a managed snapshot directory, keyed by version ID.
func (s *Service) captureWorkspaceSnapshot(
	ctx context.Context, value Workspace, versionID string,
) (EnvironmentSnapshot, error) {
	target, err := managedSnapshotPath(s.root, versionID)
	if err != nil {
		return EnvironmentSnapshot{}, err
	}
	if err := os.RemoveAll(target); err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("clear environment snapshot: %w", err)
	}
	output, err := s.git.Output(ctx, "-C", value.Path,
		"ls-files", "--others", "--ignored", "--exclude-standard", "-z")
	if err != nil {
		return EnvironmentSnapshot{}, fmt.Errorf("list environment snapshot files: %w", err)
	}
	now := time.Now().UTC()
	snapshot := EnvironmentSnapshot{
		Type: SnapshotTypeLocalCopy, Ref: target, CreatedAt: now,
	}
	var total int64
	for _, relative := range strings.Split(output, "\x00") {
		if strings.TrimSpace(relative) == "" {
			continue
		}
		if !safeRelativeWorkspacePath(relative) {
			continue
		}
		source := filepath.Join(value.Path, filepath.FromSlash(relative))
		destination := filepath.Join(target, filepath.FromSlash(relative))
		copied, err := copyWorkspaceFile(source, destination)
		if err != nil {
			_ = os.RemoveAll(target)
			return EnvironmentSnapshot{}, err
		}
		if copied == 0 {
			continue
		}
		total += copied
		if total > maxSnapshotBytes {
			_ = os.RemoveAll(target)
			return EnvironmentSnapshot{}, errors.New("environment snapshot exceeds safety limit")
		}
		snapshot.Manifest = append(snapshot.Manifest, relative)
	}
	snapshot.Bytes = total
	if len(snapshot.Manifest) == 0 {
		_ = os.RemoveAll(target)
	}
	return snapshot, nil
}

// restoreWorkspaceSnapshot materializes a captured local_copy snapshot into
// the workspace repository path.
func (s *Service) restoreWorkspaceSnapshot(ctx context.Context, value Workspace, version EnvironmentVersion) error {
	ref, err := managedSnapshotRef(s.root, version.SnapshotRef)
	if err != nil {
		return err
	}
	for _, relative := range version.SnapshotManifest {
		if !safeRelativeWorkspacePath(relative) {
			return fmt.Errorf("unsafe snapshot path %q", relative)
		}
		source := filepath.Join(ref, filepath.FromSlash(relative))
		destination := filepath.Join(value.Path, filepath.FromSlash(relative))
		if _, err := copyWorkspaceFile(source, destination); err != nil {
			return fmt.Errorf("restore snapshot file %s: %w", relative, err)
		}
	}
	return nil
}

func managedSnapshotRef(root, ref string) (string, error) {
	if strings.TrimSpace(ref) == "" {
		return "", errors.New("snapshot reference is empty")
	}
	root, err := filepath.Abs(strings.TrimSpace(root))
	if err != nil {
		return "", fmt.Errorf("resolve managed snapshot root: %w", err)
	}
	ref, err = filepath.Abs(ref)
	if err != nil {
		return "", fmt.Errorf("resolve snapshot reference: %w", err)
	}
	relative, err := filepath.Rel(root, ref)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("snapshot reference is outside the managed root")
	}
	first := strings.SplitN(relative, string(filepath.Separator), 2)[0]
	if first != snapshotDirectoryName {
		return "", errors.New("snapshot reference is not an environment snapshot")
	}
	return ref, nil
}

func managedSnapshotPath(root, versionID string) (string, error) {
	root = strings.TrimSpace(root)
	if root == "" {
		return "", errors.New("managed snapshot root is unavailable")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve managed snapshot root: %w", err)
	}
	target := filepath.Clean(filepath.Join(root, snapshotDirectoryName, filepath.Base(versionID)))
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || relative == ".." ||
		strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
		return "", errors.New("snapshot path is outside the managed root")
	}
	return target, nil
}

// safeRelativeWorkspacePath rejects absolute paths, parent traversal, and any
// path that escapes the repository or touches the Git metadata.
func safeRelativeWorkspacePath(relative string) bool {
	if relative == "" || filepath.IsAbs(relative) {
		return false
	}
	cleaned := filepath.Clean(filepath.FromSlash(relative))
	if cleaned == "." || cleaned == ".." || strings.HasPrefix(cleaned, ".."+string(filepath.Separator)) {
		return false
	}
	if cleaned == ".git" || strings.HasPrefix(cleaned, ".git"+string(filepath.Separator)) {
		return false
	}
	return true
}

func copyWorkspaceFile(source, destination string) (int64, error) {
	info, err := os.Lstat(source)
	if err != nil {
		return 0, err
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return 0, errors.New("refusing to snapshot symlink")
	}
	if !info.Mode().IsRegular() {
		return 0, nil
	}
	input, err := os.Open(source)
	if err != nil {
		return 0, err
	}
	defer input.Close()
	if err := os.MkdirAll(filepath.Dir(destination), 0o700); err != nil {
		return 0, err
	}
	output, err := os.OpenFile(destination, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode().Perm())
	if err != nil {
		return 0, err
	}
	written, copyErr := io.Copy(output, input)
	closeErr := output.Close()
	if copyErr != nil {
		return 0, copyErr
	}
	if closeErr != nil {
		return 0, closeErr
	}
	return written, nil
}
