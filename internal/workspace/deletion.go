package workspace

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

func (s *Service) Delete(ctx context.Context, id string) error {
	s.deleteMu.Lock()
	defer s.deleteMu.Unlock()

	id = strings.TrimSpace(id)
	value, err := s.store.Get(ctx, id)
	if errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	if err != nil {
		return err
	}
	workspaceDirectory, err := s.managedWorkspaceDirectory(value)
	if err != nil {
		return err
	}
	if value.Status == StatusCreating || value.Status == StatusInitializing {
		return errors.New("workspace initialization is still running; wait before deleting it")
	}
	working, err := s.store.HasWorkingSession(ctx, id)
	if err != nil {
		return fmt.Errorf("inspect running sessions: %w", err)
	}
	if working {
		return errors.New("a session is still running; stop it before deleting the workspace")
	}
	if err := s.store.UpdateStatus(ctx, id, StatusDeleting, ""); err != nil {
		return fmt.Errorf("mark workspace for deletion: %w", err)
	}
	if err := removeManagedWorkspace(s.root, workspaceDirectory); err != nil {
		return s.failDeletion(ctx, id, fmt.Errorf("remove workspace files: %w", err))
	}
	if err := s.store.Delete(ctx, id); err != nil {
		return s.failDeletion(ctx, id, err)
	}
	return nil
}

func (s *Service) managedWorkspaceDirectory(value Workspace) (string, error) {
	workspaceDirectory := filepath.Join(s.root, value.ID)
	expectedRepository := filepath.Join(workspaceDirectory, "repo")
	if filepath.Clean(value.Path) != filepath.Clean(expectedRepository) {
		return "", errors.New("workspace path is outside the managed data root")
	}
	return workspaceDirectory, nil
}

func (s *Service) failDeletion(ctx context.Context, id string, cause error) error {
	if err := s.store.UpdateStatus(ctx, id, StatusDeletionFailed, cause.Error()); err != nil {
		return errors.Join(cause, fmt.Errorf("record workspace deletion failure: %w", err))
	}
	return cause
}

func removeManagedWorkspace(root, target string) error {
	root = strings.TrimSpace(root)
	if root == "" {
		return errors.New("managed workspace root is invalid")
	}
	root, err := filepath.Abs(root)
	if err != nil {
		return errors.New("managed workspace root is invalid")
	}
	target = strings.TrimSpace(target)
	if target == "" {
		return errors.New("managed workspace path is invalid")
	}
	target, err = filepath.Abs(target)
	if err != nil {
		return errors.New("managed workspace path is invalid")
	}
	relative, err := filepath.Rel(root, target)
	if err != nil || relative == "." || filepath.Dir(relative) != "." {
		return errors.New("workspace path is outside the managed data root")
	}
	info, err := os.Lstat(target)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return errors.New("managed workspace path is not a directory")
	}
	if err := filepath.WalkDir(target, makeDirectoryWritable); err != nil {
		return err
	}
	return os.RemoveAll(target)
}

func makeDirectoryWritable(path string, entry fs.DirEntry, walkErr error) error {
	if walkErr != nil {
		return walkErr
	}
	if !entry.IsDir() {
		return nil
	}
	info, err := entry.Info()
	if err != nil {
		return err
	}
	return os.Chmod(path, info.Mode().Perm()|0o700)
}
