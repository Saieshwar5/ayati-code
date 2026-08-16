package webapp

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/workspace"
)

func (s *Server) workspaceRead(writer http.ResponseWriter, request *http.Request) {
	parts := workspaceParts(request)
	if len(parts) < 2 {
		http.NotFound(writer, request)
		return
	}
	switch {
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
		if s.chat == nil {
			s.writeError(writer, http.StatusServiceUnavailable, "Fireworks is not configured; run ayati config")
			return
		}
		messages, err := s.chat.Messages(request.Context(), parts[0], parts[2])
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "load conversation")
			return
		}
		if messages == nil {
			messages = []agent.Message{}
		}
		s.writeJSON(writer, http.StatusOK, messages)
	default:
		http.NotFound(writer, request)
	}
}

func (s *Server) workspaceSessionMutation(writer http.ResponseWriter, request *http.Request) {
	parts := workspaceParts(request)
	if request.Method == http.MethodDelete && len(parts) == 1 {
		if s.chat != nil {
			s.chat.CancelAndWait(parts[0])
		}
		if err := s.workspaces.Delete(request.Context(), parts[0]); err != nil {
			s.writeError(writer, http.StatusConflict, err.Error())
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
			Title string `json:"title"`
		}
		if !s.decode(writer, request, &input) {
			return
		}
		value, err := s.store.RenameSession(request.Context(), parts[0], parts[2], input.Title)
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

func workspaceParts(request *http.Request) []string {
	value := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/workspaces/"), "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}
