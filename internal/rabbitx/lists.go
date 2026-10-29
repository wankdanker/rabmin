package rabbitx

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
)

// Lists ask for these rate windows: list endpoints report `rate` as 0 for
// many busy objects, while `avg_rate` over a window is stable.
const (
	msgRates  = "msg_rates_age=30&msg_rates_incr=5"
	dataRates = "data_rates_age=30&data_rates_incr=5"
)

// Connection is a client connection.
type Connection struct {
	Name          string  `json:"name"`
	ClientName    string  `json:"clientName,omitempty" doc:"connection_name client property"`
	Product       string  `json:"product,omitempty"`
	Version       string  `json:"version,omitempty"`
	Platform      string  `json:"platform,omitempty"`
	User          string  `json:"user"`
	VHost         string  `json:"vhost"`
	Node          string  `json:"node"`
	Protocol      string  `json:"protocol"`
	Type          string  `json:"type" doc:"network or direct"`
	PeerHost      string  `json:"peerHost,omitempty"`
	PeerPort      int     `json:"peerPort,omitempty"`
	Host          string  `json:"host,omitempty"`
	Port          int     `json:"port,omitempty"`
	State         string  `json:"state"`
	Channels      int     `json:"channels"`
	ConnectedAt   int64   `json:"connectedAt" doc:"Unix milliseconds"`
	TLS           bool    `json:"tls"`
	TLSProtocol   string  `json:"tlsProtocol,omitempty"`
	AuthMechanism string  `json:"authMechanism,omitempty"`
	Heartbeat     int     `json:"heartbeat" doc:"Heartbeat timeout in seconds"`
	FrameMax      int     `json:"frameMax"`
	ChannelMax    int     `json:"channelMax"`
	RecvBytes     float64 `json:"recvBytes"`
	SendBytes     float64 `json:"sendBytes"`
	RecvRate      float64 `json:"recvRate" doc:"Bytes per second from the client"`
	SendRate      float64 `json:"sendRate" doc:"Bytes per second to the client"`
	SendPending   float64 `json:"sendPending"`
}

type connRaw struct {
	Name             string          `json:"name"`
	UserProvidedName string          `json:"user_provided_name"`
	ClientProperties json.RawMessage `json:"client_properties"`
	User             string          `json:"user"`
	VHost            string          `json:"vhost"`
	Node             string          `json:"node"`
	Protocol         string          `json:"protocol"`
	Type             string          `json:"type"`
	PeerHost         json.RawMessage `json:"peer_host"`
	PeerPort         json.RawMessage `json:"peer_port"`
	Host             json.RawMessage `json:"host"`
	Port             json.RawMessage `json:"port"`
	State            string          `json:"state"`
	Channels         int             `json:"channels"`
	ConnectedAt      int64           `json:"connected_at"`
	SSL              bool            `json:"ssl"`
	SSLProtocol      json.RawMessage `json:"ssl_protocol"`
	AuthMechanism    string          `json:"auth_mechanism"`
	Timeout          int             `json:"timeout"`
	FrameMax         int             `json:"frame_max"`
	ChannelMax       int             `json:"channel_max"`
	RecvOct          float64         `json:"recv_oct"`
	SendOct          float64         `json:"send_oct"`
	SendPend         float64         `json:"send_pend"`
	RecvOctDetails   rateDetails     `json:"recv_oct_details"`
	SendOctDetails   rateDetails     `json:"send_oct_details"`
}

type rateDetails struct {
	Rate    float64  `json:"rate"`
	AvgRate *float64 `json:"avg_rate"`
}

func (r rateDetails) value() float64 {
	if r.AvgRate != nil {
		return *r.AvgRate
	}
	return r.Rate
}

// str decodes a JSON string, tolerating null and non-strings.
func str(raw json.RawMessage) string {
	var s string
	if json.Unmarshal(raw, &s) == nil {
		return s
	}
	return ""
}

func num(raw json.RawMessage) int {
	var n int
	_ = json.Unmarshal(raw, &n)
	return n
}

// ClientProps decodes the interesting client properties.
type ClientProps struct {
	ConnectionName string `json:"connection_name"`
	Product        string `json:"product"`
	Version        string `json:"version"`
	Platform       string `json:"platform"`
}

func (c connRaw) conn() Connection {
	var cp ClientProps
	_ = json.Unmarshal(c.ClientProperties, &cp)
	name := c.UserProvidedName
	if name == "" {
		name = cp.ConnectionName
	}
	t := c.Type
	if t == "" {
		t = "network"
	}
	return Connection{Name: c.Name, ClientName: name, Product: cp.Product, Version: cp.Version, Platform: cp.Platform,
		User: c.User, VHost: c.VHost, Node: c.Node, Protocol: c.Protocol, Type: t, PeerHost: str(c.PeerHost), PeerPort: num(c.PeerPort),
		Host: str(c.Host), Port: num(c.Port), State: c.State, Channels: c.Channels, ConnectedAt: c.ConnectedAt, TLS: c.SSL,
		TLSProtocol: str(c.SSLProtocol), AuthMechanism: c.AuthMechanism, Heartbeat: c.Timeout, FrameMax: c.FrameMax, ChannelMax: c.ChannelMax,
		RecvBytes: c.RecvOct, SendBytes: c.SendOct, RecvRate: c.RecvOctDetails.value(), SendRate: c.SendOctDetails.value(), SendPending: c.SendPend}
}

const connColumns = "name,user_provided_name,client_properties.connection_name,client_properties.product,client_properties.version," +
	"client_properties.platform,user,vhost,node,protocol,type,peer_host,peer_port,host,port,state,channels,connected_at,ssl,ssl_protocol," +
	"auth_mechanism,timeout,frame_max,channel_max,recv_oct,send_oct,send_pend,recv_oct_details.avg_rate,send_oct_details.avg_rate"

// Connections lists every connection of the cluster.
func (c *Client) Connections(ctx context.Context) ([]Connection, error) {
	var raw []connRaw
	if err := c.Get(ctx, "/api/connections?"+dataRates+"&columns="+connColumns, &raw); err != nil {
		return nil, err
	}
	out := make([]Connection, 0, len(raw))
	for _, r := range raw {
		out = append(out, r.conn())
	}
	return out, nil
}

// ConnectionDetail is one connection with its raw management object.
type ConnectionDetail struct {
	Connection Connection      `json:"connection"`
	Properties json.RawMessage `json:"clientProperties" doc:"All client properties, including capabilities"`
	Raw        json.RawMessage `json:"raw" doc:"The management API object"`
	Channels   []Channel       `json:"channels"`
}

// Connection reads one connection and its channels.
func (c *Client) Connection(ctx context.Context, name string) (*ConnectionDetail, error) {
	var data []byte
	if err := c.Get(ctx, "/api/connections/"+Esc(name), &data); err != nil {
		return nil, err
	}
	var r connRaw
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	d := &ConnectionDetail{Connection: r.conn(), Properties: r.ClientProperties, Raw: data, Channels: []Channel{}}
	if len(d.Properties) == 0 {
		d.Properties = json.RawMessage("{}")
	}
	var chans []channelRaw
	if err := c.Get(ctx, "/api/connections/"+Esc(name)+"/channels?"+msgRates, &chans); err == nil {
		for _, ch := range chans {
			x := ch.channel()
			x.ClientName = d.Connection.ClientName
			d.Channels = append(d.Channels, x)
		}
	}
	return d, nil
}

// CloseConnection closes a connection, telling the client why.
func (c *Client) CloseConnection(ctx context.Context, name, reason string) error {
	h := http.Header{}
	if reason != "" {
		h.Set("X-Reason", reason)
	}
	return c.DoHeaders(ctx, http.MethodDelete, "/api/connections/"+Esc(name), h, nil, nil)
}

// Channel is an AMQP 0-9-1 channel.
type Channel struct {
	Name           string  `json:"name"`
	Connection     string  `json:"connection"`
	ClientName     string  `json:"clientName,omitempty"`
	Number         int     `json:"number"`
	Node           string  `json:"node"`
	User           string  `json:"user"`
	VHost          string  `json:"vhost"`
	State          string  `json:"state"`
	Consumers      int     `json:"consumers"`
	Prefetch       int     `json:"prefetch" doc:"Per-consumer prefetch (basic.qos global=false)"`
	GlobalPrefetch int     `json:"globalPrefetch" doc:"Per-channel prefetch (basic.qos global=true)"`
	Unacked        int     `json:"unacked"`
	Unconfirmed    int     `json:"unconfirmed"`
	Uncommitted    int     `json:"uncommitted"`
	AcksUncommited int     `json:"acksUncommitted"`
	Confirm        bool    `json:"confirm"`
	Transactional  bool    `json:"transactional"`
	PublishRate    float64 `json:"publishRate"`
	ConfirmRate    float64 `json:"confirmRate"`
	DeliverRate    float64 `json:"deliverRate"`
	AckRate        float64 `json:"ackRate"`
	RedeliverRate  float64 `json:"redeliverRate"`
	UnroutableRate float64 `json:"unroutableRate"`
}

type channelRaw struct {
	Name              string `json:"name"`
	ConnectionDetails struct {
		Name string `json:"name"`
	} `json:"connection_details"`
	Number                 int             `json:"number"`
	Node                   string          `json:"node"`
	User                   string          `json:"user"`
	VHost                  string          `json:"vhost"`
	State                  string          `json:"state"`
	ConsumerCount          int             `json:"consumer_count"`
	PrefetchCount          int             `json:"prefetch_count"`
	GlobalPrefetchCount    json.RawMessage `json:"global_prefetch_count"`
	MessagesUnacknowledged int             `json:"messages_unacknowledged"`
	MessagesUnconfirmed    int             `json:"messages_unconfirmed"`
	MessagesUncommitted    int             `json:"messages_uncommitted"`
	AcksUncommitted        int             `json:"acks_uncommitted"`
	Confirm                bool            `json:"confirm"`
	Transactional          bool            `json:"transactional"`
	MessageStats           stats           `json:"message_stats"`
}

func (r channelRaw) channel() Channel {
	ms := r.MessageStats
	return Channel{Name: r.Name, Connection: r.ConnectionDetails.Name, Number: r.Number, Node: r.Node, User: r.User, VHost: r.VHost,
		State: r.State, Consumers: r.ConsumerCount, Prefetch: r.PrefetchCount, GlobalPrefetch: num(r.GlobalPrefetchCount),
		Unacked: r.MessagesUnacknowledged, Unconfirmed: r.MessagesUnconfirmed, Uncommitted: r.MessagesUncommitted, AcksUncommited: r.AcksUncommitted,
		Confirm: r.Confirm, Transactional: r.Transactional, PublishRate: ms.rate("publish"), ConfirmRate: ms.rate("confirm"),
		DeliverRate: ms.rate("deliver_get"), AckRate: ms.rate("ack"), RedeliverRate: ms.rate("redeliver"),
		UnroutableRate: ms.rate("drop_unroutable") + ms.rate("return_unroutable")}
}

const channelColumns = "name,connection_details.name,number,node,user,vhost,state,consumer_count,prefetch_count,global_prefetch_count," +
	"messages_unacknowledged,messages_unconfirmed,messages_uncommitted,acks_uncommitted,confirm,transactional," +
	"message_stats.publish_details.avg_rate,message_stats.confirm_details.avg_rate,message_stats.deliver_get_details.avg_rate," +
	"message_stats.ack_details.avg_rate,message_stats.redeliver_details.avg_rate,message_stats.drop_unroutable_details.avg_rate," +
	"message_stats.return_unroutable_details.avg_rate"

// clientNames maps connection names to client-provided names.
func (c *Client) clientNames(ctx context.Context) map[string]string {
	var raw []connRaw
	out := map[string]string{}
	if c.Get(ctx, "/api/connections?columns=name,user_provided_name,client_properties.connection_name", &raw) != nil {
		return out
	}
	for _, r := range raw {
		out[r.Name] = r.conn().ClientName
	}
	return out
}

// Channels lists every channel, with the client name of its connection.
func (c *Client) Channels(ctx context.Context) ([]Channel, error) {
	var raw []channelRaw
	if err := c.Get(ctx, "/api/channels?"+msgRates+"&columns="+channelColumns, &raw); err != nil {
		return nil, err
	}
	names := c.clientNames(ctx)
	out := make([]Channel, 0, len(raw))
	for _, r := range raw {
		ch := r.channel()
		ch.ClientName = names[ch.Connection]
		out = append(out, ch)
	}
	return out, nil
}

// Consumer is a consumer on a queue.
type Consumer struct {
	Tag         string          `json:"tag"`
	VHost       string          `json:"vhost"`
	Queue       string          `json:"queue"`
	Channel     string          `json:"channel"`
	Connection  string          `json:"connection"`
	ClientName  string          `json:"clientName,omitempty"`
	Node        string          `json:"node"`
	User        string          `json:"user"`
	AckRequired bool            `json:"ackRequired"`
	Prefetch    int             `json:"prefetch"`
	Exclusive   bool            `json:"exclusive"`
	Active      bool            `json:"active"`
	Status      string          `json:"status" doc:"up, single_active (the active one) or waiting"`
	TimeoutMs   int64           `json:"timeoutMs,omitempty"`
	Arguments   json.RawMessage `json:"arguments"`
}

type consumerRaw struct {
	ConsumerTag    string          `json:"consumer_tag"`
	AckRequired    bool            `json:"ack_required"`
	PrefetchCount  int             `json:"prefetch_count"`
	Exclusive      bool            `json:"exclusive"`
	Active         bool            `json:"active"`
	ActivityStatus string          `json:"activity_status"`
	Timeout        int64           `json:"consumer_timeout"`
	Arguments      json.RawMessage `json:"arguments"`
	Queue          struct {
		Name  string `json:"name"`
		VHost string `json:"vhost"`
	} `json:"queue"`
	ChannelDetails struct {
		Name           string `json:"name"`
		ConnectionName string `json:"connection_name"`
		Node           string `json:"node"`
		User           string `json:"user"`
	} `json:"channel_details"`
}

func (r consumerRaw) consumer() Consumer {
	args := r.Arguments
	if len(args) == 0 || string(args) == "[]" {
		args = json.RawMessage("{}")
	}
	return Consumer{Tag: r.ConsumerTag, VHost: r.Queue.VHost, Queue: r.Queue.Name, Channel: r.ChannelDetails.Name,
		Connection: r.ChannelDetails.ConnectionName, Node: r.ChannelDetails.Node, User: r.ChannelDetails.User, AckRequired: r.AckRequired,
		Prefetch: r.PrefetchCount, Exclusive: r.Exclusive, Active: r.Active, Status: r.ActivityStatus, TimeoutMs: r.Timeout, Arguments: args}
}

// Consumers lists every consumer, with the client name of its connection.
func (c *Client) Consumers(ctx context.Context) ([]Consumer, error) {
	var raw []consumerRaw
	if err := c.Get(ctx, "/api/consumers", &raw); err != nil {
		return nil, err
	}
	names := c.clientNames(ctx)
	out := make([]Consumer, 0, len(raw))
	for _, r := range raw {
		x := r.consumer()
		x.ClientName = names[x.Connection]
		out = append(out, x)
	}
	return out, nil
}

// ChannelDetail is one channel with its consumers and, in detailed rates
// mode, per-exchange publishes and per-queue deliveries.
type ChannelDetail struct {
	Channel    Channel         `json:"channel"`
	Consumers  []Consumer      `json:"consumers"`
	Publishes  []ObjectRate    `json:"publishes"`
	Deliveries []ObjectRate    `json:"deliveries"`
	Raw        json.RawMessage `json:"raw"`
}

// ObjectRate is a message rate to or from one exchange or queue.
type ObjectRate struct {
	VHost string  `json:"vhost"`
	Name  string  `json:"name"`
	Rate  float64 `json:"rate"`
	Count float64 `json:"count"`
}

// Channel reads one channel.
func (c *Client) Channel(ctx context.Context, name string) (*ChannelDetail, error) {
	var data []byte
	if err := c.Get(ctx, "/api/channels/"+Esc(name)+"?"+msgRates, &data); err != nil {
		return nil, err
	}
	var r struct {
		channelRaw
		ConsumerDetails []consumerRaw `json:"consumer_details"`
		Publishes       []struct {
			Exchange struct {
				Name  string `json:"name"`
				VHost string `json:"vhost"`
			} `json:"exchange"`
			Stats stats `json:"stats"`
		} `json:"publishes"`
		Deliveries []struct {
			Queue struct {
				Name  string `json:"name"`
				VHost string `json:"vhost"`
			} `json:"queue"`
			Stats stats `json:"stats"`
		} `json:"deliveries"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	names := c.clientNames(ctx)
	d := &ChannelDetail{Channel: r.channel(), Consumers: []Consumer{}, Publishes: []ObjectRate{}, Deliveries: []ObjectRate{}, Raw: StripSamples(data)}
	d.Channel.ClientName = names[d.Channel.Connection]
	for _, cr := range r.ConsumerDetails {
		x := cr.consumer()
		x.ClientName = d.Channel.ClientName
		d.Consumers = append(d.Consumers, x)
	}
	for _, p := range r.Publishes {
		d.Publishes = append(d.Publishes, ObjectRate{VHost: p.Exchange.VHost, Name: p.Exchange.Name, Rate: p.Stats.rate("publish"), Count: p.Stats.count("publish")})
	}
	for _, p := range r.Deliveries {
		d.Deliveries = append(d.Deliveries, ObjectRate{VHost: p.Queue.VHost, Name: p.Queue.Name, Rate: p.Stats.rate("deliver_get"), Count: p.Stats.count("deliver_get")})
	}
	return d, nil
}

// StripSamples removes rate sample arrays from a management object, which
// only bloat the raw view.
func StripSamples(data []byte) json.RawMessage {
	var v any
	if json.Unmarshal(data, &v) != nil {
		return data
	}
	var walk func(any)
	walk = func(x any) {
		switch t := x.(type) {
		case map[string]any:
			delete(t, "samples")
			for _, y := range t {
				walk(y)
			}
		case []any:
			for _, y := range t {
				walk(y)
			}
		}
	}
	walk(v)
	out, err := json.Marshal(v)
	if err != nil {
		return data
	}
	return out
}

// StreamConnection is a RabbitMQ stream protocol connection.
type StreamConnection struct {
	Name        string  `json:"name"`
	ClientName  string  `json:"clientName,omitempty"`
	User        string  `json:"user"`
	VHost       string  `json:"vhost"`
	Node        string  `json:"node"`
	PeerHost    string  `json:"peerHost,omitempty"`
	PeerPort    int     `json:"peerPort,omitempty"`
	ConnectedAt int64   `json:"connectedAt"`
	TLS         bool    `json:"tls"`
	Heartbeat   int     `json:"heartbeat"`
	RecvBytes   float64 `json:"recvBytes"`
	SendBytes   float64 `json:"sendBytes"`
	RecvRate    float64 `json:"recvRate"`
	SendRate    float64 `json:"sendRate"`
	Publishers  int     `json:"publishers"`
	Consumers   int     `json:"consumers"`
}

// StreamConnections lists stream protocol connections (rabbitmq_stream_management).
func (c *Client) StreamConnections(ctx context.Context) ([]StreamConnection, error) {
	var raw []struct {
		connRaw
		Publishers int `json:"publishers"`
		Consumers  int `json:"consumers"`
	}
	if err := c.Get(ctx, "/api/stream/connections?"+dataRates, &raw); err != nil {
		return nil, err
	}
	out := make([]StreamConnection, 0, len(raw))
	for _, r := range raw {
		x := r.conn()
		out = append(out, StreamConnection{Name: x.Name, ClientName: x.ClientName, User: x.User, VHost: x.VHost, Node: x.Node, PeerHost: x.PeerHost,
			PeerPort: x.PeerPort, ConnectedAt: x.ConnectedAt, TLS: x.TLS, Heartbeat: x.Heartbeat, RecvBytes: x.RecvBytes, SendBytes: x.SendBytes,
			RecvRate: x.RecvRate, SendRate: x.SendRate, Publishers: r.Publishers, Consumers: r.Consumers})
	}
	return out, nil
}

// NodeDetail is one node with its memory breakdown and raw management object.
type NodeDetail struct {
	Node   NodeInfo     `json:"node"`
	Memory []MemoryPart `json:"memory"`
	// MemoryTotal is the total by the node's calculation strategy.
	MemoryTotal    float64         `json:"memoryTotal"`
	MemoryStrategy string          `json:"memoryStrategy"`
	Raw            json.RawMessage `json:"raw"`
}

// MemoryPart is one category of the memory breakdown.
type MemoryPart struct {
	Category string  `json:"category"`
	Group    string  `json:"group"`
	Bytes    float64 `json:"bytes"`
}

var memoryGroups = map[string]string{
	"connection_readers": "Connections", "connection_writers": "Connections", "connection_channels": "Connections", "connection_other": "Connections",
	"queue_procs": "Queues", "quorum_queue_procs": "Queues", "quorum_queue_dlx_procs": "Queues", "stream_queue_procs": "Queues",
	"stream_queue_replica_reader_procs": "Queues", "stream_queue_coordinator_procs": "Queues", "quorum_ets": "Queues", "msg_index": "Queues",
	"binary": "Binaries", "plugins": "Plugins", "mgmt_db": "Management", "metrics": "Management",
	"metadata_store": "Metadata", "metadata_store_ets": "Metadata", "mnesia": "Metadata",
	"other_proc": "Other", "other_ets": "Other", "other_system": "Other", "code": "Other", "atom": "Other",
	"allocated_unused": "Allocator", "reserved_unallocated": "Allocator",
}

// Node reads one node and its memory breakdown.
func (c *Client) Node(ctx context.Context, name string) (*NodeDetail, error) {
	var data []byte
	if err := c.Get(ctx, "/api/nodes/"+Esc(name), &data); err != nil {
		return nil, err
	}
	var r nodeRaw
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	d := &NodeDetail{Node: r.info(), Memory: []MemoryPart{}, Raw: StripSamples(data)}
	var mem struct {
		Memory map[string]json.RawMessage `json:"memory"`
	}
	if r.Running && c.Get(ctx, "/api/nodes/"+Esc(name)+"/memory", &mem) == nil {
		var total map[string]float64
		_ = json.Unmarshal(mem.Memory["total"], &total)
		_ = json.Unmarshal(mem.Memory["strategy"], &d.MemoryStrategy)
		d.MemoryTotal = total[d.MemoryStrategy]
		if d.MemoryTotal == 0 {
			d.MemoryTotal = total["rss"]
		}
		for k, v := range mem.Memory {
			if k == "total" || k == "strategy" {
				continue
			}
			var b float64
			if json.Unmarshal(v, &b) != nil {
				continue
			}
			g := memoryGroups[k]
			if g == "" {
				g = "Other"
				if strings.Contains(k, "queue") {
					g = "Queues"
				}
			}
			d.Memory = append(d.Memory, MemoryPart{Category: k, Group: g, Bytes: b})
		}
	}
	return d, nil
}
