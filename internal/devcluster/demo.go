package devcluster

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand/v2"
	"net/http"
	"time"

	amqp "github.com/rabbitmq/amqp091-go"
)

// Seed declares a demo topology: vhosts, users, exchanges, queues of every
// type, policies, and, when a "west" cluster exists, a federation upstream
// and a shovel between the clusters.
func (e *Env) Seed(ctx context.Context) error {
	east := e.Nodes[0]
	api := func(n Node, method, path string, body any) error {
		return e.API(ctx, n, method, path, body, nil)
	}
	steps := []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, "/api/vhosts/shop", map[string]any{"description": "Order processing", "tags": "production", "default_queue_type": "quorum"}},
		{http.MethodPut, "/api/vhosts/analytics", map[string]any{"description": "Event analytics", "default_queue_type": "stream"}},
		{http.MethodPut, "/api/users/monitor", map[string]any{"password": "monitor", "tags": "monitoring"}},
		{http.MethodPut, "/api/users/shop-app", map[string]any{"password": "shop-app", "tags": ""}},
		{http.MethodPut, "/api/permissions/shop/admin", map[string]any{"configure": ".*", "write": ".*", "read": ".*"}},
		{http.MethodPut, "/api/permissions/analytics/admin", map[string]any{"configure": ".*", "write": ".*", "read": ".*"}},
		{http.MethodPut, "/api/permissions/shop/shop-app", map[string]any{"configure": "^(orders|billing|notifications)", "write": ".*", "read": ".*"}},
		{http.MethodPut, "/api/permissions/shop/monitor", map[string]any{"configure": "", "write": "", "read": ".*"}},
		{http.MethodPut, "/api/user-limits/shop-app/max-connections", map[string]any{"value": 50}},
		{http.MethodPut, "/api/vhost-limits/shop/max-queues", map[string]any{"value": 500}},

		{http.MethodPut, "/api/exchanges/shop/orders", map[string]any{"type": "topic", "durable": true, "arguments": map[string]any{"alternate-exchange": "orders.unrouted"}}},
		{http.MethodPut, "/api/exchanges/shop/orders.unrouted", map[string]any{"type": "fanout", "durable": true}},
		{http.MethodPut, "/api/exchanges/shop/events", map[string]any{"type": "fanout", "durable": true}},
		{http.MethodPut, "/api/exchanges/shop/dlx", map[string]any{"type": "direct", "durable": true}},
		{http.MethodPut, "/api/exchanges/shop/priority", map[string]any{"type": "headers", "durable": true}},
		{http.MethodPut, "/api/queues/shop/orders.created", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "quorum", "x-dead-letter-exchange": "dlx", "x-dead-letter-routing-key": "orders.dead"}}},
		{http.MethodPut, "/api/queues/shop/orders.shipped", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "quorum"}}},
		{http.MethodPut, "/api/queues/shop/orders.unrouted", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "quorum", "x-max-length": 1000}}},
		{http.MethodPut, "/api/queues/shop/orders.dead", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "quorum"}}},
		{http.MethodPut, "/api/queues/shop/billing", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "classic", "x-max-priority": 5}}},
		{http.MethodPut, "/api/queues/shop/notifications", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "quorum", "x-dead-letter-exchange": "dlx", "x-dead-letter-routing-key": "orders.dead", "x-delivery-limit": 3}}},
		{http.MethodPut, "/api/queues/shop/reports.pending", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "classic"}}},
		{http.MethodPut, "/api/queues/shop/audit.log", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "stream", "x-max-age": "7D"}}},
		{http.MethodPut, "/api/queues/shop/shovel.outbox", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "quorum"}}},
		{http.MethodPut, "/api/queues/%2F/tasks", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "classic"}}},
		{http.MethodPut, "/api/queues/analytics/clicks", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "stream"}}},

		{http.MethodPost, "/api/bindings/shop/e/orders/q/orders.created", map[string]any{"routing_key": "orders.created.*"}},
		{http.MethodPost, "/api/bindings/shop/e/orders/q/orders.shipped", map[string]any{"routing_key": "orders.shipped.*"}},
		{http.MethodPost, "/api/bindings/shop/e/orders/q/billing", map[string]any{"routing_key": "orders.created.#"}},
		{http.MethodPost, "/api/bindings/shop/e/orders/e/events", map[string]any{"routing_key": "#"}},
		{http.MethodPost, "/api/bindings/shop/e/events/q/audit.log", map[string]any{"routing_key": ""}},
		{http.MethodPost, "/api/bindings/shop/e/events/q/notifications", map[string]any{"routing_key": ""}},
		{http.MethodPost, "/api/bindings/shop/e/orders.unrouted/q/orders.unrouted", map[string]any{"routing_key": ""}},
		{http.MethodPost, "/api/bindings/shop/e/dlx/q/orders.dead", map[string]any{"routing_key": "orders.dead"}},
		{http.MethodPost, "/api/bindings/shop/e/priority/q/billing", map[string]any{"routing_key": "", "arguments": map[string]any{"x-match": "all", "tier": "gold"}}},

		{http.MethodPut, "/api/policies/shop/orders-limits", map[string]any{"pattern": "^orders\\.", "apply-to": "quorum_queues", "priority": 1, "definition": map[string]any{"delivery-limit": 10, "max-length": 100000}}},
		{http.MethodPut, "/api/policies/shop/reports-ttl", map[string]any{"pattern": "^reports\\.", "apply-to": "queues", "priority": 0, "definition": map[string]any{"message-ttl": 3600000}}},
		{http.MethodPut, "/api/operator-policies/shop/cap-queue-length", map[string]any{"pattern": ".*", "apply-to": "queues", "priority": 0, "definition": map[string]any{"max-length-bytes": 1073741824}}},
	}
	for _, s := range steps {
		if err := api(east, s.method, s.path, s.body); err != nil {
			return err
		}
	}

	west := e.ClusterNodes("west")
	if len(west) == 0 {
		return nil
	}
	w := west[0]
	westSteps := []struct {
		method, path string
		body         any
	}{
		{http.MethodPut, "/api/vhosts/replica", map[string]any{"description": "Copies of east's shop traffic"}},
		{http.MethodPut, "/api/permissions/replica/admin", map[string]any{"configure": ".*", "write": ".*", "read": ".*"}},
		// Federate east's shop/events exchange into west's replica vhost.
		{http.MethodPut, "/api/parameters/federation-upstream/replica/east", map[string]any{"value": map[string]any{
			"uri": e.Nodes[0].InternalAMQPURL + "/shop", "exchange": "events", "ack-mode": "on-confirm", "trust-user-id": false}}},
		{http.MethodPut, "/api/exchanges/replica/events", map[string]any{"type": "fanout", "durable": true}},
		{http.MethodPut, "/api/queues/replica/events.copy", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "quorum"}}},
		{http.MethodPost, "/api/bindings/replica/e/events/q/events.copy", map[string]any{"routing_key": ""}},
		{http.MethodPut, "/api/policies/replica/federate-events", map[string]any{"pattern": "^events$", "apply-to": "exchanges", "definition": map[string]any{"federation-upstream-set": "all"}}},
		{http.MethodPut, "/api/queues/replica/shovel.inbox", map[string]any{"durable": true, "arguments": map[string]any{"x-queue-type": "quorum"}}},
	}
	for _, s := range westSteps {
		if err := api(w, s.method, s.path, s.body); err != nil {
			return err
		}
	}
	// Shovel east's shop/shovel.outbox to west's replica/shovel.inbox.
	return api(east, http.MethodPut, "/api/parameters/shovel/shop/outbox-to-west", map[string]any{"value": map[string]any{
		"src-protocol": "amqp091", "src-uri": "amqp:///shop", "src-queue": "shovel.outbox",
		"dest-protocol": "amqp091", "dest-uri": w.InternalAMQPURL + "/replica", "dest-queue": "shovel.inbox",
		"ack-mode": "on-confirm", "src-delete-after": "never",
	}})
}

// Traffic runs demo publishers and consumers until ctx is done. Clients
// reconnect when a node goes away.
func (e *Env) Traffic(ctx context.Context, log *slog.Logger) {
	if log == nil {
		log = slog.New(slog.DiscardHandler)
	}
	east := e.ClusterNodes("east")
	if len(east) == 0 {
		east = e.Nodes
	}
	node := func(i int) Node { return east[i%len(east)] }
	type worker struct {
		name  string
		node  Node
		vhost string
		run   func(ctx context.Context, ch *amqp.Channel) error
	}
	workers := []worker{
		{"orders-api", node(0), "shop", publisher("orders", func(n int) (string, amqp.Publishing) {
			kind := "created"
			if n%3 == 0 {
				kind = "shipped"
			}
			rk := fmt.Sprintf("orders.%s.%s", kind, []string{"eu", "us", "apac"}[n%3])
			if n%97 == 0 {
				rk = "orders.unknown.eu" // falls through to the alternate exchange
			}
			body := fmt.Sprintf(`{"order":%d,"amount":%.2f,"currency":"EUR"}`, n, rand.Float64()*500)
			return rk, amqp.Publishing{ContentType: "application/json", DeliveryMode: amqp.Persistent, Body: []byte(body),
				MessageId: fmt.Sprintf("ord-%d", n), Headers: amqp.Table{"source": "orders-api", "tier": []string{"gold", "silver"}[n%2]}}
		}, 40*time.Millisecond, true)},
		{"checkout-api", node(1), "shop", publisher("priority", func(n int) (string, amqp.Publishing) {
			return "", amqp.Publishing{ContentType: "text/plain", Body: []byte(fmt.Sprintf("invoice %d", n)), Priority: uint8(n % 5),
				Headers: amqp.Table{"tier": []string{"gold", "silver"}[n%2]}}
		}, 200*time.Millisecond, false)},
		{"order-processor", node(1), "shop", consumer("orders.created", 20, 5*time.Millisecond, 0)},
		{"shipping-service", node(2), "shop", consumer("orders.shipped", 10, 10*time.Millisecond, 0)},
		{"billing-worker", node(2), "shop", consumer("billing", 5, 60*time.Millisecond, 0)},
		{"notifier", node(0), "shop", consumer("notifications", 50, 2*time.Millisecond, 0.05)},
		{"outbox-relay", node(0), "shop", publisher("", func(n int) (string, amqp.Publishing) {
			return "shovel.outbox", amqp.Publishing{ContentType: "application/json", Body: []byte(fmt.Sprintf(`{"outbox":%d}`, n))}
		}, 500*time.Millisecond, false)},
		{"report-generator", node(2), "shop", publisher("", func(n int) (string, amqp.Publishing) {
			return "reports.pending", amqp.Publishing{ContentType: "application/json", Body: []byte(fmt.Sprintf(`{"report":%d}`, n))}
		}, 50*time.Millisecond, false)},
		{"task-producer", node(1), "/", publisher("", func(n int) (string, amqp.Publishing) {
			return "tasks", amqp.Publishing{Body: []byte(fmt.Sprintf("task %d", n))}
		}, 100*time.Millisecond, false)},
		{"task-worker-1", node(2), "/", consumer("tasks", 1, 150*time.Millisecond, 0)},
		{"task-worker-2", node(0), "/", consumer("tasks", 1, 150*time.Millisecond, 0)},
		{"clickstream", node(2), "analytics", publisher("", func(n int) (string, amqp.Publishing) {
			return "clicks", amqp.Publishing{ContentType: "application/json", Body: []byte(fmt.Sprintf(`{"page":"/p/%d","user":%d}`, n%40, n%7))}
		}, 30*time.Millisecond, false)},
	}
	if west := e.ClusterNodes("west"); len(west) > 0 {
		workers = append(workers,
			worker{"replica-reader", west[0], "replica", consumer("events.copy", 50, time.Millisecond, 0)},
			worker{"inbox-reader", west[0], "replica", consumer("shovel.inbox", 10, 20*time.Millisecond, 0)},
		)
	}
	for _, w := range workers {
		go func() {
			for ctx.Err() == nil {
				err := e.runWorker(ctx, w.node, w.vhost, w.name, w.run)
				if ctx.Err() != nil {
					return
				}
				log.Debug("demo client restarting", "client", w.name, "err", err)
				select {
				case <-ctx.Done():
					return
				case <-time.After(2 * time.Second):
				}
			}
		}()
	}
	<-ctx.Done()
}

func (e *Env) runWorker(ctx context.Context, n Node, vhost, name string, run func(context.Context, *amqp.Channel) error) error {
	cfg := amqp.Config{Properties: amqp.Table{"connection_name": name, "product": "rabmin-devcluster"}, Heartbeat: 10 * time.Second}
	conn, err := amqp.DialConfig(e.AMQPURL(n, vhost), cfg)
	if err != nil {
		return err
	}
	defer conn.Close()
	ch, err := conn.Channel()
	if err != nil {
		return err
	}
	return run(ctx, ch)
}

func publisher(exchange string, next func(n int) (string, amqp.Publishing), every time.Duration, confirm bool) func(context.Context, *amqp.Channel) error {
	return func(ctx context.Context, ch *amqp.Channel) error {
		if confirm {
			if err := ch.Confirm(false); err != nil {
				return err
			}
		}
		closed := ch.NotifyClose(make(chan *amqp.Error, 1))
		t := time.NewTicker(every)
		defer t.Stop()
		for n := 1; ; n++ {
			select {
			case <-ctx.Done():
				return nil
			case err := <-closed:
				return err
			case <-t.C:
			}
			rk, msg := next(n)
			msg.Timestamp = time.Now()
			if confirm {
				if _, err := ch.PublishWithDeferredConfirmWithContext(ctx, exchange, rk, false, false, msg); err != nil {
					return err
				}
			} else if err := ch.PublishWithContext(ctx, exchange, rk, false, false, msg); err != nil {
				return err
			}
		}
	}
}

func consumer(queue string, prefetch int, work time.Duration, rejectRatio float64) func(context.Context, *amqp.Channel) error {
	return func(ctx context.Context, ch *amqp.Channel) error {
		if err := ch.Qos(prefetch, 0, false); err != nil {
			return err
		}
		msgs, err := ch.ConsumeWithContext(ctx, queue, "", false, false, false, false, nil)
		if err != nil {
			return err
		}
		for {
			select {
			case <-ctx.Done():
				return nil
			case m, ok := <-msgs:
				if !ok {
					return fmt.Errorf("consumer on %s closed", queue)
				}
				time.Sleep(work)
				if rejectRatio > 0 && rand.Float64() < rejectRatio {
					_ = m.Nack(false, false)
				} else {
					_ = m.Ack(false)
				}
			}
		}
	}
}
