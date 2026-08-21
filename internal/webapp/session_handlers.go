package webapp

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func (s *Server) workspaceRead(writer http.ResponseWriter, request *http.Request) {
	parts := workspaceParts(request)
	if len(parts) < 2 {
		http.NotFound(writer, request)
		return
	}
	if !s.requireOwnedWorkspace(writer, request, parts[0]) {
		return
	}
	switch {
	case len(parts) == 2 && parts[1] == "environment":
		values, err := s.store.ListEnvironment(request.Context(), parts[0])
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "list workspace environment")
			return
		}
		if values == nil {
			values = []workspace.EnvironmentVariable{}
		}
		s.writeJSON(writer, http.StatusOK, values)
	case len(parts) == 2 && parts[1] == "changes":
		changes, err := s.workspaces.Changes(request.Context(), parts[0])
		if err != nil {
			s.writeError(writer, http.StatusConflict, err.Error())
			return
		}
		s.writeJSON(writer, http.StatusOK, changes)
	case len(parts) == 2 && parts[1] == "sessions":
		values, err := s.store.ListSessions(request.Context(), parts[0])
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "list workspace sessions")
			return
		}
		if values == nil {
			values = []workspace.Session{}
		}
		s.writeJSON(writer, http.StatusOK, values)
	case len(parts) == 3 && parts[1] == "sessions":
		value, err := s.store.GetSession(request.Context(), parts[0], parts[2])
		if errors.Is(err, sql.ErrNoRows) {
			s.writeError(writer, http.StatusNotFound, "session not found")
			return
		}
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "load session")
			return
		}
		s.writeJSON(writer, http.StatusOK, value)
	case len(parts) == 4 && parts[1] == "sessions" && parts[3] == "messages":
		messages, err := s.store.ConversationMessages(request.Context(), parts[2])
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "load conversation")
			return
		}
		if messages == nil {
			messages = []workspace.ConversationMessage{}
		}
		s.writeJSON(writer, http.StatusOK, messages)
	default:
		http.NotFound(writer, request)
	}
}

func (s *Server) workspaceSessionMutation(writer http.ResponseWriter, request *http.Request) {
	parts := workspaceParts(request)
	if len(parts) == 0 || !s.requireOwnedWorkspace(writer, request, parts[0]) {
		if len(parts) == 0 {
			http.NotFound(writer, request)
		}
		return
	}
	if request.Method == http.MethodDelete && len(parts) == 1 {
		if err := s.workspaces.Delete(request.Context(), parts[0]); err != nil {
			s.writeError(writer, http.StatusConflict, err.Error())
			return
		}
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if request.Method == http.MethodDelete && len(parts) == 3 && parts[1] == "environment" {
		if !s.requireMutableEnvironment(writer, request, parts[0]) {
			return
		}
		if err := s.store.DeleteEnvironment(request.Context(), parts[0], parts[2]); err != nil {
			s.writeError(writer, http.StatusNotFound, "environment variable not found")
			return
		}
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	if len(parts) != 3 || parts[1] != "sessions" {
		http.NotFound(writer, request)
		return
	}
	switch request.Method {
	case http.MethodPatch:
		var input struct {
			Title *string `json:"title"`
		}
		if !s.decode(writer, request, &input) {
			return
		}
		if input.Title == nil {
			s.writeError(writer, http.StatusBadRequest, "session title is required")
			return
		}
		value, err := s.store.RenameSession(request.Context(), parts[0], parts[2], *input.Title)
		if err != nil {
			s.writeError(writer, http.StatusBadRequest, err.Error())
			return
		}
		s.writeJSON(writer, http.StatusOK, value)
	case http.MethodDelete:
		if err := s.store.DeleteSession(request.Context(), parts[0], parts[2]); err != nil {
			s.writeError(writer, http.StatusConflict, err.Error())
			return
		}
		writer.WriteHeader(http.StatusNoContent)
	default:
		http.NotFound(writer, request)
	}
}

func (s *Server) requireMutableEnvironment(writer http.ResponseWriter, request *http.Request, id string) bool {
	account, _ := currentAccount(request.Context())
	value, err := s.store.GetForUser(request.Context(), account.ID, id)
	if errors.Is(err, sql.ErrNoRows) {
		s.writeError(writer, http.StatusNotFound, "workspace not found")
		return false
	}
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "load workspace")
		return false
	}
	if value.Status == workspace.StatusCreating || value.Status == workspace.StatusInitializing {
		s.writeError(writer, http.StatusConflict, "environment cannot change during workspace initialization")
		return false
	}
	working, err := s.store.HasWorkingSession(request.Context(), id)
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "inspect workspace sessions")
		return false
	}
	if working {
		s.writeError(writer, http.StatusConflict, "environment cannot change while an agent is working")
		return false
	}
	return true
}

func workspaceParts(request *http.Request) []string {
	value := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/workspaces/"), "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}
