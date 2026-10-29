// Command rabmin-devcluster runs local RabbitMQ clusters in Docker for
// developing and trying rabmin: a 3-node cluster "east" and a single-node
// cluster "west", joined by a federation link and a shovel.
package main

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	clyops "github.com/wankdanker/clyops/packages/go"

	"github.com/wankdanker/rabmin/internal/devcluster"
)

func main() {
	cli := clyops.New()
	cli.Name = "rabmin-devcluster"
	cli.SetDescription("Run local RabbitMQ clusters in Docker for developing and trying rabmin: a 3-node cluster east and a single-node cluster west, joined by federation and a shovel.")
	cli.SetEffects("network", "destructive")
	cli.Opt("RABMIN_DEVCLUSTER_DIR", "dir", "", ".devcluster", "Directory for generated RabbitMQ configs", "Clusters", "path")
	cli.Opt("RABMIN_DEVCLUSTER_CLUSTERS", "clusters", "", "east:3,west:1", "Clusters as comma separated name:nodes", "Clusters", `regex:^[a-z][a-z0-9-]*(:[0-9]+)?(,[a-z][a-z0-9-]*(:[0-9]+)?)*$`)
	cli.Opt("RABMIN_RABBITMQ_IMAGE", "image", "", devcluster.DefaultImage, "RabbitMQ image", "Clusters")
	cli.Opt("RABMIN_DEVCLUSTER_RANDOM_PORTS", "random-ports", "", "flag",
		"Pick free host ports instead of 15672+n (management), 5672+n (AMQP), 15692+n (Prometheus), 5552+n (stream), 1883+n (MQTT)", "Clusters")
	cli.Opt("RABMIN_DEVCLUSTER_DEMO", "demo", "", "flag", "Seed vhosts, queues, policies, federation and a shovel, and generate traffic", "Demo")
	cli.Opt("RABMIN_DEVCLUSTER_DOWN", "down", "", "flag", "Remove the containers of an earlier run and exit", "Clusters")
	cli.RequireCommand("docker", "Runs the RabbitMQ nodes", "https://docs.docker.com/engine/install/")
	v := cli.Run()

	cfg := devcluster.Config{Prefix: "rabmin-dev", Dir: v.String("RABMIN_DEVCLUSTER_DIR"), FixedPorts: !v.Bool("RABMIN_DEVCLUSTER_RANDOM_PORTS"), Image: v.String("RABMIN_RABBITMQ_IMAGE")}
	for _, c := range strings.Split(v.String("RABMIN_DEVCLUSTER_CLUSTERS"), ",") {
		name, n, _ := strings.Cut(strings.TrimSpace(c), ":")
		size, err := strconv.Atoi(n)
		if err != nil || size < 1 {
			size = 1
		}
		cfg.Clusters = append(cfg.Clusters, devcluster.ClusterSpec{Name: name, Nodes: size})
	}
	if v.Bool("RABMIN_DEVCLUSTER_DOWN") {
		cfg.Clusters = nil
		devcluster.Remove(cfg.Prefix)
		return
	}
	if !devcluster.DockerAvailable() {
		clyops.Die(1, "rabmin-devcluster needs a running Docker daemon")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Println("starting RabbitMQ nodes (this takes a minute)...")
	env, err := devcluster.Start(ctx, cfg)
	if err != nil {
		clyops.Die(1, "%v", err)
	}
	defer env.Shutdown()
	user, pass := env.User()
	fmt.Println("clusters are up")
	for _, n := range env.Nodes {
		fmt.Printf("  %-26s cluster=%-5s management %s  prometheus %s  amqp %s  stream :%d\n",
			n.Name, n.Cluster, n.ManagementURL, n.PrometheusURL, n.AMQPURL, n.StreamPort)
	}
	fmt.Printf("\nadministrator: %s / %s\n", user, pass)
	if v.Bool("RABMIN_DEVCLUSTER_DEMO") {
		if err := env.Seed(ctx); err != nil {
			clyops.Die(1, "seed: %v", err)
		}
		go env.Traffic(ctx, slog.Default())
		fmt.Println("seeded vhosts shop, analytics (east) and replica (west); generating demo traffic")
	}
	fmt.Println("press ctrl-c to stop and remove the containers")
	<-ctx.Done()
}
