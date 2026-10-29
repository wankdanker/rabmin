package api

import (
	"context"
	"errors"
	"net/http"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/danielgtaylor/huma/v2"

	"github.com/wankdanker/rabmin/internal/rabbitx"
	"github.com/wankdanker/rabmin/internal/store"
)

var tagClusters = []string{"Clusters"}

var slugRe = regexp.MustCompile(`^[a-z0-9][a-z0-9-]{0,62}$`)

// CredentialIn sets how rabmin authenticates to RabbitMQ. On update, leaving
// a secret field empty keeps the stored value.
type CredentialIn struct {
	Type     string `json:"type" enum:"basic,bearer" doc:"basic: username and password; bearer: an OAuth2 access token for brokers whose management API uses OAuth2"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	Token    string `json:"token,omitempty"`
}

// CredentialOut describes stored credentials without revealing secrets.
type CredentialOut struct {
	Type        string `json:"type"`
	Username    string `json:"username,omitempty"`
	HasPassword bool   `json:"hasPassword"`
	HasToken    bool   `json:"hasToken"`
}

// TLSIn sets TLS material. On update, an empty key keeps the stored key.
type TLSIn struct {
	CA                 string `json:"ca,omitempty" doc:"PEM CA bundle"`
	Cert               string `json:"cert,omitempty" doc:"PEM client certificate"`
	Key                string `json:"key,omitempty" doc:"PEM client key"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
}

// TLSOut describes TLS settings without the private key.
type TLSOut struct {
	CA                 string `json:"ca,omitempty"`
	Cert               string `json:"cert,omitempty"`
	HasKey             bool   `json:"hasKey"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify"`
}

// CredentialProfileOut is an extra RabbitMQ user stored for a cluster.
type CredentialProfileOut struct {
	ID        string        `json:"id"`
	Name      string        `json:"name"`
	Auth      CredentialOut `json:"auth"`
	CreatedAt time.Time     `json:"createdAt"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

// CredentialProfileIn creates or updates a credential profile.
type CredentialProfileIn struct {
	ID   string        `json:"id,omitempty" doc:"Slug; derived from name when empty"`
	Name string        `json:"name" minLength:"1" maxLength:"100"`
	Auth *CredentialIn `json:"auth,omitempty"`
}

// ClusterStatus is the live state of a cluster.
type ClusterStatus struct {
	Connected    bool       `json:"connected"`
	Error        string     `json:"error,omitempty"`
	LastPoll     *time.Time `json:"lastPoll,omitempty"`
	ClusterName  string     `json:"clusterName,omitempty" doc:"The name RabbitMQ reports"`
	Versions     []string   `json:"versions"`
	Nodes        int        `json:"nodes"`
	NodesRunning int        `json:"nodesRunning"`
	Healthy      int        `json:"healthy" doc:"Running nodes without alarms, partitions or failing health checks"`
	Alarms       int        `json:"alarms"`
	Attention    int        `json:"attention"`
	Connections  int        `json:"connections"`
	Queues       int        `json:"queues"`
	Consumers    int        `json:"consumers"`
	Messages     float64    `json:"messages"`
	PublishRate  float64    `json:"publishRate"`
	DeliverRate  float64    `json:"deliverRate"`
	RatesSource  string     `json:"ratesSource"`
}

// ClusterOut is a registered RabbitMQ cluster.
type ClusterOut struct {
	ID             string                 `json:"id"`
	Name           string                 `json:"name"`
	Description    string                 `json:"description"`
	ManagementURLs []string               `json:"managementUrls"`
	PrometheusURLs []string               `json:"prometheusUrls"`
	AMQPURLs       []string               `json:"amqpUrls"`
	TLS            *TLSOut                `json:"tls,omitempty"`
	Auth           CredentialOut          `json:"auth"`
	Credentials    []CredentialProfileOut `json:"credentials"`
	Status         ClusterStatus          `json:"status"`
	CreatedAt      time.Time              `json:"createdAt"`
	UpdatedAt      time.Time              `json:"updatedAt"`
}

// ClusterIn creates or updates a cluster. On update, omitted objects keep
// their stored values.
type ClusterIn struct {
	ID             string                `json:"id,omitempty" doc:"Slug used in URLs; derived from name when empty. Cannot change."`
	Name           string                `json:"name" minLength:"1" maxLength:"100"`
	Description    string                `json:"description,omitempty" maxLength:"1000"`
	ManagementURLs []string              `json:"managementUrls" minItems:"1" doc:"Management API base URLs (http://host:15672), ideally one per node: rabmin fails over between them and runs node-local health checks through each"`
	PrometheusURLs []string              `json:"prometheusUrls,omitempty" doc:"rabbitmq_prometheus base URLs (http://host:15692), one per node"`
	AMQPURLs       []string              `json:"amqpUrls,omitempty" doc:"AMQP URLs for messaging features (amqp://host:5672), without credentials"`
	TLS            *TLSIn                `json:"tls,omitempty"`
	RemoveTLS      bool                  `json:"removeTls,omitempty"`
	Auth           *CredentialIn         `json:"auth,omitempty" doc:"Management API credentials; required on create"`
	Credentials    []CredentialProfileIn `json:"credentials,omitempty" doc:"Credential profiles to create (create only)"`
}

// ClusterPath identifies a cluster.
type ClusterPath struct {
	Cluster string `path:"cluster" doc:"Cluster id"`
}

// CredentialPath identifies a credential profile of a cluster.
type CredentialPath struct {
	Cluster    string `path:"cluster" doc:"Cluster id"`
	Credential string `path:"credential" doc:"Credential profile id"`
}

// TestResult reports a connection test.
type TestResult struct {
	OK              bool                  `json:"ok"`
	Error           string                `json:"error,omitempty"`
	ClusterName     string                `json:"clusterName,omitempty"`
	ProductName     string                `json:"productName,omitempty"`
	RabbitMQVersion string                `json:"rabbitmqVersion,omitempty"`
	ErlangVersion   string                `json:"erlangVersion,omitempty"`
	Nodes           int                   `json:"nodes"`
	User            string                `json:"user,omitempty"`
	Tags            []string              `json:"tags"`
	IsAdministrator bool                  `json:"isAdministrator"`
	CanMonitor      bool                  `json:"canMonitor" doc:"The user can see every vhost's objects (administrator or monitoring tag)"`
	Plugins         []string              `json:"plugins" doc:"Plugins enabled on any node"`
	StatsDisabled   bool                  `json:"statsDisabled"`
	Endpoints       []rabbitx.Endpoint    `json:"endpoints"`
	Prometheus      []rabbitx.PromSummary `json:"prometheus"`
	Warnings        []string              `json:"warnings"`
}

func credOut(c *store.Credential) CredentialOut {
	if c == nil {
		return CredentialOut{Type: "none"}
	}
	return CredentialOut{Type: c.Type, Username: c.Username, HasPassword: c.Password != "", HasToken: c.Token != ""}
}

func mergeCred(old *store.Credential, in *CredentialIn) store.Credential {
	c := store.Credential{Type: in.Type, Username: in.Username, Password: in.Password, Token: in.Token}
	if old != nil && old.Type == in.Type {
		if c.Password == "" && c.Username == old.Username {
			c.Password = old.Password
		}
		if c.Token == "" {
			c.Token = old.Token
		}
	}
	switch c.Type {
	case "basic":
		c.Token = ""
	case "bearer":
		c.Username, c.Password = "", ""
	}
	return c
}

func validateCred(c store.Credential) error {
	switch c.Type {
	case "basic":
		if c.Username == "" {
			return errors.New("username is required")
		}
	case "bearer":
		if c.Token == "" {
			return errors.New("token is required")
		}
	default:
		return errors.New("type must be basic or bearer")
	}
	return nil
}

func slugify(name string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(name) {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
			dash = false
		default:
			if !dash && b.Len() > 0 {
				b.WriteByte('-')
				dash = true
			}
		}
	}
	out := strings.TrimRight(b.String(), "-")
	if len(out) > 63 {
		out = out[:63]
	}
	if out == "" {
		out = "cluster"
	}
	return out
}

// cleanURLs splits comma or newline separated URLs, trims them and checks
// they parse with one of the given schemes.
func cleanURLs(field string, in []string, schemes ...string) ([]string, error) {
	out := []string{}
	for _, u := range in {
		for _, p := range strings.FieldsFunc(u, func(r rune) bool { return r == ',' || r == '\n' }) {
			p = strings.TrimRight(strings.TrimSpace(p), "/")
			if p == "" {
				continue
			}
			pu, err := url.Parse(p)
			if err != nil || pu.Host == "" || !slices.Contains(schemes, pu.Scheme) {
				return nil, huma.Error422UnprocessableEntity(field + ": " + p + " must be a " + strings.Join(schemes, " or ") + " URL with a host")
			}
			if pu.User != nil {
				return nil, huma.Error422UnprocessableEntity(field + ": put credentials in the credential fields, not in the URL")
			}
			out = append(out, p)
		}
	}
	return out, nil
}

func (s *Server) clusterOut(ctx context.Context, c *store.Cluster) (ClusterOut, error) {
	o := ClusterOut{ID: c.ID, Name: c.Name, Description: c.Description, ManagementURLs: c.ManagementURLs, PrometheusURLs: c.PrometheusURLs,
		AMQPURLs: c.AMQPURLs, Auth: credOut(c.Auth), Credentials: []CredentialProfileOut{}, CreatedAt: c.CreatedAt, UpdatedAt: c.UpdatedAt}
	if c.TLS != nil {
		o.TLS = &TLSOut{CA: c.TLS.CA, Cert: c.TLS.Cert, HasKey: c.TLS.Key != "", InsecureSkipVerify: c.TLS.InsecureSkipVerify}
	}
	profiles, err := s.store.ListCredentials(ctx, c.ID)
	if err != nil {
		return o, err
	}
	for _, p := range profiles {
		o.Credentials = append(o.Credentials, profileOut(&p))
	}
	o.Status = ClusterStatus{Versions: []string{}, RatesSource: "none"}
	if snap := s.mgr.Snapshot(c.ID); snap != nil {
		t := snap.Time
		st := &o.Status
		st.LastPoll = &t
		st.Connected, st.Error = snap.Connected, snap.Error
		st.ClusterName, st.Versions = snap.Info.ClusterName, snap.Info.Versions
		st.Nodes, st.NodesRunning = snap.Totals.Nodes, snap.Totals.NodesRunning
		st.Alarms, st.Attention = len(snap.Alarms), len(snap.Attention)
		st.Connections, st.Queues, st.Consumers, st.Messages = snap.Totals.Connections, snap.Totals.Queues, snap.Totals.Consumers, snap.Totals.Messages
		st.PublishRate, st.DeliverRate, st.RatesSource = snap.Rates.Publish, snap.Rates.DeliverGet, snap.RatesSource
		for _, n := range snap.Nodes {
			if n.Healthy {
				st.Healthy++
			}
		}
	}
	return o, nil
}

func profileOut(p *store.CredentialProfile) CredentialProfileOut {
	return CredentialProfileOut{ID: p.ID, Name: p.Name, Auth: credOut(&p.Auth), CreatedAt: p.CreatedAt, UpdatedAt: p.UpdatedAt}
}

func profileFromIn(clusterID string, in CredentialProfileIn, old *store.Credential) (store.CredentialProfile, error) {
	id := in.ID
	if id == "" {
		id = slugify(in.Name)
	}
	if !slugRe.MatchString(id) {
		return store.CredentialProfile{}, huma.Error422UnprocessableEntity("credential id must be lowercase letters, digits and dashes")
	}
	p := store.CredentialProfile{ClusterID: clusterID, ID: id, Name: in.Name}
	switch {
	case in.Auth != nil:
		p.Auth = mergeCred(old, in.Auth)
	case old != nil:
		p.Auth = *old
	default:
		return p, huma.Error422UnprocessableEntity("auth is required")
	}
	if err := validateCred(p.Auth); err != nil {
		return p, huma.Error422UnprocessableEntity("auth: " + err.Error())
	}
	return p, nil
}

// applyURLs validates and sets the URL lists of a cluster.
func applyURLs(c *store.Cluster, b *ClusterIn) error {
	var err error
	if c.ManagementURLs, err = cleanURLs("managementUrls", b.ManagementURLs, "http", "https"); err != nil {
		return err
	}
	if len(c.ManagementURLs) == 0 {
		return huma.Error422UnprocessableEntity("managementUrls: at least one URL is required")
	}
	if c.PrometheusURLs, err = cleanURLs("prometheusUrls", b.PrometheusURLs, "http", "https"); err != nil {
		return err
	}
	c.AMQPURLs, err = cleanURLs("amqpUrls", b.AMQPURLs, "amqp", "amqps")
	return err
}

func (s *Server) registerClusters() {
	reg(s, route{method: http.MethodGet, path: "/v1/clusters", id: "list-clusters", summary: "List clusters", tags: tagClusters, role: store.RoleViewer},
		func(ctx context.Context, _ *struct{}) (*struct{ Body []ClusterOut }, error) {
			clusters, err := s.store.ListClusters(ctx)
			if err != nil {
				return nil, apiErr(err)
			}
			out := []ClusterOut{}
			for i := range clusters {
				o, err := s.clusterOut(ctx, &clusters[i])
				if err != nil {
					return nil, apiErr(err)
				}
				out = append(out, o)
			}
			return &struct{ Body []ClusterOut }{out}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}", id: "get-cluster", summary: "Get a cluster", tags: tagClusters, role: store.RoleViewer},
		func(ctx context.Context, in *ClusterPath) (*struct{ Body ClusterOut }, error) {
			c, err := s.store.GetCluster(ctx, in.Cluster)
			if err != nil {
				return nil, apiErr(err)
			}
			o, err := s.clusterOut(ctx, c)
			return &struct{ Body ClusterOut }{o}, apiErr(err)
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters", id: "create-cluster", summary: "Register a cluster", tags: tagClusters, role: store.RoleAdmin, status: http.StatusCreated},
		func(ctx context.Context, in *struct{ Body ClusterIn }) (*struct{ Body ClusterOut }, error) {
			b := in.Body
			id := b.ID
			if id == "" {
				id = slugify(b.Name)
			}
			if !slugRe.MatchString(id) {
				return nil, huma.Error422UnprocessableEntity("id must be lowercase letters, digits and dashes")
			}
			c := store.Cluster{ID: id, Name: b.Name, Description: b.Description}
			if err := applyURLs(&c, &b); err != nil {
				return nil, err
			}
			if b.TLS != nil {
				c.TLS = &store.TLSConfig{CA: b.TLS.CA, Cert: b.TLS.Cert, Key: b.TLS.Key, InsecureSkipVerify: b.TLS.InsecureSkipVerify}
			}
			if b.Auth == nil {
				return nil, huma.Error422UnprocessableEntity("auth: management API credentials are required")
			}
			cred := mergeCred(nil, b.Auth)
			if err := validateCred(cred); err != nil {
				return nil, huma.Error422UnprocessableEntity("auth: " + err.Error())
			}
			c.Auth = &cred
			profiles := []store.CredentialProfile{}
			for _, p := range b.Credentials {
				prof, err := profileFromIn(id, p, nil)
				if err != nil {
					return nil, err
				}
				profiles = append(profiles, prof)
			}
			if err := s.store.SaveCluster(ctx, &c, true); err != nil {
				return nil, apiErr(err)
			}
			for i := range profiles {
				if err := s.store.SaveCredential(ctx, &profiles[i], true); err != nil {
					return nil, apiErr(err)
				}
			}
			saved, err := s.store.GetCluster(ctx, id)
			if err != nil {
				return nil, apiErr(err)
			}
			s.mgr.Reload(*saved)
			_, _ = s.mgr.PollNow(ctx, id)
			o, err := s.clusterOut(ctx, saved)
			return &struct{ Body ClusterOut }{o}, apiErr(err)
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}", id: "update-cluster", summary: "Update a cluster", tags: tagClusters, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body ClusterIn
		}) (*struct{ Body ClusterOut }, error) {
			c, err := s.store.GetCluster(ctx, in.Cluster)
			if err != nil {
				return nil, apiErr(err)
			}
			b := in.Body
			c.Name, c.Description = b.Name, b.Description
			if err := applyURLs(c, &b); err != nil {
				return nil, err
			}
			switch {
			case b.RemoveTLS:
				c.TLS = nil
			case b.TLS != nil:
				key := b.TLS.Key
				if key == "" && c.TLS != nil {
					key = c.TLS.Key
				}
				c.TLS = &store.TLSConfig{CA: b.TLS.CA, Cert: b.TLS.Cert, Key: key, InsecureSkipVerify: b.TLS.InsecureSkipVerify}
			}
			if b.Auth != nil {
				cred := mergeCred(c.Auth, b.Auth)
				if err := validateCred(cred); err != nil {
					return nil, huma.Error422UnprocessableEntity("auth: " + err.Error())
				}
				c.Auth = &cred
			}
			if err := s.store.SaveCluster(ctx, c, false); err != nil {
				return nil, apiErr(err)
			}
			saved, err := s.store.GetCluster(ctx, c.ID)
			if err != nil {
				return nil, apiErr(err)
			}
			s.mgr.Reload(*saved)
			o, err := s.clusterOut(ctx, saved)
			return &struct{ Body ClusterOut }{o}, apiErr(err)
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}", id: "delete-cluster", summary: "Remove a cluster",
		desc: "Forgets the cluster and its stored credentials. Nothing changes on the brokers.", tags: tagClusters, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *ClusterPath) (*struct{}, error) {
			if err := s.store.DeleteCluster(ctx, in.Cluster); err != nil {
				return nil, apiErr(err)
			}
			s.mgr.Remove(in.Cluster)
			return nil, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/test", id: "test-cluster-connection", summary: "Test connection settings",
		desc: "Connects with the given settings without saving them and reports versions, plugins and what the user may do. " +
			"Pass clusterId (and credentialId) to reuse stored secrets for fields left empty.",
		tags: tagClusters, role: store.RoleAdmin, noAudit: true},
		func(ctx context.Context, in *struct {
			Body struct {
				ClusterID      string        `json:"clusterId,omitempty"`
				CredentialID   string        `json:"credentialId,omitempty"`
				ManagementURLs []string      `json:"managementUrls" minItems:"1"`
				PrometheusURLs []string      `json:"prometheusUrls,omitempty"`
				TLS            *TLSIn        `json:"tls,omitempty"`
				Auth           *CredentialIn `json:"auth,omitempty"`
			}
		}) (*struct{ Body TestResult }, error) {
			b := in.Body
			var oldCred *store.Credential
			var tlsCfg *store.TLSConfig
			if b.ClusterID != "" {
				c, err := s.store.GetCluster(ctx, b.ClusterID)
				if err != nil {
					return nil, apiErr(err)
				}
				tlsCfg, oldCred = c.TLS, c.Auth
				if b.CredentialID != "" {
					p, err := s.store.GetCredential(ctx, b.ClusterID, b.CredentialID)
					if err != nil {
						return nil, apiErr(err)
					}
					oldCred = &p.Auth
				}
			}
			if b.TLS != nil {
				key := b.TLS.Key
				if key == "" && tlsCfg != nil {
					key = tlsCfg.Key
				}
				tlsCfg = &store.TLSConfig{CA: b.TLS.CA, Cert: b.TLS.Cert, Key: key, InsecureSkipVerify: b.TLS.InsecureSkipVerify}
			}
			cred := oldCred
			if b.Auth != nil {
				c := mergeCred(oldCred, b.Auth)
				cred = &c
			}
			mgmt, err := cleanURLs("managementUrls", b.ManagementURLs, "http", "https")
			if err != nil {
				return nil, err
			}
			prom, err := cleanURLs("prometheusUrls", b.PrometheusURLs, "http", "https")
			if err != nil {
				return nil, err
			}
			return &struct{ Body TestResult }{testConnection(ctx, mgmt, prom, tlsCfg, cred)}, nil
		})

	reg(s, route{method: http.MethodGet, path: "/v1/clusters/{cluster}/credentials", id: "list-credentials", summary: "List credential profiles", tags: tagClusters, role: store.RoleViewer},
		func(ctx context.Context, in *ClusterPath) (*struct{ Body []CredentialProfileOut }, error) {
			if _, err := s.store.GetCluster(ctx, in.Cluster); err != nil {
				return nil, apiErr(err)
			}
			ps, err := s.store.ListCredentials(ctx, in.Cluster)
			if err != nil {
				return nil, apiErr(err)
			}
			out := []CredentialProfileOut{}
			for _, p := range ps {
				out = append(out, profileOut(&p))
			}
			return &struct{ Body []CredentialProfileOut }{out}, nil
		})

	reg(s, route{method: http.MethodPost, path: "/v1/clusters/{cluster}/credentials", id: "create-credential", summary: "Add a credential profile",
		desc: "An extra RabbitMQ user for this cluster, so browsing and publishing can run with less privilege than the management user.",
		tags: tagClusters, role: store.RoleAdmin, status: http.StatusCreated},
		func(ctx context.Context, in *struct {
			ClusterPath
			Body CredentialProfileIn
		}) (*struct{ Body CredentialProfileOut }, error) {
			if _, err := s.store.GetCluster(ctx, in.Cluster); err != nil {
				return nil, apiErr(err)
			}
			p, err := profileFromIn(in.Cluster, in.Body, nil)
			if err != nil {
				return nil, err
			}
			if err := s.store.SaveCredential(ctx, &p, true); err != nil {
				return nil, apiErr(err)
			}
			saved, err := s.store.GetCredential(ctx, in.Cluster, p.ID)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body CredentialProfileOut }{profileOut(saved)}, nil
		})

	reg(s, route{method: http.MethodPut, path: "/v1/clusters/{cluster}/credentials/{credential}", id: "update-credential", summary: "Update a credential profile", tags: tagClusters, role: store.RoleAdmin},
		func(ctx context.Context, in *struct {
			CredentialPath
			Body CredentialProfileIn
		}) (*struct{ Body CredentialProfileOut }, error) {
			old, err := s.store.GetCredential(ctx, in.Cluster, in.Credential)
			if err != nil {
				return nil, apiErr(err)
			}
			in.Body.ID = in.Credential
			p, err := profileFromIn(in.Cluster, in.Body, &old.Auth)
			if err != nil {
				return nil, err
			}
			if err := s.store.SaveCredential(ctx, &p, false); err != nil {
				return nil, apiErr(err)
			}
			saved, err := s.store.GetCredential(ctx, in.Cluster, in.Credential)
			if err != nil {
				return nil, apiErr(err)
			}
			return &struct{ Body CredentialProfileOut }{profileOut(saved)}, nil
		})

	reg(s, route{method: http.MethodDelete, path: "/v1/clusters/{cluster}/credentials/{credential}", id: "delete-credential", summary: "Remove a credential profile", tags: tagClusters, role: store.RoleAdmin, status: http.StatusNoContent},
		func(ctx context.Context, in *CredentialPath) (*struct{}, error) {
			return nil, apiErr(s.store.DeleteCredential(ctx, in.Cluster, in.Credential))
		})
}

func testConnection(ctx context.Context, mgmt, prom []string, tlsCfg *store.TLSConfig, cred *store.Credential) TestResult {
	r := TestResult{Tags: []string{}, Plugins: []string{}, Endpoints: []rabbitx.Endpoint{}, Prometheus: []rabbitx.PromSummary{}, Warnings: []string{}}
	if cred == nil {
		r.Error = "credentials are required"
		return r
	}
	cl, err := rabbitx.NewClient(mgmt, tlsCfg, cred, 8*time.Second)
	if err != nil {
		r.Error = err.Error()
		return r
	}
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()

	// Every endpoint, so a bad URL in the list is visible.
	r.Endpoints = make([]rabbitx.Endpoint, len(mgmt))
	var wg sync.WaitGroup
	for i, u := range mgmt {
		wg.Add(1)
		go func() {
			defer wg.Done()
			e := rabbitx.Endpoint{URL: u}
			start := time.Now()
			var o struct {
				Node string `json:"node"`
			}
			err := cl.DoAt(ctx, u, http.MethodGet, "/api/overview?columns=node", nil, &o)
			e.LatencyMs = time.Since(start).Milliseconds()
			if err != nil {
				e.Error = describe(err)
			} else {
				e.OK, e.Node = true, o.Node
			}
			r.Endpoints[i] = e
		}()
	}
	if len(prom) > 0 {
		r.Prometheus = make([]rabbitx.PromSummary, len(prom))
		tr, _ := rabbitx.Transport(tlsCfg)
		hc := &http.Client{Transport: tr, Timeout: 8 * time.Second}
		for i, u := range prom {
			wg.Add(1)
			go func() {
				defer wg.Done()
				ps := rabbitx.PromSummary{URL: u}
				if m, err := rabbitx.Scrape(ctx, hc, u); err != nil {
					ps.Error = err.Error()
				} else {
					ps.OK, ps.Node = true, m.Node
				}
				r.Prometheus[i] = ps
			}()
		}
	}
	wg.Wait()

	var who struct {
		Name string `json:"name"`
		Tags any    `json:"tags"`
	}
	if err := cl.Get(ctx, "/api/whoami", &who); err != nil {
		r.Error = describe(err)
		return r
	}
	r.OK, r.User = true, who.Name
	switch t := who.Tags.(type) {
	case []any:
		for _, x := range t {
			if s, ok := x.(string); ok && s != "" {
				r.Tags = append(r.Tags, s)
			}
		}
	case string: // RabbitMQ 3.x: comma separated
		for _, x := range strings.Split(t, ",") {
			if x = strings.TrimSpace(x); x != "" {
				r.Tags = append(r.Tags, x)
			}
		}
	}
	r.IsAdministrator = slices.Contains(r.Tags, "administrator")
	r.CanMonitor = r.IsAdministrator || slices.Contains(r.Tags, "monitoring")
	if !r.CanMonitor {
		r.Warnings = append(r.Warnings, "This user has neither the administrator nor the monitoring tag, so rabmin only sees the vhosts it has permissions in, and node details are hidden.")
	} else if !r.IsAdministrator {
		r.Warnings = append(r.Warnings, "Monitoring user: rabmin can observe the cluster but not change users, policies or definitions.")
	}

	var ov struct {
		ClusterName     string `json:"cluster_name"`
		ProductName     string `json:"product_name"`
		RabbitMQVersion string `json:"rabbitmq_version"`
		ErlangVersion   string `json:"erlang_version"`
		DisableStats    bool   `json:"disable_stats"`
	}
	if err := cl.Get(ctx, "/api/overview", &ov); err == nil {
		r.ClusterName, r.ProductName, r.RabbitMQVersion, r.ErlangVersion, r.StatsDisabled = ov.ClusterName, ov.ProductName, ov.RabbitMQVersion, ov.ErlangVersion, ov.DisableStats
		if ov.DisableStats && len(prom) == 0 {
			r.Warnings = append(r.Warnings, "The management stats database is disabled; add Prometheus URLs so rabmin can show message rates.")
		}
		if !rabbitx.AtLeast(ov.RabbitMQVersion, 3, 13) {
			r.Warnings = append(r.Warnings, "rabmin supports RabbitMQ 3.13 and later; "+ov.RabbitMQVersion+" may not work fully.")
		}
	}
	var nodes []struct {
		Name           string   `json:"name"`
		EnabledPlugins []string `json:"enabled_plugins"`
	}
	if err := cl.Get(ctx, "/api/nodes?columns=name,enabled_plugins", &nodes); err == nil {
		r.Nodes = len(nodes)
		set := map[string]bool{}
		for _, n := range nodes {
			for _, p := range n.EnabledPlugins {
				set[p] = true
			}
		}
		for p := range set {
			r.Plugins = append(r.Plugins, p)
		}
		sort.Strings(r.Plugins)
		if len(set) == 0 {
			r.Warnings = append(r.Warnings, "The nodes have not reported their statistics yet (they do a few seconds after boot), so plugins are unknown.")
		} else if !set["rabbitmq_prometheus"] {
			r.Warnings = append(r.Warnings, "rabbitmq_prometheus is not enabled.")
		}
		reached := map[string]bool{}
		for _, e := range r.Endpoints {
			reached[e.Node] = true
		}
		missing := []string{}
		for _, n := range nodes {
			if !reached[n.Name] {
				missing = append(missing, n.Name)
			}
		}
		if len(missing) > 0 && len(nodes) > 1 {
			r.Warnings = append(r.Warnings, "No management URL reaches "+strings.Join(missing, ", ")+"; add one per node for failover and per-node health checks.")
		}
	}
	return r
}

// describe turns a rabbitx error into a message for people.
func describe(err error) string {
	var ae *rabbitx.APIError
	if errors.As(err, &ae) {
		switch {
		case ae.Unauthenticated():
			return "RabbitMQ rejected the credentials (401)"
		case ae.Forbidden():
			return "RabbitMQ refused: " + ae.Reason
		}
	}
	return err.Error()
}
