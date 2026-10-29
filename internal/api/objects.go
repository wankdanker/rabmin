package api

import (
	"context"
	"encoding/base64"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/wankdanker/rabmin/internal/rabbitx"
	"github.com/wankdanker/rabmin/internal/store"
)

var (
	tagVHosts    = []string{"Virtual hosts"}
	tagExchanges = []string{"Exchanges and bindings"}
	tagQueues    = []string{"Queues"}
)

// VHostPath identifies a virtual host.
type VHostPath struct {
	Cluster string `path:"cluster" doc:"Cluster id"`
	VHost   string `path:"vhost" doc:"Virtual host (URL-encoded; / is %2F)"`
}

// ObjectPath identifies an exchange or queue.
type ObjectPath struct {
	Cluster string `path:"cluster" doc:"Cluster id"`
	VHost   string `path:"vhost" doc:"Virtual host (URL-encoded; / is %2F)"`
	Name    string `path:"name" doc:"Name (URL-encoded); the default exchange is amq.default"`
}

func exchangeName(path string) string {
	if path == "amq.default" {
		return ""
	}
	return path
}

// ExchangesPage is a page of exchanges.
type ExchangesPage struct {
	PageInfo
	Items []rabbitx.Exchange `json:"items"`
}

// QueuesPage is a page of queues.
type QueuesPage struct {
	PageInfo
	Items []rabbitx.Queue `json:"items"`
}

// VHostIn creates or updates a virtual host.
type VHostIn struct {
	Description      string   `json:"description,omitempty" maxLength:"1000"`
	Tags             []string `json:"tags,omitempty"`
	DefaultQueueType string   `json:"defaultQueueType,omitempty" enum:"classic,quorum,stream" doc:"Queue type for declarations that do not name one"`
	Protected        *bool    `json:"protected,omitempty" doc:"Deletion protection"`
	MaxConnections   *int     `json:"maxConnections,omitempty" minimum:"-1" doc:"-1 removes the limit"`
	MaxQueues        *int     `json:"maxQueues,omitempty" minimum:"-1" doc:"-1 removes the limit"`
}

// limitValue turns -1 into "remove".
func limitValue(v *int) *int {
	if v != nil && *v < 0 {
		return nil
	}
	return v
}

// QueueIn declares a queue.
type QueueIn struct {
	Type       string         `json:"type" enum:"classic,quorum,stream"`
	Durable    *bool          `json:"durable,omitempty" doc:"Defaults to true. RabbitMQ 4.x refuses non-durable, non-exclusive queues unless the deprecated transient_nonexcl_queues feature is permitted."`
	AutoDelete bool           `json:"autoDelete,omitempty"`
	Arguments  map[string]any `json:"arguments,omitempty" doc:"x-arguments other than x-queue-type"`
	Node       string         `json:"node,omitempty" doc:"Node for a classic queue, or the leader of a quorum queue or stream"`
}

// BindingIn creates a binding.
type BindingIn struct {
	VHost           string         `json:"vhost" minLength:"1"`
	Source          string         `json:"source" minLength:"1" doc:"Source exchange (the default exchange cannot be bound)"`
	Destination     string         `json:"destination" minLength:"1"`
	DestinationType string         `json:"destinationType" enum:"queue,exchange"`
	RoutingKey      string         `json:"routingKey,omitempty"`
	Arguments       map[string]any `json:"arguments,omitempty" doc:"Headers exchange matching uses x-match: all|any|all-with-x|any-with-x plus header values"`
}

// PublishIn publishes one message.
type PublishIn struct {
	RoutingKey      string               `json:"routingKey,omitempty"`
	Payload         string               `json:"payload"`
	PayloadEncoding string               `json:"payloadEncoding,omitempty" enum:"string,base64" doc:"Defaults to string"`
	Properties      rabbitx.PublishProps `json:"properties,omitempty"`
	Credential      string               `json:"credential,omitempty" doc:"Publish as this credential profile instead of the management user"`
}

// PublishOut reports whether a published message reached a queue.
type PublishOut struct {
	Routed bool `json:"routed" doc:"False when no binding matched; the message was dropped or went to an alternate exchange's queues only if one is configured"`
}

// clientAs returns the cluster's client, or one authenticating as a
// credential profile.
func (s *Server) clientAs(ctx context.Context, cluster, credential string) (*rabbitx.Client, error) {
	cl, err := s.client(cluster)
	if err != nil || credential == "" {
		return cl, err
	}
	p, err := s.store.GetCredential(ctx, cluster, credential)
	if err != nil {
		return nil, apiErr(err)
	}
	return cl.WithCredential(p.Auth), nil
}

func (s *Server) registerObjects() {
	// --- virtual hosts ---
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/vhosts", id: "list-vhosts", summary: "List virtual hosts", tags: tagVHosts, role: store.RoleViewer},
		func(ctx context.Context, in *ClusterPath) (*struct{ Body []rabbitx.VHost }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			vs, err := cl.VHosts(ctx)
			return &struct{ Body []rabbitx.VHost }{vs}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/vhosts/{vhost}", id: "put-vhost", summary: "Create or update a virtual host",
		desc: "Creates the vhost or updates its description, tags and default queue type, then applies deletion protection and limits when given.",
		tags: tagVHosts, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			VHostPath
			Body VHostIn
		}) (*struct{ Body rabbitx.VHost }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			b := in.Body
			if b.DefaultQueueType == "" {
				// Without default_queue_type, RabbitMQ resets an existing vhost to classic.
				if vs, err := cl.VHosts(ctx); err == nil {
					for _, v := range vs {
						if v.Name == in.VHost {
							b.DefaultQueueType = v.DefaultQueueType
						}
					}
				}
			}
			if err := cl.PutVHost(ctx, in.VHost, rabbitx.VHostSpec{Description: b.Description, Tags: b.Tags, DefaultQueueType: b.DefaultQueueType}); err != nil {
				return nil, apiErr(err)
			}
			if b.Protected != nil {
				if err := cl.SetVHostProtection(ctx, in.VHost, *b.Protected); err != nil {
					return nil, apiErr(err)
				}
			}
			if b.MaxConnections != nil {
				if err := cl.SetLimit(ctx, "vhost-limits", in.VHost, "max-connections", limitValue(b.MaxConnections)); err != nil {
					return nil, apiErr(err)
				}
			}
			if b.MaxQueues != nil {
				if err := cl.SetLimit(ctx, "vhost-limits", in.VHost, "max-queues", limitValue(b.MaxQueues)); err != nil {
					return nil, apiErr(err)
				}
			}
			// Reads right after a write can briefly return the old state on
			// another node, so wait (a little) for our changes to show.
			applied := func(v rabbitx.VHost) bool {
				return (b.Protected == nil || v.Protected == *b.Protected) &&
					(b.MaxQueues == nil || sameLimit(v.MaxQueues, limitValue(b.MaxQueues))) &&
					(b.MaxConnections == nil || sameLimit(v.MaxConnections, limitValue(b.MaxConnections)))
			}
			var found *rabbitx.VHost
			for i := 0; i < 10; i++ {
				vs, err := cl.VHosts(ctx)
				if err != nil {
					return nil, apiErr(err)
				}
				for _, v := range vs {
					if v.Name == in.VHost {
						found = &v
					}
				}
				if found != nil && applied(*found) {
					break
				}
				time.Sleep(200 * time.Millisecond)
			}
			if found == nil {
				return nil, huma.Error404NotFound("vhost not found after creating it")
			}
			return &struct{ Body rabbitx.VHost }{*found}, nil
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/vhosts/{vhost}", id: "delete-vhost", summary: "Delete a virtual host",
		desc: "Deletes the vhost with every queue, exchange, binding, policy and permission in it. Refused (409) while deletion protection is on.",
		tags: tagVHosts, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *VHostPath) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.DeleteVHost(ctx, in.VHost))
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/vhosts/{vhost}/start", id: "start-vhost", summary: "Start a virtual host on a node",
		desc: "Starts or restarts the vhost on one node, for example after the virtual-hosts health check failed there.", tags: tagVHosts, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			VHostPath
			Body struct {
				Node string `json:"node" minLength:"1"`
			}
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.StartVHost(ctx, in.VHost, in.Body.Node))
		})

	// --- exchanges ---
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/exchanges", id: "list-exchanges", summary: "List exchanges",
		desc: "Rates are averages over the last 30 seconds.", tags: tagExchanges, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			ListQuery
			Type     string `query:"type"`
			Internal bool   `query:"internal" doc:"Include amq.* and internal exchanges"`
		}) (*struct{ Body ExchangesPage }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			all, err := cl.Exchanges(ctx, in.VHost)
			if err != nil {
				return nil, apiErr(err)
			}
			kept := all[:0]
			for _, e := range all {
				if (in.Type != "" && e.Type != in.Type) || (!in.Internal && (e.Internal || strings.HasPrefix(e.Name, "amq."))) {
					continue
				}
				kept = append(kept, e)
			}
			items, info := filterPage(kept, in.ListQuery, func(e rabbitx.Exchange) (string, string, string, []string) {
				return e.VHost, "", "", []string{e.Name, e.Type, e.Policy}
			})
			return &struct{ Body ExchangesPage }{ExchangesPage{PageInfo: info, Items: items}}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/exchanges/{vhost}/{name}", id: "get-exchange", summary: "Exchange detail",
		desc: "The exchange with its bindings as source and as destination.", tags: tagExchanges, role: store.RoleViewer},
		func(ctx context.Context, in *ObjectPath) (*struct{ Body *rabbitx.ExchangeDetail }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			d, err := cl.Exchange(ctx, in.VHost, exchangeName(in.Name))
			return &struct{ Body *rabbitx.ExchangeDetail }{d}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/exchanges/{vhost}/{name}", id: "put-exchange", summary: "Declare an exchange",
		desc: "Declaring an existing exchange with different properties fails.", tags: tagExchanges, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ObjectPath
			Body struct {
				Type       string         `json:"type" minLength:"1" doc:"direct, fanout, topic, headers, or a plugin type from the cluster info"`
				Durable    *bool          `json:"durable,omitempty" doc:"Defaults to true"`
				AutoDelete bool           `json:"autoDelete,omitempty"`
				Internal   bool           `json:"internal,omitempty"`
				Arguments  map[string]any `json:"arguments,omitempty" doc:"For example alternate-exchange"`
			}
		}) (*struct{}, error) {
			if strings.HasPrefix(in.Name, "amq.") {
				return nil, huma.Error422UnprocessableEntity("exchange names starting with amq. are reserved")
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			b := in.Body
			durable := b.Durable == nil || *b.Durable
			return nil, apiErr(cl.PutExchange(ctx, in.VHost, in.Name, rabbitx.ExchangeSpec{Type: b.Type, Durable: durable, AutoDelete: b.AutoDelete,
				Internal: b.Internal, Arguments: b.Arguments}))
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/exchanges/{vhost}/{name}", id: "delete-exchange", summary: "Delete an exchange",
		tags: tagExchanges, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ObjectPath
			IfUnused bool `query:"ifUnused" doc:"Refuse (409) when the exchange has bindings"`
		}) (*struct{}, error) {
			if in.Name == "amq.default" || strings.HasPrefix(in.Name, "amq.") {
				return nil, huma.Error422UnprocessableEntity("built-in amq.* exchanges cannot be deleted")
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.DeleteExchange(ctx, in.VHost, in.Name, in.IfUnused))
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/exchanges/{vhost}/{name}/publish", id: "publish", summary: "Publish a message",
		desc: "Publishes one message through the management API and reports whether it was routed. Use amq.default to publish straight to a queue by name.",
		tags: tagExchanges, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			ObjectPath
			Body PublishIn
		}) (*struct{ Body PublishOut }, error) {
			b := in.Body
			if b.PayloadEncoding == "base64" {
				if _, err := base64.StdEncoding.DecodeString(b.Payload); err != nil {
					return nil, huma.Error422UnprocessableEntity("payload is not valid base64")
				}
			}
			cl, err := s.clientAs(ctx, in.Cluster, b.Credential)
			if err != nil {
				return nil, err
			}
			routed, err := cl.Publish(ctx, in.VHost, exchangeName(in.Name), b.RoutingKey, b.Payload, b.PayloadEncoding, b.Properties)
			if err != nil {
				return nil, apiErr(err)
			}
			s.audit(ctx, "publish", "cluster/"+in.Cluster+"/exchanges/"+in.VHost+"/"+in.Name, "routing key "+b.RoutingKey+", "+boolWord(routed, "routed", "unroutable"), nil)
			return &struct{ Body PublishOut }{PublishOut{Routed: routed}}, nil
		})

	// --- bindings ---
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/bindings", id: "list-bindings", summary: "List bindings", tags: tagExchanges, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			VHost       string `query:"vhost"`
			Source      string `query:"source"`
			Destination string `query:"destination"`
		}) (*struct{ Body []rabbitx.Binding }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			all, err := cl.Bindings(ctx, in.VHost)
			if err != nil {
				return nil, apiErr(err)
			}
			out := []rabbitx.Binding{}
			for _, b := range all {
				if (in.Source == "" || b.Source == exchangeName(in.Source)) && (in.Destination == "" || b.Destination == in.Destination) {
					out = append(out, b)
				}
			}
			return &struct{ Body []rabbitx.Binding }{out}, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/bindings", id: "create-binding", summary: "Bind an exchange to a queue or exchange",
		tags: tagExchanges, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body BindingIn
		}) (*struct{}, error) {
			b := in.Body
			if b.Source == "amq.default" {
				return nil, huma.Error422UnprocessableEntity("the default exchange binds every queue implicitly and cannot be bound explicitly")
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.AddBinding(ctx, b.VHost, b.Source, b.DestinationType, b.Destination, b.RoutingKey, b.Arguments))
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/bindings/{vhost}/{source}/{destinationType}/{destination}/{propertiesKey}", id: "delete-binding",
		summary: "Delete a binding", desc: "propertiesKey is the binding's propertiesKey exactly as listed, URL-encoded as a path segment like the other parameters.", tags: tagExchanges,
		role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Cluster         string `path:"cluster"`
			VHost           string `path:"vhost"`
			Source          string `path:"source"`
			DestinationType string `path:"destinationType" enum:"queue,exchange"`
			Destination     string `path:"destination"`
			PropertiesKey   string `path:"propertiesKey"`
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.DeleteBinding(ctx, in.VHost, in.Source, in.DestinationType, in.Destination, in.PropertiesKey))
		})

	// --- queues ---
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/queues", id: "list-queues", summary: "List queues and streams",
		desc: "Filter by name substring (q) or regular expression (regex), vhost, type, state and policy. Rates are averages over the last 30 seconds.",
		tags: tagQueues, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			ListQuery
			Regex  string `query:"regex" doc:"Regular expression matched against the queue name"`
			Type   string `query:"type" doc:"classic, quorum or stream"`
			State  string `query:"state" doc:"running, idle, down, crashed, minority..."`
			Policy string `query:"policy" doc:"Policy or operator policy name; - for queues without a policy"`
		}) (*struct{ Body QueuesPage }, error) {
			var re *regexp.Regexp
			if in.Regex != "" {
				var err error
				if re, err = regexp.Compile(in.Regex); err != nil {
					return nil, huma.Error422UnprocessableEntity("regex: " + err.Error())
				}
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			all, err := cl.Queues(ctx, in.VHost)
			if err != nil {
				return nil, apiErr(err)
			}
			kept := all[:0]
			for _, q := range all {
				if (in.Type != "" && q.Type != in.Type) || (in.State != "" && q.State != in.State) || (re != nil && !re.MatchString(q.Name)) {
					continue
				}
				if in.Policy == "-" && (q.Policy != "" || q.OperatorPolicy != "") {
					continue
				}
				if in.Policy != "" && in.Policy != "-" && q.Policy != in.Policy && q.OperatorPolicy != in.Policy {
					continue
				}
				kept = append(kept, q)
			}
			items, info := filterPage(kept, in.ListQuery, func(q rabbitx.Queue) (string, string, string, []string) {
				node := q.Node
				if q.Leader != "" {
					node = q.Leader
				}
				return q.VHost, "", node, []string{q.Name}
			})
			return &struct{ Body QueuesPage }{QueuesPage{PageInfo: info, Items: items}}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/queues/{vhost}/{name}", id: "get-queue", summary: "Queue detail",
		desc: "Features, effective policy, state, members, message counts and bytes, rates, consumers and bindings.", tags: tagQueues, role: store.RoleViewer},
		func(ctx context.Context, in *ObjectPath) (*struct{ Body *rabbitx.QueueDetail }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			d, err := cl.Queue(ctx, in.VHost, in.Name)
			return &struct{ Body *rabbitx.QueueDetail }{d}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/queues/{vhost}/{name}", id: "put-queue", summary: "Declare a queue or stream",
		desc: "Queues cannot be changed after they are declared: declaring an existing queue with other arguments fails (400 inequivalent arg). Use policies for changes.",
		tags: tagQueues, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ObjectPath
			Body QueueIn
		}) (*struct{}, error) {
			b := in.Body
			args := map[string]any{}
			for k, v := range b.Arguments {
				if k != "x-queue-type" {
					args[k] = v
				}
			}
			args["x-queue-type"] = b.Type
			durable := b.Durable == nil || *b.Durable
			if b.Type != "classic" && !durable {
				return nil, huma.Error422UnprocessableEntity(b.Type + " queues are always durable")
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.PutQueue(ctx, in.VHost, in.Name, rabbitx.QueueSpec{Durable: durable, AutoDelete: b.AutoDelete, Arguments: args, Node: b.Node}))
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/queues/{vhost}/{name}", id: "delete-queue", summary: "Delete a queue",
		desc: "Deletes the queue and its messages. ifEmpty and ifUnused make RabbitMQ refuse (409) a queue with messages or consumers.",
		tags: tagQueues, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ObjectPath
			IfEmpty  bool `query:"ifEmpty"`
			IfUnused bool `query:"ifUnused"`
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.DeleteQueue(ctx, in.VHost, in.Name, in.IfEmpty, in.IfUnused))
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/queues/{vhost}/{name}/purge", id: "purge-queue", summary: "Purge a queue",
		desc: "Removes every ready message. Unacknowledged messages stay. Streams cannot be purged.", tags: tagQueues, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *ObjectPath) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.PurgeQueue(ctx, in.VHost, in.Name))
		})
}

func sameLimit(a, b *int) bool {
	if a == nil || b == nil {
		return a == b
	}
	return *a == *b
}

func boolWord(b bool, yes, no string) string {
	if b {
		return yes
	}
	return no
}
