package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/sai-eshwar/no-nonsense-coding-ai/internal/terminal"
)

func main() {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	app := terminal.App{Input: os.Stdin, Output: os.Stdout, Error: os.Stderr}
	if err := app.Run(ctx, os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "ayati-micro: %v\n", err)
		os.Exit(1)
	}
}
