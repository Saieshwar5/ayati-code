package workspace

import (
	"context"
	"errors"
	"strings"
)

func (s *Service) ConfigureProjectRoot(ctx context.Context, id, root string) error {
	value, err := s.store.Get(ctx, id)
	if err != nil {
		return err
	}
	if err := requireActiveWorkspace(value); err != nil {
		return err
	}
	if value.Status != StatusNeedsConfiguration {
		return errors.New("workspace is not waiting for project configuration")
	}
	root = strings.TrimSpace(root)
	for _, candidate := range value.ConfigurationCandidates {
		if candidate.ProjectRoot == root {
			return s.store.SelectProjectRoot(ctx, id, root)
		}
	}
	return errors.New("selected project root is not an available candidate")
}

func profilePreparationDetail(profile ProjectProfile) string {
	if profile.SetupCommand == "" {
		return "No dependency installation is required"
	}
	return "Running " + profile.SetupCommand
}
