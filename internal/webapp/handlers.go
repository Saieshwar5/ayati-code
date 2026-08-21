package webapp

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/githubapp"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func (s *Server) listWorkspaces(writer http.ResponseWriter, request *http.Request) {
	account, _ := currentAccount(request.Context())
	var values []workspace.Workspace
	var err error
	if request.URL.Query().Get("archived") == "true" {
		values, err = s.store.ListArchivedForUser(request.Context(), account.ID)
	} else {
		values, err = s.store.ListForUser(request.Context(), account.ID)
	}
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "list workspaces")
		return
	}
	if values == nil {
		values = []workspace.Workspace{}
	}
	s.writeJSON(writer, http.StatusOK, values)
}

func (s *Server) createWorkspace(writer http.ResponseWriter, request *http.Request) {
	credentials, ok := s.requireCredentials(writer, request)
	if !ok {
		return
	}
	var input struct {
		Repository   string                       `json:"repository"`
		BaseBranch   string                       `json:"base_branch"`
		Branch       string                       `json:"branch"`
		CreateBranch bool                         `json:"create_branch"`
		BranchMode   string                       `json:"branch_mode"`
		Setup        string                       `json:"setup_command"`
		Environment  []workspace.EnvironmentInput `json:"environment"`
	}
	if !s.decode(writer, request, &input) {
		return
	}
	repository, err := s.authorizedRepository(request, credentials, input.Repository)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	selection, err := s.resolveBranchSelection(request, credentials.AccessToken, repository,
		input.BranchMode, input.BaseBranch, input.Branch)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	input.BaseBranch, input.Branch = selection.base, selection.working
	input.CreateBranch = selection.create
	if err := workspace.ValidateEnvironment(input.Environment); err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	account, _ := currentAccount(request.Context())
	value, err := s.createManagedWorkspace(request.Context(), workspace.Create{
		UserID:     account.ID,
		Repository: repository.FullName, CloneURL: repository.CloneURL,
		BaseBranch: input.BaseBranch, Branch: input.Branch, CreateBranch: input.CreateBranch,
		Setup: input.Setup, Root: s.workspaceRoot, Environment: input.Environment,
	})
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, "create workspace")
		return
	}
	s.writeJSON(writer, http.StatusAccepted, value)
}

func (s *Server) workspaceAction(writer http.ResponseWriter, request *http.Request) {
	parts := workspaceParts(request)
	if len(parts) < 2 {
		http.NotFound(writer, request)
		return
	}
	if !s.requireOwnedWorkspace(writer, request, parts[0]) {
		return
	}
	if len(parts) == 2 && parts[1] == "sessions" {
		var input struct {
			Title string `json:"title"`
		}
		if !s.decode(writer, request, &input) {
			return
		}
		value, err := s.store.CreateSession(request.Context(), parts[0], input.Title)
		if err != nil {
			s.writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(writer, http.StatusCreated, value)
		return
	}
	if len(parts) == 2 && parts[1] == "environment" {
		if !s.requireMutableEnvironment(writer, request, parts[0]) {
			return
		}
		var input workspace.EnvironmentInput
		if !s.decode(writer, request, &input) {
			return
		}
		value, err := s.store.UpsertEnvironment(request.Context(), parts[0], input)
		if err != nil {
			s.writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(writer, http.StatusOK, value)
		return
	}
	if len(parts) == 3 && parts[1] == "environment" && parts[2] == "rebuild" {
		if err := s.workspaces.RebuildEnvironment(request.Context(), parts[0]); err != nil {
			s.writeError(writer, http.StatusConflict, err.Error())
			return
		}
		writer.WriteHeader(http.StatusAccepted)
		return
	}
	if len(parts) != 2 {
		http.NotFound(writer, request)
		return
	}
	var err error
	switch parts[1] {
	case "initialize":
		err = s.workspaces.StartPreparation(request.Context(), parts[0])
	case "configure":
		var input struct {
			ProjectRoot string `json:"project_root"`
		}
		if !s.decode(writer, request, &input) {
			return
		}
		if err = s.workspaces.ConfigureProjectRoot(request.Context(), parts[0], input.ProjectRoot); err == nil {
			err = s.workspaces.StartPreparation(request.Context(), parts[0])
		}
	case "stop":
		err = s.workspaces.Stop(request.Context(), parts[0])
	case "resume":
		err = s.workspaces.Resume(request.Context(), parts[0])
	case "archive":
		err = s.workspaces.Archive(request.Context(), parts[0])
	case "restore":
		err = s.workspaces.RestoreArchived(request.Context(), parts[0])
	case "publish":
		credentials, ok := s.requireCredentials(writer, request)
		if !ok {
			return
		}
		var input struct {
			CommitMessage string `json:"commit_message"`
			Title         string `json:"title"`
			Body          string `json:"body"`
		}
		if !s.decode(writer, request, &input) {
			return
		}
		account, _ := currentAccount(request.Context())
		value, getErr := s.store.GetForUser(request.Context(), account.ID, parts[0])
		if getErr != nil {
			s.writeError(writer, http.StatusNotFound, "workspace not found")
			return
		}
		if strings.TrimSpace(input.CommitMessage) == "" {
			s.writeError(writer, http.StatusBadRequest, "commit message is required")
			return
		}
		if value.Branch == value.BaseBranch {
			s.writeError(writer, http.StatusConflict,
				"publishing requires a working branch different from the pull request base")
			return
		}
		if value.PullRequestNumber == 0 && strings.TrimSpace(input.Title) == "" {
			s.writeError(writer, http.StatusBadRequest, "pull request title is required")
			return
		}
		email := fmt.Sprintf("%d+%s@users.noreply.github.com", credentials.User.ID, credentials.User.Login)
		if publishErr := s.workspaces.Publish(request.Context(), value.ID,
			input.CommitMessage, credentials.User.Login, email); publishErr != nil {
			s.writeError(writer, http.StatusConflict, publishErr.Error())
			return
		}
		if value.PullRequestNumber == 0 {
			pull, pullErr := s.github.CreatePullRequest(request.Context(), credentials.AccessToken,
				value.Repository, value.BaseBranch, value.Branch, input.Title, input.Body)
			if pullErr != nil {
				s.writeError(writer, http.StatusBadGateway, "create pull request")
				return
			}
			if updateErr := s.store.UpdatePullRequest(request.Context(), value.ID, pull.Number, pull.HTMLURL); updateErr != nil {
				s.writeError(writer, http.StatusInternalServerError, "record pull request")
				return
			}
		}
		updated, _ := s.store.Get(request.Context(), value.ID)
		s.writeJSON(writer, http.StatusOK, updated)
		return
	default:
		http.NotFound(writer, request)
		return
	}
	if err != nil {
		s.writeError(writer, http.StatusConflict, err.Error())
		return
	}
	writer.WriteHeader(http.StatusAccepted)
}

func (s *Server) authorizedRepository(
	request *http.Request, credentials githubapp.Credentials, fullName string,
) (githubapp.Repository, error) {
	values, err := s.github.Repositories(request.Context(), credentials.AccessToken)
	if err != nil {
		return githubapp.Repository{}, errors.New("load authorized GitHub repositories")
	}
	for _, repository := range values {
		if repository.FullName == strings.TrimSpace(fullName) {
			return repository, nil
		}
	}
	return githubapp.Repository{}, errors.New("repository is not authorized")
}

func (s *Server) requireCredentials(writer http.ResponseWriter, request *http.Request) (githubapp.Credentials, bool) {
	if s.github == nil {
		s.writeError(writer, http.StatusServiceUnavailable, "GitHub App is not configured")
		return githubapp.Credentials{}, false
	}
	account, ok := currentAccount(request.Context())
	if !ok {
		s.writeError(writer, http.StatusUnauthorized, "GitHub authentication is required")
		return githubapp.Credentials{}, false
	}
	token, err := s.accounts.GitHubCredential(request.Context(), account.ID)
	if err != nil {
		s.writeError(writer, http.StatusUnauthorized, "GitHub authentication is required")
		return githubapp.Credentials{}, false
	}
	return githubapp.Credentials{AccessToken: token,
		User: githubapp.User{ID: account.GitHubID, Login: account.Login, AvatarURL: account.AvatarURL}}, true
}
