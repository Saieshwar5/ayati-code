package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"

	"github.com/Saieshwar5/perpetual/internal/webapp"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	os.Exit(webapp.Run(ctx, os.Args[1:], os.Stdout, os.Stderr))
}
