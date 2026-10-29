package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"time"
)

// Credential describes how rabmin authenticates to RabbitMQ.
type Credential struct {
	// Type is one of none, basic, bearer.
	Type     string `json:"type"`
	Username string `json:"username,omitempty"`
	Password string `json:"password,omitempty"`
	// Token is an OAuth2 access token sent as a bearer token, for brokers
	// whose management API uses OAuth2.
	Token string `json:"token,omitempty"`
}

// TLSConfig holds PEM encoded TLS material for connections to RabbitMQ.
type TLSConfig struct {
	CA                 string `json:"ca,omitempty"`
	Cert               string `json:"cert,omitempty"`
	Key                string `json:"key,omitempty"`
	InsecureSkipVerify bool   `json:"insecureSkipVerify,omitempty"`
}

// Cluster is a RabbitMQ cluster (or a single node) reached through one or
// more management API URLs.
type Cluster struct {
	ID          string
	Name        string
	Description string
	// ManagementURLs are tried in order; one per node lets rabmin fail over
	// and run node-local health checks on every node.
	ManagementURLs []string
	// PrometheusURLs are the rabbitmq_prometheus endpoints, one per node.
	PrometheusURLs []string
	// AMQPURLs are used for messaging features (publish, browse, move).
	AMQPURLs []string
	TLS      *TLSConfig
	Auth     *Credential
	// Settings is free-form, non-secret configuration (executor, stream URLs).
	Settings  map[string]string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// CredentialProfile is an extra RabbitMQ user for a cluster, so browsing and
// publishing can run with less privilege than the management user.
type CredentialProfile struct {
	ClusterID string
	ID        string
	Name      string
	Auth      Credential
	CreatedAt time.Time
	UpdatedAt time.Time
}

type clusterURLs struct {
	Management []string          `json:"management"`
	Prometheus []string          `json:"prometheus,omitempty"`
	AMQP       []string          `json:"amqp,omitempty"`
	Settings   map[string]string `json:"settings,omitempty"`
}

func (s *Store) sealJSON(v any) (string, error) {
	if v == nil {
		return "", nil
	}
	data, err := json.Marshal(v)
	if err != nil {
		return "", err
	}
	return s.box.Seal(data)
}

func (s *Store) openJSON(sealed string, v any) error {
	if sealed == "" {
		return nil
	}
	data, err := s.box.Open(sealed)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

const clusterCols = `id, name, description, urls, tls, auth, created_at, updated_at`

// ListClusters returns all registered clusters.
func (s *Store) ListClusters(ctx context.Context) ([]Cluster, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT `+clusterCols+` FROM clusters ORDER BY name`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Cluster{}
	for rows.Next() {
		c, err := s.scanCluster(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *c)
	}
	return out, rows.Err()
}

// GetCluster loads one cluster.
func (s *Store) GetCluster(ctx context.Context, id string) (*Cluster, error) {
	return s.scanCluster(s.db.QueryRowContext(ctx, `SELECT `+clusterCols+` FROM clusters WHERE id = ?`, id))
}

func (s *Store) scanCluster(row interface{ Scan(...any) error }) (*Cluster, error) {
	var c Cluster
	var urls, tlsSealed, authSealed string
	var created, updated int64
	if err := row.Scan(&c.ID, &c.Name, &c.Description, &urls, &tlsSealed, &authSealed, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	var u clusterURLs
	if err := json.Unmarshal([]byte(urls), &u); err != nil {
		return nil, err
	}
	c.ManagementURLs, c.PrometheusURLs, c.AMQPURLs, c.Settings = u.Management, u.Prometheus, u.AMQP, u.Settings
	if c.ManagementURLs == nil {
		c.ManagementURLs = []string{}
	}
	if c.PrometheusURLs == nil {
		c.PrometheusURLs = []string{}
	}
	if c.AMQPURLs == nil {
		c.AMQPURLs = []string{}
	}
	if tlsSealed != "" {
		c.TLS = &TLSConfig{}
		if err := s.openJSON(tlsSealed, c.TLS); err != nil {
			return nil, err
		}
	}
	if authSealed != "" {
		c.Auth = &Credential{}
		if err := s.openJSON(authSealed, c.Auth); err != nil {
			return nil, err
		}
	}
	c.CreatedAt = time.Unix(created, 0).UTC()
	c.UpdatedAt = time.Unix(updated, 0).UTC()
	return &c, nil
}

// SaveCluster inserts or updates a cluster.
func (s *Store) SaveCluster(ctx context.Context, c *Cluster, create bool) error {
	urls, err := json.Marshal(clusterURLs{Management: c.ManagementURLs, Prometheus: c.PrometheusURLs, AMQP: c.AMQPURLs, Settings: c.Settings})
	if err != nil {
		return err
	}
	var tlsSealed, authSealed string
	if c.TLS != nil {
		if tlsSealed, err = s.sealJSON(c.TLS); err != nil {
			return err
		}
	}
	if c.Auth != nil {
		if authSealed, err = s.sealJSON(c.Auth); err != nil {
			return err
		}
	}
	now := time.Now()
	if create {
		_, err = s.db.ExecContext(ctx, `INSERT INTO clusters (`+clusterCols+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
			c.ID, c.Name, c.Description, string(urls), tlsSealed, authSealed, now.Unix(), now.Unix())
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE clusters SET name = ?, description = ?, urls = ?, tls = ?, auth = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.Description, string(urls), tlsSealed, authSealed, now.Unix(), c.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCluster removes a cluster and its credential profiles.
func (s *Store) DeleteCluster(ctx context.Context, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM clusters WHERE id = ?`, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// ListCredentials returns the credential profiles of a cluster.
func (s *Store) ListCredentials(ctx context.Context, clusterID string) ([]CredentialProfile, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT cluster_id, id, name, auth, created_at, updated_at FROM credentials WHERE cluster_id = ? ORDER BY name`, clusterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []CredentialProfile{}
	for rows.Next() {
		p, err := s.scanCredential(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, *p)
	}
	return out, rows.Err()
}

// GetCredential loads one credential profile.
func (s *Store) GetCredential(ctx context.Context, clusterID, id string) (*CredentialProfile, error) {
	return s.scanCredential(s.db.QueryRowContext(ctx, `SELECT cluster_id, id, name, auth, created_at, updated_at FROM credentials WHERE cluster_id = ? AND id = ?`, clusterID, id))
}

func (s *Store) scanCredential(row interface{ Scan(...any) error }) (*CredentialProfile, error) {
	var p CredentialProfile
	var authSealed string
	var created, updated int64
	if err := row.Scan(&p.ClusterID, &p.ID, &p.Name, &authSealed, &created, &updated); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if err := s.openJSON(authSealed, &p.Auth); err != nil {
		return nil, err
	}
	p.CreatedAt = time.Unix(created, 0).UTC()
	p.UpdatedAt = time.Unix(updated, 0).UTC()
	return &p, nil
}

// SaveCredential inserts or updates a credential profile.
func (s *Store) SaveCredential(ctx context.Context, p *CredentialProfile, create bool) error {
	sealed, err := s.sealJSON(p.Auth)
	if err != nil {
		return err
	}
	now := time.Now().Unix()
	if create {
		_, err = s.db.ExecContext(ctx, `INSERT INTO credentials (cluster_id, id, name, auth, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?)`,
			p.ClusterID, p.ID, p.Name, sealed, now, now)
		if isUnique(err) {
			return ErrConflict
		}
		return err
	}
	res, err := s.db.ExecContext(ctx, `UPDATE credentials SET name = ?, auth = ?, updated_at = ? WHERE cluster_id = ? AND id = ?`,
		p.Name, sealed, now, p.ClusterID, p.ID)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}

// DeleteCredential removes a credential profile.
func (s *Store) DeleteCredential(ctx context.Context, clusterID, id string) error {
	res, err := s.db.ExecContext(ctx, `DELETE FROM credentials WHERE cluster_id = ? AND id = ?`, clusterID, id)
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
