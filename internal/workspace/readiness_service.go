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
	parts := append([]string{}, profile.Languages...)
	parts = append(parts, profile.PackageManagers...)
	if profile.ProjectRoot != "." {
		parts = append(parts, profile.ProjectRoot)
	}
	if len(parts) == 0 {
		return "Generic repository"
	}
	return strings.Join(parts, " · ")
}
