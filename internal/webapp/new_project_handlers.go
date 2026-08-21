package webapp

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/githubapp"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func (s *Server) createNewProjectWorkspace(writer http.ResponseWriter, request *http.Request) {
	credentials, ok := s.requireCredentials(writer)
	if !ok {
		return
	}
	var input struct {
		Name        string                       `json:"name"`
		Description string                       `json:"description"`
		Private     *bool                        `json:"private"`
		Branch      string                       `json:"branch"`
		Setup       string                       `json:"setup_command"`
		Environment []workspace.EnvironmentInput `json:"environment"`
	}
	if !s.decode(writer, request, &input) {
		return
	}
	input.Branch = strings.TrimSpace(input.Branch)
	if input.Branch == "" {
		s.writeError(writer, http.StatusBadRequest, "working branch is required")
		return
	}
	if err := workspace.ValidateEnvironment(input.Environment); err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	private := true
	if input.Private != nil {
		private = *input.Private
	}
	repository, err := s.github.CreateRepository(request.Context(), credentials.AccessToken,
		githubapp.CreateRepositoryInput{Name: input.Name, Description: input.Description, Private: private})
	if err != nil {
		s.writeRepositoryCreationError(writer, err)
		return
	}
	baseBranch := strings.TrimSpace(repository.DefaultBranch)
	branch := input.Branch
	if branch == baseBranch {
		s.writeError(writer, http.StatusBadRequest,
			"GitHub repository was created as "+repository.FullName+", but the working branch must differ from "+baseBranch)
		return
	}
	value, err := s.createManagedWorkspace(request.Context(), workspace.Create{
		Repository: repository.FullName, CloneURL: repository.CloneURL,
		BaseBranch: baseBranch, Branch: branch, CreateBranch: true,
		Setup: input.Setup, Root: s.workspaceRoot, Environment: input.Environment,
	})
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError,
			"GitHub repository "+repository.FullName+" was created, but its workspace could not be recorded")
		return
	}
	s.writeJSON(writer, http.StatusAccepted, value)
}

func (s *Server) createManagedWorkspace(ctx context.Context, input workspace.Create) (workspace.Workspace, error) {
	value, err := s.store.Create(ctx, input)
	if err != nil {
		return workspace.Workspace{}, err
	}
	if err := s.workspaces.StartPreparation(ctx, value.ID); err != nil {
		return workspace.Workspace{}, fmt.Errorf("enqueue workspace preparation: %w", err)
	}
	return value, nil
}

func (s *Server) writeRepositoryCreationError(writer http.ResponseWriter, err error) {
	var apiError githubapp.APIError
	switch {
	case errors.As(err, &apiError) && apiError.StatusCode == http.StatusForbidden:
		s.writeError(writer, http.StatusForbidden,
			"GitHub App needs Administration: write permission to create repositories")
	case errors.As(err, &apiError) && apiError.StatusCode == http.StatusUnprocessableEntity:
		s.writeError(writer, http.StatusConflict, "repository name is unavailable or invalid")
	default:
		s.writeError(writer, http.StatusBadGateway, "create GitHub repository")
	}
}
