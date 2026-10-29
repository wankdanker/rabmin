# RabbitMQ API notes

Behaviour of the management API and the Prometheus endpoint that rabmin depends on, verified against RabbitMQ 4.4.0 (Erlang 29) with `rabmin-devcluster`. Add to this file when the broker surprises you.

## Errors and auth

- Errors are JSON: `{"error":"not_found","reason":"Not Found"}`.
- Bad credentials return **401** `{"error":"not_authorized","reason":"Not_Authorized"}`.
- A valid user without enough privilege also gets **401**, not 403: `{"error":"not_authorised","reason":"Not administrator user"}`. rabmin maps this to 403 and bad credentials to 502, because the UI logs out on 401.
- `/api/whoami` returns `{"name","tags":[...],"is_internal_user"}`; `tags` is an array in 4.x.

## Overview and nodes

- `/api/overview`: `rabbitmq_version`, `product_version`, `erlang_version`, `cluster_name`, `node` (the node that answered), `message_stats` (counters plus `*_details.rate`), `queue_totals`, `object_totals`, `churn_rates`, `listeners`, `contexts`, `disable_stats`, `enable_queue_totals`.
- `contexts[].port` is a **string**; `listeners[].port` is a number.
- `/api/nodes`: a stopped node is listed with `running: false` and almost no other fields.
- `sockets_used` / `sockets_total` are 0 in 4.x; sockets are no longer tracked.
- `being_drained` is true while a node is in maintenance mode.
- `enabled_plugins` (per node) is the only way to see plugins over HTTP.
- `/api/nodes/{node}/memory` returns `{"memory": {...breakdown..., "total": {"erlang","rss","allocated"}, "strategy"}}`.
- `DELETE /api/reset/{node}` resets a node's stats (administrator only).

## Health checks

Node-local: they check the node that serves the request, so rabmin calls them through every configured management URL.

| Path | Notes |
|---|---|
| `/api/health/checks/alarms` | cluster-wide |
| `/api/health/checks/local-alarms` | |
| `/api/health/checks/virtual-hosts` | |
| `/api/health/checks/node-is-quorum-critical` | 503 lists `queues` (also internal ones such as `rabbitmq_metadata`, `rabbit_stream_coordinator`) |
| `/api/health/checks/is-in-service` | |
| `/api/health/checks/below-node-connection-limit` | |
| `/api/health/checks/ready-to-serve-clients` | |
| `/api/health/checks/certificate-expiration/{n}/{unit}` | unit is days, weeks, months or years; 400 otherwise |
| `/api/health/checks/port-listener/{port}` | 503 lists `ports` and `missing` |
| `/api/health/checks/protocol-listener/{protocol}` | protocols from `listeners[].protocol` (`amqp`, `stream`, `mqtt`, `http`...) |

Success is `200 {"status":"ok"}`; failure is `503 {"status":"failed","reason":...}`.

## Lists, paging and rates

- Lists accept `page`, `page_size` (max 500), `name`, `use_regex`, `sort`, `sort_reverse`, `columns`. Paged responses are `{total_count, item_count, filtered_count, page, page_size, page_count, items}`.
- `name` filters on the object's `name` only (for connections, `"ip:port -> ip:port"`, not the client-provided name), and only when `page` is set.
- `columns` accepts nested paths: `columns=name,recv_oct_details.avg_rate,client_properties.product`.
- In list responses `*_details.rate` is often 0.0 even for busy objects. Passing `msg_rates_age`/`msg_rates_incr` (messages) or `data_rates_age`/`data_rates_incr` (bytes) adds `avg_rate`, which is stable; rabmin uses `avg_rate` with a 30 s window. Requesting a single object (`/api/connections/{name}`) returns sensible `rate` values.
- Consumers (`/api/consumers`) carry `channel_details` but not the client-provided connection name; rabmin joins it from the connections list.
- Direct (in-node) connections, such as shovels, appear in `/api/connections` with a pid-like name and no network fields.

## Prometheus

- `/metrics` is aggregated per node; sum across nodes for cluster totals. Counters are floats and may use exponent notation (`1.15e4`).
- `rabbitmq_identity_info{rabbitmq_node, rabbitmq_cluster}` identifies the node; `rabbitmq_build_info` the versions.
- Message counters (`rabbitmq_global_messages_*_total`) are labelled by `protocol` and sometimes `queue_type`; sum over labels.
- `/metrics/per-object` and `/metrics/detailed?family=...` exist; detailed queue metrics are `rabbitmq_detailed_queue_*{vhost,queue}`.
