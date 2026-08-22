package environments

import (
	"bytes"
	"context"
	"fmt"
	"os"

	"github.com/Saieshwar5/perpetual/internal/vmagent"
)

// Syncer moves the working tree between the controller and a microVM through
// the authenticated data plane. Git and GitHub credentials never travel.
type Syncer struct {
	manager *Manager
}

// NewSyncer builds a repo syncer.
func NewSyncer(manager *Manager) (*Syncer, error) {
	if manager == nil {
		return nil, fmt.Errorf("repo syncer manager is required")
	}
	return &Syncer{manager: manager}, nil
}

// Push serializes tree and uploads it to the microVM working root.
func (s *Syncer) Push(ctx context.Context, instance Instance, tree string) error {
	data, err := vmagent.TarTree(tree)
	if err != nil {
		return fmt.Errorf("tar workspace tree: %w", err)
	}
	client, err := s.manager.Shell(ctx, instance)
	if err != nil {
		return err
	}
	return client.Bootstrap(ctx, bytes.NewReader(data))
}

// Pull downloads the microVM working tree and extracts it into scratch. The
// extracted content excludes .git and symlinks by the vmagent tar contract.
func (s *Syncer) Pull(ctx context.Context, instance Instance, scratch string) error {
	if err := os.MkdirAll(scratch, 0o755); err != nil {
		return err
	}
	client, err := s.manager.Shell(ctx, instance)
	if err != nil {
		return err
	}
	reader, err := client.FetchTar(ctx)
	if err != nil {
		return err
	}
	defer reader.Close()
	return vmagent.ExtractTree(reader, scratch)
}
