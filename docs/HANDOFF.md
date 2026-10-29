# rabmin: handoff

**Goal:** build `rabmin`, a modern management UI and HTTP API for RabbitMQ. It must cover everything the built-in RabbitMQ management UI does. It must also do what no single management UI does today: manage many clusters, show the messaging topology, browse messages safely, and run cluster maintenance as a workflow.

rabmin is the sibling of [natsmin](https://github.com/wankdanker/natsmin), which already exists. Reuse its architecture, code, UI shell, release pipeline and repository conventions wholesale. Only the broker layer is new.

Owner: Dan VerWeire (`wankdanker` on GitHub). License: Apache-2.0. Repository: `wankdanker/rabmin` (already created, empty).

---

## 0. Before you start

1. **natsmin on GitHub is complete.** `wankdanker/natsmin` `main` holds all four commits, including the release pipeline, `CLAUDE.md`, issue/PR templates and policies. Clone it (`add_repo` for `wankdanker/natsmin`, read access) and copy from there.
2. **Commit rules (Dan's, and in natsmin's `CLAUDE.md`):**
   - Commits are authored as `Dan VerWeire <dverweire@gmail.com>` (set `git config user.name/user.email` in the repo).
   - No `Co-Authored-By: Claude`, no `Claude-Session:` lines, and no "Generated with Claude Code" footers in commits or pull requests. Copy this rule into rabmin's `CLAUDE.md` on day one.
   - Start directly on `main`. After the first push, all work goes on feature branches with pull requests.
3. **You need a real RabbitMQ to develop against.** natsmin embeds `nats-server` in Go; RabbitMQ is Erlang and cannot be embedded. Cloud sessions may not have a Docker daemon. Pick a working option early:
   - `apt-get install rabbitmq-server` (or the Team RabbitMQ packages), with `rabbitmq-plugins enable rabbitmq_management rabbitmq_shovel rabbitmq_shovel_management rabbitmq_federation rabbitmq_federation_management rabbitmq_stream rabbitmq_stream_management rabbitmq_prometheus rabbitmq_top rabbitmq_tracing`. For a 3-node cluster on one host, run three nodes with distinct `RABBITMQ_NODENAME`, `RABBITMQ_NODE_PORT` and `RABBITMQ_DIST_PORT` values, then `rabbitmqctl join_cluster`.
   - Or Docker (`rabbitmq:4-management`) through `docker compose` and testcontainers-go where a daemon exists, for example in GitHub Actions.
   - Write a `rabmin-devcluster` helper (shell or Go) that starts three local nodes plus a second single-node cluster with federation and shovel links between them, and seeds demo traffic. Every page of rabmin needs something to show, as natsmin's `-demo` does.

---

## 1. What to reuse from natsmin

Copy, then rename `natsmin` to `rabmin` and `nats` to `rabbit`:

| natsmin path | Reuse as |
|---|---|
| `internal/store` (SQLite, users, sessions, tokens, audit, migrations) | as is; rename `systems`→`clusters`, `accounts`→`credentials` (see section 3) |
| `internal/secret` (AES-GCM for stored credentials) | as is |
| `internal/auth` (bcrypt, tokens `nm_`→`rm_`) | as is |
| `internal/api/server.go`, `helpers.go`, `auth.go`, `users.go`, `audit.go` | `reg()` with roles, auth middleware, auto-audit, error mapping, login limiter, OpenAPI via huma, SPA serving |
| `internal/natsx/manager.go` | the per-cluster manager pattern: poll loop, snapshot, history ring, SSE subscribers |
| `cmd/natsmin/main.go` | flags/env, admin bootstrap, `openapi` subcommand |
| `ui/` | everything in `src/lib` (Shell, Icon, Modal, Confirm, Toasts, Chart, Sparkline, Stat, Meter, JsonView/JsonEditor, SortTh/Sorter, CopyButton, Empty/Loading, AuthCard, CredentialFields, SystemForm pattern), the `app.css` design tokens (light and dark), the login/setup/users/tokens/audit/profile pages, and `live.svelte.ts` (SSE) |
| `Makefile`, `Dockerfile`, `docker-compose.yml`, `.github/*`, `.goreleaser.yaml`, `.golangci.yml`, `CLAUDE.md`, `CONTRIBUTING.md`, `SECURITY.md`, `RELEASING.md`, `CODE_OF_CONDUCT.md`, `.editorconfig` | same, adjusted |

Keep these decisions from natsmin:
- **Stack:** one Go binary with an embedded SvelteKit 2 / Svelte 5 SPA (adapter-static), pure-Go SQLite (`modernc.org/sqlite`), huma v2 for the API. The OpenAPI 3.1 spec is committed to `docs/openapi.json`, and the TypeScript types are generated from it (`openapi-typescript --default-non-nullable false`). Set `huma.DefaultArrayNullable = false` and always return empty slices.
- **pnpm:** pinned via `packageManager` in `ui/package.json`. Docker failed in natsmin's CI without this.
- **Roles:** `viewer`, `operator` and `admin`. Tokens are capped at their owner's role. Every non-GET request is audited.
- **API plane first:** every UI action is an API operation, and the UI is only a client.
- **Destructive UI actions:** need a confirm dialog; irreversible ones use type-to-confirm.

---

## 2. How rabmin talks to RabbitMQ

Use several channels. Each has a job:

| Channel | Used for |
|---|---|
| **Management HTTP API** (`:15672/api`, basic auth, or OAuth2 bearer where configured) | Almost all management. Treat it as the source of truth for objects and admin. |
| **Prometheus endpoint** (`rabbitmq_prometheus`, `:15692/metrics`, `/metrics/per-object`, `/metrics/detailed?family=...`) | Metrics. RabbitMQ 4.x steers metrics away from the management stats database, which can be disabled (`management_agent.disable_metrics_collector`). Poll this to build rabmin's own history. |
| **AMQP 0-9-1 / AMQP 1.0 client** (`amqp091-go`, `Azure/go-amqp` or RabbitMQ's AMQP 1.0 Go client) | Publishing, safe message browsing and moving, permission testing, DLQ replay |
| **Stream protocol client** (`rabbitmq-stream-go-client`) | Non-destructive browsing of streams and super streams by offset or timestamp |
| **Optional node agent / exec** | Operations only available in the CLI: `rabbitmq-upgrade drain/revive` (maintenance mode), `rabbitmq-diagnostics` checks, `forget_cluster_node`, log level, `rabbitmq-queues` quorum status/grow/shrink where HTTP lacks it. Offer three executors: SSH, `kubectl exec` (pods managed by the cluster operator), or a tiny `rabmin-agent` sidecar exposing an allow-listed command set over mTLS. The agent is optional; the UI should hide what is unavailable. |

**Verify every endpoint against the running broker.** The management plugin serves its own API reference at `http://host:15672/api/index.html`, and it varies by version and enabled plugins. Build the Go client against that, not from memory. Record the minimum supported version (suggest RabbitMQ 3.13+, with 4.x as the primary target) and detect capabilities via `/api/overview` (`rabbitmq_version`, `product_version`), `/api/extensions`, `/api/feature-flags` and the plugin list.

---

## 3. Data model in rabmin

- **Cluster:** name, slug, description, management URL(s) (one per node is better; fail over between them), TLS, management credentials (sealed), optional Prometheus URL(s), optional AMQP/stream URLs plus credentials for messaging features, and optional executor config (SSH, kubectl or agent).
- **Credential profiles:** extra users per cluster, so browsing and publishing can run as a less privileged user than admin. This is the counterpart of natsmin's account connections.
- A poller per cluster builds a **snapshot** every N seconds:
  - nodes, with alarms (memory, disk), partitions, uptime, version, Erlang version, fd/socket/process usage, memory breakdown and running status
  - totals: connections, channels, consumers, queues, messages ready/unacked, publish/deliver/ack rates
  - per-queue rollups for the busiest and most problematic queues
  - quorum queue leader and member distribution
  - federation and shovel link status
- Snapshots feed SSE and an in-memory history. Persisted history is a follow-up, as natsmin issue #9 is.

---

## 4. Parity checklist: the built-in management UI

All of these must exist, in a more modern form:

**Overview**
- [ ] Cluster name (editable), RabbitMQ and Erlang versions, uptime
- [ ] Totals (connections, channels, exchanges, queues, consumers) and queued messages (ready, unacked, total) with charts
- [ ] Message rates: publish, publisher confirm, deliver (manual and auto ack), redelivered, get, get-empty, ack, return/unroutable, disk reads and writes
- [ ] Churn statistics (connections, channels and queues created/closed per second)
- [ ] Nodes table: memory with breakdown, disk free vs limit, file descriptors, sockets, Erlang processes, uptime, info, reset stats, alarms
- [ ] Listening ports and web contexts
- [ ] Definitions export and import (whole cluster or per vhost), with the import previewed as a diff (see section 5)

**Connections** (AMQP 0-9-1, AMQP 1.0, MQTT, STOMP, Web-MQTT/STOMP)
- [ ] List with filter, sort and paging (the API supports `page`, `page_size`, `name`, `use_regex`, `sort`, `sort_reverse`, `columns`)
- [ ] Detail: client properties and capabilities, user, vhost, protocol, TLS details, auth mechanism, heartbeat, frame max, channel max, rates and data rates, channels on it
- [ ] Close a connection with a reason; bulk close (by user, by client name)
- [ ] Stream protocol connections, publishers and consumers (`rabbitmq_stream_management`)

**Channels**
- [ ] List and detail: prefetch (global and per consumer), unacked, unconfirmed, confirm and transactional mode, consumers, publishing per exchange and delivery per queue

**Exchanges**
- [ ] List, create (type, durable, auto-delete, internal, arguments such as `alternate-exchange`), delete (`if-unused`)
- [ ] Bindings both ways (as source and as destination), add and remove bindings, including exchange-to-exchange
- [ ] Publish a message (routing key, headers, properties, payload) with a routed/unroutable result
- [ ] Message rates in and out

**Queues and streams**
- [ ] List with filtering by name or regex, vhost, type (classic, quorum, stream), state and policy; column chooser; paging
- [ ] Create with the type-specific argument helpers that the management UI offers: TTL, expires, max-length(-bytes), overflow, dead-letter exchange and routing key, dead-letter strategy (`at-least-once` for quorum), single active consumer, delivery limit, initial group size, leader locator, max-age (streams), stream max segment size, filter size, and so on
- [ ] Detail: features, effective policy and operator policy, state, leader and members (quorum and streams) and their online status, messages (ready, unacked, in memory, persistent, bytes), rates, consumers (tag, channel, ack mode, prefetch, active/waiting for single active consumer, arguments), bindings, runtime metrics
- [ ] Get messages (see the safety notes in section 5), publish, purge, delete (`if-empty`, `if-unused`)
- [ ] Move messages to another queue (dynamic shovel)
- [ ] Quorum queue replica management: add/remove members, grow onto a node, shrink off a node, rebalance leaders (`POST /api/rebalance/queues`); confirm the exact endpoints on the target version

**Admin**
- [ ] Users: create, password/hash, tags (`administrator`, `monitoring`, `policymaker`, `management`, `impersonator`, custom), delete, bulk delete; limits (`max-connections`, `max-channels`)
- [ ] Permissions per vhost (configure/write/read regex) and topic permissions per exchange
- [ ] Virtual hosts: create with description, tags and default queue type; delete (respecting deletion protection where supported); limits (`max-connections`, `max-queues`); start/restart on a node
- [ ] Policies and operator policies: create/edit/delete with a definition editor that knows the valid keys per queue type; priority; apply-to
- [ ] Feature flags: list with state and stability; enable one or all
- [ ] Deprecated features: list and show which are in use
- [ ] Cluster tags and name
- [ ] Global and vhost parameters, including runtime parameter components (federation upstreams, dynamic shovels)

**Plugins and extensions**
- [ ] Shovels: dynamic shovel CRUD for AMQP 0-9-1 and AMQP 1.0 sources/destinations; status per node (running, starting, terminated with reason); restart; delete
- [ ] Federation: upstreams and upstream sets CRUD, link status per node, restart a link
- [ ] Top processes and ETS tables (`rabbitmq_top`)
- [ ] Tracing (`rabbitmq_tracing`): create/stop traces, view and download trace logs
- [ ] OAuth2/OIDC login to RabbitMQ itself is out of scope as a feature. rabmin has its own SSO (natsmin issue #1), but it must work against brokers that use OAuth2 for the management API, by sending a bearer token per cluster.

**Health**
- [ ] `/api/health/checks/*` on every node: alarms, local-alarms, certificate expiration, port and protocol listeners, virtual hosts, node-is-quorum-critical, and others the version provides. Show them as a health matrix of nodes by checks.

---

## 5. Beyond the management UI (why rabmin exists)

These are the reasons to switch. Prioritize them right after parity basics:

1. **Many clusters, one place.** A fleet overview with health per cluster, cross-cluster search ("where does the queue `orders.created` exist?"), and SSE live updates. This mirrors natsmin's systems.
2. **Topology map.**
   - Within a vhost: an interactive graph of exchanges, bindings and queues, with consumers as leaves.
   - Edge thickness shows message rate. Unroutable paths and queues without consumers are highlighted.
   - Filter by vhost, prefix or traffic.
   - Across clusters: federation upstreams and shovels drawn as links between clusters. This is the counterpart of natsmin's gateways.
3. **"Where does this message go?" simulator.** Given an exchange, routing key and headers, compute the matched queues from the bindings, including headers/topic/fanout semantics and exchange-to-exchange hops, plus the alternate exchange. Also show the effective policy per queue.
4. **Safe message browsing.**
   - **Streams:** a non-destructive browser by offset, timestamp or first/last, using the stream protocol. Includes super streams, with partition fan-out and per-partition offsets.
   - **Classic and quorum queues:** RabbitMQ has no non-destructive peek. `get` with requeue changes ordering and the redelivered flag. On quorum queues it also increments the delivery count, and in 4.x the default delivery limit (20) eventually dead-letters or drops messages. rabmin must make this explicit:
     - a "peek (requeue)" mode with a warning and a count shown
     - a "move to a holding stream and browse there" mode
     - `ack` mode only behind type-to-confirm
5. **Message tools:**
   - Move or copy messages between queues (dynamic shovel or rabmin-driven AMQP), with filters on headers or routing key.
   - Dead-letter queue workbench: view `x-death` history, then replay to the original exchange/routing key, optionally editing headers or body.
   - Export messages to JSONL or an object store, and import them back.
   - All of this runs as background jobs with progress, cancel and audit.
6. **Quorum queue and stream operations:**
   - Leader and replica distribution per node, with an imbalance score and one-click rebalance.
   - Grow members onto a new node, and shrink them off a node being retired.
   - Spot queues that are below quorum or have members that are not running.
7. **Guided maintenance and rolling upgrades.**
   - Pre-checks:
     - quorum-critical check
     - alarms
     - feature flags that must be enabled before the upgrade
     - deprecated features in use
     - classic mirrored queue policies (removed in 4.0)
   - Then: put the node into maintenance mode (drain, via the executor), wait for leadership transfer and client reconnection, upgrade, revive, verify. Repeat node by node.
8. **Migration assistants:**
   - Classic (and formerly mirrored) to quorum queues: find candidates, check feature compatibility (priorities, exclusivity, non-durable, `x-max-priority`, global QoS), and generate a plan. This is high value because of 4.0.
   - AMQP 0-9-1 clients still using deprecated features.
9. **Definitions as code.**
   - Diff the definitions of two clusters, or a cluster against a Git file; apply selectively.
   - Track changes over time by snapshotting definitions on every change rabmin sees.
   - Detect drift against a desired-state file.
10. **Effective-access explorer.**
   - "Can user X configure, write or read resource Y in vhost Z?", with an explanation of which regex matched.
   - "Which users can publish to exchange E?"
   - A topic permission tester.
11. **Consumer health.**
   - Prefetch saturation (unacked ≈ prefetch × consumers).
   - Queues whose consumers acknowledge slower than the publish rate, with an ETA to drain or to hit max-length.
   - Consumer utilisation and idle consumers.
12. **Client fleet view.** Group connections by `connection_name`, product/platform/version and user to find old client libraries, connection leaks and churn sources.
13. **History and alerting.** Metrics persisted beyond the management UI's short sample windows, and alert rules with webhook, Slack and email targets. These map to natsmin issues #2, #9 and #10, so share the design.
14. **Kubernetes awareness (optional).** Detect clusters managed by the RabbitMQ Cluster Operator and Messaging Topology Operator, and link objects to their CRDs. Offer to export a definition as topology-operator YAML.
15. **API plane, CLI, Terraform and MCP**, as in natsmin issues #13–#15.

---

## 6. Suggested phases

Each phase ends with passing integration tests against a real 3-node cluster. In CI, use GitHub Actions service containers (`rabbitmq:4-management`) or testcontainers.

**Phase 0: scaffold (day 1)**
- Copy the natsmin skeleton, rename everything, keep auth/users/tokens/audit/UI shell, `CLAUDE.md` and the CI/release pipeline. Push `main`.
- Write the devcluster helper and demo traffic generator.

**Phase 1: observe**
- Cluster registry (management URL failover, TLS, credentials) with a "test connection" that reports version, plugins, user tags and whether the user is an administrator.
- Poller: snapshot plus history from `/api/overview`, `/api/nodes`, health checks, and Prometheus when available.
- Pages: fleet overview, cluster overview, nodes (detail plus memory breakdown), health matrix, connections, channels, consumers.

**Phase 2: objects**
- vhosts, exchanges, queues (all three types) and bindings, with full CRUD, the argument helpers and publish.
- Users, permissions, topic permissions and limits; policies and operator policies; parameters.
- Definitions export/import with diff preview.
- Feature flags and deprecated features.

**Phase 3: messages**
- Stream browser (stream protocol), and publish for all types.
- Peek with explicit requeue semantics and warnings; get with ack behind type-to-confirm.
- Move/copy via dynamic shovel, and the DLQ workbench.
- Shovels and federation: CRUD plus status.

**Phase 4: operate**
- Topology map, routing simulator, effective-access explorer.
- Quorum queue placement, rebalance, grow/shrink.
- Executor framework (SSH, kubectl, agent) and maintenance mode / rolling upgrade wizard.
- Classic-to-quorum migration assistant.

**Phase 5: polish and teams**
- Alerting, persisted history, OIDC, per-cluster permissions.
- Packaging (Homebrew, Helm), the first tagged release, then the roadmap filed as issues, as done for natsmin.

---

## 7. Repository standards to reproduce

Mirror natsmin:
- **README:** centered logo and badges (CI, release, GHCR, Go Report Card, license), a `<picture>` hero screenshot with light and dark variants, a feature list, a screenshot gallery, install options (Docker, binaries, source), a config table, a roles table and development steps.
- **`.goreleaser.yaml`:** builds for linux, darwin, windows and freebsd on amd64 and arm64, plus linux armv7. A `before` hook builds the UI. Archives, checksums, changelog groups.
- **Release workflow:** on tag `v*`, GoReleaser publishes; multi-arch GHCR images use a cross-compiling Dockerfile (`--platform=$BUILDPLATFORM`) with `/data` owned by the distroless nonroot user (uid 65532); build provenance attestations.
- **CI:** Go vet/gofmt/test with `-race`, an OpenAPI staleness check, golangci-lint, svelte-check plus build, a GoReleaser snapshot build and a Docker build. Cancel superseded runs.
- **Repository files:** Dependabot (gomod, npm under `/ui`, actions, docker), CODEOWNERS (`* @wankdanker`), issue forms (bug, feature) plus a security contact link, a PR template, `SECURITY.md` with a deployment hardening checklist, `CODE_OF_CONDUCT.md` (Contributor Covenant 2.1 by reference), `RELEASING.md`, `.editorconfig`.
- **Backlog:** everything not built in phase 1–2 is filed as labelled GitHub issues (`roadmap`, `enhancement`, `area: …`, `tech-debt`) and linked from `ROADMAP.md`.

**Lessons from natsmin:**
- Pin pnpm via `packageManager`. Docker's corepack otherwise picks a newer pnpm, whose release-age policy rejected the lockfile.
- Vite's build empties `web/dist`, so recreate `web/dist/.gitkeep` after the build, or `go:embed` breaks on a fresh clone.
- Give each integration test its own port range; test packages run in parallel.
- Playwright and Chromium are available in cloud sessions. Use them to screenshot every page for the README and to catch console errors.
- GHCR packages start private; tell Dan to make the image public after the first release.

---

## 8. Kick-off prompt for the new session

> Read `rabmin-handoff.md`. Create the rabmin project from the natsmin codebase (`wankdanker/natsmin`). Follow the commit rules in section 0. Start with phase 0 and phase 1, verifying every management API endpoint against a running RabbitMQ 4.x cluster you start yourself. The repo currently holds only `docs/HANDOFF.md`. Push the scaffold to `main`, then use feature branches. Report what works, with screenshots, and file the remaining phases as GitHub issues.
