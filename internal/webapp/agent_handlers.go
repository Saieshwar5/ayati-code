package webapp

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

func (s *Server) agentsRead(writer http.ResponseWriter, request *http.Request) {
	parts := agentParts(request)
	if len(parts) == 0 {
		values, err := s.store.ListAgents(request.Context(), request.URL.Query().Get("archived") == "true")
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "list agents")
			return
		}
		if values == nil {
			values = []agent.Definition{}
		}
		s.writeJSON(writer, http.StatusOK, values)
		return
	}
	if len(parts) != 1 {
		http.NotFound(writer, request)
		return
	}
	value, err := s.store.GetAgent(request.Context(), parts[0])
	if errors.Is(err, sql.ErrNoRows) {
		s.writeError(writer, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "load agent")
		return
	}
	s.writeJSON(writer, http.StatusOK, value)
}

func (s *Server) createAgent(writer http.ResponseWriter, request *http.Request) {
	var input agent.DefinitionInput
	if !s.decode(writer, request, &input) {
		return
	}
	value, err := s.store.CreateAgent(request.Context(), input)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(writer, http.StatusCreated, value)
}

func (s *Server) updateAgent(writer http.ResponseWriter, request *http.Request) {
	parts := agentParts(request)
	if len(parts) != 1 {
		http.NotFound(writer, request)
		return
	}
	var input agent.DefinitionInput
	if !s.decode(writer, request, &input) {
		return
	}
	value, err := s.store.UpdateAgent(request.Context(), parts[0], input)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(writer, http.StatusOK, value)
}

func (s *Server) agentAction(writer http.ResponseWriter, request *http.Request) {
	parts := agentParts(request)
	if len(parts) != 2 {
		http.NotFound(writer, request)
		return
	}
	var value agent.Definition
	var err error
	switch parts[1] {
	case "default":
		value, err = s.store.SetDefaultAgent(request.Context(), parts[0])
	case "duplicate":
		value, err = s.store.DuplicateAgent(request.Context(), parts[0])
	case "archive":
		err = s.store.ArchiveAgent(request.Context(), parts[0])
	case "restore":
		value, err = s.store.RestoreAgent(request.Context(), parts[0])
	default:
		http.NotFound(writer, request)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		s.writeError(writer, http.StatusNotFound, "agent not found")
		return
	}
	if err != nil {
		s.writeError(writer, http.StatusConflict, err.Error())
		return
	}
	if parts[1] == "archive" {
		writer.WriteHeader(http.StatusNoContent)
		return
	}
	status := http.StatusOK
	if parts[1] == "duplicate" {
		status = http.StatusCreated
	}
	s.writeJSON(writer, status, value)
}

func agentParts(request *http.Request) []string {
	value := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/agents"), "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}
