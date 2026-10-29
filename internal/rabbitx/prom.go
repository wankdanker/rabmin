package rabbitx

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
)

// PromMetrics is one scrape of a node's rabbitmq_prometheus endpoint, with
// every metric summed over its labels.
type PromMetrics struct {
	Node    string
	Cluster string
	Version string
	Values  map[string]float64
}

// Get returns a metric, 0 when absent.
func (m *PromMetrics) Get(name string) float64 {
	if m == nil {
		return 0
	}
	return m.Values[name]
}

// Scrape reads base+"/metrics" (aggregated per node).
func Scrape(ctx context.Context, hc *http.Client, base string) (*PromMetrics, error) {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	base = strings.TrimSuffix(base, "/metrics")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, base+"/metrics", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/plain")
	resp, err := hc.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prometheus %s/metrics: %s", redact(base), resp.Status)
	}
	return ParseProm(resp.Body)
}

// ParseProm parses the Prometheus text exposition format.
func ParseProm(r io.Reader) (*PromMetrics, error) {
	m := &PromMetrics{Values: map[string]float64{}}
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 64*1024), 4<<20)
	for sc.Scan() {
		line := sc.Text()
		if line == "" || line[0] == '#' {
			continue
		}
		var name, labels, rest string
		if i := strings.IndexByte(line, '{'); i >= 0 {
			j := strings.LastIndexByte(line, '}')
			if j < i {
				continue
			}
			name, labels, rest = line[:i], line[i+1:j], strings.TrimSpace(line[j+1:])
		} else if i := strings.IndexByte(line, ' '); i >= 0 {
			name, rest = line[:i], strings.TrimSpace(line[i+1:])
		} else {
			continue
		}
		if f := strings.Fields(rest); len(f) > 0 {
			rest = f[0]
		}
		v, err := strconv.ParseFloat(rest, 64)
		if err != nil {
			continue
		}
		switch name {
		case "rabbitmq_identity_info":
			m.Node = label(labels, "rabbitmq_node")
			m.Cluster = label(labels, "rabbitmq_cluster")
		case "rabbitmq_build_info":
			m.Version = label(labels, "rabbitmq_version")
		}
		m.Values[name] += v
	}
	return m, sc.Err()
}

func label(labels, key string) string {
	for labels != "" {
		k, rest, ok := strings.Cut(labels, "=\"")
		if !ok {
			return ""
		}
		var b strings.Builder
		i := 0
		for ; i < len(rest); i++ {
			if rest[i] == '\\' && i+1 < len(rest) {
				i++
				b.WriteByte(rest[i])
				continue
			}
			if rest[i] == '"' {
				break
			}
			b.WriteByte(rest[i])
		}
		if strings.TrimSpace(k) == key {
			return b.String()
		}
		if i >= len(rest) {
			return ""
		}
		labels = strings.TrimPrefix(strings.TrimLeft(rest[i+1:], " "), ",")
	}
	return ""
}
