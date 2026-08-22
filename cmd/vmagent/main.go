package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/Saieshwar5/perpetual/internal/vmagent"
)

func main() {
	dataAddr := flag.String("data-addr", os.Getenv("VMAGENT_DATA_ADDR"), "data-plane listen address")
	if *dataAddr == "" {
		*dataAddr = ":8080"
	}
	hooksAddr := flag.String("hooks-addr", os.Getenv("VMAGENT_HOOKS_ADDR"), "lifecycle hooks listen address")
	if *hooksAddr == "" {
		*hooksAddr = ":9000"
	}
	root := flag.String("root", os.Getenv("VMAGENT_ROOT"), "workspace root directory")
	if *root == "" {
		*root = "/workspace"
	}
	flag.Parse()

	handler := &vmagent.Handler{Root: *root}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	dataServer := &http.Server{Addr: *dataAddr, Handler: handler.DataHandler(), ReadHeaderTimeout: 5 * time.Second}
	hooksServer := &http.Server{Addr: *hooksAddr, Handler: handler.HookHandler(), ReadHeaderTimeout: 5 * time.Second}

	serveErrors := make(chan error, 2)
	go func() { serveErrors <- dataServer.ListenAndServe() }()
	go func() { serveErrors <- hooksServer.ListenAndServe() }()
	log.Printf("vmagent: data plane on %s, hooks on %s, root %s", *dataAddr, *hooksAddr, *root)

	select {
	case <-ctx.Done():
	case err := <-serveErrors:
		if err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("vmagent: serve: %v", err)
		}
	}
	shutdown, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	_ = dataServer.Shutdown(shutdown)
	_ = hooksServer.Shutdown(shutdown)
}
