package webapp

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/githubapp"
)

const (
	branchModeNew      = "new"
	branchModeExisting = "existing"
	branchModeDirect   = "direct"
)

type branchSelection struct {
	base, working string
	create        bool
}

func (s *Server) resolveBranchSelection(
	request *http.Request,
	token string,
	repository githubapp.Repository,
	mode, base, working string,
) (branchSelection, error) {
	base, working = strings.TrimSpace(base), strings.TrimSpace(working)
	branches, err := s.github.Branches(request.Context(), token, repository.FullName)
	if err != nil {
		return branchSelection{}, errors.New("load authorized repository branches")
	}
	exists := make(map[string]bool, len(branches))
	for _, branch := range branches {
		exists[branch.Name] = true
	}
	switch strings.TrimSpace(mode) {
	case branchModeNew:
		if !exists[base] {
			return branchSelection{}, fmt.Errorf("base branch %q is not available", base)
		}
		if working == "" || working == base {
			return branchSelection{}, errors.New("new working branch must differ from its base")
		}
		if exists[working] {
			return branchSelection{}, fmt.Errorf("branch %q already exists; continue the existing branch instead", working)
		}
		return branchSelection{base: base, working: working, create: true}, nil
	case branchModeExisting:
		if !exists[base] || !exists[working] {
			return branchSelection{}, errors.New("working branch and pull request base must exist")
		}
		if working == base {
			return branchSelection{}, errors.New("select direct branch work when both branches are the same")
		}
		return branchSelection{base: base, working: working}, nil
	case branchModeDirect:
		if !exists[working] {
			return branchSelection{}, fmt.Errorf("working branch %q is not available", working)
		}
		return branchSelection{base: working, working: working}, nil
	default:
		return branchSelection{}, errors.New("select how Perpetual should use branches")
	}
}
