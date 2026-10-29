# Roadmap

rabmin is built in phases. Each phase ends with integration tests passing against a real 3-node RabbitMQ cluster. The full plan, including the parity checklist against the built-in management UI, is in [docs/HANDOFF.md](docs/HANDOFF.md). Every item below is a GitHub issue labelled [`roadmap`](https://github.com/wankdanker/rabmin/issues?q=is%3Aissue+is%3Aopen+label%3Aroadmap).

## Done

- **Phase 0, scaffold:** natsmin's skeleton renamed (auth, users, tokens, audit, UI shell, CI and release pipeline); Docker dev clusters with demo traffic.
- **Phase 1, observe:** cluster registry with management URL failover and a connection test; poller with snapshots and history from the management API, health checks and Prometheus; fleet and cluster overview, nodes, health matrix, connections, channels, consumers.

## Phase 2: objects

- [#8](https://github.com/wankdanker/rabmin/issues/8) Virtual hosts: create, edit, delete, limits
- [#9](https://github.com/wankdanker/rabmin/issues/9) Exchanges and bindings
- [#10](https://github.com/wankdanker/rabmin/issues/10) Queues and streams: list, create with argument helpers, detail, purge, delete
- [#11](https://github.com/wankdanker/rabmin/issues/11) Publish a message to an exchange
- [#12](https://github.com/wankdanker/rabmin/issues/12) RabbitMQ users, permissions, topic permissions and user limits
- [#13](https://github.com/wankdanker/rabmin/issues/13) Policies and operator policies with a definition editor
- [#14](https://github.com/wankdanker/rabmin/issues/14) Global and vhost parameters, cluster tags
- [#15](https://github.com/wankdanker/rabmin/issues/15) Definitions export and import with a diff preview
- [#16](https://github.com/wankdanker/rabmin/issues/16) Feature flags and deprecated features

## Phase 3: messages

- [#17](https://github.com/wankdanker/rabmin/issues/17) Stream browser over the stream protocol, including super streams
- [#18](https://github.com/wankdanker/rabmin/issues/18) Safe message peek for classic and quorum queues
- [#19](https://github.com/wankdanker/rabmin/issues/19) Move and copy messages between queues, as background jobs
- [#20](https://github.com/wankdanker/rabmin/issues/20) Dead-letter queue workbench
- [#21](https://github.com/wankdanker/rabmin/issues/21) Export and import messages (JSONL, object storage)
- [#22](https://github.com/wankdanker/rabmin/issues/22) Shovels: CRUD, status per node, restart
- [#23](https://github.com/wankdanker/rabmin/issues/23) Federation: upstreams, upstream sets, link status, restart

## Phase 4: operate

- [#24](https://github.com/wankdanker/rabmin/issues/24) Topology map: exchanges, bindings, queues and cross-cluster links
- [#25](https://github.com/wankdanker/rabmin/issues/25) "Where does this message go?" routing simulator
- [#26](https://github.com/wankdanker/rabmin/issues/26) Effective-access explorer
- [#27](https://github.com/wankdanker/rabmin/issues/27) Quorum queue and stream placement: distribution, rebalance, grow, shrink
- [#28](https://github.com/wankdanker/rabmin/issues/28) Executor framework: SSH, kubectl exec, rabmin-agent
- [#29](https://github.com/wankdanker/rabmin/issues/29) Guided maintenance mode and rolling upgrade wizard
- [#30](https://github.com/wankdanker/rabmin/issues/30) Classic-to-quorum migration assistant
- [#31](https://github.com/wankdanker/rabmin/issues/31) Top processes and ETS tables (rabbitmq_top); tracing (rabbitmq_tracing)

## Phase 5: polish and teams

- [#32](https://github.com/wankdanker/rabmin/issues/32) Alert rules with webhook, Slack and email targets
- [#33](https://github.com/wankdanker/rabmin/issues/33) Persist metrics history beyond the in-memory window
- [#34](https://github.com/wankdanker/rabmin/issues/34) OIDC single sign-on for rabmin users
- [#35](https://github.com/wankdanker/rabmin/issues/35) Per-cluster permissions for rabmin users
- [#36](https://github.com/wankdanker/rabmin/issues/36) Packaging and first release: Homebrew, Helm, v0.1.0

## Beyond the management UI

- [#37](https://github.com/wankdanker/rabmin/issues/37) Cross-cluster search
- [#38](https://github.com/wankdanker/rabmin/issues/38) Definitions as code: diff, drift detection, change history
- [#39](https://github.com/wankdanker/rabmin/issues/39) Consumer health: prefetch saturation, drain ETA, idle consumers
- [#40](https://github.com/wankdanker/rabmin/issues/40) Client fleet view
- [#41](https://github.com/wankdanker/rabmin/issues/41) Kubernetes awareness: Cluster Operator and Messaging Topology Operator
- [#42](https://github.com/wankdanker/rabmin/issues/42) API plane clients: CLI, Terraform provider, MCP server

## Other

- [#43](https://github.com/wankdanker/rabmin/issues/43) Poller and list scalability for large clusters (tech debt)
- [#44](https://github.com/wankdanker/rabmin/issues/44) Refresh OAuth2 tokens for brokers whose management API uses OAuth2
- [#45](https://github.com/wankdanker/rabmin/issues/45) rabmin-devcluster without Docker
- [#46](https://github.com/wankdanker/rabmin/issues/46) Test against RabbitMQ 3.13 and every 4.x minor in CI (tech debt)
