package rabbitx

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"sort"
)

// --- users ---

// RabbitUser is a RabbitMQ user. Password hashes are never returned.
type RabbitUser struct {
	Name             string   `json:"name"`
	Tags             []string `json:"tags"`
	HasPassword      bool     `json:"hasPassword"`
	HashingAlgorithm string   `json:"hashingAlgorithm,omitempty"`
	MaxConnections   *int     `json:"maxConnections,omitempty"`
	MaxChannels      *int     `json:"maxChannels,omitempty"`
	// VHosts lists the vhosts the user has permissions in.
	VHosts []string `json:"vhosts"`
}

type userRaw struct {
	Name             string          `json:"name"`
	Tags             any             `json:"tags"`
	HasPassword      *bool           `json:"has_password"`
	PasswordHash     string          `json:"password_hash"`
	HashingAlgorithm string          `json:"hashing_algorithm"`
	Limits           json.RawMessage `json:"limits"`
}

func tagList(v any) []string {
	switch t := v.(type) {
	case []any:
		out := []string{}
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		return splitTags([]string{t})
	}
	return []string{}
}

func (u userRaw) user() RabbitUser {
	x := RabbitUser{Name: u.Name, Tags: tagList(u.Tags), HashingAlgorithm: u.HashingAlgorithm, VHosts: []string{}}
	if u.HasPassword != nil {
		x.HasPassword = *u.HasPassword
	} else {
		x.HasPassword = u.PasswordHash != ""
	}
	var lim map[string]int
	_ = json.Unmarshal(u.Limits, &lim)
	x.MaxConnections, x.MaxChannels = intPtr(lim, "max-connections"), intPtr(lim, "max-channels")
	return x
}

// Users lists users with the vhosts they have permissions in.
func (c *Client) Users(ctx context.Context) ([]RabbitUser, error) {
	var raw []userRaw
	if err := c.Get(ctx, "/api/users/", &raw); err != nil {
		return nil, err
	}
	perms, err := c.Permissions(ctx, "", "")
	if err != nil {
		return nil, err
	}
	vhosts := map[string][]string{}
	for _, p := range perms {
		vhosts[p.User] = append(vhosts[p.User], p.VHost)
	}
	out := make([]RabbitUser, 0, len(raw))
	for _, u := range raw {
		x := u.user()
		if v := vhosts[u.Name]; v != nil {
			sort.Strings(v)
			x.VHosts = v
		}
		out = append(out, x)
	}
	return out, nil
}

// User reads one user.
func (c *Client) User(ctx context.Context, name string) (*RabbitUser, error) {
	var raw userRaw
	if err := c.Get(ctx, "/api/users/"+Esc(name), &raw); err != nil {
		return nil, err
	}
	u := raw.user()
	return &u, nil
}

// ErrUserNotInDefinitions is returned when a user's password hash cannot be
// found to preserve it.
var ErrUserNotInDefinitions = errors.New("user not found in the definitions export, so its password cannot be preserved")

// PutUser creates or updates a user. A password replaces the current one;
// without one, an existing user's password hash is read from the definitions
// export and sent back, because a PUT without password or password_hash
// removes the password.
func (c *Client) PutUser(ctx context.Context, name string, tags []string, password, passwordHash, hashingAlgorithm string, create bool) error {
	body := map[string]any{"tags": splitTags(tags)}
	switch {
	case password != "":
		body["password"] = password
	case passwordHash != "":
		body["password_hash"] = passwordHash
		if hashingAlgorithm != "" {
			body["hashing_algorithm"] = hashingAlgorithm
		}
	case !create:
		var defs struct {
			Users []userRaw `json:"users"`
		}
		if err := c.Get(ctx, "/api/definitions", &defs); err != nil {
			return err
		}
		found := false
		for _, u := range defs.Users {
			if u.Name == name {
				found = true
				if u.PasswordHash != "" {
					body["password_hash"] = u.PasswordHash
					body["hashing_algorithm"] = u.HashingAlgorithm
				}
			}
		}
		if !found {
			return ErrUserNotInDefinitions
		}
	}
	return c.Do(ctx, http.MethodPut, "/api/users/"+Esc(name), body, nil)
}

// DeleteUsers deletes users in bulk.
func (c *Client) DeleteUsers(ctx context.Context, names []string) error {
	return c.Do(ctx, http.MethodPost, "/api/users/bulk-delete", map[string][]string{"users": names}, nil)
}

// --- permissions ---

// Permission is a user's configure/write/read regexes in a vhost.
type Permission struct {
	User      string `json:"user"`
	VHost     string `json:"vhost"`
	Configure string `json:"configure"`
	Write     string `json:"write"`
	Read      string `json:"read"`
}

// Permissions lists permissions, filtered by vhost and/or user.
func (c *Client) Permissions(ctx context.Context, vhost, user string) ([]Permission, error) {
	path := "/api/permissions"
	switch {
	case vhost != "" && user == "":
		path = "/api/vhosts/" + Esc(vhost) + "/permissions"
	case user != "" && vhost == "":
		path = "/api/users/" + Esc(user) + "/permissions"
	}
	var out []Permission
	if err := c.Get(ctx, path, &out); err != nil {
		return nil, err
	}
	if vhost != "" && user != "" {
		kept := out[:0]
		for _, p := range out {
			if p.VHost == vhost && p.User == user {
				kept = append(kept, p)
			}
		}
		out = kept
	}
	if out == nil {
		out = []Permission{}
	}
	return out, nil
}

// SetPermission grants a user permissions in a vhost.
func (c *Client) SetPermission(ctx context.Context, p Permission) error {
	return c.Do(ctx, http.MethodPut, "/api/permissions/"+Esc(p.VHost)+"/"+Esc(p.User),
		map[string]string{"configure": p.Configure, "write": p.Write, "read": p.Read}, nil)
}

// DeletePermission revokes a user's permissions in a vhost.
func (c *Client) DeletePermission(ctx context.Context, vhost, user string) error {
	return c.Do(ctx, http.MethodDelete, "/api/permissions/"+Esc(vhost)+"/"+Esc(user), nil, nil)
}

// TopicPermission limits which routing keys a user may publish or consume
// on a topic exchange.
type TopicPermission struct {
	User     string `json:"user"`
	VHost    string `json:"vhost"`
	Exchange string `json:"exchange"`
	Write    string `json:"write"`
	Read     string `json:"read"`
}

// TopicPermissions lists topic permissions, filtered by vhost and/or user.
func (c *Client) TopicPermissions(ctx context.Context, vhost, user string) ([]TopicPermission, error) {
	var all []TopicPermission
	if err := c.Get(ctx, "/api/topic-permissions", &all); err != nil {
		return nil, err
	}
	out := []TopicPermission{}
	for _, p := range all {
		if (vhost == "" || p.VHost == vhost) && (user == "" || p.User == user) {
			out = append(out, p)
		}
	}
	return out, nil
}

// SetTopicPermission sets a user's topic permission for one exchange.
func (c *Client) SetTopicPermission(ctx context.Context, p TopicPermission) error {
	return c.Do(ctx, http.MethodPut, "/api/topic-permissions/"+Esc(p.VHost)+"/"+Esc(p.User),
		map[string]string{"exchange": p.Exchange, "write": p.Write, "read": p.Read}, nil)
}

// DeleteTopicPermission removes a user's topic permission for one exchange.
// RabbitMQ can only delete all of a user's topic permissions in a vhost, so
// the others are put back afterwards; this is not atomic.
func (c *Client) DeleteTopicPermission(ctx context.Context, vhost, user, exchange string) error {
	current, err := c.TopicPermissions(ctx, vhost, user)
	if err != nil {
		return err
	}
	found := false
	for _, p := range current {
		if p.Exchange == exchange {
			found = true
		}
	}
	if !found {
		return &APIError{Status: http.StatusNotFound, Code: "not_found", Reason: "no topic permission for that exchange"}
	}
	if err := c.Do(ctx, http.MethodDelete, "/api/topic-permissions/"+Esc(vhost)+"/"+Esc(user), nil, nil); err != nil {
		return err
	}
	for _, p := range current {
		if p.Exchange == exchange {
			continue
		}
		if err := c.SetTopicPermission(ctx, p); err != nil {
			return err
		}
	}
	return nil
}

// --- policies ---

// Policy is a policy or operator policy.
type Policy struct {
	VHost      string          `json:"vhost"`
	Name       string          `json:"name"`
	Pattern    string          `json:"pattern"`
	ApplyTo    string          `json:"applyTo" doc:"all, queues, classic_queues, quorum_queues, streams or exchanges"`
	Priority   int             `json:"priority"`
	Definition json.RawMessage `json:"definition"`
	Operator   bool            `json:"operator"`
}

type policyRaw struct {
	VHost      string          `json:"vhost"`
	Name       string          `json:"name"`
	Pattern    string          `json:"pattern"`
	ApplyTo    string          `json:"apply-to"`
	Priority   int             `json:"priority"`
	Definition json.RawMessage `json:"definition"`
}

func policyKind(operator bool) string {
	if operator {
		return "operator-policies"
	}
	return "policies"
}

// Policies lists policies or operator policies, in one vhost when set.
func (c *Client) Policies(ctx context.Context, operator bool, vhost string) ([]Policy, error) {
	var raw []policyRaw
	if err := c.Get(ctx, "/api/"+policyKind(operator)+vhostPath(vhost), &raw); err != nil {
		return nil, err
	}
	out := make([]Policy, 0, len(raw))
	for _, p := range raw {
		out = append(out, Policy{VHost: p.VHost, Name: p.Name, Pattern: p.Pattern, ApplyTo: p.ApplyTo, Priority: p.Priority,
			Definition: rawOrEmpty(p.Definition), Operator: operator})
	}
	return out, nil
}

// PutPolicy creates or replaces a policy.
func (c *Client) PutPolicy(ctx context.Context, p Policy) error {
	if p.ApplyTo == "" {
		p.ApplyTo = "all"
	}
	return c.Do(ctx, http.MethodPut, "/api/"+policyKind(p.Operator)+"/"+Esc(p.VHost)+"/"+Esc(p.Name),
		map[string]any{"pattern": p.Pattern, "apply-to": p.ApplyTo, "priority": p.Priority, "definition": rawOrEmpty(p.Definition)}, nil)
}

// DeletePolicy deletes a policy.
func (c *Client) DeletePolicy(ctx context.Context, operator bool, vhost, name string) error {
	return c.Do(ctx, http.MethodDelete, "/api/"+policyKind(operator)+"/"+Esc(vhost)+"/"+Esc(name), nil, nil)
}

// PolicyAppliesTo reports whether a policy's apply-to covers an object.
func PolicyAppliesTo(applyTo, kind, queueType string) bool {
	switch applyTo {
	case "", "all":
		return true
	case "exchanges":
		return kind == "exchange"
	case "queues":
		return kind == "queue"
	case "classic_queues":
		return kind == "queue" && queueType == "classic"
	case "quorum_queues":
		return kind == "queue" && queueType == "quorum"
	case "streams":
		return kind == "queue" && queueType == "stream"
	}
	return false
}

// PolicyRegexp compiles a policy pattern. RabbitMQ uses Erlang (PCRE)
// regexes; Go's RE2 accepts most patterns people write but not lookarounds
// or backreferences.
func PolicyRegexp(pattern string) (*regexp.Regexp, error) {
	return regexp.Compile(pattern)
}

// --- parameters ---

// Parameter is a vhost-scoped runtime parameter.
type Parameter struct {
	Component string          `json:"component"`
	VHost     string          `json:"vhost"`
	Name      string          `json:"name"`
	Value     json.RawMessage `json:"value" doc:"Passwords in URIs are redacted"`
}

// GlobalParameter is a cluster-wide runtime parameter.
type GlobalParameter struct {
	Name  string          `json:"name"`
	Value json.RawMessage `json:"value"`
}

// Parameters lists vhost-scoped parameters, filtered by component and vhost.
func (c *Client) Parameters(ctx context.Context, component, vhost string) ([]Parameter, error) {
	path := "/api/parameters"
	if component != "" {
		path += "/" + Esc(component) + vhostPath(vhost)
	}
	var out []Parameter
	if err := c.Get(ctx, path, &out); err != nil {
		return nil, err
	}
	kept := []Parameter{}
	for _, p := range out {
		if vhost != "" && p.VHost != vhost {
			continue
		}
		p.Value = RedactURIs(p.Value)
		kept = append(kept, p)
	}
	return kept, nil
}

// PutParameter sets a vhost-scoped parameter.
func (c *Client) PutParameter(ctx context.Context, component, vhost, name string, value json.RawMessage) error {
	return c.Do(ctx, http.MethodPut, "/api/parameters/"+Esc(component)+"/"+Esc(vhost)+"/"+Esc(name), map[string]any{"value": value}, nil)
}

// DeleteParameter deletes a vhost-scoped parameter.
func (c *Client) DeleteParameter(ctx context.Context, component, vhost, name string) error {
	return c.Do(ctx, http.MethodDelete, "/api/parameters/"+Esc(component)+"/"+Esc(vhost)+"/"+Esc(name), nil, nil)
}

// GlobalParameters lists global parameters.
func (c *Client) GlobalParameters(ctx context.Context) ([]GlobalParameter, error) {
	var out []GlobalParameter
	if err := c.Get(ctx, "/api/global-parameters", &out); err != nil {
		return nil, err
	}
	for i := range out {
		out[i].Value = RedactURIs(out[i].Value)
	}
	if out == nil {
		out = []GlobalParameter{}
	}
	return out, nil
}

// PutGlobalParameter sets a global parameter.
func (c *Client) PutGlobalParameter(ctx context.Context, name string, value json.RawMessage) error {
	return c.Do(ctx, http.MethodPut, "/api/global-parameters/"+Esc(name), map[string]any{"name": name, "value": value}, nil)
}

// DeleteGlobalParameter deletes a global parameter.
func (c *Client) DeleteGlobalParameter(ctx context.Context, name string) error {
	return c.Do(ctx, http.MethodDelete, "/api/global-parameters/"+Esc(name), nil, nil)
}

// ClusterTags reads the cluster_tags global parameter ([] when unset).
func (c *Client) ClusterTags(ctx context.Context) (map[string]string, error) {
	var p GlobalParameter
	err := c.Get(ctx, "/api/global-parameters/cluster_tags", &p)
	var ae *APIError
	if errors.As(err, &ae) && ae.Status == http.StatusNotFound {
		return map[string]string{}, nil
	}
	if err != nil {
		return nil, err
	}
	out := map[string]string{}
	var m map[string]any
	if json.Unmarshal(p.Value, &m) == nil {
		for k, v := range m {
			if s, ok := v.(string); ok {
				out[k] = s
			} else {
				b, _ := json.Marshal(v)
				out[k] = string(b)
			}
		}
	}
	return out, nil
}

// SetClusterTags replaces the cluster tags.
func (c *Client) SetClusterTags(ctx context.Context, tags map[string]string) error {
	b, _ := json.Marshal(tags)
	return c.PutGlobalParameter(ctx, "cluster_tags", b)
}

var uriPassword = regexp.MustCompile(`(amqps?|https?|mqtts?|rabbitmq-stream(\+tls)?)://([^:/@\s"]+):([^@/\s"]+)@`)

// RedactURIs replaces passwords in URIs inside a JSON value with "***".
func RedactURIs(v json.RawMessage) json.RawMessage {
	if len(v) == 0 {
		return v
	}
	return json.RawMessage(uriPassword.ReplaceAllString(string(v), "$1://$3:***@"))
}

// --- feature flags and deprecated features ---

// FeatureFlag is a feature flag.
type FeatureFlag struct {
	Name         string `json:"name"`
	Description  string `json:"description"`
	State        string `json:"state" doc:"enabled, disabled or state_changing"`
	Stability    string `json:"stability" doc:"required, stable or experimental"`
	RequireLevel string `json:"requireLevel,omitempty"`
	DocURL       string `json:"docUrl,omitempty"`
	ProvidedBy   string `json:"providedBy,omitempty"`
}

// FeatureFlags lists feature flags.
func (c *Client) FeatureFlags(ctx context.Context) ([]FeatureFlag, error) {
	var raw []struct {
		Name         string `json:"name"`
		Desc         string `json:"desc"`
		State        string `json:"state"`
		Stability    string `json:"stability"`
		RequireLevel string `json:"require_level"`
		DocURL       string `json:"doc_url"`
		ProvidedBy   string `json:"provided_by"`
	}
	if err := c.Get(ctx, "/api/feature-flags", &raw); err != nil {
		return nil, err
	}
	out := make([]FeatureFlag, 0, len(raw))
	for _, f := range raw {
		out = append(out, FeatureFlag{Name: f.Name, Description: f.Desc, State: f.State, Stability: f.Stability, RequireLevel: f.RequireLevel,
			DocURL: f.DocURL, ProvidedBy: f.ProvidedBy})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// EnableFeatureFlag enables one feature flag. Flags cannot be disabled again.
func (c *Client) EnableFeatureFlag(ctx context.Context, name string) error {
	return c.Do(ctx, http.MethodPut, "/api/feature-flags/"+url.PathEscape(name)+"/enable", nil, nil)
}

// DeprecatedFeature is a feature on its way out.
type DeprecatedFeature struct {
	Name        string `json:"name"`
	Description string `json:"description"`
	Phase       string `json:"phase" doc:"permitted_by_default, denied_by_default, disconnected or removed"`
	State       string `json:"state" doc:"permitted or denied"`
	DocURL      string `json:"docUrl,omitempty"`
	ProvidedBy  string `json:"providedBy,omitempty"`
	Used        bool   `json:"used"`
}

// DeprecatedFeatures lists deprecated features and whether each is in use.
func (c *Client) DeprecatedFeatures(ctx context.Context) ([]DeprecatedFeature, error) {
	type raw struct {
		Name       string `json:"name"`
		Desc       string `json:"desc"`
		Phase      string `json:"deprecation_phase"`
		State      string `json:"state"`
		DocURL     string `json:"doc_url"`
		ProvidedBy string `json:"provided_by"`
	}
	var all, used []raw
	if err := c.Get(ctx, "/api/deprecated-features", &all); err != nil {
		return nil, err
	}
	if err := c.Get(ctx, "/api/deprecated-features/used", &used); err != nil {
		return nil, err
	}
	inUse := map[string]bool{}
	for _, u := range used {
		inUse[u.Name] = true
	}
	out := make([]DeprecatedFeature, 0, len(all))
	for _, f := range all {
		out = append(out, DeprecatedFeature{Name: f.Name, Description: f.Desc, Phase: f.Phase, State: f.State, DocURL: f.DocURL,
			ProvidedBy: f.ProvidedBy, Used: inUse[f.Name]})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}
