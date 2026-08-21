package webapp

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Saieshwar5/perpetual/internal/accounts"
	"github.com/Saieshwar5/perpetual/internal/githubapp"
)

func TestEventBrokerKeepsLatestNoticeForSlowSubscriber(t *testing.T) {
	broker := NewEventBroker()
	events, unsubscribe := broker.subscribe()
	defer unsubscribe()
	broker.SessionChanged("workspace-1", "session-1", "run-1")
	broker.SessionChanged("workspace-2", "session-2", "run-2")
	select {
	case event := <-events:
		if event.RunID != "run-2" {
			t.Fatalf("event = %#v", event)
		}
	case <-time.After(time.Second):
		t.Fatal("event was not delivered")
	}
}

func TestEventStreamAuthenticatesAndDeliversSessionNotice(t *testing.T) {
	root := t.TempDir()
	credentials := filepath.Join(root, "github.json")
	if err := githubapp.SaveCredentials(credentials, githubapp.Credentials{
		AccessToken: "secret", User: githubapp.User{ID: 1, Login: "octocat"},
	}); err != nil {
		t.Fatalf("SaveCredentials: %v", err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	broker := NewEventBroker()
	server := &Server{
		ctx: ctx, events: broker, github: &fakeGitHub{}, credentialsPath: credentials,
	}
	requestContext, stopRequest := context.WithCancel(context.Background())
	requestContext = context.WithValue(requestContext, accountContextKey{}, accounts.User{ID: "user-1"})
	request := httptest.NewRequest(http.MethodGet, "/api/events", nil).WithContext(requestContext)
	response := newStreamResponse()
	done := make(chan struct{})
	go func() {
		server.eventStream(response, request)
		close(done)
	}()
	waitForFlush(t, response.flushed)
	if response.header.Get("Content-Type") != "text/event-stream" ||
		!strings.Contains(response.String(), "event: connected") {
		t.Fatalf("connected response: headers = %#v, body = %q", response.header, response.String())
	}
	broker.SessionChanged("workspace-1", "session-1", "run-1")
	waitForFlush(t, response.flushed)
	body := response.String()
	if !strings.Contains(body, "event: session.changed") || !strings.Contains(body, `"run_id":"run-1"`) {
		t.Fatalf("session response = %q", body)
	}
	stopRequest()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("event stream did not stop with its request")
	}
}

type streamResponse struct {
	mu      sync.Mutex
	header  http.Header
	body    bytes.Buffer
	flushed chan struct{}
}

func newStreamResponse() *streamResponse {
	return &streamResponse{header: make(http.Header), flushed: make(chan struct{}, 2)}
}

func (w *streamResponse) Header() http.Header { return w.header }
func (w *streamResponse) WriteHeader(int)     {}
func (w *streamResponse) Write(data []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.Write(data)
}
func (w *streamResponse) Flush() { w.flushed <- struct{}{} }
func (w *streamResponse) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.body.String()
}

func waitForFlush(t *testing.T, flushed <-chan struct{}) {
	t.Helper()
	select {
	case <-flushed:
	case <-time.After(time.Second):
		t.Fatal("event stream did not flush")
	}
}
