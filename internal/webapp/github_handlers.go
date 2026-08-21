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
	account, found, err := s.accountFromRequest(request)
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "load login session")
		return
	}
	if found {
		response.Authenticated = true
		response.User = &githubapp.User{ID: account.GitHubID, Login: account.Login, AvatarURL: account.AvatarURL}
	}
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
	before, _ := s.accounts.UserCount(request.Context())
	account, err := s.accounts.UpsertGitHubUser(request.Context(), user.ID, user.Login, "", user.AvatarURL)
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "save GitHub account")
		return
	}
	if err := s.accounts.SaveGitHubCredential(request.Context(), account.ID, token); err != nil {
		s.writeError(writer, http.StatusInternalServerError, "save GitHub credential")
		return
	}
	if before == 0 {
		if _, claimErr := s.store.ClaimLegacyRows(request.Context(), account.ID); claimErr != nil {
			s.logger.Printf("claim legacy rows: %v", claimErr)
		}
	}
	sessionToken, err := randomToken()
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "create login token")
		return
	}
	session, err := s.accounts.CreateSession(request.Context(), account.ID, sessionToken, accountSessionTTL)
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "create login session")
		return
	}
	setAccountSessionCookie(writer, request, sessionToken, session.ExpiresAt)
	http.Redirect(writer, request, "/", http.StatusFound)
}

func (s *Server) logout(writer http.ResponseWriter, request *http.Request) {
	if cookie, err := request.Cookie(accountSessionCookie); err == nil && cookie.Value != "" {
		if err := s.accounts.RevokeSession(request.Context(), cookie.Value); err != nil {
			s.writeError(writer, http.StatusInternalServerError, "revoke login session")
			return
		}
	}
	clearAccountSessionCookie(writer, request)
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) repositories(writer http.ResponseWriter, request *http.Request) {
	credentials, ok := s.requireCredentials(writer, request)
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
	credentials, ok := s.requireCredentials(writer, request)
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
