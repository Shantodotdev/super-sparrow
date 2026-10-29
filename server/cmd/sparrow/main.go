// Command sparrow runs the Super Sparrow server and its command-line client.
package main

import (
	"context"
	"database/sql"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"filippo.io/age"
	"github.com/shantodotdev/super-sparrow/server/internal/auth"
	"github.com/shantodotdev/super-sparrow/server/internal/backup"
	"github.com/shantodotdev/super-sparrow/server/internal/cli"
	"github.com/shantodotdev/super-sparrow/server/internal/config"
	"github.com/shantodotdev/super-sparrow/server/internal/database"
	"github.com/shantodotdev/super-sparrow/server/internal/email"
	"github.com/shantodotdev/super-sparrow/server/internal/httpserver"
	"github.com/shantodotdev/super-sparrow/server/internal/linear"
	"github.com/shantodotdev/super-sparrow/server/internal/logging"
	"github.com/shantodotdev/super-sparrow/server/internal/store"
	"github.com/shantodotdev/super-sparrow/server/migrations"
)

// version is set at build time with -ldflags "-X main.version=v1.2.3".
var version = "dev"

const usage = `Usage: sparrow <command>

Server commands (read the server settings from the environment):
  serve                                     Apply migrations and run the server
  create-owner --name NAME --email EMAIL    Add the team's first owner
  import-linear --project NAME [--team KEY] Import a Linear project (needs SPARROW_LINEAR_API_KEY)
  restore --list                            List the backup copies in R2 (needs the SPARROW_R2_ settings)
  restore --copy sync|daily/YYYY-MM-DD --identity KEY_FILE --database PATH
                                            Restore a copy into a new database file
  version                                   Print the version

` + cli.Usage

func main() {
	if len(os.Args) < 2 {
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	command, args := os.Args[1], os.Args[2:]
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var err error
	switch {
	case command == "serve" && len(args) == 0:
		err = serve(ctx)
	case command == "create-owner":
		err = createOwner(ctx, args)
	case command == "import-linear":
		err = importLinear(ctx, args)
	case command == "restore":
		err = restore(ctx, args)
	case command == "version" && len(args) == 0:
		fmt.Println(version)
	case cli.IsCommand(command):
		err = cli.Run(ctx, os.Args[1:], os.Getenv, os.Stdin, os.Stdout)
	default:
		fmt.Fprintln(os.Stderr, usage)
		os.Exit(2)
	}

	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

// openStore loads the server settings and opens and migrates the database. The caller closes db.
func openStore(ctx context.Context) (config.Config, *sql.DB, *store.Store, error) {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		return config.Config{}, nil, nil, err
	}

	db, err := database.Open(ctx, cfg.DatabasePath)
	if err != nil {
		return config.Config{}, nil, nil, err
	}
	if err := database.Migrate(ctx, db, migrations.Files); err != nil {
		db.Close()
		return config.Config{}, nil, nil, err
	}

	return cfg, db, store.New(db, time.Now), nil
}

// createOwner adds the first owner to a team that has nobody yet. They then sign in with their
// email like everyone else.
func createOwner(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("create-owner", flag.ContinueOnError)
	name := flags.String("name", "", "the owner's display name")
	address := flags.String("email", "", "the owner's email, where sign-in codes are sent")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *name == "" || *address == "" {
		return errors.New("usage: sparrow create-owner --name NAME --email EMAIL")
	}

	cfg, db, st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	owner, err := st.CreateFirstOwner(ctx, *name, *address)
	if err != nil {
		return fmt.Errorf("create first owner: %w", err)
	}

	fmt.Printf("Added %s <%s> as the owner. Sign in at %s with this email.\n", owner.DisplayName, owner.Email, cfg.PublicURL)
	return nil
}

// importLinear imports one Linear project with the API key in SPARROW_LINEAR_API_KEY, which only
// this command needs, and prints what it brought in.
func importLinear(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("import-linear", flag.ContinueOnError)
	project := flags.String("project", "", "the Linear project's name or ID")
	team := flags.String("team", "", "the team whose key the project takes, when the project has several")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 0 || *project == "" {
		return errors.New("usage: sparrow import-linear --project NAME [--team KEY]")
	}

	apiKey := strings.TrimSpace(os.Getenv("SPARROW_LINEAR_API_KEY"))
	if !strings.HasPrefix(apiKey, "lin_api_") {
		return errors.New("SPARROW_LINEAR_API_KEY is required: a Linear personal API key, which starts with lin_api_")
	}

	_, db, st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	report, err := linear.Import(ctx, st, linear.NewClient(linear.APIURL, apiKey), *project, *team)
	fmt.Printf("Project %s: %d milestones, %d issues, %d comments, %d review rounds with %d findings.\n",
		report.ProjectKey, report.Milestones, report.Issues, report.Comments, report.ReviewRounds, report.Findings)
	for _, warning := range report.Warnings {
		fmt.Println("  warning:", warning)
	}
	return err
}

// restore lists the backup copies, or restores one into a new database file. It needs only the
// R2 settings and the private key file, so it runs on a fresh machine without the server's settings.
func restore(ctx context.Context, args []string) error {
	flags := flag.NewFlagSet("restore", flag.ContinueOnError)
	list := flags.Bool("list", false, "list the copies in the bucket")
	copyName := flags.String("copy", "", "sync, daily/YYYY-MM-DD, or a key from --list")
	identityPath := flags.String("identity", "", "the age private key file that decrypts backups")
	target := flags.String("database", "", "where to write the restored database; it must not exist")
	if err := flags.Parse(args); err != nil {
		return err
	}

	r2, err := config.LoadR2(os.Getenv)
	if err != nil {
		return fmt.Errorf("invalid configuration:\n%w", err)
	}
	bucket := backup.NewR2Bucket(r2.AccountID, r2.AccessKeyID, r2.SecretAccessKey, r2.Bucket)

	if *list {
		if flags.NArg() != 0 || *copyName != "" || *identityPath != "" || *target != "" {
			return errors.New("usage: sparrow restore --list")
		}
		return listCopies(ctx, bucket)
	}
	if flags.NArg() != 0 || *copyName == "" || *identityPath == "" || *target == "" {
		return errors.New("usage: sparrow restore --copy sync|daily/YYYY-MM-DD --identity KEY_FILE --database PATH")
	}

	key, err := backup.ResolveCopy(*copyName)
	if err != nil {
		return err
	}
	identities, err := readIdentities(*identityPath)
	if err != nil {
		return err
	}
	path, err := filepath.Abs(*target)
	if err != nil {
		return fmt.Errorf("resolve --database: %w", err)
	}

	size, err := backup.Restore(ctx, bucket, key, identities, path)
	if err != nil {
		return err
	}
	fmt.Printf("Restored %s into %s (%d bytes); its integrity check passed.\n", key, path, size)
	return nil
}

func listCopies(ctx context.Context, bucket *backup.Bucket) error {
	for _, prefix := range []string{"sync/", "daily/"} {
		objects, err := bucket.List(ctx, prefix)
		if err != nil {
			return err
		}
		for _, object := range objects {
			fmt.Printf("%s\t%d bytes\t%s\n", object.Key, object.Size, object.LastModified.UTC().Format(time.RFC3339))
		}
	}
	return nil
}

func readIdentities(path string) ([]age.Identity, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("open --identity: %w", err)
	}
	defer file.Close()

	identities, err := age.ParseIdentities(io.LimitReader(file, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read --identity as an age private key file: %w", err)
	}
	return identities, nil
}

// serve migrates the database and serves HTTP until ctx ends, running the backup jobs alongside
// when they are enabled. On shutdown it ends event streams, waits for open requests to finish,
// and lets a running backup record its result.
func serve(ctx context.Context) error {
	cfg, db, st, err := openStore(ctx)
	if err != nil {
		return err
	}
	defer db.Close()

	logger := logging.New(os.Stdout, cfg.LogLevel)
	authService := auth.New(st, email.NewResend(cfg.ResendAPIKey, cfg.EmailFrom), cfg.PublicURL, cfg.SignInCodeKey, logger, time.Now)
	defer authService.Wait()

	stopStreams := make(chan struct{})
	server := &http.Server{
		Addr: cfg.ListenAddr,
		Handler: httpserver.New(httpserver.Options{
			Logger:              logger,
			DB:                  db,
			Store:               st,
			Auth:                authService,
			PublicURL:           cfg.PublicURL,
			TrustedProxies:      cfg.TrustedProxies,
			Now:                 time.Now,
			Stop:                stopStreams,
			BackupsEnabled:      cfg.Backups != nil,
			Version:             version,
			WebDir:              cfg.WebDir,
			GitHubWebhookSecret: cfg.GitHubWebhookSecret,
		}),
		ReadHeaderTimeout: 10 * time.Second,
	}
	server.RegisterOnShutdown(func() { close(stopStreams) })

	jobsCtx, stopJobs := context.WithCancel(context.Background())
	var jobs sync.WaitGroup
	if cfg.Backups != nil {
		runner := backup.NewRunner(backup.Options{
			Store:     st,
			Bucket:    backup.NewR2Bucket(cfg.Backups.R2.AccountID, cfg.Backups.R2.AccessKeyID, cfg.Backups.R2.SecretAccessKey, cfg.Backups.R2.Bucket),
			Recipient: cfg.Backups.Recipient,
			WorkDir:   filepath.Dir(cfg.DatabasePath),
			KeepDays:  cfg.Backups.KeepDays,
			DailyAt:   cfg.Backups.DailyAt,
			Logger:    logger,
			Now:       time.Now,
		})
		jobs.Go(func() { runner.Run(jobsCtx) })
	} else {
		logger.Warn("backups are disabled; set SPARROW_BACKUPS=enabled on any server that holds real data")
	}
	defer func() {
		stopJobs()
		jobs.Wait()
	}()

	// Serve in the background so this function can wait for the shutdown signal.
	serveErr := make(chan error, 1)
	go func() {
		logger.Info("server started", "address", cfg.ListenAddr, "version", version, "serves_frontend", cfg.WebDir != "")
		serveErr <- server.ListenAndServe()
	}()

	select {
	case err := <-serveErr:
		return fmt.Errorf("serve http: %w", err)
	case <-ctx.Done():
	}

	// Give open requests a bounded time to finish before exiting.
	logger.Info("server stopping")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("shut down http: %w", err)
	}

	return nil
}
