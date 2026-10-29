package rabbitx

import (
	"context"
	"encoding/json"
	"errors"
	"log/slog"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/wankdanker/rabmin/internal/store"
)

// Options configure a Manager.
type Options struct {
	PollInterval   time.Duration
	HealthInterval time.Duration
	HistoryPoints  int
	RequestTimeout time.Duration
}

// Manager owns the management API clients of every registered cluster and
// keeps the latest snapshot and history of each.
type Manager struct {
	store *store.Store
	opts  Options
	log   *slog.Logger

	mu       sync.Mutex
	clusters map[string]*clusterState
	ctx      context.Context
	cancel   context.CancelFunc
	wg       sync.WaitGroup
}

type clusterState struct {
	mu         sync.Mutex
	cluster    store.Cluster
	client     *Client
	clientErr  error
	prom       *http.Client
	snapshot   *Snapshot
	history    []RatePoint
	subs       map[chan *Snapshot]struct{}
	cancel     context.CancelFunc
	promPrev   *promTotals
	health     []HealthResult
	checks     []string
	healthAt   time.Time
	deprecated []string
	pollMu     sync.Mutex
}

// HealthChecks are the node-local checks run through every management URL.
// protocol-listeners is expanded to the protocols each node listens on.
var HealthChecks = []string{
	"local-alarms", "alarms", "virtual-hosts", "is-in-service", "ready-to-serve-clients",
	"below-node-connection-limit", "node-is-quorum-critical", "certificate-expiration", "protocol-listeners",
}

// NewManager creates a Manager.
func NewManager(st *store.Store, opts Options, log *slog.Logger) *Manager {
	if opts.PollInterval <= 0 {
		opts.PollInterval = 5 * time.Second
	}
	if opts.HealthInterval <= 0 {
		opts.HealthInterval = 3 * opts.PollInterval
	}
	if opts.HistoryPoints <= 0 {
		opts.HistoryPoints = 720
	}
	if opts.RequestTimeout <= 0 {
		opts.RequestTimeout = 10 * time.Second
	}
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{store: st, opts: opts, log: log, clusters: map[string]*clusterState{}, ctx: ctx, cancel: cancel}
}

// Options returns the manager's settings.
func (m *Manager) Options() Options { return m.opts }

// Start begins polling every registered cluster.
func (m *Manager) Start(ctx context.Context) error {
	clusters, err := m.store.ListClusters(ctx)
	if err != nil {
		return err
	}
	for _, c := range clusters {
		m.Reload(c)
	}
	return nil
}

// Stop ends all polling.
func (m *Manager) Stop() {
	m.cancel()
	m.mu.Lock()
	for id, st := range m.clusters {
		st.cancel()
		delete(m.clusters, id)
	}
	m.mu.Unlock()
	m.wg.Wait()
}

// Reload (re)starts a cluster after it is created or its settings change.
func (m *Manager) Reload(c store.Cluster) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ctx, cancel := context.WithCancel(m.ctx)
	st := &clusterState{cluster: c, subs: map[chan *Snapshot]struct{}{}, cancel: cancel}
	st.client, st.clientErr = NewClient(c.ManagementURLs, c.TLS, c.Auth, m.opts.RequestTimeout)
	if tr, err := Transport(c.TLS); err == nil {
		st.prom = &http.Client{Transport: tr, Timeout: m.opts.RequestTimeout}
	}
	if old, ok := m.clusters[c.ID]; ok {
		old.cancel()
		// Keep history and subscribers across a settings change.
		old.mu.Lock()
		st.history = old.history
		for ch := range old.subs {
			st.subs[ch] = struct{}{}
		}
		old.mu.Unlock()
	}
	m.clusters[c.ID] = st
	m.wg.Add(1)
	go m.pollLoop(ctx, st)
}

// Remove stops and forgets a cluster.
func (m *Manager) Remove(id string) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if st, ok := m.clusters[id]; ok {
		st.cancel()
		delete(m.clusters, id)
	}
}

func (m *Manager) state(id string) *clusterState {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.clusters[id]
}

// Client returns the management API client of a cluster.
func (m *Manager) Client(id string) (*Client, error) {
	st := m.state(id)
	if st == nil {
		return nil, store.ErrNotFound
	}
	return st.client, st.clientErr
}

// Snapshot returns the latest snapshot of a cluster, or nil before the first poll.
func (m *Manager) Snapshot(id string) *Snapshot {
	st := m.state(id)
	if st == nil {
		return nil
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	return st.snapshot
}

// History returns the rate history of a cluster.
func (m *Manager) History(id string) []RatePoint {
	st := m.state(id)
	if st == nil {
		return []RatePoint{}
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	out := make([]RatePoint, len(st.history))
	copy(out, st.history)
	return out
}

// Subscribe returns a channel that receives each new snapshot of a cluster.
func (m *Manager) Subscribe(id string) (<-chan *Snapshot, func()) {
	ch := make(chan *Snapshot, 4)
	st := m.state(id)
	if st == nil {
		close(ch)
		return ch, func() {}
	}
	st.mu.Lock()
	st.subs[ch] = struct{}{}
	st.mu.Unlock()
	return ch, func() {
		// Look the state up again: Reload may have replaced it.
		if cur := m.state(id); cur != nil {
			cur.mu.Lock()
			delete(cur.subs, ch)
			cur.mu.Unlock()
		}
		st.mu.Lock()
		delete(st.subs, ch)
		st.mu.Unlock()
	}
}

func (m *Manager) pollLoop(ctx context.Context, st *clusterState) {
	defer m.wg.Done()
	t := time.NewTicker(m.opts.PollInterval)
	defer t.Stop()
	for {
		m.pollOnce(ctx, st, false)
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}

// PollNow runs one poll immediately (including health checks) and returns the new snapshot.
func (m *Manager) PollNow(ctx context.Context, id string) (*Snapshot, error) {
	st := m.state(id)
	if st == nil {
		return nil, store.ErrNotFound
	}
	snap := m.pollOnce(ctx, st, true)
	if snap == nil {
		return nil, ctx.Err()
	}
	return snap, nil
}

const queueColumns = "name,vhost,type,state,messages,messages_ready,messages_unacknowledged,consumers,node,leader,members,online," +
	"message_stats.publish_details.avg_rate,message_stats.deliver_get_details.avg_rate"

func (m *Manager) pollOnce(ctx context.Context, st *clusterState, forceHealth bool) *Snapshot {
	st.pollMu.Lock()
	defer st.pollMu.Unlock()
	now := time.Now()
	c := st.cluster
	if st.clientErr != nil {
		return m.publish(st, m.failedSnapshot(st, now, st.clientErr), now)
	}
	cl := st.client
	pctx, cancel := context.WithTimeout(ctx, m.opts.RequestTimeout)
	defer cancel()

	var res pollResult
	var wg sync.WaitGroup
	var ovErr error
	ov := &overviewRaw{}
	wg.Add(3)
	go func() {
		defer wg.Done()
		if ovErr = cl.Get(pctx, "/api/overview", ov); ovErr == nil {
			res.overview = ov
		}
	}()
	go func() {
		defer wg.Done()
		_ = cl.Get(pctx, "/api/nodes", &res.nodes)
	}()
	go func() {
		defer wg.Done()
		var page struct {
			Items []queueRaw `json:"items"`
		}
		if cl.Get(pctx, "/api/queues?page=1&page_size=500&sort=messages&sort_reverse=true&msg_rates_age=30&msg_rates_incr=5&columns="+queueColumns, &page) == nil {
			res.queues = page.Items
		}
	}()

	runHealth := forceHealth || now.Sub(st.healthAt) >= m.opts.HealthInterval || st.health == nil
	res.endpoints = make([]Endpoint, len(cl.URLs()))
	for i, u := range cl.URLs() {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res.endpoints[i] = m.probeEndpoint(pctx, cl, u)
		}()
	}
	if len(c.PrometheusURLs) > 0 && st.prom != nil {
		res.prom = make([]*PromMetrics, len(c.PrometheusURLs))
		res.promSum = make([]PromSummary, len(c.PrometheusURLs))
		for i, u := range c.PrometheusURLs {
			wg.Add(1)
			go func() {
				defer wg.Done()
				pm, err := Scrape(pctx, st.prom, u)
				res.promSum[i] = PromSummary{URL: u, OK: err == nil}
				if err != nil {
					res.promSum[i].Error = err.Error()
					return
				}
				res.promSum[i].Node = pm.Node
				res.prom[i] = pm
			}()
		}
	}
	wg.Wait()
	if ctx.Err() != nil {
		return nil
	}
	scrapes := res.prom[:0]
	for _, p := range res.prom {
		if p != nil {
			scrapes = append(scrapes, p)
		}
	}
	res.prom = scrapes

	if res.overview == nil {
		return m.publish(st, m.failedSnapshot(st, now, ovErr), now)
	}
	if runHealth {
		st.health, st.checks = m.runHealth(pctx, cl, res.endpoints, ov)
		st.healthAt = now
		var used []struct {
			Name string `json:"name"`
		}
		// Absent before RabbitMQ 3.13; then nothing is reported.
		if cl.Get(pctx, "/api/deprecated-features/used", &used) == nil {
			st.deprecated = []string{}
			for _, u := range used {
				st.deprecated = append(st.deprecated, u.Name)
			}
		}
	}
	res.health, res.checks, res.deprecated = st.health, st.checks, st.deprecated

	var promRates *Rates
	if len(res.prom) > 0 {
		cur := newPromTotals(now, res.prom)
		if r, ok := cur.rates(st.promPrev); ok {
			promRates = &r
		}
		st.promPrev = cur
	}
	return m.publish(st, buildSnapshot(c.ID, now, res, promRates), now)
}

func (m *Manager) probeEndpoint(ctx context.Context, cl *Client, base string) Endpoint {
	e := Endpoint{URL: base}
	start := time.Now()
	var o struct {
		Node string `json:"node"`
	}
	err := cl.DoAt(ctx, base, http.MethodGet, "/api/overview?columns=node", nil, &o)
	e.LatencyMs = time.Since(start).Milliseconds()
	if err != nil {
		e.Error = err.Error()
		return e
	}
	e.OK, e.Node = true, o.Node
	return e
}

// runHealth runs every check through every reachable endpoint, one endpoint per node.
func (m *Manager) runHealth(ctx context.Context, cl *Client, endpoints []Endpoint, ov *overviewRaw) ([]HealthResult, []string) {
	protocols := map[string][]string{}
	for _, l := range ov.Listeners {
		if !strings.Contains(l.Protocol, "/") {
			protocols[l.Node] = append(protocols[l.Node], l.Protocol)
		}
	}
	seen := map[string]bool{}
	var mu sync.Mutex
	out := []HealthResult{}
	var wg sync.WaitGroup
	for _, e := range endpoints {
		if !e.OK || seen[e.Node] {
			continue
		}
		seen[e.Node] = true
		for _, check := range HealthChecks {
			path := "/api/health/checks/" + check
			switch check {
			case "certificate-expiration":
				path = "/api/health/checks/certificate-expiration/1/months"
			case "protocol-listeners":
				if len(protocols[e.Node]) == 0 {
					continue
				}
				path = "/api/health/checks/protocol-listener/" + strings.Join(protocols[e.Node], ",")
			}
			wg.Add(1)
			go func() {
				defer wg.Done()
				r := HealthResult{Node: e.Node, Check: check, Status: "ok"}
				err := cl.DoAt(ctx, e.URL, http.MethodGet, path, nil, nil)
				var ae *APIError
				switch {
				case err == nil:
				case errors.As(err, &ae) && ae.Status == http.StatusServiceUnavailable:
					r.Status = "failed"
					var body struct {
						Reason string `json:"reason"`
					}
					if json.Unmarshal(ae.Body, &body) == nil {
						r.Reason = body.Reason
					}
					if json.Valid(ae.Body) {
						r.Detail = ae.Body
					}
				case errors.As(err, &ae) && ae.Status == http.StatusNotFound:
					// Not available on this RabbitMQ version.
					return
				default:
					r.Status, r.Reason = "error", err.Error()
				}
				mu.Lock()
				out = append(out, r)
				mu.Unlock()
			}()
		}
	}
	wg.Wait()
	present := map[string]bool{}
	for _, r := range out {
		present[r.Check] = true
	}
	checks := []string{}
	for _, c := range HealthChecks {
		if present[c] {
			checks = append(checks, c)
		}
	}
	return out, checks
}

func (m *Manager) publish(st *clusterState, snap *Snapshot, now time.Time) *Snapshot {
	st.mu.Lock()
	defer st.mu.Unlock()
	st.snapshot = snap
	if snap.Connected {
		t, r := snap.Totals, snap.Rates
		st.history = append(st.history, RatePoint{Time: now, Publish: r.Publish, Deliver: r.DeliverGet, Ack: r.Ack, Confirm: r.Confirm,
			Redeliver: r.Redeliver, Unroutable: r.DropUnroutable + r.ReturnUnroutable, Ready: t.MessagesReady, Unacked: t.MessagesUnacked,
			Connections: t.Connections, Channels: t.Channels, Queues: t.Queues, Consumers: t.Consumers,
			ConnCreated: r.ConnCreated, ConnClosed: r.ConnClosed, ChanCreated: r.ChanCreated, ChanClosed: r.ChanClosed,
			QueueCreated: r.QueueCreated, QueueDeleted: r.QueueDeleted})
		if over := len(st.history) - m.opts.HistoryPoints; over > 0 {
			st.history = append([]RatePoint(nil), st.history[over:]...)
		}
	}
	for ch := range st.subs {
		select {
		case ch <- snap:
		default:
		}
	}
	return snap
}

func (m *Manager) failedSnapshot(st *clusterState, now time.Time, err error) *Snapshot {
	if err == nil {
		err = errors.New("no answer from the management API")
	}
	st.mu.Lock()
	prev := st.snapshot
	st.mu.Unlock()
	snap := &Snapshot{ClusterID: st.cluster.ID, Time: now, Error: err.Error(), RatesSource: "none", Nodes: []NodeInfo{}, Alarms: []Alarm{},
		Health: []HealthResult{}, HealthChecks: []string{}, Listeners: []Listener{}, Contexts: []WebContext{}, Endpoints: []Endpoint{},
		Queues: []QueueBrief{}, Attention: []Attention{}, Prometheus: []PromSummary{}, Info: Info{Versions: []string{}}}
	if prev != nil {
		// Keep the last known layout so the UI can still show it, marked stale.
		snap.Info = prev.Info
		for _, n := range prev.Nodes {
			n.Healthy = false
			snap.Nodes = append(snap.Nodes, n)
		}
	}
	m.log.Warn("poll failed", "cluster", st.cluster.ID, "err", err)
	return snap
}
