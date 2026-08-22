package webapp

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func (s *Server) enqueueRun(writer http.ResponseWriter, request *http.Request) {
	workspaceID := strings.TrimSpace(request.PathValue("workspaceID"))
	if !s.requireOwnedWorkspace(writer, request, workspaceID) {
		return
	}
	var input struct {
		SessionID string `json:"session_id"`
	}
	if !s.decode(writer, request, &input) {
		return
	}
	sessionID := strings.TrimSpace(input.SessionID)
	if sessionID == "" {
		s.writeError(writer, http.StatusBadRequest, "session_id is required")
		return
	}
	if _, err := s.store.GetSession(request.Context(), workspaceID, sessionID); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			s.writeError(writer, http.StatusNotFound, "session not found")
			return
		}
		s.writeError(writer, http.StatusInternalServerError, "load session")
		return
	}
	userID, ok := currentAccountID(request.Context())
	if !ok {
		s.writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	run, err := s.store.EnqueueRun(request.Context(), workspace.EnqueueRunInput{
		UserID: userID, WorkspaceID: workspaceID, SessionID: sessionID,
	})
	if err != nil {
		s.writeError(writer, http.StatusConflict, err.Error())
		return
	}
	s.events.RunChanged(workspaceID, sessionID, run.ID, run.State)
	s.writeJSON(writer, http.StatusCreated, run)
}

func (s *Server) listRuns(writer http.ResponseWriter, request *http.Request) {
	workspaceID := strings.TrimSpace(request.PathValue("workspaceID"))
	if !s.requireOwnedWorkspace(writer, request, workspaceID) {
		return
	}
	runs, err := s.store.RunsForWorkspace(request.Context(), workspaceID)
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "list execution rooms")
		return
	}
	if runs == nil {
		runs = []workspace.Run{}
	}
	s.writeJSON(writer, http.StatusOK, runs)
}

func (s *Server) runRead(writer http.ResponseWriter, request *http.Request) {
	run, ok := s.ownedRun(writer, request)
	if !ok {
		http.NotFound(writer, request)
		return
	}
	s.writeJSON(writer, http.StatusOK, run)
}

func (s *Server) runStepsRead(writer http.ResponseWriter, request *http.Request) {
	run, ok := s.ownedRun(writer, request)
	if !ok {
		http.NotFound(writer, request)
		return
	}
	steps, err := s.store.RunSteps(request.Context(), run.ID)
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "list execution room steps")
		return
	}
	if steps == nil {
		steps = []workspace.RunStep{}
	}
	s.writeJSON(writer, http.StatusOK, steps)
}

func (s *Server) runAction(writer http.ResponseWriter, request *http.Request) {
	run, ok := s.ownedRun(writer, request)
	if !ok {
		http.NotFound(writer, request)
		return
	}
	var err error
	state := run.State
	switch strings.TrimSpace(request.PathValue("action")) {
	case "stop":
		err = s.store.CancelRun(request.Context(), run.ID)
		state = workspace.RunCanceled
	case "pause":
		err = s.store.SetRunWaitingUser(request.Context(), run.ID)
		state = workspace.RunWaitingUser
	case "continue":
		err = s.store.ContinueRun(request.Context(), run.ID)
		state = workspace.RunQueued
	default:
		http.NotFound(writer, request)
		return
	}
	if err != nil {
		s.writeError(writer, http.StatusConflict, err.Error())
		return
	}
	s.events.RunChanged(run.WorkspaceID, run.SessionID, run.ID, state)
	updated, err := s.store.GetRun(request.Context(), run.ID)
	if err != nil {
		s.writeError(writer, http.StatusInternalServerError, "reload execution room")
		return
	}
	s.writeJSON(writer, http.StatusOK, updated)
}

// ownedRun verifies the current user owns the workspace of the run and returns
// the run. A missing run or non-owner renders as not found by the caller.
func (s *Server) ownedRun(writer http.ResponseWriter, request *http.Request) (workspace.Run, bool) {
	runID := strings.TrimSpace(request.PathValue("runID"))
	run, err := s.store.GetRun(request.Context(), runID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return workspace.Run{}, false
		}
		s.writeError(writer, http.StatusInternalServerError, "load execution room")
		return workspace.Run{}, false
	}
	ws, err := s.store.Get(request.Context(), run.WorkspaceID)
	if err != nil {
		return workspace.Run{}, false
	}
	userID, ok := currentAccountID(request.Context())
	if !ok || ws.UserID != userID {
		return workspace.Run{}, false
	}
	return run, true
}
