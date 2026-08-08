package terminal

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"github.com/Saieshwar5/ayati-code/internal/agent"
	"github.com/Saieshwar5/ayati-code/internal/config"
	"github.com/Saieshwar5/ayati-code/internal/install"
	"github.com/Saieshwar5/ayati-code/internal/provider"
	"github.com/Saieshwar5/ayati-code/internal/session"
	"github.com/Saieshwar5/ayati-code/internal/shell"
)

const defaultModel = "accounts/fireworks/models/deepseek-v4-flash-0731"

type App struct {
	Input  io.Reader
	Output io.Writer
	Error  io.Writer
}

func (a App) Run(ctx context.Context, args []string) error {
	configPath, err := config.Path()
	if err != nil {
		return err
	}
	values, err := config.Load(configPath)
	if err != nil {
		return err
	}

	flags := flag.NewFlagSet("ayati-code", flag.ContinueOnError)
	flags.SetOutput(a.Error)
	cwd := flags.String("cwd", "", "project working directory")
	model := flags.String("model", config.Effective(values, "NCA_MODEL", defaultModel), "Fireworks model ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	remaining := flags.Args()
	command := "continue"
	if len(remaining) > 0 {
		command = remaining[0]
	}

	switch command {
	case "help":
		a.printHelp()
		return nil
	case "setup":
		return a.setup(configPath, values)
	case "config":
		return a.configCommand(configPath, values, remaining[1:])
	case "model":
		return a.modelCommand(configPath, values, remaining[1:])
	case "install":
		return a.install()
	}

	if *cwd == "" {
		current, err := os.Getwd()
		if err != nil {
			return fmt.Errorf("get working directory: %w", err)
		}
		*cwd = current
	}
	absCWD, err := filepath.Abs(*cwd)
	if err != nil {
		return fmt.Errorf("resolve working directory: %w", err)
	}
	if info, err := os.Stat(absCWD); err != nil || !info.IsDir() {
		return fmt.Errorf("working directory does not exist: %s", absCWD)
	}

	sessionDir, err := configuredSessionDir(values)
	if err != nil {
		return err
	}
	store := session.Store{Dir: sessionDir}
	if command == "sessions" {
		return a.printSessions(store)
	}

	var current *session.Session
	switch command {
	case "new":
		current, err = store.Create(absCWD)
	case "continue":
		current, err = store.ContinueRecent(absCWD)
	case "open":
		if len(remaining) < 2 {
			return fmt.Errorf("usage: ayati-code [flags] open <session-id>")
		}
		current, err = store.Open(remaining[1])
	default:
		return fmt.Errorf("unknown command %q; use ayati-code help", command)
	}
	if err != nil {
		return err
	}
	return a.interact(ctx, store, current, *model, configPath, values)
}

func (a App) interact(ctx context.Context, store session.Store, current *session.Session, model, configPath string, values config.Values) error {
	executor := &shell.Executor{
		WorkingDir: current.Header.CWD,
		Timeout:    durationValue(config.Effective(values, "NCA_SHELL_TIMEOUT", "2m"), 2*time.Minute),
		MaxOutput:  intValue(config.Effective(values, "NCA_MAX_OUTPUT", "32768"), 32<<10),
	}
	fireworks := &provider.Fireworks{
		APIKey:  config.Effective(values, "FIREWORKS_API_KEY", ""),
		Model:   model,
		BaseURL: config.Effective(values, "NCA_FIREWORKS_URL", ""),
	}
	codingAgent := &agent.Agent{
		Provider:              fireworks,
		Shell:                 executor,
		Store:                 store,
		Session:               current,
		Output:                a.Output,
		MaxToolCalls:          intValue(config.Effective(values, "NCA_MAX_TOOL_CALLS", "30"), 30),
		MaxContextToolPairs:   intValue(config.Effective(values, "NCA_MAX_CONTEXT_TOOL_PAIRS", "100"), 100),
		ContextPercent:        intValue(config.Effective(values, "NCA_CONTEXT_PERCENT", "70"), 70),
		FallbackContextTokens: intValue(config.Effective(values, "NCA_MODEL_CONTEXT_TOKENS", strconv.Itoa(fallbackContextTokens(model))), fallbackContextTokens(model)),
	}

	fmt.Fprintf(a.Output, "Ayati Code\nsession: %s\nproject: %s\nmodel: %s\nType /help for commands.\n\n", current.Header.ID, current.Header.CWD, model)
	scanner := bufio.NewScanner(a.Input)
	scanner.Buffer(make([]byte, 64<<10), 1<<20)
	for {
		fmt.Fprint(a.Output, "> ")
		if !scanner.Scan() {
			if err := scanner.Err(); err != nil {
				return fmt.Errorf("read terminal input: %w", err)
			}
			fmt.Fprintln(a.Output)
			return nil
		}
		input := strings.TrimSpace(scanner.Text())
		if input == "" {
			continue
		}
		if strings.HasPrefix(input, "/") {
			quit, err := a.handleCommand(ctx, store, codingAgent, executor, fireworks, configPath, values, input)
			if err != nil {
				fmt.Fprintf(a.Error, "error: %v\n", err)
			}
			if quit {
				return nil
			}
			continue
		}
		if err := codingAgent.Prompt(ctx, input); err != nil {
			fmt.Fprintf(a.Error, "error: %v\n", err)
		}
		fmt.Fprintln(a.Output)
	}
}

func (a App) handleCommand(ctx context.Context, store session.Store, codingAgent *agent.Agent, executor *shell.Executor, fireworks *provider.Fireworks, configPath string, values config.Values, input string) (bool, error) {
	parts := strings.Fields(input)
	switch parts[0] {
	case "/quit", "/exit":
		return true, nil
	case "/help":
		fmt.Fprintln(a.Output, "/new                 create a new session for this project")
		fmt.Fprintln(a.Output, "/sessions            list saved sessions")
		fmt.Fprintln(a.Output, "/open ID             open a saved session")
		fmt.Fprintln(a.Output, "/session             show current session information")
		fmt.Fprintln(a.Output, "/model               show the active model")
		fmt.Fprintln(a.Output, "/model MODEL         change model for this process")
		fmt.Fprintln(a.Output, "/model save MODEL    change and save the default model")
		fmt.Fprintln(a.Output, "/compact             summarize older context now")
		fmt.Fprintln(a.Output, "/quit                exit")
	case "/session":
		fmt.Fprintf(a.Output, "id: %s\nproject: %s\nmessages: %d\nfile: %s\n", codingAgent.Session.Header.ID, codingAgent.Session.Header.CWD, len(codingAgent.Session.Messages), codingAgent.Session.Path)
	case "/sessions":
		return false, a.printSessions(store)
	case "/new":
		created, err := store.Create(codingAgent.Session.Header.CWD)
		if err != nil {
			return false, err
		}
		codingAgent.Session = created
		fmt.Fprintf(a.Output, "new session: %s\n", created.Header.ID)
	case "/open":
		if len(parts) != 2 {
			return false, fmt.Errorf("usage: /open <session-id>")
		}
		opened, err := store.Open(parts[1])
		if err != nil {
			return false, err
		}
		if info, err := os.Stat(opened.Header.CWD); err != nil || !info.IsDir() {
			return false, fmt.Errorf("session project does not exist: %s", opened.Header.CWD)
		}
		codingAgent.Session = opened
		executor.WorkingDir = opened.Header.CWD
		fmt.Fprintf(a.Output, "opened session %s\nproject: %s\n", opened.Header.ID, opened.Header.CWD)
	case "/model":
		if len(parts) == 1 {
			fmt.Fprintln(a.Output, fireworks.Model)
			break
		}
		if len(parts) == 2 {
			fireworks.Model = parts[1]
			codingAgent.ResetModelContext(fallbackContextTokens(parts[1]))
			fmt.Fprintf(a.Output, "model changed for this process: %s\n", fireworks.Model)
			break
		}
		if len(parts) == 3 && parts[1] == "save" {
			fireworks.Model = parts[2]
			codingAgent.ResetModelContext(fallbackContextTokens(parts[2]))
			values["NCA_MODEL"] = parts[2]
			if err := config.Save(configPath, values); err != nil {
				return false, err
			}
			fmt.Fprintf(a.Output, "default model saved: %s\n", fireworks.Model)
			break
		}
		return false, fmt.Errorf("usage: /model [MODEL] or /model save MODEL")
	case "/compact":
		if len(parts) != 1 {
			return false, fmt.Errorf("usage: /compact")
		}
		compacted, err := codingAgent.Compact(ctx)
		if err != nil {
			return false, err
		}
		if compacted {
			fmt.Fprintln(a.Output, "older context summarized and saved")
		} else {
			fmt.Fprintln(a.Output, "nothing old enough to compact")
		}
	default:
		return false, fmt.Errorf("unknown command %q", parts[0])
	}
	return false, nil
}

func (a App) setup(path string, values config.Values) error {
	currentKey := config.Effective(values, "FIREWORKS_API_KEY", "")
	fmt.Fprintf(a.Output, "Fireworks API key [%s]: ", config.Mask(currentKey))
	key, err := a.readSecret()
	if err != nil {
		return err
	}
	if key != "" {
		values["FIREWORKS_API_KEY"] = key
	} else if currentKey != "" {
		values["FIREWORKS_API_KEY"] = currentKey
	}
	fmt.Fprintf(a.Output, "Fireworks model [%s]: ", config.Effective(values, "NCA_MODEL", defaultModel))
	model, err := readLine(a.Input)
	if err != nil && err != io.EOF {
		return fmt.Errorf("read model: %w", err)
	}
	if strings.TrimSpace(model) != "" {
		values["NCA_MODEL"] = strings.TrimSpace(model)
	} else if values["NCA_MODEL"] == "" {
		values["NCA_MODEL"] = defaultModel
	}
	if values["FIREWORKS_API_KEY"] == "" {
		return fmt.Errorf("Fireworks API key is required")
	}
	if err := config.Save(path, values); err != nil {
		return err
	}
	fmt.Fprintf(a.Output, "Configuration saved securely to %s\n", path)
	return nil
}

func (a App) configCommand(path string, values config.Values, args []string) error {
	if len(args) == 0 || args[0] == "show" {
		fmt.Fprintf(a.Output, "API key: %s\n", config.Mask(config.Effective(values, "FIREWORKS_API_KEY", "")))
		fmt.Fprintf(a.Output, "Model: %s\n", config.Effective(values, "NCA_MODEL", defaultModel))
		fmt.Fprintf(a.Output, "Configuration: %s\n", path)
		return nil
	}
	if args[0] == "key" {
		fmt.Fprint(a.Output, "New Fireworks API key: ")
		key, err := a.readSecret()
		if err != nil {
			return err
		}
		if key == "" {
			return fmt.Errorf("API key cannot be empty")
		}
		values["FIREWORKS_API_KEY"] = key
		if err := config.Save(path, values); err != nil {
			return err
		}
		fmt.Fprintf(a.Output, "API key saved to %s\n", path)
		return nil
	}
	return fmt.Errorf("usage: ayati-code config [show|key]")
}

func (a App) modelCommand(path string, values config.Values, args []string) error {
	if len(args) == 0 {
		fmt.Fprintln(a.Output, config.Effective(values, "NCA_MODEL", defaultModel))
		return nil
	}
	if len(args) != 1 {
		return fmt.Errorf("usage: ayati-code model [MODEL_ID]")
	}
	values["NCA_MODEL"] = args[0]
	if err := config.Save(path, values); err != nil {
		return err
	}
	fmt.Fprintf(a.Output, "default model saved: %s\n", args[0])
	return nil
}

func (a App) install() error {
	path, err := install.Current()
	if err != nil {
		return err
	}
	fmt.Fprintf(a.Output, "Installed command: %s\n", path)
	if !pathContains(filepath.Dir(path)) {
		fmt.Fprintf(a.Output, "Add this to your shell configuration:\nexport PATH=\"$HOME/.local/bin:$PATH\"\n")
	}
	return nil
}

func (a App) printSessions(store session.Store) error {
	infos, err := store.List()
	if err != nil {
		return err
	}
	if len(infos) == 0 {
		fmt.Fprintln(a.Output, "No saved sessions.")
		return nil
	}
	for _, info := range infos {
		fmt.Fprintf(a.Output, "%s  %s  %s\n", info.ID, info.UpdatedAt.Format("2006-01-02 15:04"), info.CWD)
	}
	return nil
}

func (a App) printHelp() {
	fmt.Fprintln(a.Output, "Ayati Code")
	fmt.Fprintln(a.Output, "usage: ayati-code [flags] [continue|new|sessions|open ID]")
	fmt.Fprintln(a.Output, "       ayati-code setup|config|model|install")
	fmt.Fprintln(a.Output, "flags: -cwd PATH  -model MODEL_ID")
}

func (a App) readSecret() (string, error) {
	file, isFile := a.Input.(*os.File)
	hidden := false
	if isFile {
		command := exec.Command("stty", "-echo")
		command.Stdin = file
		hidden = command.Run() == nil
	}
	if hidden {
		defer func() {
			command := exec.Command("stty", "echo")
			command.Stdin = file
			_ = command.Run()
			fmt.Fprintln(a.Output)
		}()
	}
	line, err := readLine(a.Input)
	if err != nil && err != io.EOF {
		return "", fmt.Errorf("read secret: %w", err)
	}
	return strings.TrimSpace(line), nil
}

func readLine(reader io.Reader) (string, error) {
	var line strings.Builder
	buffer := make([]byte, 1)
	for {
		count, err := reader.Read(buffer)
		if count == 1 {
			if buffer[0] == '\n' {
				return strings.TrimSuffix(line.String(), "\r"), nil
			}
			line.WriteByte(buffer[0])
		}
		if err != nil {
			return strings.TrimSuffix(line.String(), "\r"), err
		}
	}
}

func configuredSessionDir(values config.Values) (string, error) {
	if value := config.Effective(values, "NCA_SESSION_DIR", ""); value != "" {
		return filepath.Abs(value)
	}
	return session.DefaultDir()
}

func intValue(value string, fallback int) int {
	parsed, err := strconv.Atoi(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func durationValue(value string, fallback time.Duration) time.Duration {
	parsed, err := time.ParseDuration(value)
	if err != nil || parsed <= 0 {
		return fallback
	}
	return parsed
}

func pathContains(directory string) bool {
	for _, entry := range filepath.SplitList(os.Getenv("PATH")) {
		if entry == directory {
			return true
		}
	}
	return false
}

func fallbackContextTokens(model string) int {
	if strings.Contains(model, "deepseek-v4-flash") || strings.Contains(model, "deepseek-v4-pro") {
		return 1048576
	}
	return 128000
}
