# Roadmap

rabmin is built in phases. Each phase ends with integration tests passing against a real 3-node RabbitMQ cluster. The full plan, including the parity checklist against the built-in management UI, is in [docs/HANDOFF.md](docs/HANDOFF.md).

| Phase | Scope |
|---|---|
| 0. Scaffold | natsmin skeleton renamed: auth, users, tokens, audit, UI shell, CI and release pipeline; Docker dev clusters with demo traffic |
| 1. Observe | Cluster registry with management URL failover and connection test; poller with snapshots and history from the management API, health checks and Prometheus; fleet and cluster overview, nodes, health matrix, connections, channels, consumers |
| 2. Objects | Vhosts, exchanges, queues (classic, quorum, stream), bindings, publish; users, permissions, limits, policies, parameters; definitions export/import with diff; feature flags and deprecated features |
| 3. Messages | Stream browser, peek with explicit requeue semantics, move/copy, DLQ workbench, shovels and federation |
| 4. Operate | Topology map, routing simulator, effective-access explorer, quorum queue placement, executors and maintenance wizard, classic-to-quorum migration |
| 5. Polish and teams | Alerting, persisted history, OIDC, per-cluster permissions, packaging, first release |

Planned work is tracked as [GitHub issues labelled `roadmap`](https://github.com/wankdanker/rabmin/issues?q=is%3Aissue+is%3Aopen+label%3Aroadmap).
