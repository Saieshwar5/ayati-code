package webapp

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	compute "github.com/Saieshwar5/perpetual/internal/environment"
)

func (s *Server) listEnvironments(writer http.ResponseWriter, request *http.Request) {
	if !s.requireEnvironmentManagement(writer) {
		return
	}
	values, err := s.environments.List(request.Context())
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "list environments")
		return
	}
	if values == nil {
		values = []compute.Environment{}
	}
	s.writeJSON(writer, http.StatusOK, values)
}

func (s *Server) createEnvironment(writer http.ResponseWriter, request *http.Request) {
	if !s.requireEnvironmentManagement(writer) {
		return
	}
	var input compute.CreateInput
	if !s.decode(writer, request, &input) {
		return
	}
	value, err := s.environments.Create(request.Context(), input)
	if err != nil {
		if value.ID != "" {
			s.writeJSON(writer, http.StatusUnprocessableEntity, value)
			return
		}
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(writer, http.StatusCreated, value)
}

func (s *Server) repairEnvironment(writer http.ResponseWriter, request *http.Request) {
	id, ok := environmentPath(request.URL.Path, "repair")
	if !ok {
		http.NotFound(writer, request)
		return
	}
	if !s.requireEnvironmentManagement(writer) {
		return
	}
	value, err := s.environments.Repair(request.Context(), id)
	if err != nil {
		if errors.Is(err, compute.ErrEnvironmentQuarantined) {
			s.writeEnvironmentError(writer, err)
			return
		}
		if value.ID != "" && value.ProvisioningState == compute.ProvisioningFailed {
			s.writeJSON(writer, http.StatusUnprocessableEntity, value)
			return
		}
		s.writeEnvironmentError(writer, err)
		return
	}
	s.writeJSON(writer, http.StatusOK, value)
}

func (s *Server) deleteEnvironment(writer http.ResponseWriter, request *http.Request) {
	id, ok := environmentPath(request.URL.Path, "")
	if !ok {
		http.NotFound(writer, request)
		return
	}
	if !s.requireEnvironmentManagement(writer) {
		return
	}
	if err := s.environments.Delete(request.Context(), id); err != nil {
		s.writeEnvironmentError(writer, err)
		return
	}
	writer.WriteHeader(http.StatusNoContent)
}

func (s *Server) requireEnvironmentManagement(writer http.ResponseWriter) bool {
	if s.environments == nil {
		s.writeError(writer, http.StatusNotImplemented, "environment management is unavailable")
		return false
	}
	return true
}

func (s *Server) writeEnvironmentError(writer http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, sql.ErrNoRows):
		s.writeError(writer, http.StatusNotFound, "environment not found")
	case errors.Is(err, compute.ErrEnvironmentOccupied), errors.Is(err, compute.ErrLeaseState):
		s.writeError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, compute.ErrEnvironmentReady):
		s.writeError(writer, http.StatusConflict, err.Error())
	case errors.Is(err, compute.ErrEnvironmentQuarantined):
		s.writeError(writer, http.StatusConflict, err.Error())
	default:
		s.writeError(writer, http.StatusInternalServerError, "manage environment")
	}
}

func environmentPath(path, action string) (string, bool) {
	parts := strings.Split(strings.Trim(strings.TrimPrefix(path, "/api/environments"), "/"), "/")
	if len(parts) == 1 && action == "" && strings.TrimSpace(parts[0]) != "" {
		return strings.TrimSpace(parts[0]), true
	}
	if len(parts) == 2 && parts[1] == action && strings.TrimSpace(parts[0]) != "" {
		return strings.TrimSpace(parts[0]), true
	}
	return "", false
}
