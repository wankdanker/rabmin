// Command rabmin-devcluster runs local RabbitMQ clusters in Docker for
// developing and trying rabmin: a 3-node cluster "east" and a single-node
// cluster "west", joined by a federation link and a shovel.
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/wankdanker/rabmin/internal/devcluster"
)

func main() {
	dir := flag.String("dir", ".devcluster", "directory for generated RabbitMQ configs")
	random := flag.Bool("random-ports", false, "pick free host ports instead of 15672+n (management), 5672+n (AMQP), 15692+n (Prometheus), 5552+n (stream), 1883+n (MQTT)")
	clusters := flag.String("clusters", "east:3,west:1", "comma separated name:nodes")
	image := flag.String("image", "", "RabbitMQ image (default "+devcluster.DefaultImage+", or RABMIN_RABBITMQ_IMAGE)")
	demo := flag.Bool("demo", false, "seed vhosts, queues, policies, federation and a shovel, and generate traffic")
	down := flag.Bool("down", false, "remove the containers of an earlier run and exit")
	flag.Parse()

	cfg := devcluster.Config{Prefix: "rabmin-dev", Dir: *dir, FixedPorts: !*random, Image: *image}
	for _, c := range strings.Split(*clusters, ",") {
		name, n, _ := strings.Cut(strings.TrimSpace(c), ":")
		size, err := strconv.Atoi(n)
		if err != nil || size < 1 {
			size = 1
		}
		cfg.Clusters = append(cfg.Clusters, devcluster.ClusterSpec{Name: name, Nodes: size})
	}
	if *down {
		cfg.Clusters = nil
		devcluster.Remove(cfg.Prefix)
		return
	}
	if !devcluster.DockerAvailable() {
		log.Fatal("rabmin-devcluster needs a running Docker daemon")
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	fmt.Println("starting RabbitMQ nodes (this takes a minute)...")
	env, err := devcluster.Start(ctx, cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer env.Shutdown()
	user, pass := env.User()
	fmt.Println("clusters are up")
	for _, n := range env.Nodes {
		fmt.Printf("  %-26s cluster=%-5s management %s  prometheus %s  amqp %s  stream :%d\n",
			n.Name, n.Cluster, n.ManagementURL, n.PrometheusURL, n.AMQPURL, n.StreamPort)
	}
	fmt.Printf("\nadministrator: %s / %s\n", user, pass)
	if *demo {
		if err := env.Seed(ctx); err != nil {
			log.Fatalf("seed: %v", err)
		}
		go env.Traffic(ctx, slog.Default())
		fmt.Println("seeded vhosts shop, analytics (east) and replica (west); generating demo traffic")
	}
	fmt.Println("press ctrl-c to stop and remove the containers")
	<-ctx.Done()
}
