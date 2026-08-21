package webapp

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"
)

const eventHeartbeatInterval = 20 * time.Second

type browserEvent struct {
	Type        string `json:"type"`
	WorkspaceID string `json:"workspace_id,omitempty"`
	SessionID   string `json:"session_id,omitempty"`
	RunID       string `json:"run_id,omitempty"`
}

// EventBroker fans small invalidation notices out to connected browsers.
// SQLite remains the source of truth; slow browsers receive the latest notice.
type EventBroker struct {
	mu          sync.Mutex
	subscribers map[chan browserEvent]struct{}
}

func NewEventBroker() *EventBroker {
	return &EventBroker{subscribers: make(map[chan browserEvent]struct{})}
}

func (b *EventBroker) SessionChanged(workspaceID, sessionID, runID string) {
	b.publish(browserEvent{
		Type: "session.changed", WorkspaceID: workspaceID, SessionID: sessionID, RunID: runID,
	})
}

func (b *EventBroker) publish(event browserEvent) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for subscriber := range b.subscribers {
		select {
		case subscriber <- event:
		default:
			select {
			case <-subscriber:
			default:
			}
			select {
			case subscriber <- event:
			default:
			}
		}
	}
}

func (b *EventBroker) subscribe() (<-chan browserEvent, func()) {
	channel := make(chan browserEvent, 1)
	b.mu.Lock()
	b.subscribers[channel] = struct{}{}
	b.mu.Unlock()
	return channel, func() {
		b.mu.Lock()
		delete(b.subscribers, channel)
		b.mu.Unlock()
	}
}

func (s *Server) eventStream(writer http.ResponseWriter, request *http.Request) {
	if _, ok := currentAccount(request.Context()); !ok {
		s.writeError(writer, http.StatusUnauthorized, "authentication required")
		return
	}
	flusher, ok := writer.(http.Flusher)
	if !ok {
		s.writeError(writer, http.StatusInternalServerError, "event streaming is unavailable")
		return
	}
	writer.Header().Set("Content-Type", "text/event-stream")
	writer.Header().Set("Cache-Control", "no-cache, no-transform")
	writer.Header().Set("X-Content-Type-Options", "nosniff")
	writer.Header().Set("X-Accel-Buffering", "no")
	events, unsubscribe := s.events.subscribe()
	defer unsubscribe()
	_, _ = fmt.Fprint(writer, "retry: 3000\nevent: connected\ndata: {}\n\n")
	flusher.Flush()
	heartbeat := time.NewTicker(eventHeartbeatInterval)
	defer heartbeat.Stop()
	for {
		select {
		case event := <-events:
			payload, err := json.Marshal(event)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(writer, "event: %s\ndata: %s\n\n", event.Type, payload); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprint(writer, ": keep-alive\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-request.Context().Done():
			return
		case <-s.ctx.Done():
			return
		}
	}
}
