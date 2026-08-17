package webapp

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/agent"
)

func (s *Server) skillsRead(writer http.ResponseWriter, request *http.Request) {
	parts := skillParts(request)
	if len(parts) == 0 {
		values, err := s.store.ListSkills(request.Context(), request.URL.Query().Get("archived") == "true")
		if err != nil {
			s.writeError(writer, http.StatusInternalServerError, "list skills")
			return
		}
		if values == nil {
			values = []agent.Skill{}
		}
		s.writeJSON(writer, http.StatusOK, values)
		return
	}
	if len(parts) != 1 {
		http.NotFound(writer, request)
		return
	}
	value, err := s.store.GetSkill(request.Context(), parts[0])
	if errors.Is(err, sql.ErrNoRows) {
		s.writeError(writer, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "load skill")
		return
	}
	s.writeJSON(writer, http.StatusOK, value)
}

func (s *Server) createSkill(writer http.ResponseWriter, request *http.Request) {
	var input agent.SkillInput
	if !s.decode(writer, request, &input) {
		return
	}
	value, err := s.store.CreateSkill(request.Context(), input)
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(writer, http.StatusCreated, value)
}

func (s *Server) updateSkill(writer http.ResponseWriter, request *http.Request) {
	parts := skillParts(request)
	if len(parts) != 1 {
		http.NotFound(writer, request)
		return
	}
	var input agent.SkillInput
	if !s.decode(writer, request, &input) {
		return
	}
	value, err := s.store.UpdateSkill(request.Context(), parts[0], input)
	if errors.Is(err, sql.ErrNoRows) {
		s.writeError(writer, http.StatusNotFound, "skill not found")
		return
	}
	if err != nil {
		s.writeError(writer, http.StatusBadRequest, err.Error())
		return
	}
	s.writeJSON(writer, http.StatusOK, value)
}

func (s *Server) skillAction(writer http.ResponseWriter, request *http.Request) {
	parts := skillParts(request)
	if len(parts) != 2 {
		http.NotFound(writer, request)
		return
	}
	var value agent.Skill
	var err error
	switch parts[1] {
	case "archive":
		err = s.store.ArchiveSkill(request.Context(), parts[0])
	case "restore":
		value, err = s.store.RestoreSkill(request.Context(), parts[0])
	default:
		http.NotFound(writer, request)
		return
	}
	if errors.Is(err, sql.ErrNoRows) {
		s.writeError(writer, http.StatusNotFound, "skill not found")
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
	s.writeJSON(writer, http.StatusOK, value)
}

func skillParts(request *http.Request) []string {
	value := strings.Trim(strings.TrimPrefix(request.URL.Path, "/api/skills"), "/")
	if value == "" {
		return nil
	}
	return strings.Split(value, "/")
}
