package webapp

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/githubapp"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

func (s *Server) listWorkspaces(writer http.ResponseWriter, request *http.Request) {
	values, err := s.store.List(request.Context())
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
	credentials, ok := s.requireCredentials(writer)
	if !ok {
		return
	}
	var input struct {
		Repository   string                       `json:"repository"`
		BaseBranch   string                       `json:"base_branch"`
		Branch       string                       `json:"branch"`
		CreateBranch bool                         `json:"create_branch"`
		Authority    string                       `json:"authority"`
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
	input.BaseBranch = strings.TrimSpace(input.BaseBranch)
	input.Branch = strings.TrimSpace(input.Branch)
	authority, err := workspace.ParseAuthority(input.Authority)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	if input.BaseBranch == "" {
		s.writeError(writer, http.StatusBadRequest, "starting branch is required")
		return
	}
	if authority == workspace.AuthorityExplore {
		input.Branch = input.BaseBranch
		input.CreateBranch = false
	} else if input.Branch == "" || input.BaseBranch == input.Branch {
		s.writeError(writer, http.StatusBadRequest,
			"Develop authority requires a working branch different from the starting branch")
		return
	}
	if err := workspace.ValidateEnvironment(input.Environment); err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	value, err := s.store.Create(request.Context(), workspace.Create{
		Repository: repository.FullName, CloneURL: repository.CloneURL,
		BaseBranch: input.BaseBranch, Branch: input.Branch, CreateBranch: input.CreateBranch,
		Authority: authority, Setup: input.Setup, Root: s.workspaceRoot, Environment: input.Environment,
	})
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, "create workspace")
		return
	}
	go func() {
		if err := s.workspaces.Initialize(s.ctx, value.ID); err != nil {
			s.logger.Printf("initialize workspace %s: %v", value.ID, err)
		}
	}()
	s.writeJSON(writer, http.StatusAccepted, value)
}

func (s *Server) workspaceAction(writer http.ResponseWriter, request *http.Request) {
	parts := workspaceParts(request)
	if len(parts) < 2 {
		http.NotFound(writer, request)
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
	if len(parts) == 4 && parts[1] == "sessions" && parts[3] == "messages" {
		if s.chat == nil {
			s.writeError(writer, http.StatusServiceUnavailable, "Fireworks is not configured; run ayati config")
			return
		}
		var input struct {
			Text string `json:"text"`
		}
		if !s.decode(writer, request, &input) {
			return
		}
		completion, err := s.chat.Send(request.Context(), parts[0], parts[2], input.Text)
		if err != nil {
			s.writeError(writer, http.StatusConflict, err.Error())
			return
		}
		s.writeJSON(writer, http.StatusOK, completion)
		return
	}
	if len(parts) != 2 {
		http.NotFound(writer, request)
		return
	}
	var err error
	switch parts[1] {
	case "initialize":
		go func() {
			if runErr := s.workspaces.Initialize(s.ctx, parts[0]); runErr != nil {
				s.logger.Printf("initialize workspace %s: %v", parts[0], runErr)
			}
		}()
	case "configure":
		var input struct {
			ProjectRoot string `json:"project_root"`
		}
		if !s.decode(writer, request, &input) {
			return
		}
		if err = s.workspaces.ConfigureProjectRoot(request.Context(), parts[0], input.ProjectRoot); err == nil {
			go func() {
				if runErr := s.workspaces.Initialize(s.ctx, parts[0]); runErr != nil {
					s.logger.Printf("initialize configured workspace %s: %v", parts[0], runErr)
				}
			}()
		}
	case "authority":
		var input workspace.AuthorityChange
		if !s.decode(writer, request, &input) {
			return
		}
		var updated workspace.Workspace
		change := func() error {
			var changeErr error
			updated, changeErr = s.workspaces.ChangeAuthority(request.Context(), parts[0], input)
			return changeErr
		}
		if s.chat != nil {
			err = s.chat.WithWorkspaceIdle(parts[0], change)
		} else {
			err = change()
		}
		if err == nil {
			s.writeJSON(writer, http.StatusOK, updated)
			return
		}
	case "stop":
		if s.chat != nil {
			s.chat.CancelAndWait(parts[0])
		}
		err = s.workspaces.Stop(request.Context(), parts[0])
	case "publish":
		credentials, ok := s.requireCredentials(writer)
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
		value, getErr := s.store.Get(request.Context(), parts[0])
		if getErr != nil {
			s.writeError(writer, http.StatusNotFound, "workspace not found")
			return
		}
		if strings.TrimSpace(input.CommitMessage) == "" {
			s.writeError(writer, http.StatusBadRequest, "commit message is required")
			return
		}
		if value.Authority != workspace.AuthorityDevelop {
			s.writeError(writer, http.StatusConflict, "publishing requires Develop authority")
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

func (s *Server) requireCredentials(writer http.ResponseWriter) (githubapp.Credentials, bool) {
	if s.github == nil {
		s.writeError(writer, http.StatusServiceUnavailable, "GitHub App is not configured")
		return githubapp.Credentials{}, false
	}
	credentials, err := s.credentials()
	if err != nil {
		s.writeError(writer, http.StatusUnauthorized, "GitHub authentication is required")
		return githubapp.Credentials{}, false
	}
	return credentials, true
}
