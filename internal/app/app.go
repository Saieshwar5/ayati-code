package app

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/fireworks"
	"github.com/Saieshwar5/ayati-code/internal/session"
	"github.com/Saieshwar5/ayati-code/internal/shell"
	"github.com/Saieshwar5/ayati-code/internal/ui"
)

const version = "dev"

func Run(ctx context.Context, args []string, input io.Reader, output, errorOutput io.Writer) int {
	flags := flag.NewFlagSet("ayati", flag.ContinueOnError)
	flags.SetOutput(errorOutput)
	showVersion := flags.Bool("version", false, "print the Ayati version")
	workspaceFlag := flags.String("workspace", ".", "project directory available to shell")
	modelFlag := flags.String("model", strings.TrimSpace(os.Getenv("AYATI_MODEL")), "Fireworks model id")
	sessionFlag := flags.String("session", "", "resume a session by id or unique prefix")
	if err := flags.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return 0
		}
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintf(errorOutput, "ayati: unexpected arguments: %v\n", flags.Args())
		return 2
	}
	if *showVersion {
		fmt.Fprintf(output, "ayati %s\n", version)
		return 0
	}
	apiKey := strings.TrimSpace(os.Getenv("FIREWORKS_API_KEY"))
	if apiKey == "" {
		fmt.Fprintln(errorOutput, "ayati: FIREWORKS_API_KEY is not set")
		return 2
	}
	workspace, err := canonicalWorkspace(*workspaceFlag)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	root, err := session.DefaultRoot()
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	store, err := session.Open(root)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	active, err := initialSession(store, workspace, *modelFlag, *sessionFlag)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	provider, err := fireworks.New(apiKey)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: %v\n", err)
		return 1
	}
	executor, err := shell.New(workspace)
	if err != nil {
		fmt.Fprintf(errorOutput, "ayati: initialize shell: %v\n", err)
		return 1
	}
	console := ui.New(input, output, errorOutput)
	console.Header(active.Info)
	for {
		if ctx.Err() != nil {
			return 130
		}
		text, err := console.Prompt()
		if errors.Is(err, io.EOF) {
			console.Newline()
			return 0
		}
		if err != nil {
			console.Error(err)
			continue
		}
		text = strings.TrimSpace(text)
		if text == "" {
			continue
		}
		handled, exit := handleCommand(text, store, workspace, &active, console)
		if exit {
			return 0
		}
		if handled {
			continue
		}
		loop := agent.Loop{
			Provider: provider, Shell: executor, Recorder: active, Observer: console, Model: active.Info.Model,
		}
		if _, err := loop.Run(ctx, &active.Messages, text); err != nil {
			console.Error(err)
		}
	}
}

func initialSession(store *session.Store, workspace, model, reference string) (*session.Session, error) {
	if strings.TrimSpace(reference) != "" {
		loaded, err := store.Load(reference)
		if err != nil {
			return nil, err
		}
		if loaded.Info.Workspace != workspace {
			return nil, fmt.Errorf("session workspace is %s", loaded.Info.Workspace)
		}
		return loaded, nil
	}
	if strings.TrimSpace(model) == "" {
		return nil, fmt.Errorf("--model or AYATI_MODEL is required for a new session")
	}
	return store.New(workspace, model)
}

func handleCommand(
	command string,
	store *session.Store,
	workspace string,
	active **session.Session,
	console *ui.Console,
) (bool, bool) {
	switch command {
	case "/exit":
		return true, true
	case "/help":
		console.Help()
		return true, false
	case "/new":
		next, err := store.New(workspace, (*active).Info.Model)
		if err != nil {
			console.Error(err)
			return true, false
		}
		*active = next
		console.SessionActivated(next.Info)
		return true, false
	case "/sessions":
		infos, err := store.List(workspace)
		if err != nil {
			console.Error(err)
			return true, false
		}
		console.Sessions(infos, (*active).Info.ID)
		return true, false
	}
	fields := strings.Fields(command)
	if len(fields) == 2 && fields[0] == "/resume" {
		next, err := store.Load(fields[1])
		if err == nil && next.Info.Workspace != workspace {
			err = fmt.Errorf("session workspace is %s", next.Info.Workspace)
		}
		if err != nil {
			console.Error(err)
			return true, false
		}
		*active = next
		console.SessionActivated(next.Info)
		return true, false
	}
	if strings.HasPrefix(command, "/") {
		console.Error(fmt.Errorf("unknown command %q; use /help", command))
		return true, false
	}
	return false, false
}

func canonicalWorkspace(value string) (string, error) {
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", fmt.Errorf("resolve workspace: %w", err)
	}
	resolved, err := filepath.EvalSymlinks(absolute)
	if err != nil {
		return "", fmt.Errorf("resolve workspace links: %w", err)
	}
	info, err := os.Stat(resolved)
	if err != nil {
		return "", fmt.Errorf("inspect workspace: %w", err)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("workspace is not a directory: %s", resolved)
	}
	return resolved, nil
}
