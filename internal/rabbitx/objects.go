package rabbitx

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"sort"
	"strings"

	"github.com/wankdanker/rabmin/internal/store"
)

// rawOrEmpty returns {} for missing or [] arguments (RabbitMQ sends [] for
// empty objects in some places).
func rawOrEmpty(r json.RawMessage) json.RawMessage {
	if len(r) == 0 || string(r) == "[]" || string(r) == "null" {
		return json.RawMessage("{}")
	}
	return r
}

// --- virtual hosts ---

// VHost is a virtual host with its limits and message counts.
type VHost struct {
	Name             string      `json:"name"`
	Description      string      `json:"description"`
	Tags             []string    `json:"tags"`
	DefaultQueueType string      `json:"defaultQueueType"`
	Protected        bool        `json:"protected" doc:"Deletion protection"`
	Tracing          bool        `json:"tracing"`
	Messages         float64     `json:"messages"`
	Ready            float64     `json:"ready"`
	Unacked          float64     `json:"unacked"`
	PublishRate      float64     `json:"publishRate"`
	DeliverRate      float64     `json:"deliverRate"`
	MaxConnections   *int        `json:"maxConnections,omitempty" doc:"Limit; absent when unlimited"`
	MaxQueues        *int        `json:"maxQueues,omitempty" doc:"Limit; absent when unlimited"`
	NodeStates       []NodeState `json:"nodeStates" doc:"Whether the vhost runs on each node"`
}

// NodeState is the state of something on one node.
type NodeState struct {
	Node  string `json:"node"`
	State string `json:"state"`
}

type vhostRaw struct {
	Name                  string            `json:"name"`
	Description           string            `json:"description"`
	Tags                  []string          `json:"tags"`
	DefaultQueueType      string            `json:"default_queue_type"`
	ProtectedFromDeletion bool              `json:"protected_from_deletion"`
	Tracing               bool              `json:"tracing"`
	Messages              float64           `json:"messages"`
	MessagesReady         float64           `json:"messages_ready"`
	MessagesUnacked       float64           `json:"messages_unacknowledged"`
	MessageStats          stats             `json:"message_stats"`
	ClusterState          map[string]string `json:"cluster_state"`
}

type limitsRaw struct {
	VHost string         `json:"vhost"`
	User  string         `json:"user"`
	Value map[string]int `json:"value"`
}

func intPtr(m map[string]int, k string) *int {
	if v, ok := m[k]; ok && v >= 0 {
		return &v
	}
	return nil
}

// VHosts lists virtual hosts with their limits.
func (c *Client) VHosts(ctx context.Context) ([]VHost, error) {
	var raw []vhostRaw
	if err := c.Get(ctx, "/api/vhosts?"+msgRates, &raw); err != nil {
		return nil, err
	}
	var lim []limitsRaw
	_ = c.Get(ctx, "/api/vhost-limits", &lim)
	limits := map[string]map[string]int{}
	for _, l := range lim {
		limits[l.VHost] = l.Value
	}
	out := make([]VHost, 0, len(raw))
	for _, v := range raw {
		x := VHost{Name: v.Name, Description: v.Description, Tags: v.Tags, DefaultQueueType: v.DefaultQueueType, Protected: v.ProtectedFromDeletion,
			Tracing: v.Tracing, Messages: v.Messages, Ready: v.MessagesReady, Unacked: v.MessagesUnacked,
			PublishRate: v.MessageStats.rate("publish"), DeliverRate: v.MessageStats.rate("deliver_get"),
			MaxConnections: intPtr(limits[v.Name], "max-connections"), MaxQueues: intPtr(limits[v.Name], "max-queues"), NodeStates: []NodeState{}}
		if x.Tags == nil {
			x.Tags = []string{}
		}
		for n, s := range v.ClusterState {
			x.NodeStates = append(x.NodeStates, NodeState{Node: n, State: s})
		}
		sort.Slice(x.NodeStates, func(i, j int) bool { return x.NodeStates[i].Node < x.NodeStates[j].Node })
		out = append(out, x)
	}
	return out, nil
}

// VHostSpec creates or updates a virtual host.
type VHostSpec struct {
	Description      string   `json:"description"`
	Tags             []string `json:"tags"`
	DefaultQueueType string   `json:"default_queue_type,omitempty"`
}

// PutVHost creates a vhost or updates its metadata.
func (c *Client) PutVHost(ctx context.Context, name string, spec VHostSpec) error {
	if spec.Tags == nil {
		spec.Tags = []string{}
	}
	return c.Do(ctx, http.MethodPut, "/api/vhosts/"+Esc(name), spec, nil)
}

// DeleteVHost deletes a vhost and everything in it.
func (c *Client) DeleteVHost(ctx context.Context, name string) error {
	return c.Do(ctx, http.MethodDelete, "/api/vhosts/"+Esc(name), nil, nil)
}

// SetVHostProtection turns deletion protection on or off.
func (c *Client) SetVHostProtection(ctx context.Context, name string, on bool) error {
	m := http.MethodDelete
	if on {
		m = http.MethodPost
	}
	return c.Do(ctx, m, "/api/vhosts/"+Esc(name)+"/deletion/protection", nil, nil)
}

// StartVHost starts (or restarts) a vhost on one node.
func (c *Client) StartVHost(ctx context.Context, name, node string) error {
	return c.Do(ctx, http.MethodPost, "/api/vhosts/"+Esc(name)+"/start/"+Esc(node), nil, nil)
}

// SetLimit sets a vhost or user limit (kind "vhost-limits" or "user-limits");
// a nil value removes it.
func (c *Client) SetLimit(ctx context.Context, kind, owner, limit string, value *int) error {
	path := "/api/" + kind + "/" + Esc(owner) + "/" + Esc(limit)
	if value == nil {
		err := c.Do(ctx, http.MethodDelete, path, nil, nil)
		if ae, ok := err.(*APIError); ok && ae.Status == http.StatusNotFound {
			return nil
		}
		return err
	}
	return c.Do(ctx, http.MethodPut, path, map[string]int{"value": *value}, nil)
}

// --- exchanges and bindings ---

// Exchange is an exchange.
type Exchange struct {
	VHost      string          `json:"vhost"`
	Name       string          `json:"name" doc:"Empty for the default exchange, which paths address as amq.default"`
	Type       string          `json:"type"`
	Durable    bool            `json:"durable"`
	AutoDelete bool            `json:"autoDelete"`
	Internal   bool            `json:"internal"`
	Arguments  json.RawMessage `json:"arguments"`
	Policy     string          `json:"policy,omitempty"`
	RateIn     float64         `json:"rateIn"`
	RateOut    float64         `json:"rateOut"`
}

type exchangeRaw struct {
	VHost        string          `json:"vhost"`
	Name         string          `json:"name"`
	Type         string          `json:"type"`
	Durable      bool            `json:"durable"`
	AutoDelete   bool            `json:"auto_delete"`
	Internal     bool            `json:"internal"`
	Arguments    json.RawMessage `json:"arguments"`
	Policy       json.RawMessage `json:"policy"`
	MessageStats stats           `json:"message_stats"`
}

func (e exchangeRaw) exchange() Exchange {
	return Exchange{VHost: e.VHost, Name: e.Name, Type: e.Type, Durable: e.Durable, AutoDelete: e.AutoDelete, Internal: e.Internal,
		Arguments: rawOrEmpty(e.Arguments), Policy: str(e.Policy), RateIn: e.MessageStats.rate("publish_in"), RateOut: e.MessageStats.rate("publish_out")}
}

const exchangeColumns = "vhost,name,type,durable,auto_delete,internal,arguments,policy," +
	"message_stats.publish_in_details.avg_rate,message_stats.publish_out_details.avg_rate"

// ExchangePath returns the path segment for an exchange name.
func ExchangePath(name string) string {
	if name == "" {
		return "amq.default"
	}
	return Esc(name)
}

func vhostPath(vhost string) string {
	if vhost == "" {
		return ""
	}
	return "/" + Esc(vhost)
}

// Exchanges lists exchanges, in one vhost when vhost is set.
func (c *Client) Exchanges(ctx context.Context, vhost string) ([]Exchange, error) {
	var raw []exchangeRaw
	if err := c.Get(ctx, "/api/exchanges"+vhostPath(vhost)+"?"+msgRates+"&columns="+exchangeColumns, &raw); err != nil {
		return nil, err
	}
	out := make([]Exchange, 0, len(raw))
	for _, e := range raw {
		out = append(out, e.exchange())
	}
	return out, nil
}

// ExchangeDetail is an exchange with its bindings.
type ExchangeDetail struct {
	Exchange Exchange `json:"exchange"`
	// Source are the bindings from this exchange; Destination those to it.
	Source      []Binding       `json:"source"`
	Destination []Binding       `json:"destination"`
	Raw         json.RawMessage `json:"raw"`
}

// Exchange reads one exchange and its bindings.
func (c *Client) Exchange(ctx context.Context, vhost, name string) (*ExchangeDetail, error) {
	base := "/api/exchanges/" + Esc(vhost) + "/" + ExchangePath(name)
	var data []byte
	if err := c.Get(ctx, base+"?"+msgRates, &data); err != nil {
		return nil, err
	}
	var r exchangeRaw
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	d := &ExchangeDetail{Exchange: r.exchange(), Source: []Binding{}, Destination: []Binding{}, Raw: StripSamples(data)}
	if name != "" {
		var src, dst []bindingRaw
		if err := c.Get(ctx, base+"/bindings/source", &src); err != nil {
			return nil, err
		}
		if err := c.Get(ctx, base+"/bindings/destination", &dst); err != nil {
			return nil, err
		}
		for _, b := range src {
			d.Source = append(d.Source, b.binding())
		}
		for _, b := range dst {
			d.Destination = append(d.Destination, b.binding())
		}
	}
	return d, nil
}

// ExchangeSpec declares an exchange.
type ExchangeSpec struct {
	Type       string         `json:"type"`
	Durable    bool           `json:"durable"`
	AutoDelete bool           `json:"auto_delete"`
	Internal   bool           `json:"internal"`
	Arguments  map[string]any `json:"arguments"`
}

// PutExchange declares an exchange.
func (c *Client) PutExchange(ctx context.Context, vhost, name string, spec ExchangeSpec) error {
	if spec.Arguments == nil {
		spec.Arguments = map[string]any{}
	}
	return c.Do(ctx, http.MethodPut, "/api/exchanges/"+Esc(vhost)+"/"+Esc(name), spec, nil)
}

// DeleteExchange deletes an exchange, refusing when bound and ifUnused is set.
func (c *Client) DeleteExchange(ctx context.Context, vhost, name string, ifUnused bool) error {
	q := ""
	if ifUnused {
		q = "?if-unused=true"
	}
	return c.Do(ctx, http.MethodDelete, "/api/exchanges/"+Esc(vhost)+"/"+Esc(name)+q, nil, nil)
}

// Binding binds a source exchange to a queue or exchange.
type Binding struct {
	VHost           string          `json:"vhost"`
	Source          string          `json:"source" doc:"Empty for the default exchange"`
	Destination     string          `json:"destination"`
	DestinationType string          `json:"destinationType" enum:"queue,exchange"`
	RoutingKey      string          `json:"routingKey"`
	Arguments       json.RawMessage `json:"arguments"`
	PropertiesKey   string          `json:"propertiesKey" doc:"Identifies the binding for deletion"`
}

type bindingRaw struct {
	VHost           string          `json:"vhost"`
	Source          string          `json:"source"`
	Destination     string          `json:"destination"`
	DestinationType string          `json:"destination_type"`
	RoutingKey      string          `json:"routing_key"`
	Arguments       json.RawMessage `json:"arguments"`
	PropertiesKey   string          `json:"properties_key"`
}

func (b bindingRaw) binding() Binding {
	return Binding{VHost: b.VHost, Source: b.Source, Destination: b.Destination, DestinationType: b.DestinationType,
		RoutingKey: b.RoutingKey, Arguments: rawOrEmpty(b.Arguments), PropertiesKey: b.PropertiesKey}
}

// Bindings lists bindings, in one vhost when vhost is set.
func (c *Client) Bindings(ctx context.Context, vhost string) ([]Binding, error) {
	var raw []bindingRaw
	if err := c.Get(ctx, "/api/bindings"+vhostPath(vhost), &raw); err != nil {
		return nil, err
	}
	out := make([]Binding, 0, len(raw))
	for _, b := range raw {
		out = append(out, b.binding())
	}
	return out, nil
}

func destLetter(destType string) (string, error) {
	switch destType {
	case "queue", "q":
		return "q", nil
	case "exchange", "e":
		return "e", nil
	}
	return "", fmt.Errorf("destination type must be queue or exchange, not %q", destType)
}

// AddBinding creates a binding and returns its properties key.
func (c *Client) AddBinding(ctx context.Context, vhost, source, destType, dest, routingKey string, args map[string]any) error {
	d, err := destLetter(destType)
	if err != nil {
		return err
	}
	if args == nil {
		args = map[string]any{}
	}
	return c.Do(ctx, http.MethodPost, "/api/bindings/"+Esc(vhost)+"/e/"+Esc(source)+"/"+d+"/"+Esc(dest),
		map[string]any{"routing_key": routingKey, "arguments": args}, nil)
}

// DeleteBinding deletes one binding by its properties key.
func (c *Client) DeleteBinding(ctx context.Context, vhost, source, destType, dest, propsKey string) error {
	d, err := destLetter(destType)
	if err != nil {
		return err
	}
	// The properties key is already URL-encoded; the API expects it encoded once more.
	return c.Do(ctx, http.MethodDelete, "/api/bindings/"+Esc(vhost)+"/e/"+Esc(source)+"/"+d+"/"+Esc(dest)+"/"+url.PathEscape(propsKey), nil, nil)
}

// --- queues ---

// Queue is a classic, quorum or stream queue.
type Queue struct {
	VHost            string          `json:"vhost"`
	Name             string          `json:"name"`
	Type             string          `json:"type" doc:"classic, quorum or stream"`
	State            string          `json:"state"`
	Durable          bool            `json:"durable"`
	AutoDelete       bool            `json:"autoDelete"`
	Exclusive        bool            `json:"exclusive"`
	Arguments        json.RawMessage `json:"arguments"`
	Policy           string          `json:"policy,omitempty"`
	OperatorPolicy   string          `json:"operatorPolicy,omitempty"`
	Messages         float64         `json:"messages"`
	Ready            float64         `json:"ready"`
	Unacked          float64         `json:"unacked"`
	MessageBytes     float64         `json:"messageBytes"`
	Memory           float64         `json:"memory"`
	Consumers        int             `json:"consumers"`
	ConsumerCapacity float64         `json:"consumerCapacity" doc:"0..1: how often the queue could deliver to consumers immediately"`
	PublishRate      float64         `json:"publishRate"`
	DeliverRate      float64         `json:"deliverRate"`
	AckRate          float64         `json:"ackRate"`
	Node             string          `json:"node,omitempty"`
	Leader           string          `json:"leader,omitempty"`
	Members          []string        `json:"members"`
	Online           []string        `json:"online"`
	DeliveryLimit    *int            `json:"deliveryLimit,omitempty"`
}

type queueFullRaw struct {
	Name                   string          `json:"name"`
	VHost                  string          `json:"vhost"`
	Type                   string          `json:"type"`
	State                  string          `json:"state"`
	Durable                bool            `json:"durable"`
	AutoDelete             bool            `json:"auto_delete"`
	Exclusive              bool            `json:"exclusive"`
	Arguments              json.RawMessage `json:"arguments"`
	Policy                 json.RawMessage `json:"policy"`
	OperatorPolicy         json.RawMessage `json:"operator_policy"`
	Messages               float64         `json:"messages"`
	MessagesReady          float64         `json:"messages_ready"`
	MessagesUnacknowledged float64         `json:"messages_unacknowledged"`
	MessageBytes           float64         `json:"message_bytes"`
	Memory                 float64         `json:"memory"`
	Consumers              int             `json:"consumers"`
	ConsumerCapacity       float64         `json:"consumer_capacity"`
	Node                   string          `json:"node"`
	Leader                 string          `json:"leader"`
	Members                []string        `json:"members"`
	Online                 []string        `json:"online"`
	DeliveryLimit          json.RawMessage `json:"delivery_limit"`
	MessageStats           stats           `json:"message_stats"`
}

func (q queueFullRaw) queue() Queue {
	x := Queue{VHost: q.VHost, Name: q.Name, Type: q.Type, State: q.State, Durable: q.Durable, AutoDelete: q.AutoDelete, Exclusive: q.Exclusive,
		Arguments: rawOrEmpty(q.Arguments), Policy: str(q.Policy), OperatorPolicy: str(q.OperatorPolicy), Messages: q.Messages,
		Ready: q.MessagesReady, Unacked: q.MessagesUnacknowledged, MessageBytes: q.MessageBytes, Memory: q.Memory, Consumers: q.Consumers,
		ConsumerCapacity: q.ConsumerCapacity, Node: q.Node, Leader: q.Leader, Members: q.Members, Online: q.Online,
		PublishRate: q.MessageStats.rate("publish"), DeliverRate: q.MessageStats.rate("deliver_get"), AckRate: q.MessageStats.rate("ack")}
	if x.Members == nil {
		x.Members = []string{}
	}
	if x.Online == nil {
		x.Online = []string{}
	}
	var dl int
	if json.Unmarshal(q.DeliveryLimit, &dl) == nil && len(q.DeliveryLimit) > 0 {
		x.DeliveryLimit = &dl
	}
	if x.State == "" {
		x.State = "running"
	}
	return x
}

const queueListColumns = "name,vhost,type,state,durable,auto_delete,exclusive,arguments,policy,operator_policy,messages,messages_ready," +
	"messages_unacknowledged,message_bytes,memory,consumers,consumer_capacity,node,leader,members,online,delivery_limit," +
	"message_stats.publish_details.avg_rate,message_stats.deliver_get_details.avg_rate,message_stats.ack_details.avg_rate"

// Queues lists queues, in one vhost when vhost is set.
func (c *Client) Queues(ctx context.Context, vhost string) ([]Queue, error) {
	var raw []queueFullRaw
	if err := c.Get(ctx, "/api/queues"+vhostPath(vhost)+"?"+msgRates+"&columns="+queueListColumns, &raw); err != nil {
		return nil, err
	}
	out := make([]Queue, 0, len(raw))
	for _, q := range raw {
		out = append(out, q.queue())
	}
	return out, nil
}

// QueueDetail is one queue with its effective policy, consumers and bindings.
type QueueDetail struct {
	Queue              Queue           `json:"queue"`
	EffectivePolicy    json.RawMessage `json:"effectivePolicy" doc:"Policy and operator policy keys in effect"`
	MessagesPersistent float64         `json:"messagesPersistent"`
	MessagesRAM        float64         `json:"messagesRam"`
	BytesReady         float64         `json:"bytesReady"`
	BytesUnacked       float64         `json:"bytesUnacked"`
	Consumers          []Consumer      `json:"consumers"`
	Bindings           []Binding       `json:"bindings"`
	Raw                json.RawMessage `json:"raw"`
}

// Queue reads one queue with its consumers and bindings.
func (c *Client) Queue(ctx context.Context, vhost, name string) (*QueueDetail, error) {
	base := "/api/queues/" + Esc(vhost) + "/" + Esc(name)
	var data []byte
	if err := c.Get(ctx, base+"?"+msgRates, &data); err != nil {
		return nil, err
	}
	var r struct {
		queueFullRaw
		EffectivePolicy    json.RawMessage `json:"effective_policy_definition"`
		MessagesPersistent float64         `json:"messages_persistent"`
		MessagesRAM        float64         `json:"messages_ram"`
		BytesReady         float64         `json:"message_bytes_ready"`
		BytesUnacked       float64         `json:"message_bytes_unacknowledged"`
		ConsumerDetails    []consumerRaw   `json:"consumer_details"`
	}
	if err := json.Unmarshal(data, &r); err != nil {
		return nil, err
	}
	d := &QueueDetail{Queue: r.queue(), EffectivePolicy: rawOrEmpty(r.EffectivePolicy), MessagesPersistent: r.MessagesPersistent,
		MessagesRAM: r.MessagesRAM, BytesReady: r.BytesReady, BytesUnacked: r.BytesUnacked, Consumers: []Consumer{}, Bindings: []Binding{},
		Raw: StripSamples(data)}
	names := c.clientNames(ctx)
	for _, cr := range r.ConsumerDetails {
		x := cr.consumer()
		x.ClientName = names[x.Connection]
		d.Consumers = append(d.Consumers, x)
	}
	var bs []bindingRaw
	if err := c.Get(ctx, base+"/bindings", &bs); err == nil {
		for _, b := range bs {
			d.Bindings = append(d.Bindings, b.binding())
		}
	}
	return d, nil
}

// QueueSpec declares a queue.
type QueueSpec struct {
	Durable    bool           `json:"durable"`
	AutoDelete bool           `json:"auto_delete"`
	Arguments  map[string]any `json:"arguments"`
	Node       string         `json:"node,omitempty"`
}

// PutQueue declares a queue. Re-declaring with different arguments fails.
func (c *Client) PutQueue(ctx context.Context, vhost, name string, spec QueueSpec) error {
	if spec.Arguments == nil {
		spec.Arguments = map[string]any{}
	}
	return c.Do(ctx, http.MethodPut, "/api/queues/"+Esc(vhost)+"/"+Esc(name), spec, nil)
}

// DeleteQueue deletes a queue, optionally only when empty or unused.
func (c *Client) DeleteQueue(ctx context.Context, vhost, name string, ifEmpty, ifUnused bool) error {
	q := url.Values{}
	if ifEmpty {
		q.Set("if-empty", "true")
	}
	if ifUnused {
		q.Set("if-unused", "true")
	}
	path := "/api/queues/" + Esc(vhost) + "/" + Esc(name)
	if len(q) > 0 {
		path += "?" + q.Encode()
	}
	return c.Do(ctx, http.MethodDelete, path, nil, nil)
}

// PurgeQueue removes every ready message from a queue.
func (c *Client) PurgeQueue(ctx context.Context, vhost, name string) error {
	return c.Do(ctx, http.MethodDelete, "/api/queues/"+Esc(vhost)+"/"+Esc(name)+"/contents", nil, nil)
}

// --- publish ---

// PublishProps are AMQP 0-9-1 message properties.
type PublishProps struct {
	ContentType     string         `json:"contentType,omitempty"`
	ContentEncoding string         `json:"contentEncoding,omitempty"`
	DeliveryMode    int            `json:"deliveryMode,omitempty" minimum:"0" maximum:"2" doc:"1 transient, 2 persistent"`
	Priority        *int           `json:"priority,omitempty" minimum:"0" maximum:"255"`
	CorrelationID   string         `json:"correlationId,omitempty"`
	ReplyTo         string         `json:"replyTo,omitempty"`
	Expiration      string         `json:"expiration,omitempty" doc:"Per-message TTL in milliseconds, as a string"`
	MessageID       string         `json:"messageId,omitempty"`
	Timestamp       int64          `json:"timestamp,omitempty" doc:"Unix seconds"`
	Type            string         `json:"type,omitempty"`
	UserID          string         `json:"userId,omitempty"`
	AppID           string         `json:"appId,omitempty"`
	Headers         map[string]any `json:"headers,omitempty"`
}

func (p PublishProps) raw() map[string]any {
	m := map[string]any{}
	set := func(k, v string) {
		if v != "" {
			m[k] = v
		}
	}
	set("content_type", p.ContentType)
	set("content_encoding", p.ContentEncoding)
	set("correlation_id", p.CorrelationID)
	set("reply_to", p.ReplyTo)
	set("expiration", p.Expiration)
	set("message_id", p.MessageID)
	set("type", p.Type)
	set("user_id", p.UserID)
	set("app_id", p.AppID)
	if p.DeliveryMode > 0 {
		m["delivery_mode"] = p.DeliveryMode
	}
	if p.Priority != nil {
		m["priority"] = *p.Priority
	}
	if p.Timestamp > 0 {
		m["timestamp"] = p.Timestamp
	}
	if len(p.Headers) > 0 {
		m["headers"] = p.Headers
	}
	return m
}

// Publish publishes one message through the management API and reports
// whether it was routed to at least one queue.
func (c *Client) Publish(ctx context.Context, vhost, exchange, routingKey, payload, encoding string, props PublishProps) (bool, error) {
	if encoding == "" {
		encoding = "string"
	}
	var out struct {
		Routed bool `json:"routed"`
	}
	body := map[string]any{"routing_key": routingKey, "payload": payload, "payload_encoding": encoding, "properties": props.raw()}
	err := c.Do(ctx, http.MethodPost, "/api/exchanges/"+Esc(vhost)+"/"+ExchangePath(exchange)+"/publish", body, &out)
	return out.Routed, err
}

// WithCredential returns a client for the same URLs that authenticates as cred.
func (c *Client) WithCredential(cred store.Credential) *Client {
	return &Client{urls: c.urls, hc: c.hc, cred: cred}
}

// splitTags normalises tags given as a list or comma-separated string.
func splitTags(in []string) []string {
	out := []string{}
	for _, t := range in {
		for _, p := range strings.Split(t, ",") {
			if p = strings.TrimSpace(p); p != "" {
				out = append(out, p)
			}
		}
	}
	return out
}
