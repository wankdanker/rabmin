// Package devcluster runs throwaway RabbitMQ clusters in Docker for
// development and integration tests. RabbitMQ cannot be embedded the way
// nats-server can, so this drives the docker CLI instead.
package devcluster

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// DefaultImage is the RabbitMQ image used when Config.Image is empty. It can
// be overridden with RABMIN_RABBITMQ_IMAGE.
const DefaultImage = "rabbitmq:4-management"

// Plugins enabled on every node.
var Plugins = []string{
	"rabbitmq_management", "rabbitmq_prometheus",
	"rabbitmq_shovel", "rabbitmq_shovel_management",
	"rabbitmq_federation", "rabbitmq_federation_management",
	"rabbitmq_stream", "rabbitmq_stream_management",
	"rabbitmq_top", "rabbitmq_tracing",
	"rabbitmq_mqtt", "rabbitmq_stomp",
}

// ClusterSpec describes one RabbitMQ cluster.
type ClusterSpec struct {
	Name  string
	Nodes int
}

// Config describes the clusters to start.
type Config struct {
	// Prefix names the Docker network and containers. Distinct prefixes let
	// test packages run in parallel.
	Prefix   string
	Image    string
	Clusters []ClusterSpec
	// Dir holds the generated configuration files.
	Dir string
	// FixedPorts publishes node n (counting from 0 across clusters) on the
	// standard ports plus n: management 15672+n, AMQP 5672+n, Prometheus
	// 15692+n, stream 5552+n, MQTT 1883+n. Otherwise free ports are picked.
	FixedPorts bool
	// User and Password are the default administrator. Defaults: admin/admin.
	User, Password string
	// Timeout bounds the wait for every node to boot.
	Timeout time.Duration
}

// Node is one running RabbitMQ node.
type Node struct {
	Name      string // Erlang node name, rabbit@<container>
	Cluster   string
	Container string
	// Host-side URLs.
	ManagementURL string
	PrometheusURL string
	AMQPURL       string // without credentials
	StreamPort    int
	MQTTPort      int
	// InternalAMQPURL reaches the node from other containers (federation, shovels).
	InternalAMQPURL string
}

// Env is a set of running clusters.
type Env struct {
	cfg     Config
	Nodes   []Node
	network string
}

// ClusterNodes returns the nodes of one cluster.
func (e *Env) ClusterNodes(cluster string) []Node {
	var out []Node
	for _, n := range e.Nodes {
		if n.Cluster == cluster {
			out = append(out, n)
		}
	}
	return out
}

// User returns the administrator credentials.
func (e *Env) User() (string, string) { return e.cfg.User, e.cfg.Password }

// DockerAvailable reports whether a Docker daemon answers.
func DockerAvailable() bool {
	if _, err := exec.LookPath("docker"); err != nil {
		return false
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, "docker", "info").Run() == nil
}

func docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	var out, errb bytes.Buffer
	cmd.Stdout, cmd.Stderr = &out, &errb
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("docker %s: %v: %s", args[0], err, strings.TrimSpace(errb.String()))
	}
	return strings.TrimSpace(out.String()), nil
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// Start launches the clusters and waits until every node is running and
// clustered.
func Start(ctx context.Context, cfg Config) (*Env, error) {
	if cfg.Prefix == "" {
		cfg.Prefix = "rabmin-dev"
	}
	if cfg.Image == "" {
		cfg.Image = os.Getenv("RABMIN_RABBITMQ_IMAGE")
	}
	if cfg.Image == "" {
		cfg.Image = DefaultImage
	}
	if cfg.User == "" {
		cfg.User, cfg.Password = "admin", "admin"
	}
	if cfg.Timeout <= 0 {
		cfg.Timeout = 3 * time.Minute
	}
	if len(cfg.Clusters) == 0 {
		cfg.Clusters = []ClusterSpec{{Name: "east", Nodes: 3}}
	}
	if cfg.Dir == "" {
		d, err := os.MkdirTemp("", cfg.Prefix+"-")
		if err != nil {
			return nil, err
		}
		cfg.Dir = d
	}
	if err := os.MkdirAll(cfg.Dir, 0o755); err != nil {
		return nil, err
	}
	// Containers read the mounted files as the rabbitmq user.
	_ = os.Chmod(cfg.Dir, 0o755)
	e := &Env{cfg: cfg, network: cfg.Prefix + "-net"}
	// Remove leftovers of an earlier run with the same prefix.
	e.removeContainers(context.Background())
	if _, err := docker(ctx, "network", "create", e.network); err != nil && !strings.Contains(err.Error(), "already exists") {
		return nil, err
	}

	plugins := filepath.Join(cfg.Dir, "enabled_plugins")
	if err := os.WriteFile(plugins, []byte("["+strings.Join(Plugins, ",")+"].\n"), 0o644); err != nil {
		return nil, err
	}

	idx := 0
	for _, cs := range cfg.Clusters {
		for i := 1; i <= cs.Nodes; i++ {
			container := fmt.Sprintf("%s-%s-%d", cfg.Prefix, cs.Name, i)
			if cs.Nodes == 1 {
				container = fmt.Sprintf("%s-%s", cfg.Prefix, cs.Name)
			}
			ports := make([]int, 5)
			for k, std := range []int{15672, 5672, 15692, 5552, 1883} {
				if cfg.FixedPorts {
					ports[k] = std + idx
				} else {
					p, err := freePort()
					if err != nil {
						return nil, err
					}
					ports[k] = p
				}
			}
			mgmt, amqp, prom, stream, mqtt := ports[0], ports[1], ports[2], ports[3], ports[4]
			var conf strings.Builder
			fmt.Fprintf(&conf, "cluster_name = %s\n", cs.Name)
			if cs.Nodes > 1 {
				conf.WriteString("cluster_formation.peer_discovery_backend = classic_config\n")
				for j := 1; j <= cs.Nodes; j++ {
					fmt.Fprintf(&conf, "cluster_formation.classic_config.nodes.%d = rabbit@%s-%s-%d\n", j, cfg.Prefix, cs.Name, j)
				}
			}
			fmt.Fprintf(&conf, "default_user = %s\ndefault_pass = %s\nloopback_users = none\n", cfg.User, cfg.Password)
			// Streams advertise the host-side port so clients outside Docker can connect.
			fmt.Fprintf(&conf, "stream.listeners.tcp.1 = %d\nstream.advertised_host = localhost\nstream.advertised_port = %d\n", stream, stream)
			conf.WriteString("collect_statistics_interval = 2000\n")
			confPath := filepath.Join(cfg.Dir, container+".conf")
			if err := os.WriteFile(confPath, []byte(conf.String()), 0o644); err != nil {
				return nil, err
			}
			args := []string{"run", "-d", "--name", container, "--hostname", container, "--network", e.network,
				"--label", "rabmin.devcluster=" + cfg.Prefix,
				"-e", "RABBITMQ_ERLANG_COOKIE=" + cfg.Prefix + "-" + cs.Name + "-cookie",
				"-v", confPath + ":/etc/rabbitmq/rabbitmq.conf:ro",
				"-v", plugins + ":/etc/rabbitmq/enabled_plugins:ro",
				"-p", fmt.Sprintf("127.0.0.1:%d:15672", mgmt),
				"-p", fmt.Sprintf("127.0.0.1:%d:5672", amqp),
				"-p", fmt.Sprintf("127.0.0.1:%d:15692", prom),
				"-p", fmt.Sprintf("127.0.0.1:%d:%d", stream, stream),
				"-p", fmt.Sprintf("127.0.0.1:%d:1883", mqtt),
				cfg.Image}
			if _, err := docker(ctx, args...); err != nil {
				e.Shutdown()
				return nil, err
			}
			e.Nodes = append(e.Nodes, Node{
				Name: "rabbit@" + container, Cluster: cs.Name, Container: container,
				ManagementURL:   fmt.Sprintf("http://127.0.0.1:%d", mgmt),
				PrometheusURL:   fmt.Sprintf("http://127.0.0.1:%d", prom),
				AMQPURL:         fmt.Sprintf("amqp://127.0.0.1:%d", amqp),
				StreamPort:      stream,
				MQTTPort:        mqtt,
				InternalAMQPURL: fmt.Sprintf("amqp://%s:%s@%s:5672", cfg.User, cfg.Password, container),
			})
			idx++
		}
	}

	wctx, cancel := context.WithTimeout(ctx, cfg.Timeout)
	defer cancel()
	var wg sync.WaitGroup
	errs := make([]error, len(cfg.Clusters))
	for i, cs := range cfg.Clusters {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = e.waitCluster(wctx, cs)
		}()
	}
	wg.Wait()
	if err := errors.Join(errs...); err != nil {
		e.Shutdown()
		return nil, err
	}
	return e, nil
}

// waitCluster polls every node of a cluster until all of them report all
// cluster members running.
func (e *Env) waitCluster(ctx context.Context, cs ClusterSpec) error {
	nodes := e.ClusterNodes(cs.Name)
	for _, n := range nodes {
		for {
			if running, err := e.runningNodes(ctx, n); err == nil && running == cs.Nodes {
				break
			}
			select {
			case <-ctx.Done():
				logs, _ := docker(context.Background(), "logs", "--tail", "30", n.Container)
				return fmt.Errorf("node %s did not become ready: %w\n%s", n.Name, ctx.Err(), logs)
			case <-time.After(time.Second):
			}
		}
	}
	return nil
}

func (e *Env) runningNodes(ctx context.Context, n Node) (int, error) {
	var nodes []struct {
		Running bool     `json:"running"`
		Plugins []string `json:"enabled_plugins"`
	}
	if err := e.API(ctx, n, http.MethodGet, "/api/nodes?columns=name,running,enabled_plugins", nil, &nodes); err != nil {
		return 0, err
	}
	// Node details appear with the first statistics emission, a few
	// seconds after boot; count a node only once they are there.
	running := 0
	for _, x := range nodes {
		if x.Running && len(x.Plugins) > 0 {
			running++
		}
	}
	return running, nil
}

// API calls the management API of a node as the administrator.
func (e *Env) API(ctx context.Context, n Node, method, path string, body, out any) error {
	var rd *bytes.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(b)
	} else {
		rd = bytes.NewReader(nil)
	}
	req, err := http.NewRequestWithContext(ctx, method, n.ManagementURL+path, rd)
	if err != nil {
		return err
	}
	req.SetBasicAuth(e.cfg.User, e.cfg.Password)
	req.Header.Set("Content-Type", "application/json")
	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	var buf bytes.Buffer
	_, _ = buf.ReadFrom(resp.Body)
	if resp.StatusCode >= 300 {
		return fmt.Errorf("%s %s: %s: %s", method, path, resp.Status, strings.TrimSpace(buf.String()))
	}
	if out != nil && buf.Len() > 0 {
		return json.Unmarshal(buf.Bytes(), out)
	}
	return nil
}

// AMQPURL returns an AMQP URL with the administrator credentials for a node and vhost.
func (e *Env) AMQPURL(n Node, vhost string) string {
	u := strings.Replace(n.AMQPURL, "amqp://", fmt.Sprintf("amqp://%s:%s@", e.cfg.User, e.cfg.Password), 1)
	if vhost != "" && vhost != "/" {
		u += "/" + vhost
	}
	return u
}

// Exec runs a command in a node's container (rabbitmqctl, rabbitmq-diagnostics...).
func (e *Env) Exec(ctx context.Context, n Node, args ...string) (string, error) {
	return docker(ctx, append([]string{"exec", n.Container}, args...)...)
}

// Stop stops a node's container without removing it.
func (e *Env) Stop(ctx context.Context, n Node) error {
	_, err := docker(ctx, "stop", n.Container)
	return err
}

// StartNode starts a stopped node again.
func (e *Env) StartNode(ctx context.Context, n Node) error {
	_, err := docker(ctx, "start", n.Container)
	return err
}

func (e *Env) removeContainers(ctx context.Context) {
	ids, err := docker(ctx, "ps", "-aq", "--filter", "label=rabmin.devcluster="+e.cfg.Prefix)
	if err != nil || ids == "" {
		return
	}
	_, _ = docker(ctx, append([]string{"rm", "-f", "-v"}, strings.Fields(ids)...)...)
}

// Shutdown removes the containers and the network.
func (e *Env) Shutdown() {
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	e.removeContainers(ctx)
	_, _ = docker(ctx, "network", "rm", e.network)
}

// Remove deletes the containers and network of a prefix, for example after
// an interrupted run.
func Remove(prefix string) {
	(&Env{cfg: Config{Prefix: prefix}, network: prefix + "-net"}).Shutdown()
}
