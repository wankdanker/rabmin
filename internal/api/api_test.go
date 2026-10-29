package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wankdanker/rabmin/internal/api"
	"github.com/wankdanker/rabmin/internal/secret"
	"github.com/wankdanker/rabmin/internal/store"
)

type client struct {
	t     *testing.T
	base  string
	hc    *http.Client
	token string
}

func newClient(t *testing.T, base string) *client {
	jar, _ := cookiejar.New(nil)
	return &client{t: t, base: base, hc: &http.Client{Jar: jar, Timeout: 30 * time.Second}}
}

func (c *client) do(method, path string, body any, out any) int {
	c.t.Helper()
	var r io.Reader
	ct := "application/json"
	switch b := body.(type) {
	case nil:
	case []byte:
		r = bytes.NewReader(b)
		ct = "application/octet-stream"
	case string:
		r = strings.NewReader(b)
	default:
		data, _ := json.Marshal(b)
		r = bytes.NewReader(data)
	}
	req, _ := http.NewRequest(method, c.base+path, r)
	if r != nil {
		req.Header.Set("Content-Type", ct)
	}
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		c.t.Fatalf("%s %s: %v", method, path, err)
	}
	defer resp.Body.Close()
	data, _ := io.ReadAll(resp.Body)
	if out != nil && resp.StatusCode < 300 && len(data) > 0 {
		if raw, ok := out.(*[]byte); ok {
			*raw = data
		} else if err := json.Unmarshal(data, out); err != nil {
			c.t.Fatalf("%s %s: decode %v: %s", method, path, err, data)
		}
	}
	if resp.StatusCode >= 400 && out != nil {
		c.t.Logf("%s %s -> %d %s", method, path, resp.StatusCode, data)
	}
	return resp.StatusCode
}

func (c *client) must(method, path string, body any, out any, want int) {
	c.t.Helper()
	if got := c.do(method, path, body, out); got != want {
		c.t.Fatalf("%s %s: want %d, got %d", method, path, want, got)
	}
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	box, _ := secret.New(make([]byte, 32))
	st, err := store.Open(filepath.Join(t.TempDir(), "db.sqlite"), box)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	srv := httptest.NewServer(api.New(api.Config{AllowSetup: true}, st, nil, nil))
	t.Cleanup(srv.Close)
	return srv
}

func TestAuthUsersTokensAudit(t *testing.T) {
	srv := newServer(t)
	admin := newClient(t, srv.URL+"/api")

	// Setup and auth.
	var status api.StatusBody
	admin.must("GET", "/v1/status", nil, &status, 200)
	if !status.NeedsSetup {
		t.Fatal("expected setup to be needed")
	}
	admin.must("GET", "/v1/users", nil, nil, 401)
	admin.must("POST", "/v1/setup", map[string]string{"username": "admin", "password": "supersecret"}, nil, 200)
	admin.must("POST", "/v1/setup", map[string]string{"username": "evil", "password": "supersecret"}, nil, 409)
	var me api.MeBody
	admin.must("GET", "/v1/auth/me", nil, &me, 200)
	if me.Role != store.RoleAdmin {
		t.Fatalf("want admin, got %s", me.Role)
	}

	// RBAC: a viewer token can read but not write.
	admin.must("POST", "/v1/users", map[string]any{"username": "vera", "password": "viewerpass", "role": "viewer"}, nil, 201)
	viewer := newClient(t, srv.URL+"/api")
	viewer.must("POST", "/v1/auth/login", map[string]string{"username": "vera", "password": "wrong"}, nil, 401)
	viewer.must("POST", "/v1/auth/login", map[string]string{"username": "vera", "password": "viewerpass"}, nil, 200)
	var tok struct {
		Token string `json:"token"`
		Role  string `json:"role"`
	}
	viewer.must("POST", "/v1/tokens", map[string]any{"name": "ci", "role": "admin"}, &tok, 201)
	if tok.Role != "viewer" || !strings.HasPrefix(tok.Token, "rm_") {
		t.Fatalf("token role must be capped at viewer: %+v", tok)
	}
	bot := newClient(t, srv.URL+"/api")
	bot.token = tok.Token
	bot.must("GET", "/v1/auth/me", nil, nil, 200)
	bot.must("GET", "/v1/users", nil, nil, 403)
	bot.must("POST", "/v1/tokens", map[string]any{"name": "escalate"}, nil, 403)
	bot.token = "rm_invalid"
	bot.must("GET", "/v1/auth/me", nil, nil, 401)

	// The last admin cannot be removed.
	admin.must("PATCH", fmt.Sprintf("/v1/users/%d", me.User.ID), map[string]any{"role": "viewer"}, nil, 409)

	var audit []store.AuditEntry
	admin.must("GET", "/v1/audit?limit=500", nil, &audit, 200)
	actions := map[string]bool{}
	for _, e := range audit {
		actions[e.Action] = true
	}
	for _, want := range []string{"setup", "create-user", "create-token", "login"} {
		if !actions[want] {
			t.Errorf("audit log is missing %q", want)
		}
	}
	viewer.must("GET", "/v1/audit", nil, nil, 403)
}
