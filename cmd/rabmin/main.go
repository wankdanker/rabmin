// Command rabmin serves the rabmin API and UI.
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	clyops "github.com/wankdanker/clyops/packages/go"

	"github.com/wankdanker/rabmin/internal/api"
	"github.com/wankdanker/rabmin/internal/auth"
	"github.com/wankdanker/rabmin/internal/rabbitx"
	"github.com/wankdanker/rabmin/internal/secret"
	"github.com/wankdanker/rabmin/internal/store"
	"github.com/wankdanker/rabmin/internal/version"
	"github.com/wankdanker/rabmin/web"
)

// durationRule accepts Go durations such as 5s, 1m30s or 500ms.
const durationRule = `regex:^([0-9]+(\.[0-9]+)?(ns|us|ms|s|m|h))+$`

// config is the resolved command line.
type config struct {
	addr, dataDir, secretKey string
	poll                     time.Duration
	secure, allowSetup       bool
	tlsCert, tlsKey          string
	adminUser, adminPassword string
}

// newCLI declares rabmin's command line. Every option falls back to the
// environment variable of the same name.
func newCLI() *clyops.Cli {
	cli := clyops.New()
	cli.Name = "rabmin"
	cli.SetDescription("Management UI and HTTP API for RabbitMQ. Without a command, rabmin serves.")
	serve := cli.Command("serve", "Serve the API and UI (the default)")
	serve.SetEffects("network")
	serve.Opt("RABMIN_ADDR", "addr", "", ":8080", "Listen address", "Server")
	serve.Opt("RABMIN_DATA", "data", "", "./data", "Directory for the database and the secret key", "Server", "path")
	serve.Opt("RABMIN_POLL_INTERVAL", "poll", "", "5s", "How often each cluster is polled", "Server", durationRule)
	serve.Opt("RABMIN_LOG_JSON", "log-json", "", "flag", "Log as JSON", "Server")
	serve.Opt("RABMIN_TLS_CERT", "tls-cert", "", "optional", "Serve HTTPS with this certificate", "Security", "file:readable")
	serve.Opt("RABMIN_TLS_KEY", "tls-key", "", "optional", "Key for --tls-cert", "Security", "file:readable")
	serve.Opt("RABMIN_SECURE_COOKIES", "secure-cookies", "", "flag", "Mark session cookies Secure; enable behind HTTPS (implied by --tls-cert)", "Security")
	serve.Opt("RABMIN_SECRET_KEY", "secret-key", "", "optional",
		"Base64 32-byte key that encrypts stored credentials; generated in the data directory when unset", "Security", "secret")
	serve.Opt("RABMIN_ALLOW_SETUP", "allow-setup", "", "true", "Let the first visitor create the admin while no users exist", "Users", "bool")
	serve.Opt("RABMIN_ADMIN_USER", "admin-user", "", "optional", "Create this admin on startup when no users exist", "Users")
	serve.Opt("RABMIN_ADMIN_PASSWORD", "admin-password", "", "optional", "Password for --admin-user", "Users", "secret")
	serve.Requires("tls-key", "tls-cert")
	serve.Requires("tls-cert", "tls-key")
	serve.Requires("admin-password", "admin-user")
	serve.Requires("admin-user", "admin-password")
	openapi := cli.Command("openapi", "Print the OpenAPI document")
	openapi.SetEffects("read-only")
	openapi.SetStdout("OpenAPI 3.1 document", "application/json")
	cli.Command("version", "Print the version").SetEffects("read-only")
	return cli
}

// withDefaultCommand runs serve when no command word is given, so `rabmin`
// and `rabmin --addr :9090` keep working.
func withDefaultCommand(args []string) []string {
	if len(args) == 0 {
		return []string{"serve"}
	}
	switch args[0] {
	case "serve", "openapi", "version", "-h", "--help", "--help-json-schema", "--completion", "--bash-completion":
		return args
	case "--version", "-v":
		return []string{"version"}
	}
	if strings.HasPrefix(args[0], "-") {
		return append([]string{"serve"}, args...)
	}
	return args
}

func main() {
	cli := newCLI()
	v := cli.RunArgs(withDefaultCommand(os.Args[1:]))
	switch cli.CommandPath()[0] {
	case "openapi":
		dumpOpenAPI()
		return
	case "version":
		fmt.Println("rabmin", version.Version, version.Commit)
		return
	}
	poll, err := time.ParseDuration(v.String("RABMIN_POLL_INTERVAL"))
	if err != nil || poll <= 0 {
		clyops.Die(1, "--poll must be a positive duration, got '%s'", v.String("RABMIN_POLL_INTERVAL"))
	}
	cfg := config{addr: v.String("RABMIN_ADDR"), dataDir: v.String("RABMIN_DATA"), secretKey: v.String("RABMIN_SECRET_KEY"), poll: poll,
		secure: v.Bool("RABMIN_SECURE_COOKIES"), allowSetup: v.Bool("RABMIN_ALLOW_SETUP"), tlsCert: v.String("RABMIN_TLS_CERT"),
		tlsKey: v.String("RABMIN_TLS_KEY"), adminUser: v.String("RABMIN_ADMIN_USER"), adminPassword: v.String("RABMIN_ADMIN_PASSWORD")}
	cfg.secure = cfg.secure || cfg.tlsCert != ""

	var handler slog.Handler = slog.NewTextHandler(os.Stderr, nil)
	if v.Bool("RABMIN_LOG_JSON") {
		handler = slog.NewJSONHandler(os.Stderr, nil)
	}
	log := slog.New(handler)
	if err := run(log, cfg); err != nil {
		log.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run(log *slog.Logger, cfg config) error {
	addr, dataDir, poll, tlsCert, tlsKey := cfg.addr, cfg.dataDir, cfg.poll, cfg.tlsCert, cfg.tlsKey
	if err := os.MkdirAll(dataDir, 0o700); err != nil {
		return err
	}
	box, err := secret.Load(cfg.secretKey, filepath.Join(dataDir, "secret.key"))
	if err != nil {
		return fmt.Errorf("secret key: %w", err)
	}
	st, err := store.Open(filepath.Join(dataDir, "rabmin.db"), box)
	if err != nil {
		return fmt.Errorf("open database: %w", err)
	}
	defer st.Close()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := bootstrapAdmin(ctx, st, log, cfg.adminUser, cfg.adminPassword); err != nil {
		return err
	}

	mgr := rabbitx.NewManager(st, rabbitx.Options{PollInterval: poll}, log)
	if err := mgr.Start(ctx); err != nil {
		return err
	}
	defer mgr.Stop()

	ui := web.FS()
	if ui == nil {
		log.Warn("UI not built; serving the API only (run `make ui`)")
	}
	h := api.New(api.Config{SecureCookies: cfg.secure, AllowSetup: cfg.allowSetup}, st, mgr, ui, log)
	srv := &http.Server{Addr: addr, Handler: h, ReadHeaderTimeout: 10 * time.Second}

	go func() {
		t := time.NewTicker(time.Hour)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = st.PruneSessions(ctx)
			}
		}
	}()

	errc := make(chan error, 1)
	go func() {
		log.Info("rabmin listening", "addr", addr, "version", version.Version, "tls", tlsCert != "")
		if tlsCert != "" {
			errc <- srv.ListenAndServeTLS(tlsCert, tlsKey)
		} else {
			errc <- srv.ListenAndServe()
		}
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
		log.Info("shutting down")
		sctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		_ = srv.Shutdown(sctx)
	}
	return nil
}

// bootstrapAdmin creates the admin from RABMIN_ADMIN_USER/PASSWORD when no
// users exist yet.
func bootstrapAdmin(ctx context.Context, st *store.Store, log *slog.Logger, user, pass string) error {
	n, err := st.CountUsers(ctx)
	if err != nil || n > 0 {
		return err
	}
	if user == "" || pass == "" {
		log.Info("no users yet: open the UI to create the first admin, or set --admin-user and --admin-password (RABMIN_ADMIN_USER, RABMIN_ADMIN_PASSWORD)")
		return nil
	}
	hash, err := auth.HashPassword(pass)
	if err != nil {
		return err
	}
	if _, err := st.CreateUser(ctx, user, hash, store.RoleAdmin); err != nil {
		return err
	}
	log.Info("created admin user from environment", "user", user)
	return nil
}

func dumpOpenAPI() {
	h := api.New(api.Config{}, nil, nil, nil, nil)
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/openapi.json", nil))
	var v any
	if err := json.Unmarshal(rec.Body.Bytes(), &v); err != nil {
		fmt.Fprintln(os.Stderr, "openapi:", err, rec.Body.String())
		os.Exit(1)
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(v)
}
