package webapp

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/githubapp"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
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
		Authority   string                       `json:"authority"`
		Branch      string                       `json:"branch"`
		Setup       string                       `json:"setup_command"`
		Environment []workspace.EnvironmentInput `json:"environment"`
	}
	if !s.decode(writer, request, &input) {
		return
	}
	authority, err := workspace.ParseAuthority(input.Authority)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	input.Branch = strings.TrimSpace(input.Branch)
	if authority == workspace.AuthorityDevelop && input.Branch == "" {
		s.writeError(writer, http.StatusBadRequest, "Develop authority requires a working branch")
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
	createBranch := authority == workspace.AuthorityDevelop
	branch := baseBranch
	if createBranch {
		branch = input.Branch
		if branch == baseBranch {
			s.writeError(writer, http.StatusBadRequest,
				"GitHub repository was created as "+repository.FullName+", but the working branch must differ from "+baseBranch)
			return
		}
	}
	value, err := s.createManagedWorkspace(request.Context(), workspace.Create{
		Repository: repository.FullName, CloneURL: repository.CloneURL,
		BaseBranch: baseBranch, Branch: branch, CreateBranch: createBranch,
		Authority: authority, Setup: input.Setup, Root: s.workspaceRoot, Environment: input.Environment,
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
	go func() {
		if err := s.workspaces.Initialize(s.ctx, value.ID); err != nil {
			s.logger.Printf("initialize workspace %s: %v", value.ID, err)
		}
	}()
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
