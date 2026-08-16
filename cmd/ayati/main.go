package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Saieshwar5/ayati-code/internal/config"
	"github.com/Saieshwar5/ayati-code/internal/webapp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	args := os.Args[1:]
	if len(args) == 1 && args[0] == "config" {
		os.Exit(config.Configure(ctx, os.Stdin, os.Stdout, os.Stderr))
	}
	os.Exit(webapp.Run(ctx, args, os.Stdout, os.Stderr))
}
