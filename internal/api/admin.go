package api

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"

	"github.com/danielgtaylor/huma/v2"

	"github.com/wankdanker/rabmin/internal/rabbitx"
	"github.com/wankdanker/rabmin/internal/store"
)

var (
	tagRabbitUsers  = []string{"RabbitMQ users and permissions"}
	tagPolicies     = []string{"Policies"}
	tagParameters   = []string{"Parameters"}
	tagDefinitions  = []string{"Definitions"}
	tagFeatureFlags = []string{"Feature flags"}
)

// UserPath identifies a RabbitMQ user.
type UserPath struct {
	Cluster string `path:"cluster" doc:"Cluster id"`
	User    string `path:"user" doc:"RabbitMQ user name (URL-encoded)"`
}

// RabbitUserIn creates or updates a RabbitMQ user.
type RabbitUserIn struct {
	Tags []string `json:"tags" doc:"administrator, monitoring, policymaker, management, impersonator, or custom tags"`
	// Password sets a new password. When both it and passwordHash are
	// empty on an update, the current password is kept.
	Password         string `json:"password,omitempty" maxLength:"512"`
	PasswordHash     string `json:"passwordHash,omitempty" doc:"A hash from POST /api/auth/hash_password or another cluster's definitions"`
	HashingAlgorithm string `json:"hashingAlgorithm,omitempty" doc:"With passwordHash, e.g. rabbit_password_hashing_sha256"`
	NoPassword       bool   `json:"noPassword,omitempty" doc:"Create the user without a password (certificate or OAuth2 login only)"`
	MaxConnections   *int   `json:"maxConnections,omitempty" minimum:"-1" doc:"-1 removes the limit"`
	MaxChannels      *int   `json:"maxChannels,omitempty" minimum:"-1" doc:"-1 removes the limit"`
}

// PolicyIn creates or replaces a policy.
type PolicyIn struct {
	Pattern    string          `json:"pattern" minLength:"1" doc:"Regular expression matched against queue or exchange names"`
	ApplyTo    string          `json:"applyTo" enum:"all,queues,classic_queues,quorum_queues,streams,exchanges"`
	Priority   int             `json:"priority" minimum:"0"`
	Definition json.RawMessage `json:"definition" doc:"Policy keys and values, e.g. {\"max-length\": 1000}"`
}

// PolicyMatch is one queue or exchange a policy pattern matches.
type PolicyMatch struct {
	Kind string `json:"kind" enum:"queue,exchange"`
	Name string `json:"name"`
	Type string `json:"type"`
	// Current is the policy (or operator policy) applied today.
	Current string `json:"current,omitempty"`
	// Wins is true when this policy would be the one applied, given the
	// priorities of the other policies that match.
	Wins bool `json:"wins"`
	// Tie names another policy with the same priority that also matches;
	// RabbitMQ then picks one of them.
	Tie string `json:"tie,omitempty"`
}

// PolicyPreview lists what a policy would match.
type PolicyPreview struct {
	Matches []PolicyMatch `json:"matches"`
	// Approximate is set when the pattern uses features Go's regex engine
	// does not share with Erlang's, so matches are estimated.
	Approximate bool   `json:"approximate"`
	Note        string `json:"note,omitempty"`
}

// ClusterTagsBody is the cluster_tags global parameter.
type ClusterTagsBody struct {
	Tags map[string]string `json:"tags"`
}

// DefinitionsIn is a definitions file to compare or import.
type DefinitionsIn struct {
	Definitions json.RawMessage `json:"definitions" doc:"A RabbitMQ definitions export (whole cluster, or one vhost when vhost is set)"`
	VHost       string          `json:"vhost,omitempty" doc:"Compare and import against this vhost; the file is a vhost export"`
}

func (s *Server) registerAdmin() {
	// --- users ---
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/users", id: "list-rabbit-users", summary: "List RabbitMQ users",
		desc: "Users with tags, limits and the vhosts they have permissions in. Password hashes are never returned.", tags: tagRabbitUsers, role: store.RoleViewer},
		func(ctx context.Context, in *ClusterPath) (*struct{ Body []rabbitx.RabbitUser }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			us, err := cl.Users(ctx)
			return &struct{ Body []rabbitx.RabbitUser }{us}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/users/{user}", id: "put-rabbit-user", summary: "Create or update a RabbitMQ user",
		desc: "On an update without a password, the current password is kept: rabmin reads the user's hash from the definitions export and sends it back, " +
			"because RabbitMQ removes the password of a user updated without one.",
		tags: tagRabbitUsers, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			UserPath
			Create bool `query:"create" doc:"Fail with 409 if the user exists"`
			Body   RabbitUserIn
		}) (*struct{ Body rabbitx.RabbitUser }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			b := in.Body
			_, getErr := cl.User(ctx, in.User)
			exists := getErr == nil
			if getErr != nil && !isNotFound(getErr) {
				return nil, apiErr(getErr)
			}
			if in.Create && exists {
				return nil, huma.Error409Conflict("user " + in.User + " already exists")
			}
			if !exists && b.Password == "" && b.PasswordHash == "" && !b.NoPassword {
				return nil, huma.Error422UnprocessableEntity("a new user needs a password, a password hash, or noPassword")
			}
			if err := cl.PutUser(ctx, in.User, b.Tags, b.Password, b.PasswordHash, b.HashingAlgorithm, !exists); err != nil {
				return nil, apiErr(err)
			}
			if b.MaxConnections != nil {
				if err := cl.SetLimit(ctx, "user-limits", in.User, "max-connections", limitValue(b.MaxConnections)); err != nil {
					return nil, apiErr(err)
				}
			}
			if b.MaxChannels != nil {
				if err := cl.SetLimit(ctx, "user-limits", in.User, "max-channels", limitValue(b.MaxChannels)); err != nil {
					return nil, apiErr(err)
				}
			}
			u, err := cl.User(ctx, in.User)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body rabbitx.RabbitUser }{*u}, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/users/delete", id: "delete-rabbit-users", summary: "Delete RabbitMQ users",
		desc: "Deletes one or more users with their permissions. Their open connections are closed by RabbitMQ.", tags: tagRabbitUsers, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body struct {
				Users []string `json:"users" minItems:"1"`
			}
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			s.audit(ctx, "delete-rabbit-users", "cluster/"+in.Cluster, strings.Join(in.Body.Users, ", "), nil)
			return nil, apiErr(cl.DeleteUsers(ctx, in.Body.Users))
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/hash-password", id: "hash-password", summary: "Hash a password",
		desc: "Hashes a password with the cluster's configured algorithm, for definitions files.", tags: tagRabbitUsers, role: store.RoleAdmin, noAudit: true},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body struct {
				Password string `json:"password" minLength:"1"`
			}
		}) (*struct {
			Body struct {
				Hash string `json:"hash"`
			}
		}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			var out struct {
				OK string `json:"ok"`
			}
			if err := cl.Do(ctx, http.MethodPost, "/api/auth/hash_password", map[string]string{"password": in.Body.Password}, &out); err != nil {
				return nil, apiErr(err)
			}
			res := &struct {
				Body struct {
					Hash string `json:"hash"`
				}
			}{}
			res.Body.Hash = out.OK
			return res, nil
		})

	// --- permissions ---
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/permissions", id: "list-permissions", summary: "List permissions", tags: tagRabbitUsers, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			VHost string `query:"vhost"`
			User  string `query:"user"`
		}) (*struct{ Body []rabbitx.Permission }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			ps, err := cl.Permissions(ctx, in.VHost, in.User)
			return &struct{ Body []rabbitx.Permission }{ps}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/permissions/{vhost}/{user}", id: "set-permission", summary: "Grant permissions in a vhost",
		desc: "Configure, write and read are regular expressions over resource names; an empty string grants nothing.", tags: tagRabbitUsers, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Cluster string `path:"cluster"`
			VHost   string `path:"vhost"`
			User    string `path:"user"`
			Body    struct {
				Configure string `json:"configure"`
				Write     string `json:"write"`
				Read      string `json:"read"`
			}
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.SetPermission(ctx, rabbitx.Permission{User: in.User, VHost: in.VHost, Configure: in.Body.Configure, Write: in.Body.Write, Read: in.Body.Read}))
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/permissions/{vhost}/{user}", id: "delete-permission", summary: "Revoke permissions in a vhost",
		tags: tagRabbitUsers, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Cluster string `path:"cluster"`
			VHost   string `path:"vhost"`
			User    string `path:"user"`
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.DeletePermission(ctx, in.VHost, in.User))
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/topic-permissions", id: "list-topic-permissions", summary: "List topic permissions", tags: tagRabbitUsers, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			VHost string `query:"vhost"`
			User  string `query:"user"`
		}) (*struct{ Body []rabbitx.TopicPermission }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			ps, err := cl.TopicPermissions(ctx, in.VHost, in.User)
			return &struct{ Body []rabbitx.TopicPermission }{ps}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/topic-permissions/{vhost}/{user}", id: "set-topic-permission", summary: "Set a topic permission",
		desc: "Limits the routing keys a user may publish (write) or bind/consume (read) on one topic exchange.", tags: tagRabbitUsers, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Cluster string `path:"cluster"`
			VHost   string `path:"vhost"`
			User    string `path:"user"`
			Body    struct {
				Exchange string `json:"exchange" minLength:"1"`
				Write    string `json:"write"`
				Read     string `json:"read"`
			}
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.SetTopicPermission(ctx, rabbitx.TopicPermission{User: in.User, VHost: in.VHost, Exchange: in.Body.Exchange, Write: in.Body.Write, Read: in.Body.Read}))
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/topic-permissions/{vhost}/{user}", id: "delete-topic-permission", summary: "Remove a topic permission",
		desc: "Removes the permission for one exchange. RabbitMQ can only delete all of a user's topic permissions in a vhost, so rabmin deletes them and puts the others back; this is not atomic.",
		tags: tagRabbitUsers, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Cluster  string `path:"cluster"`
			VHost    string `path:"vhost"`
			User     string `path:"user"`
			Exchange string `query:"exchange" required:"true"`
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.DeleteTopicPermission(ctx, in.VHost, in.User, in.Exchange))
		})

	// --- policies ---
	for _, operator := range []bool{false, true} {
		kind, base, noun := "policy", "/v1/clusters/{cluster}/policies", "policy"
		if operator {
			kind, base, noun = "operator-policy", "/v1/clusters/{cluster}/operator-policies", "operator policy"
		}
		reg(s, route{method: http.MethodGet, path: base, id: "list-" + kind + "s", summary: "List " + noun + "s", tags: tagPolicies, role: store.RoleViewer},
			func(ctx context.Context, in *struct {
				ClusterPath
				VHost string `query:"vhost"`
			}) (*struct{ Body []rabbitx.Policy }, error) {
				cl, err := s.client(in.Cluster)
				if err != nil {
					return nil, err
				}
				ps, err := cl.Policies(ctx, operator, in.VHost)
				return &struct{ Body []rabbitx.Policy }{ps}, apiErr(err)
			})

		reg(s, route{method: http.MethodPut, path: base + "/{vhost}/{name}", id: "put-" + kind, summary: "Create or replace a " + noun,
			desc: "RabbitMQ validates the definition and rejects unknown keys.", tags: tagPolicies, role: store.RoleAdmin, status: http.StatusNoContent},
			func(ctx context.Context, in *struct {
				ObjectPath
				Body PolicyIn
			}) (*struct{}, error) {
				if _, err := rabbitx.PolicyRegexp(in.Body.Pattern); err != nil && !strings.ContainsAny(in.Body.Pattern, "(?") {
					return nil, huma.Error422UnprocessableEntity("pattern: " + err.Error())
				}
				cl, err := s.client(in.Cluster)
				if err != nil {
					return nil, err
				}
				return nil, apiErr(cl.PutPolicy(ctx, rabbitx.Policy{VHost: in.VHost, Name: in.Name, Pattern: in.Body.Pattern, ApplyTo: in.Body.ApplyTo,
					Priority: in.Body.Priority, Definition: in.Body.Definition, Operator: operator}))
			})

		reg(s, route{method: http.MethodDelete, path: base + "/{vhost}/{name}", id: "delete-" + kind, summary: "Delete a " + noun, tags: tagPolicies, role: store.RoleAdmin, status: http.StatusNoContent},
			func(ctx context.Context, in *ObjectPath) (*struct{}, error) {
				cl, err := s.client(in.Cluster)
				if err != nil {
					return nil, err
				}
				return nil, apiErr(cl.DeletePolicy(ctx, operator, in.VHost, in.Name))
			})
	}

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/policies/preview", id: "preview-policy", summary: "Preview what a policy matches",
		desc: "Lists the queues and exchanges of a vhost that a pattern and apply-to match, and whether the policy would win against the other policies " +
			"of the same kind given their priorities. Matching uses Go regular expressions, which agree with RabbitMQ's for common patterns.",
		tags: tagPolicies, role: store.RoleViewer, noAudit: true},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body struct {
				VHost    string `json:"vhost" minLength:"1"`
				Name     string `json:"name,omitempty" doc:"The policy's name, so an existing policy of that name is replaced in the comparison"`
				Pattern  string `json:"pattern" minLength:"1"`
				ApplyTo  string `json:"applyTo" enum:"all,queues,classic_queues,quorum_queues,streams,exchanges"`
				Priority int    `json:"priority"`
				Operator bool   `json:"operator,omitempty"`
			}
		}) (*struct{ Body PolicyPreview }, error) {
			b := in.Body
			re, err := rabbitx.PolicyRegexp(b.Pattern)
			if err != nil {
				return &struct{ Body PolicyPreview }{PolicyPreview{Matches: []PolicyMatch{}, Approximate: true,
					Note: "This pattern uses regex features rabmin cannot evaluate (" + err.Error() + "); RabbitMQ may still accept it."}}, nil
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			others, err := cl.Policies(ctx, b.Operator, b.VHost)
			if err != nil {
				return nil, apiErr(err)
			}
			queues, err := cl.Queues(ctx, b.VHost)
			if err != nil {
				return nil, apiErr(err)
			}
			exchanges, err := cl.Exchanges(ctx, b.VHost)
			if err != nil {
				return nil, apiErr(err)
			}
			type obj struct{ kind, name, typ, current string }
			objs := []obj{}
			for _, q := range queues {
				cur := q.Policy
				if b.Operator {
					cur = q.OperatorPolicy
				}
				objs = append(objs, obj{"queue", q.Name, q.Type, cur})
			}
			for _, e := range exchanges {
				if e.Name == "" || strings.HasPrefix(e.Name, "amq.") {
					continue
				}
				objs = append(objs, obj{"exchange", e.Name, e.Type, e.Policy})
			}
			out := PolicyPreview{Matches: []PolicyMatch{}}
			for _, o := range objs {
				if !rabbitx.PolicyAppliesTo(b.ApplyTo, o.kind, o.typ) || !re.MatchString(o.name) {
					continue
				}
				m := PolicyMatch{Kind: o.kind, Name: o.name, Type: o.typ, Current: o.current, Wins: true}
				for _, p := range others {
					if p.Name == b.Name || !rabbitx.PolicyAppliesTo(p.ApplyTo, o.kind, o.typ) {
						continue
					}
					pre, err := rabbitx.PolicyRegexp(p.Pattern)
					if err != nil || !pre.MatchString(o.name) {
						continue
					}
					if p.Priority > b.Priority {
						m.Wins = false
					} else if p.Priority == b.Priority {
						m.Tie = p.Name
					}
				}
				out.Matches = append(out.Matches, m)
			}
			sort.Slice(out.Matches, func(i, j int) bool {
				if out.Matches[i].Kind != out.Matches[j].Kind {
					return out.Matches[i].Kind > out.Matches[j].Kind
				}
				return out.Matches[i].Name < out.Matches[j].Name
			})
			return &struct{ Body PolicyPreview }{out}, nil
		})

	// --- parameters ---
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/parameters", id: "list-parameters", summary: "List vhost parameters",
		desc: "Runtime parameters of every component (federation-upstream, shovel, vhost-limits, operator_policy...). Passwords in URIs are redacted.",
		tags: tagParameters, role: store.RoleViewer},
		func(ctx context.Context, in *struct {
			ClusterPath
			Component string `query:"component"`
			VHost     string `query:"vhost"`
		}) (*struct{ Body []rabbitx.Parameter }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			ps, err := cl.Parameters(ctx, in.Component, in.VHost)
			return &struct{ Body []rabbitx.Parameter }{ps}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/parameters/{component}/{vhost}/{name}", id: "put-parameter", summary: "Set a vhost parameter",
		tags: tagParameters, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Cluster   string `path:"cluster"`
			Component string `path:"component"`
			VHost     string `path:"vhost"`
			Name      string `path:"name"`
			Body      struct {
				Value json.RawMessage `json:"value"`
			}
		}) (*struct{}, error) {
			if strings.Contains(string(in.Body.Value), ":***@") {
				return nil, huma.Error422UnprocessableEntity("the value contains a redacted password (***); enter the real password")
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.PutParameter(ctx, in.Component, in.VHost, in.Name, in.Body.Value))
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/parameters/{component}/{vhost}/{name}", id: "delete-parameter", summary: "Delete a vhost parameter",
		tags: tagParameters, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Cluster   string `path:"cluster"`
			Component string `path:"component"`
			VHost     string `path:"vhost"`
			Name      string `path:"name"`
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.DeleteParameter(ctx, in.Component, in.VHost, in.Name))
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/global-parameters", id: "list-global-parameters", summary: "List global parameters",
		tags: tagParameters, role: store.RoleViewer},
		func(ctx context.Context, in *ClusterPath) (*struct{ Body []rabbitx.GlobalParameter }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			ps, err := cl.GlobalParameters(ctx)
			return &struct{ Body []rabbitx.GlobalParameter }{ps}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/global-parameters/{name}", id: "put-global-parameter", summary: "Set a global parameter",
		desc: "cluster_name and internal_cluster_id are managed by RabbitMQ; use set-cluster-name to rename the cluster.", tags: tagParameters, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Cluster string `path:"cluster"`
			Name    string `path:"name"`
			Body    struct {
				Value json.RawMessage `json:"value"`
			}
		}) (*struct{}, error) {
			if in.Name == "internal_cluster_id" || in.Name == "cluster_name" {
				return nil, huma.Error422UnprocessableEntity(in.Name + " is managed by RabbitMQ")
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.PutGlobalParameter(ctx, in.Name, in.Body.Value))
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/global-parameters/{name}", id: "delete-global-parameter", summary: "Delete a global parameter",
		tags: tagParameters, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			Cluster string `path:"cluster"`
			Name    string `path:"name"`
		}) (*struct{}, error) {
			if in.Name == "internal_cluster_id" || in.Name == "cluster_name" {
				return nil, huma.Error422UnprocessableEntity(in.Name + " is managed by RabbitMQ")
			}
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			return nil, apiErr(cl.DeleteGlobalParameter(ctx, in.Name))
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/cluster-tags", id: "get-cluster-tags", summary: "Cluster tags", tags: tagParameters, role: store.RoleViewer},
		func(ctx context.Context, in *ClusterPath) (*struct{ Body ClusterTagsBody }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			tags, err := cl.ClusterTags(ctx)
			return &struct{ Body ClusterTagsBody }{ClusterTagsBody{Tags: tags}}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/cluster-tags", id: "set-cluster-tags", summary: "Replace the cluster tags",
		desc: "Key/value tags stored in the cluster_tags global parameter and reported by /api/overview.", tags: tagParameters, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body ClusterTagsBody
		}) (*struct{}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			if in.Body.Tags == nil {
				in.Body.Tags = map[string]string{}
			}
			return nil, apiErr(cl.SetClusterTags(ctx, in.Body.Tags))
		})

	// --- definitions ---
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/definitions", id: "export-definitions", summary: "Export definitions",
		desc: "The RabbitMQ definitions of the whole cluster, or of one vhost. Contains password hashes, so admin only.", tags: tagDefinitions, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			ClusterPath
			VHost string `query:"vhost"`
		}) (*struct {
			ContentDisposition string `header:"Content-Disposition"`
			Body               json.RawMessage
		}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			d, err := cl.Definitions(ctx, in.VHost)
			if err != nil {
				return nil, apiErr(err)
			}
			name := in.Cluster
			if in.VHost != "" {
				name += "-" + strings.NewReplacer("/", "_", `"`, "").Replace(in.VHost)
			}
			s.audit(ctx, "export-definitions", "cluster/"+in.Cluster, "vhost "+in.VHost, nil)
			return &struct {
				ContentDisposition string `header:"Content-Disposition"`
				Body               json.RawMessage
			}{fmt.Sprintf(`attachment; filename="%s-definitions.json"`, name), d}, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/definitions/diff", id: "diff-definitions", summary: "Compare definitions with the cluster",
		desc: "Shows what importing a definitions file would do. Import never deletes (cluster-only objects stay) and skips exchanges and queues that exist with " +
			"other properties (conflicts).", tags: tagDefinitions, role: store.RoleAdmin, noAudit: true, maxBody: 64 << 20},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body DefinitionsIn
		}) (*struct{ Body *rabbitx.DefinitionsDiff }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			cur, err := cl.Definitions(ctx, in.Body.VHost)
			if err != nil {
				return nil, apiErr(err)
			}
			d, err := rabbitx.DiffDefinitions(cur, in.Body.Definitions, in.Body.VHost)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			return &struct{ Body *rabbitx.DefinitionsDiff }{d}, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/definitions/import", id: "import-definitions", summary: "Import definitions",
		desc: "Imports the added and changed objects of a definitions file, or only those whose change ids are listed in select. " +
			"Conflicts and cluster-only objects are never touched.", tags: tagDefinitions, role: store.RoleAdmin, maxBody: 64 << 20},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body struct {
				DefinitionsIn
				Select []string `json:"select,omitempty" doc:"Change ids from diff-definitions; empty imports every added and changed object"`
			}
		}) (*struct {
			Body struct {
				Imported int `json:"imported"`
			}
		}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			cur, err := cl.Definitions(ctx, in.Body.VHost)
			if err != nil {
				return nil, apiErr(err)
			}
			d, err := rabbitx.DiffDefinitions(cur, in.Body.Definitions, in.Body.VHost)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			doc, n, err := rabbitx.SelectDefinitions(d, in.Body.Definitions, in.Body.VHost, in.Body.Select)
			if err != nil {
				return nil, huma.Error422UnprocessableEntity(err.Error())
			}
			res := &struct {
				Body struct {
					Imported int `json:"imported"`
				}
			}{}
			if n == 0 {
				return res, nil
			}
			if err := cl.ImportDefinitions(ctx, doc); err != nil {
				return nil, apiErr(err)
			}
			s.audit(ctx, "import-definitions", "cluster/"+in.Cluster, fmt.Sprintf("%d objects, vhost %q", n, in.Body.VHost), nil)
			res.Body.Imported = n
			return res, nil
		})

	// --- feature flags ---
	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/feature-flags", id: "list-feature-flags", summary: "List feature flags", tags: tagFeatureFlags, role: store.RoleViewer},
		func(ctx context.Context, in *ClusterPath) (*struct{ Body []rabbitx.FeatureFlag }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			ff, err := cl.FeatureFlags(ctx)
			return &struct{ Body []rabbitx.FeatureFlag }{ff}, apiErr(err)
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/feature-flags/enable", id: "enable-feature-flags", summary: "Enable feature flags",
		desc: "Enables the named flags, or every disabled stable flag when names is empty. Feature flags cannot be disabled again. Experimental flags must be named explicitly.",
		tags: tagFeatureFlags, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body struct {
				Names []string `json:"names,omitempty"`
			}
		}) (*struct {
			Body struct {
				Enabled []string `json:"enabled"`
			}
		}, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			names := in.Body.Names
			if len(names) == 0 {
				ff, err := cl.FeatureFlags(ctx)
				if err != nil {
					return nil, apiErr(err)
				}
				for _, f := range ff {
					if f.State == "disabled" && f.Stability == "stable" {
						names = append(names, f.Name)
					}
				}
			}
			res := &struct {
				Body struct {
					Enabled []string `json:"enabled"`
				}
			}{}
			res.Body.Enabled = []string{}
			for _, n := range names {
				if err := cl.EnableFeatureFlag(ctx, n); err != nil {
					return nil, apiErr(fmt.Errorf("enable %s: %w", n, err))
				}
				res.Body.Enabled = append(res.Body.Enabled, n)
			}
			s.audit(ctx, "enable-feature-flags", "cluster/"+in.Cluster, strings.Join(res.Body.Enabled, ", "), nil)
			return res, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/deprecated-features", id: "list-deprecated-features", summary: "List deprecated features",
		desc: "Deprecated features with their phase and whether the cluster uses them now.", tags: tagFeatureFlags, role: store.RoleViewer},
		func(ctx context.Context, in *ClusterPath) (*struct{ Body []rabbitx.DeprecatedFeature }, error) {
			cl, err := s.client(in.Cluster)
			if err != nil {
				return nil, err
			}
			df, err := cl.DeprecatedFeatures(ctx)
			return &struct{ Body []rabbitx.DeprecatedFeature }{df}, apiErr(err)
		})
}
