package devcluster_test

import (
	"context"
	"net/http"
	"testing"
	"time"

	"github.com/wankdanker/rabmin/internal/devcluster"
)

func TestClusterAndSeed(t *testing.T) {
	if testing.Short() || !devcluster.DockerAvailable() {
		t.Skip("needs Docker")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	env, err := devcluster.Start(ctx, devcluster.Config{Prefix: "rabmin-test-devcluster", Dir: t.TempDir(),
		Clusters: []devcluster.ClusterSpec{{Name: "east", Nodes: 3}, {Name: "west", Nodes: 1}}})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Shutdown()

	var nodes []struct {
		Name    string `json:"name"`
		Running bool   `json:"running"`
	}
	if err := env.API(ctx, env.Nodes[2], http.MethodGet, "/api/nodes", nil, &nodes); err != nil {
		t.Fatal(err)
	}
	if len(nodes) != 3 {
		t.Fatalf("east should have 3 nodes, got %+v", nodes)
	}
	if err := env.Seed(ctx); err != nil {
		t.Fatal(err)
	}
	var shovels []struct {
		Name  string `json:"name"`
		State string `json:"state"`
	}
	deadline := time.Now().Add(30 * time.Second)
	for time.Now().Before(deadline) {
		if err := env.API(ctx, env.Nodes[0], http.MethodGet, "/api/shovels", nil, &shovels); err == nil && len(shovels) == 1 && shovels[0].State == "running" {
			break
		}
		time.Sleep(time.Second)
	}
	if len(shovels) != 1 || shovels[0].State != "running" {
		t.Fatalf("shovel to west is not running: %+v", shovels)
	}
}
