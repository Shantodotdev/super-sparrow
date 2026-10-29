// Command sparrow runs the Super Sparrow server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/config"
	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/email"
	"github.com/shantodotdev/super-sparrow/server/internal/httpserver"
	"github.com/shantodotdev/super-sparrow/server/internal/logging"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

const usage = `Usage: sparrow <command>

Commands:
  serve                                     Apply migrations and run the server
  create-owner --name NAME --email EMAIL    Add the team's first owner
  version                                   Print the version`

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	command, args := os.Args[1], os.Args[2:]
	if command != "create-owner" && len(args) != 0 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	var err error
	switch command {
	case "serve":
		err = serve()
	case "create-owner":
		err = createOwner(args)
	case "version":
		fmt.Println(version)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// createOwner adds the first owner to a team that has nobody yet. They then sign in with their
// email like everyone else.
func createOwner(args []string) error {
	flags := flag.NewFlagSet("create-owner", flag.ContinueOnError)
	name := flags.String("name", "", "the owner's display name")
	address := flags.String("email", "", "the owner's email, where sign-in codes are sent")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *name == "" || *address == "" {
		return errors.New("usage: sparrow create-owner --name NAME --email EMAIL")
	}

	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return err
	}

	ctx := context.Background()
	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return err
	}
	defer db.Close()

	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		return err
	}

	owner, err := store.New(db, time.Now).CreateFirstOwner(ctx, *name, *address)
	if err != nil {
		return fmt.Errorf("create first owner: %w", err)
	}

	fmt.Printf("Added %s <%s> as the owner. Sign in at %s with this email.\n", owner.DisplayName, owner.Email, cfg.PublicURL)
	return nil
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

	st := store.New(db, time.Now)
	authService := auth.New(st, email.NewResend(cfg.ResendAPIKey, cfg.EmailFrom), cfg.PublicURL, logger, time.Now)
	defer authService.Wait()

	server := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: httpserver.New(httpserver.Options{
			Logger:         logger,
			DB:             db,
			Store:          st,
			Auth:           authService,
			PublicURL:      cfg.PublicURL,
			TrustedProxies: cfg.TrustedProxies,
			Now:            time.Now,
		}),
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
