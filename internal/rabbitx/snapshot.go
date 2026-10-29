package rabbitx

import (
	"encoding/json"
	"sort"
	"strconv"
	"strings"
	"time"
)

// Snapshot is the state of one cluster at one poll.
type Snapshot struct {
	ClusterID string    `json:"clusterId"`
	Time      time.Time `json:"time"`
	Connected bool      `json:"connected"`
	Error     string    `json:"error,omitempty"`
	Info      Info      `json:"info"`
	Totals    Totals    `json:"totals"`
	Rates     Rates     `json:"rates"`
	// RatesSource tells where message rates came from: the management
	// stats database, rabmin's own Prometheus counter deltas, or nowhere.
	RatesSource string         `json:"ratesSource" enum:"management,prometheus,none"`
	Nodes       []NodeInfo     `json:"nodes"`
	Alarms      []Alarm        `json:"alarms"`
	Health      []HealthResult `json:"health"`
	// HealthChecks lists the check ids in matrix column order.
	HealthChecks []string      `json:"healthChecks"`
	Listeners    []Listener    `json:"listeners"`
	Contexts     []WebContext  `json:"contexts"`
	Endpoints    []Endpoint    `json:"endpoints"`
	Queues       []QueueBrief  `json:"queues" doc:"The deepest queues (up to 500 inspected), plus any with problems"`
	Attention    []Attention   `json:"attention"`
	Prometheus   []PromSummary `json:"prometheus"`
}

// Info describes the cluster software.
type Info struct {
	ClusterName       string `json:"clusterName"`
	ProductName       string `json:"productName"`
	ProductVersion    string `json:"productVersion"`
	RabbitMQVersion   string `json:"rabbitmqVersion"`
	ManagementVersion string `json:"managementVersion"`
	ErlangVersion     string `json:"erlangVersion"`
	ErlangFullVersion string `json:"erlangFullVersion"`
	RatesMode         string `json:"ratesMode"`
	StatsDisabled     bool   `json:"statsDisabled"`
	DefaultQueueType  string `json:"defaultQueueType"`
	// Versions lists every distinct RabbitMQ version across running nodes.
	Versions []string `json:"versions"`
}

// Totals are cluster-wide counts.
type Totals struct {
	Nodes           int     `json:"nodes"`
	NodesRunning    int     `json:"nodesRunning"`
	Connections     int     `json:"connections"`
	Channels        int     `json:"channels"`
	Exchanges       int     `json:"exchanges"`
	Queues          int     `json:"queues"`
	Consumers       int     `json:"consumers"`
	Messages        float64 `json:"messages"`
	MessagesReady   float64 `json:"messagesReady"`
	MessagesUnacked float64 `json:"messagesUnacked"`
}

// Rates are per-second message and churn rates.
type Rates struct {
	Publish          float64 `json:"publish"`
	Confirm          float64 `json:"confirm"`
	Deliver          float64 `json:"deliver" doc:"Deliveries to consumers with manual ack"`
	DeliverNoAck     float64 `json:"deliverNoAck" doc:"Deliveries to consumers with automatic ack"`
	DeliverGet       float64 `json:"deliverGet" doc:"All deliveries and gets"`
	Get              float64 `json:"get"`
	GetNoAck         float64 `json:"getNoAck"`
	GetEmpty         float64 `json:"getEmpty"`
	Redeliver        float64 `json:"redeliver"`
	Ack              float64 `json:"ack"`
	ReturnUnroutable float64 `json:"returnUnroutable"`
	DropUnroutable   float64 `json:"dropUnroutable"`
	DiskReads        float64 `json:"diskReads"`
	DiskWrites       float64 `json:"diskWrites"`
	ConnCreated      float64 `json:"connCreated"`
	ConnClosed       float64 `json:"connClosed"`
	ChanCreated      float64 `json:"chanCreated"`
	ChanClosed       float64 `json:"chanClosed"`
	QueueCreated     float64 `json:"queueCreated"`
	QueueDeclared    float64 `json:"queueDeclared"`
	QueueDeleted     float64 `json:"queueDeleted"`
}

// NodeInfo is one node of the cluster.
type NodeInfo struct {
	Name            string   `json:"name"`
	Running         bool     `json:"running"`
	Type            string   `json:"type"`
	RabbitMQVersion string   `json:"rabbitmqVersion"`
	ErlangVersion   string   `json:"erlangVersion"`
	UptimeMs        int64    `json:"uptimeMs"`
	MemUsed         float64  `json:"memUsed"`
	MemLimit        float64  `json:"memLimit"`
	MemAlarm        bool     `json:"memAlarm"`
	DiskFree        float64  `json:"diskFree"`
	DiskFreeLimit   float64  `json:"diskFreeLimit"`
	DiskAlarm       bool     `json:"diskAlarm"`
	FdUsed          float64  `json:"fdUsed"`
	FdTotal         float64  `json:"fdTotal"`
	ProcUsed        float64  `json:"procUsed"`
	ProcTotal       float64  `json:"procTotal"`
	Processors      int      `json:"processors"`
	RunQueue        float64  `json:"runQueue"`
	Partitions      []string `json:"partitions"`
	BeingDrained    bool     `json:"beingDrained" doc:"In maintenance mode"`
	EnabledPlugins  []string `json:"enabledPlugins"`
	// Endpoint is the configured management URL served by this node, if any.
	Endpoint string `json:"endpoint,omitempty"`
	// Healthy is false when the node is down, alarmed, partitioned or fails a health check.
	Healthy bool `json:"healthy"`
}

// Alarm is a resource alarm in effect.
type Alarm struct {
	Node     string `json:"node"`
	Resource string `json:"resource" enum:"memory,disk"`
}

// HealthResult is one cell of the node × check health matrix.
type HealthResult struct {
	Node   string `json:"node"`
	Check  string `json:"check"`
	Status string `json:"status" enum:"ok,failed,error"`
	Reason string `json:"reason,omitempty"`
	// Detail holds the check's JSON answer when it failed.
	Detail json.RawMessage `json:"detail,omitempty"`
}

// Listener is a protocol listener on a node.
type Listener struct {
	Node     string `json:"node"`
	Protocol string `json:"protocol"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	TLS      bool   `json:"tls"`
}

// WebContext is an HTTP listener (management, Prometheus, Web-MQTT...).
type WebContext struct {
	Node        string `json:"node"`
	Description string `json:"description"`
	Path        string `json:"path"`
	Port        string `json:"port"`
	Protocol    string `json:"protocol,omitempty"`
	TLS         bool   `json:"tls"`
}

// Endpoint is the state of one configured management URL.
type Endpoint struct {
	URL       string `json:"url"`
	Node      string `json:"node,omitempty"`
	OK        bool   `json:"ok"`
	Error     string `json:"error,omitempty"`
	LatencyMs int64  `json:"latencyMs"`
}

// PromSummary is the state of one configured Prometheus URL.
type PromSummary struct {
	URL   string `json:"url"`
	Node  string `json:"node,omitempty"`
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// QueueBrief is a queue as seen by the poller.
type QueueBrief struct {
	VHost       string   `json:"vhost"`
	Name        string   `json:"name"`
	Type        string   `json:"type"`
	State       string   `json:"state"`
	Messages    float64  `json:"messages"`
	Ready       float64  `json:"ready"`
	Unacked     float64  `json:"unacked"`
	Consumers   int      `json:"consumers"`
	PublishRate float64  `json:"publishRate"`
	DeliverRate float64  `json:"deliverRate"`
	Node        string   `json:"node,omitempty"`
	Leader      string   `json:"leader,omitempty"`
	Members     []string `json:"members"`
	Online      []string `json:"online"`
}

// Attention is something an operator should look at.
type Attention struct {
	Severity string `json:"severity" enum:"warn,bad"`
	Kind     string `json:"kind" doc:"node-down, alarm, partition, health, maintenance, version-mismatch, queue-no-consumers, queue-state, queue-members-offline, endpoint"`
	Subject  string `json:"subject"`
	Message  string `json:"message"`
}

// RatePoint is one entry of a cluster's history.
type RatePoint struct {
	Time         time.Time `json:"t"`
	Publish      float64   `json:"publish"`
	Deliver      float64   `json:"deliver"`
	Ack          float64   `json:"ack"`
	Confirm      float64   `json:"confirm"`
	Redeliver    float64   `json:"redeliver"`
	Unroutable   float64   `json:"unroutable"`
	Ready        float64   `json:"ready"`
	Unacked      float64   `json:"unacked"`
	Connections  int       `json:"connections"`
	Channels     int       `json:"channels"`
	Queues       int       `json:"queues"`
	Consumers    int       `json:"consumers"`
	ConnCreated  float64   `json:"connCreated"`
	ConnClosed   float64   `json:"connClosed"`
	ChanCreated  float64   `json:"chanCreated"`
	ChanClosed   float64   `json:"chanClosed"`
	QueueCreated float64   `json:"queueCreated"`
	QueueDeleted float64   `json:"queueDeleted"`
}

// --- raw management API shapes ---

type stats map[string]json.RawMessage

func (s stats) count(key string) float64 {
	var v float64
	_ = json.Unmarshal(s[key], &v)
	return v
}

func (s stats) rate(key string) float64 {
	var d struct {
		Rate    float64  `json:"rate"`
		AvgRate *float64 `json:"avg_rate"`
	}
	_ = json.Unmarshal(s[key+"_details"], &d)
	if d.AvgRate != nil {
		return *d.AvgRate
	}
	return d.Rate
}

type overviewRaw struct {
	ProductName       string `json:"product_name"`
	ProductVersion    string `json:"product_version"`
	RabbitMQVersion   string `json:"rabbitmq_version"`
	ManagementVersion string `json:"management_version"`
	ErlangVersion     string `json:"erlang_version"`
	ErlangFullVersion string `json:"erlang_full_version"`
	ClusterName       string `json:"cluster_name"`
	Node              string `json:"node"`
	RatesMode         string `json:"rates_mode"`
	DisableStats      bool   `json:"disable_stats"`
	DefaultQueueType  string `json:"default_queue_type"`
	MessageStats      stats  `json:"message_stats"`
	ChurnRates        stats  `json:"churn_rates"`
	QueueTotals       stats  `json:"queue_totals"`
	ObjectTotals      struct {
		Channels    int `json:"channels"`
		Consumers   int `json:"consumers"`
		Exchanges   int `json:"exchanges"`
		Connections int `json:"connections"`
		Queues      int `json:"queues"`
	} `json:"object_totals"`
	Listeners []struct {
		Node      string `json:"node"`
		Protocol  string `json:"protocol"`
		IPAddress string `json:"ip_address"`
		Port      int    `json:"port"`
		TLS       bool   `json:"tls"`
	} `json:"listeners"`
	Contexts []struct {
		Node        string          `json:"node"`
		Description string          `json:"description"`
		Path        string          `json:"path"`
		Port        json.RawMessage `json:"port"`
		Protocol    string          `json:"protocol"`
		SSLOpts     json.RawMessage `json:"ssl_opts"`
	} `json:"contexts"`
}

type nodeRaw struct {
	Name            string   `json:"name"`
	Running         bool     `json:"running"`
	Type            string   `json:"type"`
	RabbitMQVersion string   `json:"rabbitmq_version"`
	ErlangVersion   string   `json:"erlang_version"`
	Uptime          int64    `json:"uptime"`
	MemUsed         float64  `json:"mem_used"`
	MemLimit        float64  `json:"mem_limit"`
	MemAlarm        bool     `json:"mem_alarm"`
	DiskFree        float64  `json:"disk_free"`
	DiskFreeLimit   float64  `json:"disk_free_limit"`
	DiskFreeAlarm   bool     `json:"disk_free_alarm"`
	FdUsed          float64  `json:"fd_used"`
	FdTotal         float64  `json:"fd_total"`
	ProcUsed        float64  `json:"proc_used"`
	ProcTotal       float64  `json:"proc_total"`
	Processors      int      `json:"processors"`
	RunQueue        float64  `json:"run_queue"`
	Partitions      []string `json:"partitions"`
	BeingDrained    bool     `json:"being_drained"`
	EnabledPlugins  []string `json:"enabled_plugins"`
}

func (n nodeRaw) info() NodeInfo {
	ni := NodeInfo{Name: n.Name, Running: n.Running, Type: n.Type, RabbitMQVersion: n.RabbitMQVersion, ErlangVersion: n.ErlangVersion,
		UptimeMs: n.Uptime, MemUsed: n.MemUsed, MemLimit: n.MemLimit, MemAlarm: n.MemAlarm, DiskFree: n.DiskFree,
		DiskFreeLimit: n.DiskFreeLimit, DiskAlarm: n.DiskFreeAlarm, FdUsed: n.FdUsed, FdTotal: n.FdTotal, ProcUsed: n.ProcUsed,
		ProcTotal: n.ProcTotal, Processors: n.Processors, RunQueue: n.RunQueue, Partitions: n.Partitions,
		BeingDrained: n.BeingDrained, EnabledPlugins: n.EnabledPlugins}
	if ni.Partitions == nil {
		ni.Partitions = []string{}
	}
	if ni.EnabledPlugins == nil {
		ni.EnabledPlugins = []string{}
	}
	return ni
}

type queueRaw struct {
	Name                   string   `json:"name"`
	VHost                  string   `json:"vhost"`
	Type                   string   `json:"type"`
	State                  string   `json:"state"`
	Messages               float64  `json:"messages"`
	MessagesReady          float64  `json:"messages_ready"`
	MessagesUnacknowledged float64  `json:"messages_unacknowledged"`
	Consumers              int      `json:"consumers"`
	Node                   string   `json:"node"`
	Leader                 string   `json:"leader"`
	Members                []string `json:"members"`
	Online                 []string `json:"online"`
	MessageStats           stats    `json:"message_stats"`
}

func (q queueRaw) brief() QueueBrief {
	b := QueueBrief{VHost: q.VHost, Name: q.Name, Type: q.Type, State: q.State, Messages: q.Messages, Ready: q.MessagesReady,
		Unacked: q.MessagesUnacknowledged, Consumers: q.Consumers, Node: q.Node, Leader: q.Leader, Members: q.Members, Online: q.Online,
		PublishRate: q.MessageStats.rate("publish"), DeliverRate: q.MessageStats.rate("deliver_get")}
	if b.Members == nil {
		b.Members = []string{}
	}
	if b.Online == nil {
		b.Online = []string{}
	}
	return b
}

// pollResult collects the raw answers of one poll.
type pollResult struct {
	overview  *overviewRaw
	nodes     []nodeRaw
	queues    []queueRaw
	endpoints []Endpoint
	health    []HealthResult
	checks    []string
	prom      []*PromMetrics
	promSum   []PromSummary
}

// promCounters maps a Rates field to the Prometheus counters summed for it.
var promCounters = map[string][]string{
	"publish":          {"rabbitmq_global_messages_received_total"},
	"confirm":          {"rabbitmq_global_messages_confirmed_total"},
	"deliver":          {"rabbitmq_global_messages_delivered_consume_manual_ack_total"},
	"deliverNoAck":     {"rabbitmq_global_messages_delivered_consume_auto_ack_total"},
	"deliverGet":       {"rabbitmq_global_messages_delivered_total"},
	"get":              {"rabbitmq_global_messages_delivered_get_manual_ack_total"},
	"getNoAck":         {"rabbitmq_global_messages_delivered_get_auto_ack_total"},
	"getEmpty":         {"rabbitmq_global_messages_get_empty_total"},
	"redeliver":        {"rabbitmq_global_messages_redelivered_total"},
	"ack":              {"rabbitmq_global_messages_acknowledged_total"},
	"returnUnroutable": {"rabbitmq_global_messages_unroutable_returned_total"},
	"dropUnroutable":   {"rabbitmq_global_messages_unroutable_dropped_total"},
	"connCreated":      {"rabbitmq_connections_opened_total"},
	"connClosed":       {"rabbitmq_connections_closed_total"},
	"chanCreated":      {"rabbitmq_channels_opened_total"},
	"chanClosed":       {"rabbitmq_channels_closed_total"},
	"queueCreated":     {"rabbitmq_queues_created_total"},
	"queueDeclared":    {"rabbitmq_queues_declared_total"},
	"queueDeleted":     {"rabbitmq_queues_deleted_total"},
}

func setRate(r *Rates, key string, v float64) {
	switch key {
	case "publish":
		r.Publish = v
	case "confirm":
		r.Confirm = v
	case "deliver":
		r.Deliver = v
	case "deliverNoAck":
		r.DeliverNoAck = v
	case "deliverGet":
		r.DeliverGet = v
	case "get":
		r.Get = v
	case "getNoAck":
		r.GetNoAck = v
	case "getEmpty":
		r.GetEmpty = v
	case "redeliver":
		r.Redeliver = v
	case "ack":
		r.Ack = v
	case "returnUnroutable":
		r.ReturnUnroutable = v
	case "dropUnroutable":
		r.DropUnroutable = v
	case "connCreated":
		r.ConnCreated = v
	case "connClosed":
		r.ConnClosed = v
	case "chanCreated":
		r.ChanCreated = v
	case "chanClosed":
		r.ChanClosed = v
	case "queueCreated":
		r.QueueCreated = v
	case "queueDeclared":
		r.QueueDeclared = v
	case "queueDeleted":
		r.QueueDeleted = v
	}
}

// promTotals sums counters per node so rates can be computed across polls
// even when a node restarts (its counters reset; we skip negative deltas).
type promTotals struct {
	at     time.Time
	byNode map[string]map[string]float64
}

func newPromTotals(at time.Time, scrapes []*PromMetrics) *promTotals {
	t := &promTotals{at: at, byNode: map[string]map[string]float64{}}
	for _, m := range scrapes {
		if m == nil || m.Node == "" {
			continue
		}
		vals := map[string]float64{}
		for key, names := range promCounters {
			for _, n := range names {
				vals[key] += m.Get(n)
			}
		}
		t.byNode[m.Node] = vals
	}
	return t
}

func (t *promTotals) rates(prev *promTotals) (Rates, bool) {
	var r Rates
	if prev == nil || len(t.byNode) == 0 {
		return r, false
	}
	secs := t.at.Sub(prev.at).Seconds()
	if secs <= 0 {
		return r, false
	}
	sums := map[string]float64{}
	for node, vals := range t.byNode {
		old, ok := prev.byNode[node]
		if !ok {
			continue
		}
		for k, v := range vals {
			if d := v - old[k]; d > 0 {
				sums[k] += d
			}
		}
	}
	for k, v := range sums {
		setRate(&r, k, v/secs)
	}
	return r, true
}

func buildSnapshot(clusterID string, now time.Time, res pollResult, promRates *Rates) *Snapshot {
	s := &Snapshot{ClusterID: clusterID, Time: now, Connected: true, RatesSource: "none",
		Nodes: []NodeInfo{}, Alarms: []Alarm{}, Health: res.health, HealthChecks: res.checks, Listeners: []Listener{},
		Contexts: []WebContext{}, Endpoints: res.endpoints, Queues: []QueueBrief{}, Attention: []Attention{}, Prometheus: res.promSum}
	if s.Health == nil {
		s.Health = []HealthResult{}
	}
	if s.HealthChecks == nil {
		s.HealthChecks = []string{}
	}
	if s.Prometheus == nil {
		s.Prometheus = []PromSummary{}
	}
	attn := func(sev, kind, subject, msg string) {
		s.Attention = append(s.Attention, Attention{Severity: sev, Kind: kind, Subject: subject, Message: msg})
	}
	o := res.overview
	s.Info = Info{ClusterName: o.ClusterName, ProductName: o.ProductName, ProductVersion: o.ProductVersion, RabbitMQVersion: o.RabbitMQVersion,
		ManagementVersion: o.ManagementVersion, ErlangVersion: o.ErlangVersion, ErlangFullVersion: o.ErlangFullVersion, RatesMode: o.RatesMode,
		StatsDisabled: o.DisableStats, DefaultQueueType: o.DefaultQueueType, Versions: []string{}}
	ot := o.ObjectTotals
	s.Totals = Totals{Connections: ot.Connections, Channels: ot.Channels, Exchanges: ot.Exchanges, Queues: ot.Queues, Consumers: ot.Consumers,
		Messages: o.QueueTotals.count("messages"), MessagesReady: o.QueueTotals.count("messages_ready"), MessagesUnacked: o.QueueTotals.count("messages_unacknowledged")}

	ms, cr := o.MessageStats, o.ChurnRates
	if len(ms) > 0 && !o.DisableStats {
		s.RatesSource = "management"
		s.Rates = Rates{Publish: ms.rate("publish"), Confirm: ms.rate("confirm"), Deliver: ms.rate("deliver"), DeliverNoAck: ms.rate("deliver_no_ack"),
			DeliverGet: ms.rate("deliver_get"), Get: ms.rate("get"), GetNoAck: ms.rate("get_no_ack"), GetEmpty: ms.rate("get_empty"),
			Redeliver: ms.rate("redeliver"), Ack: ms.rate("ack"), ReturnUnroutable: ms.rate("return_unroutable"), DropUnroutable: ms.rate("drop_unroutable"),
			DiskReads: ms.rate("disk_reads"), DiskWrites: ms.rate("disk_writes"),
			ConnCreated: cr.rate("connection_created"), ConnClosed: cr.rate("connection_closed"), ChanCreated: cr.rate("channel_created"),
			ChanClosed: cr.rate("channel_closed"), QueueCreated: cr.rate("queue_created"), QueueDeclared: cr.rate("queue_declared"), QueueDeleted: cr.rate("queue_deleted")}
	} else if promRates != nil {
		s.RatesSource = "prometheus"
		s.Rates = *promRates
	}
	// Without the stats database, take counts from Prometheus when available.
	if len(res.prom) > 0 && (o.DisableStats || len(o.QueueTotals) == 0) {
		var ready, unacked float64
		for _, m := range res.prom {
			ready += m.Get("rabbitmq_queue_messages_ready")
			unacked += m.Get("rabbitmq_queue_messages_unacked")
		}
		s.Totals.MessagesReady, s.Totals.MessagesUnacked, s.Totals.Messages = ready, unacked, ready+unacked
	}

	byURLNode := map[string]string{}
	for _, e := range res.endpoints {
		if e.Node != "" {
			byURLNode[e.Node] = e.URL
		}
	}
	failedChecks := map[string][]string{}
	for _, h := range res.health {
		if h.Status != "ok" {
			failedChecks[h.Node] = append(failedChecks[h.Node], h.Check)
		}
	}
	versions := map[string]bool{}
	for _, n := range res.nodes {
		ni := n.info()
		ni.Endpoint = byURLNode[n.Name]
		ni.Healthy = n.Running && !n.MemAlarm && !n.DiskFreeAlarm && len(n.Partitions) == 0 && len(failedChecks[n.Name]) == 0
		s.Nodes = append(s.Nodes, ni)
		s.Totals.Nodes++
		if !n.Running {
			attn("bad", "node-down", n.Name, "node is not running")
			continue
		}
		s.Totals.NodesRunning++
		if n.RabbitMQVersion != "" {
			versions[n.RabbitMQVersion] = true
		}
		if n.MemAlarm {
			s.Alarms = append(s.Alarms, Alarm{Node: n.Name, Resource: "memory"})
			attn("bad", "alarm", n.Name, "memory alarm: publishers are blocked")
		}
		if n.DiskFreeAlarm {
			s.Alarms = append(s.Alarms, Alarm{Node: n.Name, Resource: "disk"})
			attn("bad", "alarm", n.Name, "disk alarm: publishers are blocked")
		}
		if len(n.Partitions) > 0 {
			attn("bad", "partition", n.Name, "network partition with "+strings.Join(n.Partitions, ", "))
		}
		if n.BeingDrained {
			attn("warn", "maintenance", n.Name, "node is in maintenance mode")
		}
		if checks := failedChecks[n.Name]; len(checks) > 0 {
			attn("warn", "health", n.Name, "failing health checks: "+strings.Join(checks, ", "))
		}
	}
	sort.Slice(s.Nodes, func(i, j int) bool { return s.Nodes[i].Name < s.Nodes[j].Name })
	for v := range versions {
		s.Info.Versions = append(s.Info.Versions, v)
	}
	sort.Strings(s.Info.Versions)
	if len(s.Info.Versions) > 1 {
		attn("warn", "version-mismatch", s.Info.ClusterName, "nodes run different RabbitMQ versions: "+strings.Join(s.Info.Versions, ", "))
	}
	for _, e := range res.endpoints {
		if !e.OK {
			attn("warn", "endpoint", e.URL, "management endpoint unreachable: "+e.Error)
		}
	}

	for _, l := range o.Listeners {
		s.Listeners = append(s.Listeners, Listener{Node: l.Node, Protocol: l.Protocol, IP: l.IPAddress, Port: l.Port, TLS: l.TLS})
	}
	for _, c := range o.Contexts {
		port := strings.Trim(string(c.Port), `"`)
		if _, err := strconv.Atoi(port); err != nil {
			port = ""
		}
		tls := len(c.SSLOpts) > 2 && string(c.SSLOpts) != "[]"
		s.Contexts = append(s.Contexts, WebContext{Node: c.Node, Description: c.Description, Path: c.Path, Port: port,
			Protocol: strings.Trim(c.Protocol, "'"), TLS: tls})
	}

	for i, q := range res.queues {
		b := q.brief()
		problem := false
		if q.State != "" && q.State != "running" && q.State != "idle" {
			problem = true
			attn("bad", "queue-state", q.VHost+"/"+q.Name, "queue state is "+q.State)
		}
		if q.Type == "quorum" && len(q.Members) > 0 && len(q.Online) < len(q.Members) {
			problem = true
			sev := "warn"
			if len(q.Online) <= len(q.Members)/2 {
				sev = "bad"
			}
			attn(sev, "queue-members-offline", q.VHost+"/"+q.Name, strconv.Itoa(len(q.Online))+" of "+strconv.Itoa(len(q.Members))+" members online")
		}
		if q.Consumers == 0 && q.MessagesReady >= 1000 && q.Type != "stream" {
			problem = true
			attn("warn", "queue-no-consumers", q.VHost+"/"+q.Name, strconv.FormatFloat(q.MessagesReady, 'f', 0, 64)+" ready messages and no consumers")
		}
		if i < 10 || problem {
			s.Queues = append(s.Queues, b)
		}
	}
	return s
}
