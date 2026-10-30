package api

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/wankdanker/rabmin/internal/auth"
	"github.com/wankdanker/rabmin/internal/rabbitx"
	"github.com/wankdanker/rabmin/internal/store"
)

const sessionCookie = "rabmin_session"

type ctxKey int

const (
	principalKey ctxKey = iota
	remoteKey
)

// Principal is the authenticated caller.
type Principal struct {
	User   *store.User
	Role   store.Role
	Method string
}

func principal(ctx context.Context) *Principal {
	p, _ := ctx.Value(principalKey).(*Principal)
	return p
}

func remoteAddr(ctx context.Context) string {
	r, _ := ctx.Value(remoteKey).(string)
	return r
}

func actor(ctx context.Context) string {
	if p := principal(ctx); p != nil {
		return p.User.Username
	}
	return "anonymous"
}

// route describes one operation.
type route struct {
	method  string
	path    string
	id      string
	summary string
	desc    string
	tags    []string
	// role is the minimum role required; empty means public.
	role store.Role
	// audit forces auditing of a read operation, or (with noAudit) skips a write.
	noAudit bool
	status  int
	// maxBody overrides the request size limit.
	maxBody int64
}

func reg[I, O any](s *Server, r route, h func(context.Context, *I) (*O, error)) {
	op := huma.Operation{
		OperationID:   r.id,
		Method:        r.method,
		Path:          r.path,
		Summary:       r.summary,
		Description:   r.desc,
		Tags:          r.tags,
		DefaultStatus: r.status,
		MaxBodyBytes:  r.maxBody,
		Metadata:      map[string]any{"role": r.role, "noAudit": r.noAudit},
	}
	if r.role == "" {
		op.Security = []map[string][]string{}
	} else {
		op.Description = strings.TrimSpace(op.Description + "\n\nRequires role: **" + string(r.role) + "**.")
	}
	huma.Register(s.api, op, h)
}

func (s *Server) authMiddleware(ctx huma.Context, next func(huma.Context)) {
	op := ctx.Operation()
	need, _ := op.Metadata["role"].(store.Role)
	remote := ctx.RemoteAddr()
	if host, _, err := net.SplitHostPort(remote); err == nil {
		remote = host
	}
	ctx = huma.WithValue(ctx, remoteKey, remote)

	p, err := s.resolvePrincipal(ctx)
	if err != nil {
		_ = huma.WriteErr(s.api, ctx, http.StatusInternalServerError, "auth lookup failed", err)
		return
	}
	if p != nil {
		ctx = huma.WithValue(ctx, principalKey, p)
	}
	if need != "" {
		if p == nil {
			_ = huma.WriteErr(s.api, ctx, http.StatusUnauthorized, "authentication required")
			return
		}
		if !p.Role.Allows(need) {
			_ = huma.WriteErr(s.api, ctx, http.StatusForbidden, fmt.Sprintf("requires role %s, you have %s", need, p.Role))
			return
		}
	}

	next(ctx)

	noAudit, _ := op.Metadata["noAudit"].(bool)
	if op.Method != http.MethodGet && !noAudit && need != "" {
		status := ctx.Status()
		u := ctx.URL()
		e := store.AuditEntry{Actor: actor(ctx.Context()), Action: op.OperationID, Target: u.EscapedPath(),
			OK: status < 400, Remote: remote}
		if status >= 400 {
			e.Error = http.StatusText(status)
		}
		if s.store != nil {
			_ = s.store.AddAudit(context.Background(), e)
		}
	}
}

func (s *Server) resolvePrincipal(ctx huma.Context) (*Principal, error) {
	if s.store == nil {
		return nil, nil
	}
	if h := ctx.Header("Authorization"); h != "" {
		tok, ok := strings.CutPrefix(h, "Bearer ")
		if !ok {
			return nil, nil
		}
		u, role, err := s.store.TokenUser(ctx.Context(), auth.HashToken(strings.TrimSpace(tok)))
		if errors.Is(err, store.ErrNotFound) {
			return nil, nil
		}
		if err != nil {
			return nil, err
		}
		return &Principal{User: u, Role: role, Method: "token"}, nil
	}
	c, err := huma.ReadCookie(ctx, sessionCookie)
	if err != nil || c.Value == "" {
		return nil, nil
	}
	u, err := s.store.SessionUser(ctx.Context(), auth.HashToken(c.Value))
	if errors.Is(err, store.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &Principal{User: u, Role: u.Role, Method: "session"}, nil
}

// audit records an extra audit entry with detail, for operations where the
// path alone does not tell the story.
func (s *Server) audit(ctx context.Context, action, target, detail string, err error) {
	e := store.AuditEntry{Actor: actor(ctx), Action: action, Target: target, Detail: detail, OK: err == nil, Remote: remoteAddr(ctx)}
	if err != nil {
		e.Error = err.Error()
	}
	_ = s.store.AddAudit(context.Background(), e)
}

// apiErr maps internal errors to HTTP errors.
func apiErr(err error) error {
	if err == nil {
		return nil
	}
	var se huma.StatusError
	if errors.As(err, &se) {
		return err
	}
	var ae *rabbitx.APIError
	if errors.As(err, &ae) {
		// Never pass a broker 401 through: the UI logs out on 401.
		switch {
		case ae.Unauthenticated():
			return huma.NewError(http.StatusBadGateway, "RabbitMQ rejected rabmin's stored credentials for this cluster")
		case ae.Forbidden():
			return huma.Error403Forbidden("RabbitMQ refused the operation for rabmin's user: " + ae.Reason)
		case ae.Status == http.StatusNotFound:
			if ae.Reason != "" && ae.Reason != "Not Found" {
				return huma.Error404NotFound("not found in RabbitMQ: " + ae.Reason)
			}
			return huma.Error404NotFound("not found in RabbitMQ")
		case ae.Status == http.StatusBadRequest && (strings.HasSuffix(ae.Reason, " not empty") || strings.HasSuffix(ae.Reason, " in use")):
			// if-empty / if-unused refusals.
			return huma.Error409Conflict("RabbitMQ: " + ae.Reason)
		case ae.Status == http.StatusBadRequest:
			return huma.Error400BadRequest("RabbitMQ: " + ae.Reason)
		case ae.Status == http.StatusConflict, ae.Status == http.StatusPreconditionFailed:
			return huma.Error409Conflict("RabbitMQ: " + ae.Reason)
		}
		return huma.NewError(http.StatusBadGateway, ae.Error())
	}
	switch {
	case errors.Is(err, rabbitx.ErrUserNotInDefinitions):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, rabbitx.ErrUnreachable):
		return huma.NewError(http.StatusBadGateway, err.Error())
	case errors.Is(err, store.ErrNotFound):
		return huma.Error404NotFound(err.Error())
	case errors.Is(err, store.ErrConflict):
		return huma.Error409Conflict(err.Error())
	case errors.Is(err, context.DeadlineExceeded):
		return huma.NewError(http.StatusGatewayTimeout, "request timed out: "+err.Error())
	}
	return huma.Error500InternalServerError(err.Error())
}

// loginLimiter slows down password guessing per remote address and username.
type loginLimiter struct {
	mu   sync.Mutex
	fail map[string][]time.Time
}

func newLoginLimiter() *loginLimiter { return &loginLimiter{fail: map[string][]time.Time{}} }

const (
	loginWindow   = 5 * time.Minute
	loginMaxFails = 8
)

func (l *loginLimiter) blocked(key string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	cut := time.Now().Add(-loginWindow)
	kept := l.fail[key][:0]
	for _, t := range l.fail[key] {
		if t.After(cut) {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(l.fail, key)
		return false
	}
	l.fail[key] = kept
	return len(kept) >= loginMaxFails
}

func (l *loginLimiter) failed(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	now := time.Now()
	if len(l.fail) > 10000 {
		// Drop keys whose failures are all outside the window.
		cut := now.Add(-loginWindow)
		for k, ts := range l.fail {
			if len(ts) == 0 || ts[len(ts)-1].Before(cut) {
				delete(l.fail, k)
			}
		}
	}
	l.fail[key] = append(l.fail[key], now)
}

func (l *loginLimiter) reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.fail, key)
}

// Empty is a body-less response.
type Empty struct{}

// RawBody carries native RabbitMQ JSON.
type RawBody struct {
	Body json.RawMessage
}

func isNotFound(err error) bool {
	var ae *rabbitx.APIError
	return errors.As(err, &ae) && ae.Status == http.StatusNotFound
}
