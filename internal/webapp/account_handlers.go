package webapp

import (
	"context"
	"database/sql"
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Saieshwar5/perpetual/internal/accounts"
)

const (
	accountSessionCookie = "perpetual_session"
	accountSessionTTL    = 30 * 24 * time.Hour
)

type accountContextKey struct{}

func (s *Server) requireUser(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		user, found, err := s.accountFromRequest(request)
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "load login session")
			return
		}
		if !found {
			s.writeError(writer, http.StatusUnauthorized, "authentication required")
			return
		}
		ctx := context.WithValue(request.Context(), accountContextKey{}, user)
		next(writer, request.WithContext(ctx))
	}
}

func (s *Server) me(writer http.ResponseWriter, request *http.Request) {
	user, _ := currentAccount(request.Context())
	s.writeJSON(writer, http.StatusOK, user)
}

func currentAccount(ctx context.Context) (accounts.User, bool) {
	value, ok := ctx.Value(accountContextKey{}).(accounts.User)
	return value, ok
}

func currentAccountID(ctx context.Context) (string, bool) {
	user, ok := currentAccount(ctx)
	return user.ID, ok
}

func (s *Server) accountFromRequest(request *http.Request) (accounts.User, bool, error) {
	cookie, err := request.Cookie(accountSessionCookie)
	if err != nil || strings.TrimSpace(cookie.Value) == "" {
		return accounts.User{}, false, nil
	}
	user, found, err := s.accounts.UserBySessionToken(request.Context(), cookie.Value)
	if err != nil {
		return accounts.User{}, false, err
	}
	return user, found, nil
}

func setAccountSessionCookie(writer http.ResponseWriter, request *http.Request, token string, expires time.Time) {
	if strings.TrimSpace(token) == "" {
		return
	}
	http.SetCookie(writer, &http.Cookie{
		Name: accountSessionCookie, Value: token, Path: "/", Expires: expires.UTC(),
		MaxAge: int(time.Until(expires).Seconds()), HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: request.TLS != nil,
	})
}

func clearAccountSessionCookie(writer http.ResponseWriter, request *http.Request) {
	http.SetCookie(writer, &http.Cookie{
		Name: accountSessionCookie, Path: "/", MaxAge: -1, HttpOnly: true,
		SameSite: http.SameSiteLaxMode, Secure: request.TLS != nil,
	})
}

func (s *Server) requireOwnedWorkspace(writer http.ResponseWriter, request *http.Request, id string) bool {
	userID, ok := currentAccountID(request.Context())
	if !ok {
		s.writeError(writer, http.StatusUnauthorized, "authentication required")
		return false
	}
	_, err := s.store.GetForUser(request.Context(), userID, id)
	if errors.Is(err, sql.ErrNoRows) {
		s.writeError(writer, http.StatusNotFound, "workspace not found")
		return false
	}
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "load workspace")
		return false
	}
	return true
}
