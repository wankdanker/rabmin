// Package rabbitx is rabmin's RabbitMQ side: a management API client that
// fails over between node URLs, Prometheus scraping, and the per-cluster
// poller that builds snapshots and history.
package rabbitx

import (
	"bytes"
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/wankdanker/rabmin/internal/store"
)

// APIError is a non-2xx answer from the management API.
type APIError struct {
	Status int
	// Code and Reason are RabbitMQ's {"error": ..., "reason": ...}.
	Code   string
	Reason string
	// Body is the raw response, for health checks that explain failures in JSON.
	Body   []byte
	Method string
	URL    string
}

func (e *APIError) Error() string {
	msg := e.Reason
	if msg == "" {
		msg = e.Code
	}
	if msg == "" {
		msg = http.StatusText(e.Status)
	}
	return fmt.Sprintf("RabbitMQ %s %s: %d %s", e.Method, e.URL, e.Status, msg)
}

// Unauthenticated reports whether RabbitMQ rejected the credentials themselves.
func (e *APIError) Unauthenticated() bool {
	return e.Status == http.StatusUnauthorized && e.Code == "not_authorized"
}

// Forbidden reports whether the credentials are valid but lack privilege.
// RabbitMQ answers these with 401 and error "not_authorised".
func (e *APIError) Forbidden() bool {
	return e.Status == http.StatusForbidden || (e.Status == http.StatusUnauthorized && e.Code != "not_authorized")
}

// ErrUnreachable wraps network errors after every URL was tried.
var ErrUnreachable = errors.New("no management endpoint reachable")

// Client calls the RabbitMQ management API of one cluster.
type Client struct {
	urls []string
	hc   *http.Client
	cred store.Credential

	mu        sync.Mutex
	preferred int
}

// NewClient builds a client for a list of management base URLs
// (http://host:15672, optionally with a path prefix).
func NewClient(urls []string, tlsCfg *store.TLSConfig, cred *store.Credential, timeout time.Duration) (*Client, error) {
	if len(urls) == 0 {
		return nil, errors.New("at least one management URL is required")
	}
	clean := make([]string, 0, len(urls))
	for _, u := range urls {
		u = strings.TrimRight(strings.TrimSpace(u), "/")
		u = strings.TrimSuffix(u, "/api")
		p, err := url.Parse(u)
		if err != nil || (p.Scheme != "http" && p.Scheme != "https") || p.Host == "" {
			return nil, fmt.Errorf("invalid management URL %q: want http(s)://host:port", u)
		}
		clean = append(clean, u)
	}
	tr, err := Transport(tlsCfg)
	if err != nil {
		return nil, err
	}
	if timeout <= 0 {
		timeout = 10 * time.Second
	}
	c := &Client{urls: clean, hc: &http.Client{Transport: tr, Timeout: timeout}}
	if cred != nil {
		c.cred = *cred
	}
	return c, nil
}

// Transport returns an HTTP transport honouring the TLS settings.
func Transport(tlsCfg *store.TLSConfig) (*http.Transport, error) {
	tr := http.DefaultTransport.(*http.Transport).Clone()
	tr.MaxIdleConnsPerHost = 8
	if tlsCfg == nil {
		return tr, nil
	}
	tc := &tls.Config{InsecureSkipVerify: tlsCfg.InsecureSkipVerify} //nolint:gosec // explicit user choice
	if tlsCfg.CA != "" {
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM([]byte(tlsCfg.CA)) {
			return nil, errors.New("TLS CA: no certificates found in PEM")
		}
		tc.RootCAs = pool
	}
	if tlsCfg.Cert != "" || tlsCfg.Key != "" {
		cert, err := tls.X509KeyPair([]byte(tlsCfg.Cert), []byte(tlsCfg.Key))
		if err != nil {
			return nil, fmt.Errorf("TLS client certificate: %w", err)
		}
		tc.Certificates = []tls.Certificate{cert}
	}
	tr.TLSClientConfig = tc
	return tr, nil
}

// URLs returns the management base URLs.
func (c *Client) URLs() []string { return c.urls }

// Do calls path (starting with /api/) on the preferred URL, failing over to
// the next URL on network errors. body is JSON-encoded; out is decoded from
// JSON (a *[]byte receives the raw body).
func (c *Client) Do(ctx context.Context, method, path string, body, out any) error {
	return c.DoHeaders(ctx, method, path, nil, body, out)
}

// DoHeaders is Do with extra request headers (for example X-Reason).
func (c *Client) DoHeaders(ctx context.Context, method, path string, hdr http.Header, body, out any) error {
	c.mu.Lock()
	start := c.preferred
	c.mu.Unlock()
	var errs []error
	for i := range c.urls {
		idx := (start + i) % len(c.urls)
		err := c.do(ctx, c.urls[idx], method, path, hdr, body, out)
		var ae *APIError
		if err == nil || errors.As(err, &ae) || ctx.Err() != nil {
			if err == nil || ae != nil {
				c.mu.Lock()
				c.preferred = idx
				c.mu.Unlock()
			}
			return err
		}
		errs = append(errs, err)
	}
	return fmt.Errorf("%w: %w", ErrUnreachable, errors.Join(errs...))
}

// DoAt calls one specific base URL without failover, for node-local calls
// such as health checks.
func (c *Client) DoAt(ctx context.Context, base, method, path string, body, out any) error {
	return c.do(ctx, base, method, path, nil, body, out)
}

// Get is Do with GET.
func (c *Client) Get(ctx context.Context, path string, out any) error {
	return c.Do(ctx, http.MethodGet, path, nil, out)
}

func (c *Client) do(ctx context.Context, base, method, path string, hdr http.Header, body, out any) error {
	var rd io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, rd)
	if err != nil {
		return err
	}
	for k, v := range hdr {
		req.Header[k] = v
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if method == http.MethodGet {
		// Some resources (DELETE /api/reset) answer 406 to Accept: application/json.
		req.Header.Set("Accept", "application/json")
	}
	switch c.cred.Type {
	case "basic":
		req.SetBasicAuth(c.cred.Username, c.cred.Password)
	case "bearer":
		req.Header.Set("Authorization", "Bearer "+c.cred.Token)
	}
	resp, err := c.hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, 256<<20))
	if err != nil {
		return err
	}
	if resp.StatusCode >= 300 {
		ae := &APIError{Status: resp.StatusCode, Body: data, Method: method, URL: redact(base) + path}
		var e struct {
			Error  string `json:"error"`
			Reason string `json:"reason"`
		}
		if json.Unmarshal(data, &e) == nil {
			ae.Code, ae.Reason = e.Error, e.Reason
		}
		return ae
	}
	if out == nil || len(data) == 0 {
		return nil
	}
	if raw, ok := out.(*[]byte); ok {
		*raw = data
		return nil
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("decode %s%s: %w", redact(base), path, err)
	}
	return nil
}

func redact(u string) string {
	p, err := url.Parse(u)
	if err != nil || p.User == nil {
		return u
	}
	p.User = nil
	return p.String()
}

// Esc escapes one path segment the way the management API expects ("/" becomes %2F).
func Esc(s string) string { return url.PathEscape(s) }

// AtLeast reports whether a RabbitMQ version string is at least major.minor.
// Unparseable versions count as new enough.
func AtLeast(version string, major, minor int) bool {
	parts := strings.SplitN(version, ".", 3)
	if len(parts) < 2 {
		return true
	}
	ma, err1 := strconv.Atoi(parts[0])
	mi, err2 := strconv.Atoi(strings.TrimFunc(parts[1], func(r rune) bool { return r < '0' || r > '9' }))
	if err1 != nil || err2 != nil {
		return true
	}
	return ma > major || (ma == major && mi >= minor)
}
