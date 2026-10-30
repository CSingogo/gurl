// Command gurl-server serves the gurl docs site and test endpoints.
package main

import (
	"context"
	"errors"
	"flag"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/CSingogo/gurl/internal/server"
)

// version is set at build time: go build -ldflags "-X main.version=1.2.3"
var version = "dev"

func main() {
	addr := flag.String("addr", envOr("GURL_ADDR", "127.0.0.1:8088"), "listen address (Caddy proxies to this)")
	flag.Parse()

	log := slog.New(slog.NewTextHandler(os.Stderr, nil)) // journald captures stderr under systemd

	srv, err := server.New(server.Config{Logger: log, Version: version})
	if err != nil {
		log.Error("startup failed", "err", err)
		os.Exit(1)
	}
	defer srv.Close()

	httpSrv := &http.Server{
		Addr:              *addr,
		Handler:           srv,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      30 * time.Second, // must exceed the longest /delay
		IdleTimeout:       60 * time.Second,
	}

	// systemd stops a service with SIGTERM; Ctrl-C sends SIGINT. Either way,
	// finish in-flight requests instead of cutting them off.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	errc := make(chan error, 1)
	go func() { errc <- httpSrv.ListenAndServe() }()
	log.Info("listening", "addr", *addr, "version", version)

	select {
	case err := <-errc:
		log.Error("server stopped", "err", err)
		os.Exit(1)
	case <-ctx.Done():
	}

	log.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	if err := httpSrv.Shutdown(shutdownCtx); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		log.Error("shutdown", "err", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
