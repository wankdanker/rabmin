// Package api is rabmin's HTTP API plane. Every capability of the UI is an
// operation here, described by the OpenAPI document at /api/openapi.json.
package api

import (
	"io/fs"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humago"

	"github.com/wankdanker/rabmin/internal/store"
	"github.com/wankdanker/rabmin/internal/version"
)

// Config holds API settings.
type Config struct {
	// SessionTTL is how long a login lasts.
	SessionTTL time.Duration
	// SecureCookies forces the Secure flag on the session cookie.
	SecureCookies bool
	// AllowSetup lets the first visitor create the initial admin when no
	// users exist.
	AllowSetup bool
}

func init() {
	// Handlers always return empty slices rather than nil, so arrays are
	// documented as non-nullable.
	huma.DefaultArrayNullable = false
}

// Server wires the API to its dependencies.
type Server struct {
	cfg     Config
	store   *store.Store
	log     *slog.Logger
	api     huma.API
	limiter *loginLimiter
}

// New builds the HTTP handler: the API under /api and the UI (if ui is
// non-nil) everywhere else.
func New(cfg Config, st *store.Store, ui fs.FS, log *slog.Logger) http.Handler {
	if cfg.SessionTTL <= 0 {
		cfg.SessionTTL = 7 * 24 * time.Hour
	}
	s := &Server{cfg: cfg, store: st, log: log, limiter: newLoginLimiter()}

	mux := http.NewServeMux()
	hcfg := huma.DefaultConfig("rabmin", version.Version)
	hcfg.CreateHooks = nil
	hcfg.OpenAPIPath = "/openapi"
	hcfg.DocsPath = "/docs"
	hcfg.Info.Description = "rabmin API plane: manage and monitor RabbitMQ clusters.\n\n" +
		"Authenticate with a session cookie (POST /api/v1/auth/login) or an API token sent as `Authorization: Bearer rm_...`."
	hcfg.Info.License = &huma.License{Name: "Apache-2.0", URL: "https://www.apache.org/licenses/LICENSE-2.0"}
	hcfg.Servers = []*huma.Server{{URL: "/api"}}
	hcfg.Components.SecuritySchemes = map[string]*huma.SecurityScheme{
		"bearer":  {Type: "http", Scheme: "bearer", Description: "API token (rm_...)"},
		"session": {Type: "apiKey", In: "cookie", Name: sessionCookie},
	}
	hcfg.Security = []map[string][]string{{"bearer": {}}, {"session": {}}}

	s.api = humago.NewWithPrefix(mux, "/api", hcfg)
	s.api.UseMiddleware(s.authMiddleware)

	s.registerAuth()
	s.registerUsers()
	s.registerAudit()

	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, _ = w.Write([]byte("ok\n"))
	})
	if ui != nil {
		mux.Handle("/", spaHandler(ui))
	}
	return securityHeaders(mux)
}

// API exposes the huma API, e.g. to dump the OpenAPI document.
func (s *Server) API() huma.API { return s.api }

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := w.Header()
		h.Set("X-Content-Type-Options", "nosniff")
		h.Set("Referrer-Policy", "same-origin")
		if !strings.HasPrefix(r.URL.Path, "/api/docs") {
			h.Set("X-Frame-Options", "DENY")
		}
		next.ServeHTTP(w, r)
	})
}

// spaHandler serves the built UI, falling back to index.html for client routes.
func spaHandler(ui fs.FS) http.Handler {
	files := http.FileServerFS(ui)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := strings.TrimPrefix(r.URL.Path, "/")
		if p == "" {
			p = "index.html"
		}
		if f, err := ui.Open(p); err == nil {
			st, _ := f.Stat()
			f.Close()
			if st != nil && !st.IsDir() {
				if strings.HasPrefix(p, "_app/immutable/") {
					w.Header().Set("Cache-Control", "public, max-age=31536000, immutable")
				}
				files.ServeHTTP(w, r)
				return
			}
		}
		if strings.HasPrefix(r.URL.Path, "/api/") {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-cache")
		r2 := r.Clone(r.Context())
		r2.URL.Path = "/"
		files.ServeHTTP(w, r2)
	})
}
