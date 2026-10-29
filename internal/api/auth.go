package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/wankdanker/rabmin/internal/auth"
	"github.com/wankdanker/rabmin/internal/store"
	"github.com/wankdanker/rabmin/internal/version"
)

var tagAuth = []string{"Auth"}

// MeBody describes the current caller.
type MeBody struct {
	User       store.User `json:"user"`
	Role       store.Role `json:"role" doc:"Effective role (an API token may be limited below its user's role)"`
	AuthMethod string     `json:"authMethod" enum:"session,token"`
}

// StatusBody is the public server status used by the login screen.
type StatusBody struct {
	Version       string `json:"version"`
	NeedsSetup    bool   `json:"needsSetup" doc:"True when no users exist and the first admin can be created with POST /v1/setup"`
	Authenticated bool   `json:"authenticated"`
}

func (s *Server) registerAuth() {
	reg(s, route{method: http.MethodGet, path: "/v1/status", id: "get-status", summary: "Server status", tags: tagAuth},
		func(ctx context.Context, _ *struct{}) (*struct{ Body StatusBody }, error) {
			n, err := s.store.CountUsers(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body StatusBody }{StatusBody{Version: version.Version, NeedsSetup: n == 0 && s.cfg.AllowSetup, Authenticated: principal(ctx) != nil}}, nil
		})

	type credsBody struct {
		Username string `json:"username" minLength:"1" maxLength:"64"`
		Password string `json:"password" minLength:"1" maxLength:"256"`
	}

	reg(s, route{method: http.MethodPost, path: "/v1/setup", id: "setup", summary: "Create the first admin",
		desc: "Only works while no users exist. Logs the new admin in.", tags: tagAuth},
		func(ctx context.Context, in *struct{ Body credsBody }) (*sessionOut, error) {
			if !s.cfg.AllowSetup {
				return nil, huma.Error403Forbidden("setup is disabled; create the admin with RABMIN_ADMIN_USER/RABMIN_ADMIN_PASSWORD")
			}
			n, err := s.store.CountUsers(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			if n > 0 {
				return nil, huma.Error409Conflict("setup already completed")
			}
			if len(in.Body.Password) < 8 {
				return nil, huma.Error422UnprocessableEntity("password must be at least 8 characters")
			}
			hash, err := auth.HashPassword(in.Body.Password)
			if err != nil {
				return nil, apiErr(err)
			}
			u, err := s.store.CreateFirstUser(ctx, in.Body.Username, hash, store.RoleAdmin)
			if errors.Is(err, store.ErrConflict) {
				return nil, huma.Error409Conflict("setup already completed")
			}
			if err != nil {
				return nil, apiErr(err)
			}
			s.audit(ctx, "setup", "user/"+u.Username, "created first admin", nil)
			return s.startSession(ctx, u)
		})

	reg(s, route{method: http.MethodPost, path: "/v1/auth/login", id: "login", summary: "Log in", tags: tagAuth},
		func(ctx context.Context, in *struct{ Body credsBody }) (*sessionOut, error) {
			key := remoteAddr(ctx) + "|" + in.Body.Username
			if s.limiter.blocked(key) {
				return nil, huma.NewError(http.StatusTooManyRequests, "too many failed logins, try again in a few minutes")
			}
			u, err := s.store.GetUserByUsername(ctx, in.Body.Username)
			if err != nil && !errors.Is(err, store.ErrNotFound) {
				return nil, apiErr(err)
			}
			if u == nil || u.Disabled || !auth.CheckPassword(u.PasswordHash, in.Body.Password) {
				if u == nil {
					auth.CheckPassword(dummyHash, in.Body.Password)
				}
				s.limiter.failed(key)
				s.audit(ctx, "login", "user/"+in.Body.Username, "", errors.New("invalid credentials"))
				return nil, huma.Error401Unauthorized("invalid username or password")
			}
			s.limiter.reset(key)
			s.audit(ctx, "login", "user/"+u.Username, "", nil)
			return s.startSession(ctx, u)
		})

	reg(s, route{method: http.MethodPost, path: "/v1/auth/logout", id: "logout", summary: "Log out", tags: tagAuth, status: http.StatusNoContent, noAudit: true},
		func(ctx context.Context, in *struct {
			Session string `cookie:"rabmin_session"`
		}) (*struct {
			SetCookie http.Cookie `header:"Set-Cookie"`
		}, error) {
			if in.Session != "" {
				_ = s.store.DeleteSession(ctx, auth.HashToken(in.Session))
			}
			return &struct {
				SetCookie http.Cookie `header:"Set-Cookie"`
			}{http.Cookie{Name: sessionCookie, Value: "", Path: "/", MaxAge: -1, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.cfg.SecureCookies}}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/auth/me", id: "get-me", summary: "Current user", tags: tagAuth, role: store.RoleViewer},
		func(ctx context.Context, _ *struct{}) (*struct{ Body MeBody }, error) {
			p := principal(ctx)
			return &struct{ Body MeBody }{MeBody{User: *p.User, Role: p.Role, AuthMethod: p.Method}}, nil
		})

	reg(s, route{method: http.MethodPut, path: "/v1/auth/password", id: "change-password", summary: "Change your password", tags: tagAuth, role: store.RoleViewer, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Body struct {
				Current string `json:"current"`
				New     string `json:"new" minLength:"8" maxLength:"256"`
			}
		}) (*struct{}, error) {
			p := principal(ctx)
			if p.Method != "session" {
				return nil, huma.Error403Forbidden("passwords can only be changed from a login session")
			}
			if !auth.CheckPassword(p.User.PasswordHash, in.Body.Current) {
				return nil, huma.Error401Unauthorized("current password is wrong")
			}
			hash, err := auth.HashPassword(in.Body.New)
			if err != nil {
				return nil, apiErr(err)
			}
			u := *p.User
			u.PasswordHash = hash
			return nil, apiErr(s.store.UpdateUser(ctx, &u))
		})
}

type sessionOut struct {
	SetCookie http.Cookie `header:"Set-Cookie"`
	Body      MeBody
}

// dummyHash is compared against when a login names an unknown user, so the
// response takes as long as a real password check.
var dummyHash, _ = auth.HashPassword(auth.RandomToken(16))

func (s *Server) startSession(ctx context.Context, u *store.User) (*sessionOut, error) {
	tok := auth.RandomToken(32)
	exp := time.Now().Add(s.cfg.SessionTTL)
	if err := s.store.CreateSession(ctx, auth.HashToken(tok), u.ID, exp); err != nil {
		return nil, apiErr(err)
	}
	return &sessionOut{
		SetCookie: http.Cookie{Name: sessionCookie, Value: tok, Path: "/", Expires: exp, HttpOnly: true, SameSite: http.SameSiteStrictMode, Secure: s.cfg.SecureCookies},
		Body:      MeBody{User: *u, Role: u.Role, AuthMethod: "session"},
	}, nil
}
