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
- `/api/nodes`: node details (`enabled_plugins`, memory, file descriptors) appear only after the node's first statistics emission, a few seconds after boot; until then a running node shows little more than its name.
- `/api/nodes`: a stopped node is listed with `running: false` and almost no other fields.
- `sockets_used` / `sockets_total` are 0 in 4.x; sockets are no longer tracked.
- `being_drained` is true while a node is in maintenance mode.
- `enabled_plugins` (per node) is the only way to see plugins over HTTP.
- `/api/nodes/{node}/memory` returns `{"memory": {...breakdown..., "total": {"erlang","rss","allocated"}, "strategy"}}`.
- `DELETE /api/reset` and `DELETE /api/reset/{node}` reset statistics (administrator only). They answer **406** when the request carries `Accept: application/json`; rabmin only sends that header on GET.

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

## Shovels

- `/api/shovels` lists shovel status (`state`, `forwarded`, `blocked_status`, `src_uri`, `dest_uri`); it is provided by rabbitmq_shovel_management and absent from `/api/index.html`.
- In a dynamic shovel definition, `"src-uri": "amqp://"` connects to the local node's **default vhost `/`**, not to the vhost the shovel is declared in. Use `amqp:///shop` for vhost `shop`. A shovel pointed at a queue in the wrong vhost still reports `state: running` and simply forwards nothing, so `running` is not proof that a shovel works; watch `forwarded`.

## Errors, continued

- A **PUT without `Content-Type: application/json` answers 415**, even when it has no body (`PUT /api/feature-flags/{name}/enable`). POST and DELETE do not care. rabmin sends the header on every PUT and POST.

- `reason` is not always a string. Erlang terms come through as JSON, with char lists as integer arrays: an invalid permission regex answers `{"error":"bad_request","reason":["invalid_regexp",[40],[[109,105,...],1]]}`. rabmin renders such reasons readably.
- `DELETE /api/queues/{vhost}/{name}?if-empty=true` and `?if-unused=true` refuse with **400** (`queue '...' not empty` / `in use`); rabmin maps them to 409.
- A protected vhost refuses deletion with **412** `precondition_failed`.

## Virtual hosts

- `PUT /api/vhosts/{name}` on an existing vhost updates `description`, `tags` and `default_queue_type` in place. `tags` accepts a comma-separated string or an array.
- `GET` returns `protected_from_deletion`; protection is toggled with `POST`/`DELETE /api/vhosts/{name}/deletion/protection`.
- On a 3-node cluster, a `GET /api/vhost-limits` right after changing a limit (or a vhost's protection) can still return the old value for a moment. rabmin's vhost update waits for its own change to show before answering.
- `PUT /api/vhosts/{name}` without `default_queue_type` **resets an existing vhost's default queue type to classic**. rabmin keeps the current one when it is not given.
- Limits live under `/api/vhost-limits/{vhost}/{max-connections|max-queues}` with body `{"value": n}`; unknown names answer 400. User limits are the same under `/api/user-limits/{user}/{max-connections|max-channels}`.

## Exchanges, bindings, publish

- The default exchange has the name `""` in lists and is addressed as `amq.default` in paths.
- Binding `properties_key` comes back URL-encoded already (`orders.created.%23`); headers bindings get a hash (`~fiybLg`). The `Location` header of a created binding encodes it once more (`%2523`), and DELETE accepts that form.
- `POST /api/exchanges/{vhost}/{name}/publish` needs a `properties` object (400 `key_missing,properties` without it) and answers `{"routed": bool}`.

## Queues

- RabbitMQ 4.x refuses non-durable, non-exclusive queues by default: declaring one fails with 400 "Feature `transient_nonexcl_queues` is deprecated". rabmin creates durable queues unless told otherwise and explains the error.
- A queue cannot be edited: re-declaring with different arguments fails with `inequivalent arg ...`. Changes go through policies.
- Queue arguments are validated per type (`invalid arg 'x-max-priority' ... of queue type rabbit_quorum_queue`).
- Streams cannot be purged (`queue.purge not supported by stream queues`).
- Queue objects in 4.4 carry `effective_policy_definition`, `policy`, `operator_policy`, `delivery_limit`, `consumer_capacity`, and for quorum and stream queues `leader`, `members`, `online`.

## Users and permissions

- `/api/users/{name}` no longer returns `password_hash` in 4.x, only `has_password`.
- **A `PUT /api/users/{name}` without `password` or `password_hash` removes the user's password** (they can no longer log in with it). To change tags while keeping the password, rabmin reads the current hash from `GET /api/definitions` and sends it back with `hashing_algorithm`.
- `DELETE /api/topic-permissions/{vhost}/{user}` removes the user's topic permissions for **every** exchange in the vhost; neither `?exchange=` nor a body narrows it. rabmin removes one exchange's entry by deleting all and re-adding the rest (not atomic).
- `PUT /api/permissions/{vhost}/{user}` on a missing vhost or user answers 400 `vhost_or_user_not_found`.

## Policies and parameters

- The broker validates policy definitions and rejects unknown keys (`... are not recognised policy settings`). Classic mirrored queue keys (`ha-mode`...) are rejected in 4.x. Operator policies accept only a subset of keys.
- `/api/parameters` lists every component, including `operator_policy` and `vhost-limits`, and shovel and federation values **with credentials in their URIs**. rabmin redacts URI passwords when listing them.
- `cluster_tags` is a global parameter whose value is `[]` when empty and an object once set; `/api/overview` shows it as `cluster_tags`.

## Definitions

- `POST /api/definitions` and `POST /api/definitions/{vhost}` are additive: nothing is deleted.
- Import updates policies, users, permissions and parameters, but **silently skips exchanges and queues that already exist with different properties** (answers 204, nothing changes). rabmin's import preview shows those as conflicts.
- Importing objects into a missing vhost fails with 400 "Please create virtual host ... prior to importing definitions."

## Feature flags

- `/api/feature-flags` returns `state` (enabled, disabled), `stability` (required, stable, experimental) and `require_level`. On a fresh 4.4 cluster every flag is enabled.
- `PUT /api/feature-flags/{name}/enable` answers 204 (also when already enabled) and 400 `unsupported` for unknown flags.
- `/api/deprecated-features` lists `deprecation_phase` and `state`; `/api/deprecated-features/used` lists those in use.
