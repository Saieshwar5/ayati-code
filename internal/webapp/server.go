package webapp

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"log"
	"net/http"
	"strings"

	"github.com/Saieshwar5/perpetual/internal/githubapp"
	"github.com/Saieshwar5/perpetual/internal/workspace"
)

const maxRequestBytes = 1 << 20

type githubClient interface {
	LoginURL() string
	AuthorizeURL(string) string
	Exchange(context.Context, string) (string, error)
	CurrentUser(context.Context, string) (githubapp.User, error)
	Repositories(context.Context, string) ([]githubapp.Repository, error)
	CreateRepository(context.Context, string, githubapp.CreateRepositoryInput) (githubapp.Repository, error)
	Branches(context.Context, string, string) ([]githubapp.Branch, error)
	CreatePullRequest(context.Context, string, string, string, string, string, string) (githubapp.PullRequest, error)
}

type workspaceService interface {
	StartPreparation(context.Context, string) error
	RebuildEnvironment(context.Context, string) error
	ConfigureProjectRoot(context.Context, string, string) error
	Stop(context.Context, string) error
	Resume(context.Context, string) error
	Archive(context.Context, string) error
	RestoreArchived(context.Context, string) error
	Delete(context.Context, string) error
	Changes(context.Context, string) (workspace.Changes, error)
	Publish(context.Context, string, string, string, string) error
}

type Server struct {
	ctx             context.Context
	store           *workspace.Store
	workspaces      workspaceService
	github          githubClient
	credentialsPath string
	workspaceRoot   string
	logger          *log.Logger
	assets          http.Handler
	events          *EventBroker
}

type Options struct {
	Context         context.Context
	Store           *workspace.Store
	Workspaces      workspaceService
	GitHub          githubClient
	CredentialsPath string
	WorkspaceRoot   string
	Logger          *log.Logger
	Events          *EventBroker
}

func New(options Options) (*Server, error) {
	if options.Store == nil || options.Workspaces == nil {
		return nil, errors.New("workspace store and service are required")
	}
	if strings.TrimSpace(options.CredentialsPath) == "" || strings.TrimSpace(options.WorkspaceRoot) == "" {
		return nil, errors.New("credential path and workspace root are required")
	}
	if options.Context == nil {
		options.Context = context.Background()
	}
	if options.Logger == nil {
		options.Logger = log.New(io.Discard, "", 0)
	}
	if options.Events == nil {
		options.Events = NewEventBroker()
	}
	static, err := fs.Sub(assets, "dist")
	if err != nil {
		return nil, err
	}
	return &Server{
		ctx: options.Context, store: options.Store, workspaces: options.Workspaces,
		github: options.GitHub, credentialsPath: options.CredentialsPath,
		workspaceRoot: options.WorkspaceRoot, logger: options.Logger,
		assets: http.FileServer(http.FS(static)), events: options.Events,
	}, nil
}

func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /api/health", s.health)
	mux.HandleFunc("GET /api/session", s.session)
	mux.HandleFunc("GET /api/events", s.eventStream)
	mux.HandleFunc("GET /auth/github", s.githubLogin)
	mux.HandleFunc("GET /auth/github/callback", s.githubCallback)
	mux.HandleFunc("POST /api/logout", s.mutate(s.logout))
	mux.HandleFunc("GET /api/repositories", s.repositories)
	mux.HandleFunc("GET /api/repositories/", s.branches)
	mux.HandleFunc("GET /api/workspaces", s.listWorkspaces)
	mux.HandleFunc("GET /api/workspaces/", s.workspaceRead)
	mux.HandleFunc("POST /api/workspaces", s.mutate(s.createWorkspace))
	mux.HandleFunc("POST /api/workspaces/new-project", s.mutate(s.createNewProjectWorkspace))
	mux.HandleFunc("POST /api/workspaces/", s.mutate(s.workspaceAction))
	mux.HandleFunc("PATCH /api/workspaces/", s.mutate(s.workspaceSessionMutation))
	mux.HandleFunc("DELETE /api/workspaces/", s.mutate(s.workspaceSessionMutation))
	mux.HandleFunc("GET /", s.index)
	mux.Handle("GET /assets/", s.assets)
	return s.recover(mux)
}

func (s *Server) index(writer http.ResponseWriter, request *http.Request) {
	if !applicationPath(request.URL.Path) {
		http.NotFound(writer, request)
		return
	}
	data, err := assets.ReadFile("dist/index.html")
	if err != nil {
		http.Error(writer, "load interface", http.StatusInternalServerError)
		return
	}
	writer.Header().Set("Content-Type", "text/html; charset=utf-8")
	_, _ = writer.Write(data)
}

func applicationPath(path string) bool {
	if path == "/" || path == "/workspaces" || path == "/workspaces/new" ||
		path == "/workspaces/archived" || path == "/environments" {
		return true
	}
	parts := strings.Split(strings.Trim(path, "/"), "/")
	return len(parts) == 2 && parts[0] == "workspaces" ||
		len(parts) == 4 && parts[0] == "workspaces" && parts[2] == "sessions"
}

func (s *Server) health(writer http.ResponseWriter, _ *http.Request) {
	s.writeJSON(writer, http.StatusOK, map[string]string{"status": "ok"})
}

func (s *Server) mutate(next http.HandlerFunc) http.HandlerFunc {
	return func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("X-Perpetual-Request") != "1" {
			s.writeError(writer, http.StatusForbidden, "missing Perpetual request header")
			return
		}
		next(writer, request)
	}
}

func (s *Server) recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		defer func() {
			if value := recover(); value != nil {
				s.logger.Printf("panic serving %s: %v", request.URL.Path, value)
				s.writeError(writer, http.StatusInternalServerError, "internal error")
			}
		}()
		next.ServeHTTP(writer, request)
	})
}

func (s *Server) decode(writer http.ResponseWriter, request *http.Request, output any) bool {
	request.Body = http.MaxBytesReader(writer, request.Body, maxRequestBytes)
	decoder := json.NewDecoder(request.Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(output); err != nil {
		s.writeError(writer, http.StatusBadRequest, "invalid request")
		return false
	}
	return true
}

func (s *Server) credentials() (githubapp.Credentials, error) {
	return githubapp.LoadCredentials(s.credentialsPath)
}

func (s *Server) writeJSON(writer http.ResponseWriter, status int, value any) {
	writer.Header().Set("Content-Type", "application/json")
	writer.WriteHeader(status)
	_ = json.NewEncoder(writer).Encode(value)
}

func (s *Server) writeError(writer http.ResponseWriter, status int, message string) {
	s.writeJSON(writer, status, map[string]string{"error": message})
}

func randomToken() (string, error) {
	var value [24]byte
	if _, err := rand.Read(value[:]); err != nil {
		return "", err
	}
	return hex.EncodeToString(value[:]), nil
}

func setStateCookie(writer http.ResponseWriter, request *http.Request, value string) {
	http.SetCookie(writer, &http.Cookie{
		Name: "perpetual_github_state", Value: value, Path: "/auth/github/callback",
		MaxAge: 600, HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: request.TLS != nil,
	})
}

func clearStateCookie(writer http.ResponseWriter, request *http.Request) {
	http.SetCookie(writer, &http.Cookie{
		Name: "perpetual_github_state", Path: "/auth/github/callback", MaxAge: -1,
		HttpOnly: true, SameSite: http.SameSiteLaxMode,
		Secure: request.TLS != nil,
	})
}

func formatError(prefix string, err error) string {
	return fmt.Sprintf("%s: %v", prefix, err)
}
