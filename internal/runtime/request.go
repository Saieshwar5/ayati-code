package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func validateRequest(request Request) error {
	if request.Version != 0 && request.Version != ProtocolVersion {
		return fmt.Errorf("unsupported request version %d", request.Version)
	}
	if strings.TrimSpace(request.RunID) == "" {
		return fmt.Errorf("run_id is required")
	}
	if strings.TrimSpace(request.Prompt) == "" {
		return fmt.Errorf("prompt is required")
	}
	if !filepath.IsAbs(request.Workspace) {
		return fmt.Errorf("workspace must be an absolute path")
	}
	info, err := os.Stat(request.Workspace)
	if err != nil {
		return fmt.Errorf("inspect workspace: %w", err)
	}
	if !info.IsDir() {
		return fmt.Errorf("workspace is not a directory")
	}
	return nil
}
