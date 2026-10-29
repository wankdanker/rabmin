package api

import (
	"context"
	"fmt"
	"net/http"
	"reflect"
	"sort"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/sse"

	"github.com/wankdanker/rabmin/internal/rabbitx"
	"github.com/wankdanker/rabmin/internal/store"
)

var (
	tagMonitor = []string{"Monitoring"}
	tagClients = []string{"Connections, channels and consumers"}
)

// ListQuery filters, sorts and pages a list. RabbitMQ's own filters only
// match object names, so rabmin filters the full list itself.
type ListQuery struct {
	Q        string `query:"q" doc:"Case-insensitive substring matched against names, client name, user, vhost and node"`
	VHost    string `query:"vhost"`
	User     string `query:"user"`
	Node     string `query:"node"`
	Sort     string `query:"sort" doc:"Field to sort by (a JSON field name of the items)"`
	Desc     bool   `query:"desc"`
	Page     int    `query:"page" minimum:"1" default:"1"`
	PageSize int    `query:"pageSize" minimum:"1" maximum:"2000" default:"100"`
}

// PageInfo describes one page of a filtered list.
type PageInfo struct {
	Total    int `json:"total" doc:"Items before filtering"`
	Filtered int `json:"filtered" doc:"Items matching the filters"`
	Page     int `json:"page"`
	PageSize int `json:"pageSize"`
}

// ConnectionsPage is a page of connections.
type ConnectionsPage struct {
	PageInfo
	Items []rabbitx.Connection `json:"items"`
}

// ChannelsPage is a page of channels.
type ChannelsPage struct {
	PageInfo
	Items []rabbitx.Channel `json:"items"`
}

// ConsumersPage is a page of consumers.
type ConsumersPage struct {
	PageInfo
	Items []rabbitx.Consumer `json:"items"`
}

// StreamConnectionsPage is a page of stream protocol connections.
type StreamConnectionsPage struct {
	PageInfo
	Items []rabbitx.StreamConnection `json:"items"`
	// Available is false when rabbitmq_stream_management is not enabled.
	Available bool `json:"available"`
}

// NodePath identifies a node.
type NodePath struct {
	Cluster string `path:"cluster" doc:"Cluster id"`
	Node    string `path:"node" doc:"Erlang node name, e.g. rabbit@host"`
}

// NamePath identifies a connection or channel by its management API name.
type NamePath struct {
	Cluster string `path:"cluster" doc:"Cluster id"`
	Name    string `path:"name" doc:"Name as listed by the management API (URL-encoded)"`
}

// filterPage applies q/vhost/user/node, sorts and pages.
func filterPage[T any](items []T, q ListQuery, fields func(T) (vhost, user, node string, text []string)) ([]T, PageInfo) {
	info := PageInfo{Total: len(items), Page: max(q.Page, 1), PageSize: q.PageSize}
	if info.PageSize <= 0 {
		info.PageSize = 100
	}
	needle := strings.ToLower(strings.TrimSpace(q.Q))
	out := make([]T, 0, len(items))
	for _, it := range items {
		vh, user, node, text := fields(it)
		if (q.VHost != "" && vh != q.VHost) || (q.User != "" && user != q.User) || (q.Node != "" && node != q.Node) {
			continue
		}
		if needle != "" {
			hit := false
			for _, t := range append(text, vh, user, node) {
				if strings.Contains(strings.ToLower(t), needle) {
					hit = true
					break
				}
			}
			if !hit {
				continue
			}
		}
		out = append(out, it)
	}
	sortByJSONField(out, q.Sort, q.Desc)
	info.Filtered = len(out)
	start := (info.Page - 1) * info.PageSize
	if start > len(out) {
		start = len(out)
	}
	end := min(start+info.PageSize, len(out))
	return out[start:end], info
}

// sortByJSONField sorts structs by the field with the given JSON name.
func sortByJSONField[T any](items []T, field string, desc bool) {
	if field == "" || len(items) < 2 {
		return
	}
	t := reflect.TypeOf(items[0])
	idx := -1
	for i := range t.NumField() {
		if name, _, _ := strings.Cut(t.Field(i).Tag.Get("json"), ","); name == field {
			idx = i
			break
		}
	}
	if idx < 0 {
		return
	}
	less := func(a, b reflect.Value) bool {
		switch a.Kind() {
		case reflect.String:
			return strings.ToLower(a.String()) < strings.ToLower(b.String())
		case reflect.Int, reflect.Int64, reflect.Int32:
			return a.Int() < b.Int()
		case reflect.Float64, reflect.Float32:
			return a.Float() < b.Float()
		case reflect.Bool:
			return !a.Bool() && b.Bool()
		}
		return false
	}
	sort.SliceStable(items, func(i, j int) bool {
		a, b := reflect.ValueOf(items[i]).Field(idx), reflect.ValueOf(items[j]).Field(idx)
		if desc {
			return less(b, a)
		}
		return less(a, b)
	})
}

func (s *Server) client(id string) (*rabbitx.Client, error) {
	cl, err := s.mgr.Client(id)
	if err != nil {
		return nil, apiErr(err)
	}
	return cl, nil
}

func (s *Server) registerMonitor() {
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/snapshot", id: "get-snapshot", summary: "Latest snapshot",
		desc: "The latest poll: versions, totals, message and churn rates, nodes, alarms, the node × check health matrix, listeners, endpoint status, the deepest queues and anything that needs attention.",
		tags: tagMonitor, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			Refresh bool `query:"refresh" doc:"Poll now (including health checks) instead of returning the cached snapshot"`
		}) (*struct{ Body *rabbitx.Snapshot }, error) {
			if _, err := s.store.GetCluster(ctx, in.Cluster); err != nil {
				return nil, apiErr(err)
			}
			snap := s.mgr.Snapshot(in.Cluster)
			if in.Refresh || snap == nil {
				var err error
				if snap, err = s.mgr.PollNow(ctx, in.Cluster); err != nil {
					return nil, apiErr(err)
				}
			}
			return &struct{ Body *rabbitx.Snapshot }{snap}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/history", id: "get-history", summary: "Rate history",
		desc: "Rates and counts from every poll since rabmin started (in memory, up to 720 points).", tags: tagMonitor, role: store.RoleViewer},
		func(ctx context.Context, in *ClusterPath) (*struct{ Body []rabbitx.RatePoint }, error) {
			if _, err := s.store.GetCluster(ctx, in.Cluster); err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body []rabbitx.RatePoint }{s.mgr.History(in.Cluster)}, nil
		})

	sse.Register(s.api, huma.Operation{
		OperationID: "stream-snapshots", Method: http.MethodGet, Path: "/v1/clusters/{cluster}/events",
		Summary: "Live snapshots (SSE)", Description: "Server-sent events: a `snapshot` event after every poll. Requires role: **viewer**.",
		Tags: tagMonitor, Metadata: map[string]any{"role": store.RoleViewer},
	}, map[string]any{"snapshot": rabbitx.Snapshot{}}, func(ctx context.Context, in *ClusterPath, send sse.Sender) {
		ch, unsub := s.mgr.Subscribe(in.Cluster)
		defer unsub()
		if snap := s.mgr.Snapshot(in.Cluster); snap != nil {
			if send.Data(snap) != nil {
				return
			}
		}
		keepalive := time.NewTicker(15 * time.Second)
		defer keepalive.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case snap, ok := <-ch:
				if !ok {
					return
				}
				if send.Data(snap) != nil {
					return
				}
			case <-keepalive.C:
				if send.Comment("keepalive") != nil {
					return
				}
			}
		}
	})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/cluster-name", id: "set-cluster-name", summary: "Rename the RabbitMQ cluster",
		desc: "Sets the name RabbitMQ reports (`cluster_name`). rabmin's own display name is changed with update-cluster.", tags: tagMonitor, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body struct {
				Name string `json:"name" minLength:"1" maxLength:"255"`
			}
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			if err := cl.Do(ctx, http.MethodPut, "/api/cluster-name", map[string]string{"name": in.Body.Name}, nil); err != nil {
				return nil, apiErr(err)
			}
			_, _ = s.mgr.PollNow(ctx, in.Cluster)
			return nil, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/nodes/{node}", id: "get-node", summary: "Node detail",
		desc: "A node with its memory breakdown and the raw management object (applications, config files, auth mechanisms, exchange types...).",
		tags: tagMonitor, role: store.RoleViewer},
		func(ctx context.Context, in *NodePath) (*struct{ Body *rabbitx.NodeDetail }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			d, err := cl.Node(ctx, in.Node)
			if err != nil {
				return nil, apiErr(err)
			}
			if snap := s.mgr.Snapshot(in.Cluster); snap != nil {
				for _, n := range snap.Nodes {
					if n.Name == in.Node {
						d.Node.Endpoint, d.Node.Healthy = n.Endpoint, n.Healthy
					}
				}
			}
			return &struct{ Body *rabbitx.NodeDetail }{d}, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/nodes/{node}/reset-stats", id: "reset-node-stats", summary: "Reset a node's statistics",
		desc: "Clears the management statistics database on one node (the built-in UI's \"Reset stats\"). Messages and objects are not affected.",
		tags: tagMonitor, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *NodePath) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.Do(ctx, http.MethodDelete, "/api/reset/"+rabbitx.Esc(in.Node), nil, nil))
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/reset-stats", id: "reset-stats", summary: "Reset statistics on every node",
		desc: "Clears the management statistics database on all nodes. Messages and objects are not affected.",
		tags: tagMonitor, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *ClusterPath) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.Do(ctx, http.MethodDelete, "/api/reset", nil, nil))
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/connections", id: "list-connections", summary: "List connections",
		desc: "AMQP 0-9-1, AMQP 1.0, MQTT and STOMP connections (and direct connections such as shovels). Rates are averages over the last 30 seconds.",
		tags: tagClients, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			ListQuery
			Protocol string `query:"protocol" doc:"Substring of the protocol, e.g. AMQP 0-9-1, MQTT"`
		}) (*struct{ Body ConnectionsPage }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			all, err := cl.Connections(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			if in.Protocol != "" {
				kept := all[:0]
				for _, c := range all {
					if strings.Contains(strings.ToLower(c.Protocol), strings.ToLower(in.Protocol)) {
						kept = append(kept, c)
					}
				}
				all = kept
			}
			items, info := filterPage(all, in.ListQuery, func(c rabbitx.Connection) (string, string, string, []string) {
				return c.VHost, c.User, c.Node, []string{c.Name, c.ClientName, c.Product, c.PeerHost, c.Protocol}
			})
			return &struct{ Body ConnectionsPage }{ConnectionsPage{PageInfo: info, Items: items}}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/connections/{name}", id: "get-connection", summary: "Connection detail",
		desc: "Client properties and capabilities, TLS, negotiated limits, rates and the connection's channels.", tags: tagClients, role: store.RoleViewer},
		func(ctx context.Context, in *NamePath) (*struct{ Body *rabbitx.ConnectionDetail }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			d, err := cl.Connection(ctx, in.Name)
			if err != nil {
				return nil, apiErr(err)
			}
			d.Raw = rabbitx.StripSamples(d.Raw)
			return &struct{ Body *rabbitx.ConnectionDetail }{d}, nil
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/connections/{name}", id: "close-connection", summary: "Close a connection",
		desc: "The client is told the reason in the connection.close frame.", tags: tagClients, role: store.RoleOperator, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			NamePath
			Reason string `query:"reason" maxLength:"255"`
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			reason := in.Reason
			if reason == "" {
				reason = "Closed by " + actor(ctx) + " via rabmin"
			}
			return nil, apiErr(cl.CloseConnection(ctx, in.Name, reason))
		})

	type closeResult struct {
		Closed int      `json:"closed"`
		Names  []string `json:"names"`
		Errors []string `json:"errors"`
	}
	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/connections/close", id: "close-connections", summary: "Close connections in bulk",
		desc: "Closes every connection matching all the given filters (at least one is required): explicit names, a user, a vhost, or a client name.",
		tags: tagClients, role: store.RoleOperator},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body struct {
				Names      []string `json:"names,omitempty"`
				User       string   `json:"user,omitempty"`
				VHost      string   `json:"vhost,omitempty"`
				ClientName string   `json:"clientName,omitempty" doc:"Exact client-provided connection name"`
				Reason     string   `json:"reason,omitempty" maxLength:"255"`
			}
		}) (*struct{ Body closeResult }, error) {
			b := in.Body
			if len(b.Names) == 0 && b.User == "" && b.VHost == "" && b.ClientName == "" {
				return nil, huma.Error422UnprocessableEntity("give at least one of names, user, vhost or clientName")
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			all, err := cl.Connections(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			names := map[string]bool{}
			for _, n := range b.Names {
				names[n] = true
			}
			reason := b.Reason
			if reason == "" {
				reason = "Closed by " + actor(ctx) + " via rabmin"
			}
			res := closeResult{Names: []string{}, Errors: []string{}}
			for _, c := range all {
				if (len(names) > 0 && !names[c.Name]) || (b.User != "" && c.User != b.User) || (b.VHost != "" && c.VHost != b.VHost) ||
					(b.ClientName != "" && c.ClientName != b.ClientName) || c.Type == "direct" {
					continue
				}
				if err := cl.CloseConnection(ctx, c.Name, reason); err != nil {
					res.Errors = append(res.Errors, fmt.Sprintf("%s: %v", c.Name, describe(err)))
					continue
				}
				res.Closed++
				res.Names = append(res.Names, c.Name)
			}
			s.audit(ctx, "close-connections", "cluster/"+in.Cluster, fmt.Sprintf("closed %d: %s", res.Closed, strings.Join(res.Names, "; ")), nil)
			return &struct{ Body closeResult }{res}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/stream-connections", id: "list-stream-connections", summary: "List stream protocol connections",
		desc: "Needs rabbitmq_stream_management; `available` is false without it.", tags: tagClients, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			ListQuery
		}) (*struct{ Body StreamConnectionsPage }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			all, err := cl.StreamConnections(ctx)
			if isNotFound(err) {
				return &struct{ Body StreamConnectionsPage }{StreamConnectionsPage{Items: []rabbitx.StreamConnection{}, PageInfo: PageInfo{Page: 1, PageSize: in.PageSize}}}, nil
			}
			if err != nil {
				return nil, apiErr(err)
			}
			items, info := filterPage(all, in.ListQuery, func(c rabbitx.StreamConnection) (string, string, string, []string) {
				return c.VHost, c.User, c.Node, []string{c.Name, c.ClientName, c.PeerHost}
			})
			return &struct{ Body StreamConnectionsPage }{StreamConnectionsPage{PageInfo: info, Items: items, Available: true}}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/channels", id: "list-channels", summary: "List channels",
		desc: "Rates are averages over the last 30 seconds.", tags: tagClients, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			ListQuery
		}) (*struct{ Body ChannelsPage }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			all, err := cl.Channels(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			items, info := filterPage(all, in.ListQuery, func(c rabbitx.Channel) (string, string, string, []string) {
				return c.VHost, c.User, c.Node, []string{c.Name, c.ClientName}
			})
			return &struct{ Body ChannelsPage }{ChannelsPage{PageInfo: info, Items: items}}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/channels/{name}", id: "get-channel", summary: "Channel detail",
		desc: "Prefetch, unacked and unconfirmed counts, consumers, and (in the detailed rates mode) publishing per exchange and delivery per queue.",
		tags: tagClients, role: store.RoleViewer},
		func(ctx context.Context, in *NamePath) (*struct{ Body *rabbitx.ChannelDetail }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			d, err := cl.Channel(ctx, in.Name)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body *rabbitx.ChannelDetail }{d}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/consumers", id: "list-consumers", summary: "List consumers", tags: tagClients, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			ListQuery
			Queue string `query:"queue" doc:"Exact queue name"`
		}) (*struct{ Body ConsumersPage }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			all, err := cl.Consumers(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			if in.Queue != "" {
				kept := all[:0]
				for _, c := range all {
					if c.Queue == in.Queue {
						kept = append(kept, c)
					}
				}
				all = kept
			}
			items, info := filterPage(all, in.ListQuery, func(c rabbitx.Consumer) (string, string, string, []string) {
				return c.VHost, c.User, c.Node, []string{c.Tag, c.Queue, c.ClientName, c.Channel}
			})
			return &struct{ Body ConsumersPage }{ConsumersPage{PageInfo: info, Items: items}}, nil
		})
}
