package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Saieshwar5/ayati-runtime/internal/protocol"
	"github.com/Saieshwar5/ayati-runtime/internal/provider"
	agentruntime "github.com/Saieshwar5/ayati-runtime/internal/runtime"
	"github.com/Saieshwar5/ayati-runtime/internal/shell"
)

func main() {
	os.Exit(run(os.Args[1:], os.Stdin, os.Stdout, os.Stderr))
}

func run(args []string, input io.Reader, output, errors io.Writer) int {
	return runWithClient(args, input, output, errors, nil)
}

func runWithClient(args []string, input io.Reader, output, errors io.Writer, client *http.Client) int {
	if len(args) == 0 || args[0] != "run" {
		fmt.Fprintln(errors, "usage: ayati-runtime run --config PATH < request.json")
		return 2
	}
	flags := flag.NewFlagSet("ayati-runtime run", flag.ContinueOnError)
	flags.SetOutput(errors)
	configPath := flags.String("config", "/etc/ayati/runtime.json", "runtime configuration file")
	if err := flags.Parse(args[1:]); err != nil {
		return 2
	}
	if flags.NArg() != 0 {
		fmt.Fprintln(errors, "ayati-runtime run does not accept positional arguments")
		return 2
	}

	config, err := protocol.LoadConfig(*configPath)
	if err != nil {
		fmt.Fprintf(errors, "error: %v\n", err)
		return 2
	}
	request, err := protocol.DecodeRequest(input)
	if err != nil {
		fmt.Fprintf(errors, "error: %v\n", err)
		return 2
	}
	apiKey := os.Getenv(config.Provider.APIKeyEnv)
	if strings.TrimSpace(apiKey) == "" {
		fmt.Fprintf(errors, "error: provider API key environment variable %s is empty\n", config.Provider.APIKeyEnv)
		return 2
	}

	providerConfig := provider.Config{
		APIKey:          apiKey,
		Model:           config.Provider.Model,
		Endpoint:        config.Provider.Endpoint,
		MaxOutputTokens: config.Provider.MaxOutputTokens,
		Client:          client,
	}
	model, err := provider.New(config.Provider.Kind, providerConfig)
	if err != nil {
		fmt.Fprintf(errors, "error: %v\n", err)
		return 2
	}

	limits := config.RuntimeLimits()
	executor := &shell.Executor{
		WorkingDir: request.Workspace,
		ShellPath:  config.Shell.Path,
		MaxOutput:  limits.MaxOutputBytes,
		Env:        selectedEnvironment(config.Shell.PassEnv),
	}
	runner := &agentruntime.Runtime{
		Model:     model,
		Provider:  config.Provider.Kind,
		ModelName: config.Provider.Model,
		Shell:     executor,
		Sink:      protocol.NewJSONLSink(output),
		Limits:    limits,
		Context: agentruntime.ContextPolicy{
			WindowTokens:    config.Provider.ContextWindowTokens,
			MaxOutputTokens: config.Provider.MaxOutputTokens,
		},
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	result, err := runner.Run(ctx, request)
	if err != nil {
		fmt.Fprintf(errors, "error: %v\n", err)
		return 1
	}
	if result.Status == agentruntime.StatusCompleted {
		return 0
	}
	return 1
}

func selectedEnvironment(names []string) []string {
	if names == nil {
		return nil
	}
	result := make([]string, 0, len(names))
	for _, name := range names {
		if value, ok := os.LookupEnv(name); ok {
			result = append(result, name+"="+value)
		}
	}
	return result
}
