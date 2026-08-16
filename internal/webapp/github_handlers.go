package webapp

import (
	"net/http"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/githubapp"
)

func (s *Server) session(writer http.ResponseWriter, _ *http.Request) {
	response := struct {
		GitHubConfigured bool            `json:"github_configured"`
		Authenticated    bool            `json:"authenticated"`
		User             *githubapp.User `json:"user,omitempty"`
	}{GitHubConfigured: s.github != nil}
	if credentials, err := s.credentials(); err == nil {
		response.Authenticated = true
		response.User = &credentials.User
	}
	s.writeJSON(writer, http.StatusOK, response)
}

func (s *Server) githubLogin(writer http.ResponseWriter, request *http.Request) {
	if s.github == nil {
		s.writeError(writer, http.StatusServiceUnavailable, "GitHub App is not configured")
		return
	}
	state, err := randomToken()
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "create authorization state")
		return
	}
	setStateCookie(writer, state)
	http.Redirect(writer, request, s.github.AuthorizeURL(state), http.StatusFound)
}

func (s *Server) githubCallback(writer http.ResponseWriter, request *http.Request) {
	if s.github == nil {
		s.writeError(writer, http.StatusServiceUnavailable, "GitHub App is not configured")
		return
	}
	cookie, err := request.Cookie("ayati_github_state")
	if err != nil || cookie.Value == "" || cookie.Value != request.URL.Query().Get("state") {
		s.writeError(writer, http.StatusBadRequest, "invalid GitHub authorization state")
		return
	}
	clearStateCookie(writer)
	token, err := s.github.Exchange(request.Context(), request.URL.Query().Get("code"))
	if err != nil {
		s.writeError(writer, http.StatusBadGateway, "GitHub authorization failed")
		return
	}
	user, err := s.github.CurrentUser(request.Context(), token)
	if err != nil {
		s.writeError(writer, http.StatusBadGateway, "load GitHub user")
		return
	}
	credentials := githubapp.Credentials{AccessToken: token, User: user}
	if err := githubapp.SaveCredentials(s.credentialsPath, credentials); err != nil {
		s.writeError(writer, http.StatusInternalServerError, "save GitHub authorization")
		return
	}
	http.Redirect(writer, request, "/", http.StatusFound)
}

func (s *Server) logout(writer http.ResponseWriter, _ *http.Request) {
	if err := githubapp.RemoveCredentials(s.credentialsPath); err != nil {
		s.writeError(writer, http.StatusInternalServerError, "remove GitHub authorization")
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) repositories(writer http.ResponseWriter, request *http.Request) {
	credentials, ok := s.requireCredentials(writer)
	if !ok {
		return
	}
	repositories, err := s.github.Repositories(request.Context(), credentials.AccessToken)
	if err != nil {
		s.writeError(writer, http.StatusBadGateway, "load GitHub repositories")
		return
	}
	s.writeJSON(writer, http.StatusOK, repositories)
}

func (s *Server) branches(writer http.ResponseWriter, request *http.Request) {
	credentials, ok := s.requireCredentials(writer)
	if !ok {
		return
	}
	path := strings.TrimPrefix(request.URL.Path, "/api/repositories/")
	parts := strings.Split(path, "/")
	if len(parts) != 3 || parts[2] != "branches" {
		http.NotFound(writer, request)
		return
	}
	repository := parts[0] + "/" + parts[1]
	branches, err := s.github.Branches(request.Context(), credentials.AccessToken, repository)
	if err != nil {
		s.writeError(writer, http.StatusBadGateway, "load GitHub branches")
		return
	}
	s.writeJSON(writer, http.StatusOK, branches)
}
