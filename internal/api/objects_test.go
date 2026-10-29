package api_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/wankdanker/rabmin/internal/api"
	"github.com/wankdanker/rabmin/internal/devcluster"
	"github.com/wankdanker/rabmin/internal/rabbitx"
)

// TestObjectsAndAdmin covers phase 2 against a real 3-node cluster: vhosts,
// exchanges, bindings, queues, publish, users, permissions, policies,
// parameters, definitions and feature flags.
func TestObjectsAndAdmin(t *testing.T) {
	if testing.Short() || !devcluster.DockerAvailable() {
		t.Skip("needs Docker")
	}
	ctx := context.Background()
	env, err := devcluster.Start(ctx, devcluster.Config{Prefix: "rabmin-test-objects", Dir: t.TempDir(),
		Clusters: []devcluster.ClusterSpec{{Name: "east", Nodes: 3}}})
	if err != nil {
		t.Fatal(err)
	}
	defer env.Shutdown()

	srv := newServer(t)
	admin := newClient(t, srv.URL+"/api")
	admin.must("POST", "/v1/setup", map[string]string{"username": "admin", "password": "supersecret"}, nil, 200)
	user, pass := env.User()
	var mgmt []string
	for _, n := range env.Nodes {
		mgmt = append(mgmt, n.ManagementURL)
	}
	admin.must("POST", "/v1/clusters", map[string]any{"name": "East", "managementUrls": mgmt,
		"auth": map[string]string{"type": "basic", "username": user, "password": pass}}, nil, 201)
	c := "/v1/clusters/east"
	esc := url.PathEscape

	// Roles: operators manage queues and exchanges, admins the rest.
	admin.must("POST", "/v1/users", map[string]any{"username": "oscar", "password": "operatorpass", "role": "operator"}, nil, 201)
	op := newClient(t, srv.URL+"/api")
	op.must("POST", "/v1/auth/login", map[string]string{"username": "oscar", "password": "operatorpass"}, nil, 200)
	op.must("PUT", c+"/vhosts/app", map[string]any{}, nil, 403)

	// Virtual hosts.
	var vh rabbitx.VHost
	admin.must("PUT", c+"/vhosts/app", map[string]any{"description": "Test app", "tags": []string{"test"}, "defaultQueueType": "quorum",
		"protected": true, "maxQueues": 50}, &vh, 200)
	if vh.Description != "Test app" || vh.DefaultQueueType != "quorum" || !vh.Protected || vh.MaxQueues == nil || *vh.MaxQueues != 50 || len(vh.NodeStates) != 3 {
		t.Fatalf("vhost: %+v", vh)
	}
	admin.must("DELETE", c+"/vhosts/app", nil, nil, 409)
	admin.must("PUT", c+"/vhosts/app", map[string]any{"description": "Test app", "tags": []string{"test"}, "protected": false, "maxQueues": -1}, &vh, 200)
	// Limits are runtime parameters in the metadata store; a read right
	// after a write may still see the old value on another node.
	eventually(t, func() bool {
		var vs []rabbitx.VHost
		admin.must("GET", c+"/vhosts", nil, &vs, 200)
		for _, v := range vs {
			if v.Name == "app" {
				vh = v
			}
		}
		return !vh.Protected && vh.MaxQueues == nil
	}, "protection and limit not removed: %+v", &vh)
	admin.must("POST", c+"/vhosts/app/start", map[string]string{"node": env.Nodes[1].Name}, nil, 204)
	var vhosts []rabbitx.VHost
	admin.must("GET", c+"/vhosts", nil, &vhosts, 200)
	if len(vhosts) != 2 {
		t.Fatalf("want / and app, got %+v", vhosts)
	}
	admin.must("PUT", "/v1/clusters/east/permissions/app/"+user, map[string]string{"configure": ".*", "write": ".*", "read": ".*"}, nil, 204)

	// Exchanges and bindings.
	op.must("PUT", c+"/exchanges/app/orders", map[string]any{"type": "topic", "arguments": map[string]any{"alternate-exchange": "unrouted"}}, nil, 204)
	op.must("PUT", c+"/exchanges/app/unrouted", map[string]any{"type": "fanout"}, nil, 204)
	op.must("PUT", c+"/exchanges/app/routing", map[string]any{"type": "headers"}, nil, 204)
	op.must("PUT", c+"/exchanges/app/amq.mine", map[string]any{"type": "direct"}, nil, 422)
	// Queues of every type; argument validation comes from RabbitMQ.
	op.must("PUT", c+"/queues/app/orders.created", map[string]any{"type": "quorum", "arguments": map[string]any{"x-delivery-limit": 5}}, nil, 204)
	op.must("PUT", c+"/queues/app/orders.all", map[string]any{"type": "classic", "arguments": map[string]any{"x-max-priority": 3}}, nil, 204)
	op.must("PUT", c+"/queues/app/audit", map[string]any{"type": "stream", "arguments": map[string]any{"x-max-age": "1D"}}, nil, 204)
	op.must("PUT", c+"/queues/app/unrouted.q", map[string]any{"type": "classic"}, nil, 204)
	op.must("PUT", c+"/queues/app/bad", map[string]any{"type": "quorum", "arguments": map[string]any{"x-max-priority": 3}}, nil, 400)
	op.must("PUT", c+"/queues/app/transient", map[string]any{"type": "classic", "durable": false}, nil, 400)
	op.must("PUT", c+"/queues/app/orders.created", map[string]any{"type": "quorum", "arguments": map[string]any{"x-delivery-limit": 6}}, nil, 400)
	for _, b := range []map[string]any{
		{"vhost": "app", "source": "orders", "destination": "orders.created", "destinationType": "queue", "routingKey": "orders.created.*"},
		{"vhost": "app", "source": "orders", "destination": "orders.all", "destinationType": "queue", "routingKey": "orders.#"},
		{"vhost": "app", "source": "orders", "destination": "routing", "destinationType": "exchange", "routingKey": "#"},
		{"vhost": "app", "source": "routing", "destination": "audit", "destinationType": "queue", "arguments": map[string]any{"x-match": "any", "audit": "yes"}},
		{"vhost": "app", "source": "unrouted", "destination": "unrouted.q", "destinationType": "queue"},
	} {
		op.must("POST", c+"/bindings", b, nil, 204)
	}
	var ex rabbitx.ExchangeDetail
	admin.must("GET", c+"/exchanges/app/orders", nil, &ex, 200)
	if ex.Exchange.Type != "topic" || len(ex.Source) != 3 || !strings.Contains(string(ex.Exchange.Arguments), "unrouted") {
		t.Fatalf("exchange detail: %+v", ex)
	}
	var exs api.ExchangesPage
	admin.must("GET", c+"/exchanges?vhost=app", nil, &exs, 200)
	// The default exchange is listed (as the built-in UI does); amq.* are not.
	if exs.Filtered != 4 || exs.Items[0].Name != "" {
		t.Fatalf("want the default and 3 user exchanges in app, got %+v", exs)
	}

	// Publish: routed, through the alternate exchange, and with headers.
	var pub api.PublishOut
	op.must("POST", c+"/exchanges/app/orders/publish", map[string]any{"routingKey": "orders.created.eu", "payload": `{"n":1}`,
		"properties": map[string]any{"contentType": "application/json", "deliveryMode": 2, "headers": map[string]any{"audit": "yes"}}}, &pub, 200)
	if !pub.Routed {
		t.Fatal("orders.created.eu should route")
	}
	op.must("POST", c+"/exchanges/app/amq.default/publish", map[string]any{"routingKey": "nowhere", "payload": "aGk=", "payloadEncoding": "base64"}, &pub, 200)
	if pub.Routed {
		t.Fatal("publish to a missing queue must not route")
	}
	op.must("POST", c+"/exchanges/app/amq.default/publish", map[string]any{"routingKey": "x", "payload": "!!", "payloadEncoding": "base64"}, nil, 422)

	var qd rabbitx.QueueDetail
	eventually(t, func() bool {
		admin.must("GET", c+"/queues/app/orders.created", nil, &qd, 200)
		return qd.Queue.Messages == 1
	}, "queue detail: %+v", &qd)
	if qd.Queue.Type != "quorum" || len(qd.Queue.Members) != 3 || len(qd.Bindings) != 2 || qd.Queue.DeliveryLimit == nil {
		t.Fatalf("queue detail: %+v", qd.Queue)
	}
	var qs api.QueuesPage
	eventually(t, func() bool {
		admin.must("GET", c+"/queues?vhost=app&type=classic", nil, &qs, 200)
		return qs.Filtered == 2
	}, "classic queues: %+v", &qs)
	admin.must("GET", c+"/queues?vhost=app&regex=^orders", nil, &qs, 200)
	if qs.Filtered != 2 {
		t.Fatalf("regex filter: %+v", qs)
	}
	op.must("DELETE", c+"/queues/app/orders.all?ifEmpty=true", nil, nil, 409)
	op.must("POST", c+"/queues/app/orders.all/purge", nil, nil, 204)
	op.must("POST", c+"/queues/app/audit/purge", nil, nil, 400)
	op.must("DELETE", c+"/queues/app/orders.all?ifEmpty=true", nil, nil, 204)

	var binds []rabbitx.Binding
	admin.must("GET", c+"/bindings?vhost=app&source=routing", nil, &binds, 200)
	if len(binds) != 1 {
		t.Fatalf("bindings from routing: %+v", binds)
	}
	b := binds[0]
	op.must("DELETE", c+"/bindings/app/routing/queue/audit/"+esc(b.PropertiesKey), nil, nil, 204)
	admin.must("GET", c+"/bindings?vhost=app&source=orders", nil, &binds, 200)
	for _, b := range binds {
		if b.RoutingKey == "orders.created.*" {
			op.must("DELETE", c+"/bindings/app/orders/queue/orders.created/"+esc(b.PropertiesKey), nil, nil, 204)
		}
	}
	admin.must("GET", c+"/bindings?vhost=app&destination=orders.created", nil, &binds, 200)
	for _, b := range binds {
		if b.Source == "orders" {
			t.Fatalf("binding with # in its key was not deleted: %+v", b)
		}
	}

	// RabbitMQ users: updating tags keeps the password.
	var ru rabbitx.RabbitUser
	admin.must("PUT", c+"/users/app-user?create=true", map[string]any{"tags": []string{"management"}, "password": "pw-one", "maxConnections": 10}, &ru, 200)
	if !ru.HasPassword || ru.MaxConnections == nil || *ru.MaxConnections != 10 {
		t.Fatalf("user: %+v", ru)
	}
	admin.must("PUT", c+"/users/app-user?create=true", map[string]any{"tags": []string{}, "password": "x"}, nil, 409)
	admin.must("PUT", c+"/users/app-user", map[string]any{"tags": []string{"monitoring", "custom"}}, &ru, 200)
	if !ru.HasPassword || len(ru.Tags) != 2 {
		t.Fatalf("tag update lost the password or tags: %+v", ru)
	}
	if code := basicGet(env.Nodes[0].ManagementURL+"/api/whoami", "app-user", "pw-one"); code != 200 {
		t.Fatalf("app-user cannot log in after a tag update: %d", code)
	}
	admin.must("PUT", c+"/users/nopass", map[string]any{"tags": []string{}}, nil, 422)
	admin.must("PUT", c+"/permissions/app/app-user", map[string]string{"configure": "^app\\.", "write": ".*", "read": ".*"}, nil, 204)
	admin.must("PUT", c+"/permissions/app/app-user", map[string]string{"configure": "(", "write": "", "read": ""}, nil, 400)
	var perms []rabbitx.Permission
	admin.must("GET", c+"/permissions?user=app-user", nil, &perms, 200)
	if len(perms) != 1 || perms[0].Configure != "^app\\." {
		t.Fatalf("permissions: %+v", perms)
	}
	for _, x := range []string{"orders", "amq.topic"} {
		admin.must("PUT", c+"/topic-permissions/app/app-user", map[string]string{"exchange": x, "write": "^orders\\.", "read": ".*"}, nil, 204)
	}
	admin.must("DELETE", c+"/topic-permissions/app/app-user?exchange=orders", nil, nil, 204)
	var tps []rabbitx.TopicPermission
	admin.must("GET", c+"/topic-permissions?user=app-user", nil, &tps, 200)
	if len(tps) != 1 || tps[0].Exchange != "amq.topic" {
		t.Fatalf("deleting one topic permission must keep the other: %+v", tps)
	}
	var users []rabbitx.RabbitUser
	admin.must("GET", c+"/users", nil, &users, 200)
	found := false
	for _, u := range users {
		if u.Name == "app-user" {
			found = len(u.VHosts) == 1 && u.VHosts[0] == "app"
		}
	}
	if !found {
		t.Fatalf("app-user missing or without its vhost: %+v", users)
	}
	raw, _ := json.Marshal(users)
	if strings.Contains(string(raw), `"password_hash"`) || strings.Contains(string(raw), `"passwordHash"`) {
		t.Fatal("password hashes leaked")
	}

	// Policies, with a preview of what they match.
	var prev api.PolicyPreview
	admin.must("POST", c+"/policies/preview", map[string]any{"vhost": "app", "name": "orders", "pattern": "^orders", "applyTo": "queues", "priority": 1}, &prev, 200)
	if len(prev.Matches) != 1 || prev.Matches[0].Name != "orders.created" || !prev.Matches[0].Wins {
		t.Fatalf("policy preview: %+v", prev)
	}
	admin.must("PUT", c+"/policies/app/orders", map[string]any{"pattern": "^orders", "applyTo": "queues", "priority": 1,
		"definition": map[string]any{"max-length": 1000}}, nil, 204)
	admin.must("PUT", c+"/policies/app/bogus", map[string]any{"pattern": ".*", "applyTo": "queues", "priority": 0,
		"definition": map[string]any{"ha-mode": "all"}}, nil, 400)
	admin.must("POST", c+"/policies/preview", map[string]any{"vhost": "app", "name": "weak", "pattern": ".*", "applyTo": "all", "priority": 0}, &prev, 200)
	for _, m := range prev.Matches {
		if m.Name == "orders.created" && m.Wins {
			t.Fatalf("a lower-priority policy cannot win: %+v", m)
		}
	}
	op.must("PUT", c+"/operator-policies/app/cap", map[string]any{"pattern": ".*", "applyTo": "queues", "definition": map[string]any{"max-length-bytes": 1 << 20}}, nil, 403)
	admin.must("PUT", c+"/operator-policies/app/cap", map[string]any{"pattern": ".*", "applyTo": "queues", "priority": 0,
		"definition": map[string]any{"max-length-bytes": 1 << 20}}, nil, 204)
	eventually(t, func() bool {
		admin.must("GET", c+"/queues/app/orders.created", nil, &qd, 200)
		return qd.Queue.Policy == "orders" && qd.Queue.OperatorPolicy == "cap" && strings.Contains(string(qd.EffectivePolicy), "max-length")
	}, "effective policy: %+v", &qd)

	// Parameters: URI passwords are redacted.
	admin.must("PUT", c+"/parameters/federation-upstream/app/up", map[string]any{"value": map[string]any{"uri": "amqp://feduser:s3cret@upstream:5672"}}, nil, 204)
	var params []rabbitx.Parameter
	admin.must("GET", c+"/parameters?component=federation-upstream", nil, &params, 200)
	if len(params) != 1 || strings.Contains(string(params[0].Value), "s3cret") || !strings.Contains(string(params[0].Value), "feduser:***@") {
		t.Fatalf("parameter not redacted: %+v", params)
	}
	admin.must("PUT", c+"/parameters/federation-upstream/app/up", map[string]any{"value": json.RawMessage(params[0].Value)}, nil, 422)
	admin.must("DELETE", c+"/parameters/federation-upstream/app/up", nil, nil, 204)
	admin.must("PUT", c+"/cluster-tags", map[string]any{"tags": map[string]string{"region": "eu", "env": "test"}}, nil, 204)
	var tags api.ClusterTagsBody
	admin.must("GET", c+"/cluster-tags", nil, &tags, 200)
	if tags.Tags["region"] != "eu" {
		t.Fatalf("cluster tags: %+v", tags)
	}
	admin.must("PUT", c+"/global-parameters/internal_cluster_id", map[string]any{"value": "x"}, nil, 422)

	// Definitions: export, change, diff, selective import.
	var defs map[string]any
	admin.must("GET", c+"/definitions?vhost=app", nil, &defs, 200)
	op.must("GET", c+"/definitions", nil, nil, 403)
	for _, p := range defs["policies"].([]any) {
		p.(map[string]any)["definition"] = map[string]any{"max-length": 2000}
	}
	for _, e := range defs["exchanges"].([]any) {
		if m := e.(map[string]any); m["name"] == "unrouted" {
			m["type"] = "direct" // a conflict: import will not change it
		}
	}
	defs["queues"] = append(defs["queues"].([]any), map[string]any{"name": "imported", "durable": true, "auto_delete": false, "arguments": map[string]any{"x-queue-type": "quorum"}})
	defs["queues"] = append(defs["queues"].([]any), map[string]any{"name": "skipped", "durable": true, "auto_delete": false, "arguments": map[string]any{}})
	var diff rabbitx.DefinitionsDiff
	admin.must("POST", c+"/definitions/diff", map[string]any{"vhost": "app", "definitions": defs}, &diff, 200)
	byName := map[string]rabbitx.DefinitionChange{}
	for _, ch := range diff.Changes {
		byName[ch.Kind+"/"+ch.Name] = ch
	}
	if byName["policies/orders"].Change != "changed" || byName["exchanges/unrouted"].Change != "conflict" ||
		byName["queues/imported"].Change != "added" || byName["queues/audit"].Change != "unchanged" {
		t.Fatalf("diff: %+v", diff.Counts)
	}
	var imp struct{ Imported int }
	admin.must("POST", c+"/definitions/import", map[string]any{"vhost": "app", "definitions": defs,
		"select": []string{byName["policies/orders"].ID, byName["queues/imported"].ID, byName["exchanges/unrouted"].ID}}, &imp, 200)
	if imp.Imported != 2 {
		t.Fatalf("want 2 imported (the conflict is skipped), got %d", imp.Imported)
	}
	admin.must("GET", c+"/queues/app/imported", nil, nil, 200)
	admin.must("GET", c+"/queues/app/skipped", nil, nil, 404)
	var pols []rabbitx.Policy
	admin.must("GET", c+"/policies?vhost=app", nil, &pols, 200)
	if len(pols) != 1 || !strings.Contains(string(pols[0].Definition), "2000") {
		t.Fatalf("policy not imported: %+v", pols)
	}

	// Feature flags and deprecated features.
	var flags []rabbitx.FeatureFlag
	admin.must("GET", c+"/feature-flags", nil, &flags, 200)
	if len(flags) < 10 {
		t.Fatalf("feature flags: %d", len(flags))
	}
	var enabled struct{ Enabled []string }
	admin.must("POST", c+"/feature-flags/enable", map[string]any{}, &enabled, 200)
	admin.must("POST", c+"/feature-flags/enable", map[string]any{"names": []string{"no_such_flag"}}, nil, 400)
	var deprecated []rabbitx.DeprecatedFeature
	admin.must("GET", c+"/deprecated-features", nil, &deprecated, 200)
	if len(deprecated) == 0 {
		t.Fatal("no deprecated features listed")
	}

	// Cleanup: users and the vhost.
	admin.must("POST", c+"/users/delete", map[string]any{"users": []string{"app-user"}}, nil, 204)
	admin.must("DELETE", c+"/vhosts/app", nil, nil, 204)
	admin.must("GET", c+"/queues/app/imported", nil, nil, 404)
}

func basicGet(u, user, pass string) int {
	req, _ := http.NewRequest(http.MethodGet, u, nil)
	req.SetBasicAuth(user, pass)
	cl := &http.Client{Timeout: 10 * time.Second}
	resp, err := cl.Do(req)
	if err != nil {
		return 0
	}
	resp.Body.Close()
	return resp.StatusCode
}
