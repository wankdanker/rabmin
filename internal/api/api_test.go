package api_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/http/httptest"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/wankdanker/rabmin/internal/api"
	"github.com/wankdanker/rabmin/internal/devcluster"
	"github.com/wankdanker/rabmin/internal/rabbitx"
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
	mgr := rabbitx.NewManager(st, rabbitx.Options{PollInterval: time.Second, HealthInterval: time.Second}, nil)
	t.Cleanup(mgr.Stop)
	srv := httptest.NewServer(api.New(api.Config{AllowSetup: true}, st, mgr, nil, nil))
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

func TestObserveCluster(t *testing.T) {
	if testing.Short() || !devcluster.DockerAvailable() {
		t.Skip("needs Docker")
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	env, err := devcluster.Start(ctx, devcluster.Config{Prefix: "rabmin-test-api", Dir: t.TempDir(),
		Clusters: []devcluster.ClusterSpec{{Name: "east", Nodes: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Shutdown()
	if err := env.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	go env.Traffic(ctx, nil)

	srv := newServer(t)
	admin := newClient(t, srv.URL+"/api")
	admin.must("POST", "/v1/setup", map[string]string{"username": "admin", "password": "supersecret"}, nil, 200)

	var mgmt, prom []string
	for _, n := range env.Nodes {
		mgmt = append(mgmt, n.ManagementURL)
		prom = append(prom, n.PrometheusURL)
	}
	user, pass := env.User()
	auth := map[string]string{"type": "basic", "username": user, "password": pass}

	// Connection test: versions, plugins, tags; the monitoring user is not an administrator.
	var test api.TestResult
	admin.must("POST", "/v1/clusters/test", map[string]any{"managementUrls": mgmt, "prometheusUrls": prom, "auth": auth}, &test, 200)
	if !test.OK || test.Nodes != 3 || !test.IsAdministrator || test.ClusterName != "east" || !strings.HasPrefix(test.RabbitMQVersion, "4.") {
		t.Fatalf("unexpected test result: %+v", test)
	}
	for _, e := range test.Endpoints {
		if !e.OK || e.Node == "" {
			t.Fatalf("endpoint not reached: %+v", e)
		}
	}
	if len(test.Plugins) == 0 || len(test.Prometheus) != 3 || !test.Prometheus[0].OK {
		t.Fatalf("plugins or prometheus missing: %+v", test)
	}
	admin.must("POST", "/v1/clusters/test", map[string]any{"managementUrls": mgmt[:1],
		"auth": map[string]string{"type": "basic", "username": "monitor", "password": "monitor"}}, &test, 200)
	if !test.OK || test.IsAdministrator || !test.CanMonitor {
		t.Fatalf("monitoring user misreported: %+v", test)
	}
	admin.must("POST", "/v1/clusters/test", map[string]any{"managementUrls": mgmt[:1],
		"auth": map[string]string{"type": "basic", "username": user, "password": "wrong"}}, &test, 200)
	if test.OK || !strings.Contains(test.Error, "rejected") {
		t.Fatalf("bad credentials not reported: %+v", test)
	}

	// Register the cluster.
	var c api.ClusterOut
	admin.must("POST", "/v1/clusters", map[string]any{"name": "East", "managementUrls": mgmt, "prometheusUrls": prom, "auth": auth,
		"credentials": []map[string]any{{"name": "Monitor", "auth": map[string]string{"type": "basic", "username": "monitor", "password": "monitor"}}}}, &c, 201)
	if c.ID != "east" || len(c.Credentials) != 1 || !c.Auth.HasPassword || !c.Status.Connected {
		t.Fatalf("unexpected cluster: %+v", c)
	}
	raw, _ := json.Marshal(c)
	if strings.Contains(string(raw), `"password"`) {
		t.Fatal("password leaked in response")
	}
	admin.must("POST", "/v1/clusters", map[string]any{"name": "Bad", "managementUrls": []string{"ftp://x"}, "auth": auth}, nil, 422)

	// Snapshot: every node healthy, health matrix complete, rates flowing.
	var snap rabbitx.Snapshot
	deadline := time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		admin.must("GET", "/v1/clusters/east/snapshot?refresh=true", nil, &snap, 200)
		if snap.Totals.NodesRunning == 3 && snap.Rates.Publish > 0 && snap.Totals.Consumers >= 5 {
			break
		}
		time.Sleep(time.Second)
	}
	if snap.Totals.NodesRunning != 3 || snap.Rates.Publish <= 0 || snap.RatesSource != "management" {
		t.Fatalf("unexpected snapshot: totals=%+v rates=%+v source=%s err=%s", snap.Totals, snap.Rates, snap.RatesSource, snap.Error)
	}
	for _, n := range snap.Nodes {
		if !n.Healthy || n.Endpoint == "" || n.MemUsed == 0 {
			t.Fatalf("node not healthy or unmapped: %+v", n)
		}
	}
	if len(snap.Health) != 3*len(snap.HealthChecks) || len(snap.HealthChecks) < 7 {
		t.Fatalf("health matrix incomplete: %d results for %v", len(snap.Health), snap.HealthChecks)
	}
	if len(snap.Prometheus) != 3 || len(snap.Listeners) == 0 || len(snap.Queues) == 0 {
		t.Fatalf("snapshot missing parts: prom=%d listeners=%d queues=%d", len(snap.Prometheus), len(snap.Listeners), len(snap.Queues))
	}
	var hist []rabbitx.RatePoint
	admin.must("GET", "/v1/clusters/east/history", nil, &hist, 200)
	if len(hist) == 0 {
		t.Fatal("no history")
	}

	// Connections, channels, consumers, once every demo client is up.
	var conns api.ConnectionsPage
	deadline = time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		admin.must("GET", "/v1/clusters/east/connections?q=rabmin-devcluster", nil, &conns, 200)
		if conns.Filtered >= 12 {
			break
		}
		time.Sleep(time.Second)
	}
	admin.must("GET", "/v1/clusters/east/connections?q=orders-api", nil, &conns, 200)
	if conns.Filtered != 1 || conns.Items[0].ClientName != "orders-api" || conns.Items[0].VHost != "shop" {
		t.Fatalf("connection filter: %+v", conns)
	}
	admin.must("GET", "/v1/clusters/east/connections?sort=clientName&pageSize=2&page=2", nil, &conns, 200)
	if len(conns.Items) != 2 || conns.Total < 4 || conns.Items[0].ClientName > conns.Items[1].ClientName {
		t.Fatalf("connection paging: %+v", conns)
	}
	// Clients connect before they set basic.qos and start consuming, so poll.
	var detail rabbitx.ConnectionDetail
	eventually(t, func() bool {
		admin.must("GET", "/v1/clusters/east/connections?q=billing-worker", nil, &conns, 200)
		if conns.Filtered != 1 {
			return false
		}
		admin.must("GET", "/v1/clusters/east/connections/"+url.PathEscape(conns.Items[0].Name), nil, &detail, 200)
		return len(detail.Channels) == 1 && detail.Channels[0].Prefetch == 5 && strings.Contains(string(detail.Properties), "capabilities")
	}, "connection detail: %+v", &detail)
	var chans api.ChannelsPage
	eventually(t, func() bool {
		admin.must("GET", "/v1/clusters/east/channels?q=order-processor", nil, &chans, 200)
		return chans.Filtered == 1 && chans.Items[0].Prefetch == 20 && chans.Items[0].Consumers == 1
	}, "channels: %+v", &chans)
	var chd rabbitx.ChannelDetail
	admin.must("GET", "/v1/clusters/east/channels/"+url.PathEscape(chans.Items[0].Name), nil, &chd, 200)
	if len(chd.Consumers) != 1 || chd.Consumers[0].Queue != "orders.created" {
		t.Fatalf("channel detail: %+v", chd)
	}
	var cons api.ConsumersPage
	eventually(t, func() bool {
		admin.must("GET", "/v1/clusters/east/consumers?queue=tasks", nil, &cons, 200)
		return cons.Filtered == 2 && cons.Items[0].ClientName != ""
	}, "consumers: %+v", &cons)
	var nd rabbitx.NodeDetail
	admin.must("GET", "/v1/clusters/east/nodes/"+url.PathEscape(env.Nodes[1].Name), nil, &nd, 200)
	if nd.MemoryTotal == 0 || len(nd.Memory) < 10 || !nd.Node.Running {
		t.Fatalf("node detail: %+v", nd.Node)
	}
	admin.must("POST", "/v1/clusters/east/nodes/"+url.PathEscape(env.Nodes[1].Name)+"/reset-stats", nil, nil, 204)

	// RBAC: a viewer cannot close connections.
	admin.must("POST", "/v1/users", map[string]any{"username": "vera", "password": "viewerpass", "role": "viewer"}, nil, 201)
	viewer := newClient(t, srv.URL+"/api")
	viewer.must("POST", "/v1/auth/login", map[string]string{"username": "vera", "password": "viewerpass"}, nil, 200)
	viewer.must("GET", "/v1/clusters/east/connections", nil, nil, 200)
	viewer.must("POST", "/v1/clusters/east/connections/close", map[string]any{"clientName": "task-worker-1"}, nil, 403)
	var closed struct{ Closed int }
	admin.must("POST", "/v1/clusters/east/connections/close", map[string]any{"clientName": "task-worker-1", "reason": "test"}, &closed, 200)
	if closed.Closed != 1 {
		t.Fatalf("want 1 closed, got %d", closed.Closed)
	}

	// Broker credential failures must not look like a rabmin logout.
	admin.must("POST", "/v1/clusters", map[string]any{"name": "Wrong", "managementUrls": mgmt[:1], "auth": map[string]string{"type": "basic", "username": user, "password": "nope"}}, nil, 201)
	admin.must("GET", "/v1/clusters/wrong/connections", nil, nil, 502)

	// Failover: stop the first node; the cluster stays observable through the others.
	if err := env.Stop(ctx, env.Nodes[0]); err != nil {
		t.Fatal(err)
	}
	deadline = time.Now().Add(60 * time.Second)
	for time.Now().Before(deadline) {
		admin.must("GET", "/v1/clusters/east/snapshot?refresh=true", nil, &snap, 200)
		if snap.Connected && snap.Totals.NodesRunning == 2 {
			break
		}
		time.Sleep(time.Second)
	}
	if !snap.Connected || snap.Totals.NodesRunning != 2 {
		t.Fatalf("failover did not work: connected=%v running=%d err=%s", snap.Connected, snap.Totals.NodesRunning, snap.Error)
	}
	kinds := map[string]bool{}
	for _, a := range snap.Attention {
		kinds[a.Kind] = true
	}
	if !kinds["node-down"] || !kinds["endpoint"] {
		t.Fatalf("missing attention items: %+v", snap.Attention)
	}
	critical := false
	for _, h := range snap.Health {
		if h.Check == "node-is-quorum-critical" && h.Status == "failed" {
			critical = true
		}
	}
	if !critical {
		t.Errorf("expected node-is-quorum-critical to fail with a node down: %+v", snap.Health)
	}

	var audit []store.AuditEntry
	admin.must("GET", "/v1/audit?limit=500", nil, &audit, 200)
	actions := map[string]bool{}
	for _, e := range audit {
		actions[e.Action] = true
	}
	for _, want := range []string{"create-cluster", "close-connections", "reset-node-stats"} {
		if !actions[want] {
			t.Errorf("audit log is missing %q", want)
		}
	}
}

// eventually retries cond for up to 30 seconds, then fails with msg and v.
func eventually(t *testing.T, cond func() bool, msg string, v any) {
	t.Helper()
	deadline := time.Now().Add(30 * time.Second)
	for !cond() {
		if time.Now().After(deadline) {
			t.Fatalf(msg, v)
		}
		time.Sleep(500 * time.Millisecond)
	}
}
