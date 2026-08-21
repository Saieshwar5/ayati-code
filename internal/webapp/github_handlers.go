package webapp

import (
	"errors"
	"net/http"
	"net/url"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/githubapp"
)

func (s *Server) session(writer http.ResponseWriter, request *http.Request) {
	response := struct {
		GitHubConfigured bool            `json:"github_configured"`
		Authenticated    bool            `json:"authenticated"`
		User             *githubapp.User `json:"user,omitempty"`
	}{GitHubConfigured: s.github != nil}
	if s.github == nil {
		s.writeJSON(writer, http.StatusOK, response)
		return
	}
	credentials, err := s.credentials()
	if err != nil {
		s.writeJSON(writer, http.StatusOK, response)
		return
	}
	user, err := s.github.CurrentUser(request.Context(), credentials.AccessToken)
	if err != nil {
		if githubAuthorizationExpired(err) {
			s.writeJSON(writer, http.StatusOK, response)
			return
		}
		s.writeError(writer, http.StatusBadGateway, "validate GitHub authorization")
		return
	}
	response.Authenticated = true
	response.User = &user
	s.writeJSON(writer, http.StatusOK, response)
}

func (s *Server) githubLogin(writer http.ResponseWriter, request *http.Request) {
	if s.github == nil {
		s.writeError(writer, http.StatusServiceUnavailable, "GitHub App is not configured")
		return
	}
	loginURL := s.github.LoginURL()
	if canonical, err := url.Parse(loginURL); err == nil &&
		!strings.EqualFold(request.Host, canonical.Host) {
		http.Redirect(writer, request, loginURL, http.StatusFound)
		return
	}
	state, err := randomToken()
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "create authorization state")
		return
	}
	setStateCookie(writer, request, state)
	http.Redirect(writer, request, s.github.AuthorizeURL(state), http.StatusFound)
}

func (s *Server) githubCallback(writer http.ResponseWriter, request *http.Request) {
	if s.github == nil {
		s.writeError(writer, http.StatusServiceUnavailable, "GitHub App is not configured")
		return
	}
	cookie, err := request.Cookie("perpetual_github_state")
	if err != nil || cookie.Value == "" || cookie.Value != request.URL.Query().Get("state") {
		s.writeError(writer, http.StatusBadRequest, "invalid GitHub authorization state")
		return
	}
	clearStateCookie(writer, request)
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
		s.writeGitHubReadError(writer, "load GitHub repositories", err)
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
		s.writeGitHubReadError(writer, "load GitHub branches", err)
		return
	}
	s.writeJSON(writer, http.StatusOK, branches)
}

func (s *Server) writeGitHubReadError(writer http.ResponseWriter, fallback string, err error) {
	if githubAuthorizationExpired(err) {
		s.writeError(writer, http.StatusUnauthorized, "GitHub authorization expired; reconnect GitHub")
		return
	}
	s.writeError(writer, http.StatusBadGateway, fallback)
}

func githubAuthorizationExpired(err error) bool {
	var apiError githubapp.APIError
	return errors.As(err, &apiError) && apiError.StatusCode == http.StatusUnauthorized
}
