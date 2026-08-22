package webapp

import (
	"context"
	"encoding/json"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Saieshwar5/perpetual/internal/workspace"
)

func TestRunControlAPIEndToEnd(t *testing.T) {
	handler, store, _, _ := testHandler(t)
	ws, err := store.Create(context.Background(), workspace.Create{
		UserID: testAccountUserID, Repository: "owner/run", CloneURL: "https://github.com/owner/run.git",
		BaseBranch: "main", Branch: "main", Path: filepath.Join(t.TempDir(), "repo"),
	})
	if err != nil {
		t.Fatalf("Create workspace: %v", err)
	}
	session, err := store.CreateSession(context.Background(), ws.ID, "run session")
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	enqueuePath := "/api/workspaces/" + ws.ID + "/runs"
	body := `{"session_id":"` + session.ID + `"}`
	response := serve(handler, http.MethodPost, enqueuePath, body, true)
	if response.Code != http.StatusCreated {
		t.Fatalf("enqueue status = %d, body = %s", response.Code, response.Body.String())
	}
	var run workspace.Run
	if err := json.Unmarshal(response.Body.Bytes(), &run); err != nil {
		t.Fatalf("decode run: %v", err)
	}
	if run.ID == "" || run.State != workspace.RunQueued {
		t.Fatalf("run = %#v", run)
	}

	list := serve(handler, http.MethodGet, enqueuePath, "", false)
	if list.Code != http.StatusOK || !strings.Contains(list.Body.String(), run.ID) {
		t.Fatalf("list status = %d, body = %s", list.Code, list.Body.String())
	}

	detail := serve(handler, http.MethodGet, "/api/runs/"+run.ID, "", false)
	if detail.Code != http.StatusOK || !strings.Contains(detail.Body.String(), run.ID) {
		t.Fatalf("detail status = %d, body = %s", detail.Code, detail.Body.String())
	}

	stop := serve(handler, http.MethodPost, "/api/runs/"+run.ID+"/stop", "", true)
	if stop.Code != http.StatusOK || !strings.Contains(stop.Body.String(), workspace.RunCanceled) {
		t.Fatalf("stop status = %d, body = %s", stop.Code, stop.Body.String())
	}
}

func TestRunControlAPIRejectsGuest(t *testing.T) {
	handler, _, _, _ := testHandler(t)
	response := serveGuest(handler, http.MethodGet, "/api/runs/any", "", false)
	if response.Code != http.StatusUnauthorized {
		t.Fatalf("guest status = %d", response.Code)
	}
}

func TestEventBrokerPublishesRunChanged(t *testing.T) {
	broker := NewEventBroker()
	events, unsubscribe := broker.subscribe()
	defer unsubscribe()
	broker.RunChanged("ws", "session", "run", "running")
	event := <-events
	if event.Type != "run.changed" || event.RunID != "run" || event.State != "running" {
		t.Fatalf("event = %#v", event)
	}
}
