# Architecture

```
browser ──HTTP/SSE──▶ rabmin (one Go binary)
                       ├── /api/v1/...   huma operations → OpenAPI 3.1
                       ├── /*            embedded SvelteKit SPA (web/dist)
                       ├── store         SQLite: users, sessions, tokens, clusters, credentials, audit
                       └── rabbitx.Manager
                             └── per cluster: management API client (failover across node URLs),
                                 Prometheus scraper, poller → snapshot + history → SSE
                                      │
                                      ▼
                     RabbitMQ nodes: management API :15672, Prometheus :15692,
                     AMQP 0-9-1 / 1.0 :5672, stream :5552
```

## Packages

| Path | |
|---|---|
| `cmd/rabmin` | The server. Flags/env, admin bootstrap, `rabmin openapi`. |
| `cmd/rabmin-devcluster` | RabbitMQ clusters in Docker for development, with optional demo topology and traffic. |
| `internal/api` | HTTP API. One file per area; every route is registered with `reg()` so it gets a role requirement, OpenAPI docs and auditing. |
| `internal/rabbitx` | RabbitMQ side: the management API client, Prometheus parsing, the poller and snapshots. |
| `internal/store` | SQLite persistence (pure Go driver, no CGO). Credentials and TLS keys are sealed with `internal/secret` before they are written. |
| `internal/devcluster` | Starts RabbitMQ containers through the docker CLI; used by the dev command and by integration tests. |
| `web` | `go:embed` of the built UI. |
| `ui` | SvelteKit 2 / Svelte 5 SPA. `src/lib/api/schema.d.ts` is generated from `docs/openapi.json`. |

## How rabmin talks to RabbitMQ

| Channel | Used for |
|---|---|
| Management HTTP API | Objects, admin, health checks. The source of truth. Basic auth, or a bearer token where the management API uses OAuth2. |
| Prometheus (`rabbitmq_prometheus`) | Metrics, also when the management stats database is disabled. |
| AMQP and stream clients | Publishing, safe browsing, moving messages (later phases). |
| Optional executor (SSH, kubectl, agent) | CLI-only operations such as maintenance mode (later phases). |

## Monitoring

For each registered cluster the manager polls every `RABMIN_POLL_INTERVAL`, in parallel:

- `/api/overview`, `/api/nodes`, and the 500 deepest queues (`/api/queues?sort=messages`, projected with `columns`) through the management client, which tries the URL that last worked first and fails over to the others on network errors;
- `/api/overview?columns=node` through every configured URL, to learn which node each URL reaches;
- RabbitMQ's health checks through every reachable URL, one per node (at most every `3 × poll interval`), forming the node × check matrix;
- `/metrics` on every Prometheus URL.

`buildSnapshot` combines them into one `Snapshot`: info and versions, totals, rates, nodes, alarms, health, listeners, endpoint status, the deepest and problematic queues, and an attention list. Message rates come from the management stats database; when it is disabled, rabmin computes them from Prometheus counter deltas per node (skipping counter resets). Snapshots go to SSE subscribers (`/api/v1/clusters/{id}/events`) and an in-memory history used for the charts.

Lists (connections, channels, consumers) are read on demand. RabbitMQ's own list filters only match object names, so rabmin fetches the projected list and filters, sorts and pages it itself; list rates use `avg_rate` over a 30 second window, because `rate` in list responses is often 0. See `docs/rabbitmq-api-notes.md`.

## Auth

- Sessions: random token in an `HttpOnly`, `SameSite=Strict` cookie; only its SHA-256 is stored.
- API tokens: `rm_` prefix, SHA-256 stored, limited to a role no higher than the owner's, optional expiry.
- Every non-GET operation is written to the audit log by the auth middleware, with actor, operation id, path and result.
- Login attempts are rate limited per remote address and username.

## Adding an API operation

1. Add a `reg(s, route{...}, handler)` call in the matching `internal/api/*.go` file, setting `role`.
2. Cover it in `internal/api/api_test.go`.
3. `make openapi` to regenerate the spec and the UI client types.
