package rabbitx

import (
	"strings"
	"testing"
	"time"
)

const sampleMetrics = `# TYPE rabbitmq_identity_info untyped
rabbitmq_identity_info{rabbitmq_node="rabbit@a",rabbitmq_cluster="east",rabbitmq_endpoint="aggregated"} 1
rabbitmq_build_info{rabbitmq_version="4.4.0",erlang_version="29.1.2"} 1
rabbitmq_global_messages_received_total{protocol="amqp091"} 1.15e4
rabbitmq_global_messages_received_total{protocol="mqtt50"} 0.0
rabbitmq_global_messages_delivered_total{protocol="amqp091",queue_type="rabbit_quorum_queue"} 100
rabbitmq_queue_messages_ready 42
weird_label{a="x\"y,z",b="2"} 3
`

func TestParseProm(t *testing.T) {
	m, err := ParseProm(strings.NewReader(sampleMetrics))
	if err != nil {
		t.Fatal(err)
	}
	if m.Node != "rabbit@a" || m.Cluster != "east" || m.Version != "4.4.0" {
		t.Fatalf("identity: %+v", m)
	}
	if m.Get("rabbitmq_global_messages_received_total") != 11500 || m.Get("rabbitmq_queue_messages_ready") != 42 || m.Get("weird_label") != 3 {
		t.Fatalf("values: %v", m.Values)
	}
	if got := label(`a="x\"y,z",b="2"`, "b"); got != "2" {
		t.Fatalf("label after escaped quote: %q", got)
	}
}

func TestPromRatesSurviveNodeRestart(t *testing.T) {
	t0 := time.Now()
	scrape := func(node string, received float64) *PromMetrics {
		return &PromMetrics{Node: node, Values: map[string]float64{"rabbitmq_global_messages_received_total": received}}
	}
	prev := newPromTotals(t0, []*PromMetrics{scrape("a", 100), scrape("b", 1000)})
	// b restarted: its counter went down and must not produce a negative rate.
	cur := newPromTotals(t0.Add(10*time.Second), []*PromMetrics{scrape("a", 200), scrape("b", 5)})
	r, ok := cur.rates(prev)
	if !ok || r.Publish != 10 {
		t.Fatalf("want 10/s publish, got %+v ok=%v", r, ok)
	}
}

func TestBuildSnapshotUsesPrometheusWithoutStats(t *testing.T) {
	ov := &overviewRaw{ClusterName: "east", DisableStats: true}
	res := pollResult{overview: ov, nodes: []nodeRaw{{Name: "rabbit@a", Running: true, RabbitMQVersion: "4.4.0"}, {Name: "rabbit@b", RabbitMQVersion: "4.3.1", Running: true, MemAlarm: true}},
		prom: []*PromMetrics{{Node: "rabbit@a", Values: map[string]float64{"rabbitmq_queue_messages_ready": 7, "rabbitmq_queue_messages_unacked": 3}}}}
	rates := Rates{Publish: 5}
	s := buildSnapshot("east", time.Now(), res, &rates)
	if s.RatesSource != "prometheus" || s.Rates.Publish != 5 || s.Totals.Messages != 10 {
		t.Fatalf("prometheus fallback: source=%s rates=%+v totals=%+v", s.RatesSource, s.Rates, s.Totals)
	}
	kinds := map[string]bool{}
	for _, a := range s.Attention {
		kinds[a.Kind] = true
	}
	if !kinds["alarm"] || !kinds["version-mismatch"] || len(s.Alarms) != 1 {
		t.Fatalf("attention: %+v", s.Attention)
	}
}

func TestAtLeast(t *testing.T) {
	for v, want := range map[string]bool{"3.12.14": false, "3.13.0": true, "4.0.5": true, "10.1.0": true, "2.8.1": false, "4.1.0-rc.1": true, "weird": true} {
		if AtLeast(v, 3, 13) != want {
			t.Errorf("AtLeast(%q, 3, 13) != %v", v, want)
		}
	}
}
