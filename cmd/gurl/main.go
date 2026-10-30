// Command gurl is a curl-inspired HTTP client.
package main

import (
	"context"
	"os"
	"os/signal"

	"github.com/CSingogo/gurl/internal/app"
)

func main() {
	// Ctrl-C cancels the context, which aborts the in-flight request cleanly.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	code := app.Run(ctx, os.Args[1:], os.Stdin, os.Stdout, os.Stderr)
	stop() // os.Exit skips deferred calls, so release the signal handler first
	os.Exit(code)
}
