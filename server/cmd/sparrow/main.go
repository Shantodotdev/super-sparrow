// Command sparrow runs the Super Sparrow server.
package main

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/config"
	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/httpserver"
	"github.com/shantodotdev/super-sparrow/server/internal/logging"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

const usage = `Usage: sparrow <command>

Commands:
  serve     Apply migrations and run the server
  version   Print the version`

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	switch os.Args[1] {
	case "serve":
		if err := serve(); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}
}

// serve loads the configuration, migrates the database, and serves HTTP until it receives
// SIGINT or SIGTERM, then waits for open requests to finish.
func serve() error {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	logger := logging.New(os.Stdout, cfg.LogLevel)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           httpserver.New(logger, db),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Serve in the background so this function can wait for the shutdown signal.
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server started", "address", cfg.ListenAddr, "version", version)
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	// Give open requests and event streams a bounded time to finish before exiting.
	logger.Info("server stopping")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shut down http: %w", err)
	}

	return nil
}
