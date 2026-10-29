<p align="center">
  <img src="docs/logo.svg" width="72" height="72" alt="">
</p>

<h1 align="center">rabmin</h1>

<p align="center">
  A modern management UI and HTTP API for <a href="https://www.rabbitmq.com">RabbitMQ</a>.<br>
  Many clusters in one place, the messaging topology at a glance, safe message browsing, and cluster maintenance as a workflow.
</p>

<p align="center">
  <a href="https://github.com/wankdanker/rabmin/actions/workflows/ci.yml"><img src="https://github.com/wankdanker/rabmin/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
  <a href="https://github.com/wankdanker/rabmin/releases/latest"><img src="https://img.shields.io/github/v/release/wankdanker/rabmin?include_prereleases&sort=semver" alt="Release"></a>
  <a href="https://github.com/wankdanker/rabmin/pkgs/container/rabmin"><img src="https://img.shields.io/badge/ghcr.io-rabmin-c2410c?logo=docker&logoColor=white" alt="Container image"></a>
  <a href="https://goreportcard.com/report/github.com/wankdanker/rabmin"><img src="https://goreportcard.com/badge/github.com/wankdanker/rabmin" alt="Go Report Card"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/wankdanker/rabmin" alt="License"></a>
</p>

> **Status: early development.** This is the project scaffold: users, roles, API tokens, the audit log, the UI shell and the development clusters. Cluster monitoring and management arrive phase by phase; see the [roadmap](ROADMAP.md).

rabmin is one Go binary with the UI built in, the sibling of [natsmin](https://github.com/wankdanker/natsmin). It talks to RabbitMQ through the management HTTP API, the Prometheus endpoint, and AMQP and stream clients. Nothing is installed on your brokers.

## Goals

- Everything the built-in management UI does, in a more modern form.
- Many clusters in one place, with a fleet overview and cross-cluster search.
- A topology map of exchanges, bindings and queues, and a "where does this message go?" simulator.
- Safe message browsing: streams read non-destructively, and classic and quorum queues browsed with explicit requeue semantics.
- Maintenance as a workflow: pre-checks, drain, upgrade, revive, node by node.
- An API plane: every UI action is a documented API operation.

## Install

**Docker**

```bash
docker run -p 8080:8080 -v rabmin:/data ghcr.io/wankdanker/rabmin
```

**From source**

```bash
make ui build && ./bin/rabmin
```

Open http://localhost:8080 and create the admin account.

## Try it with local RabbitMQ clusters

`rabmin-devcluster` starts a 3-node RabbitMQ 4.x cluster `east` and a single-node cluster `west` in Docker, joined by a federation link and a shovel, and can seed vhosts, queues of every type, policies and demo traffic.

```bash
go run ./cmd/rabmin-devcluster -demo
```

Management APIs are on http://localhost:15672 to 15675 (user `admin` / `admin`), Prometheus on 15692 to 15695, AMQP on 5672 to 5675. Stop with ctrl-c, or `go run ./cmd/rabmin-devcluster -down` after an interrupted run.

Or with Docker Compose (3-node cluster plus rabmin, login `admin` / `changeme`):

```bash
docker compose up --build
```

## Configuration

Flags or environment variables:

| Variable | Default | |
|---|---|---|
| `RABMIN_ADDR` | `:8080` | Listen address |
| `RABMIN_DATA` | `./data` | Database and secret key directory |
| `RABMIN_SECRET_KEY` | | Base64 32-byte key for encrypting credentials. If unset, one is generated in the data directory. |
| `RABMIN_ADMIN_USER`, `RABMIN_ADMIN_PASSWORD` | | Create the first admin on startup |
| `RABMIN_ALLOW_SETUP` | `true` | Let the first visitor create the admin when no users exist. Turn off when exposing rabmin before setup. |
| `RABMIN_POLL_INTERVAL` | `5s` | How often each cluster is polled |
| `RABMIN_SECURE_COOKIES` | `false` | Mark session cookies `Secure` (enable behind HTTPS) |
| `RABMIN_TLS_CERT`, `RABMIN_TLS_KEY` | | Serve HTTPS directly |
| `RABMIN_LOG_JSON` | `false` | JSON logs |

`rabmin openapi` prints the OpenAPI document.

## Roles

| | viewer | operator | admin |
|---|:-:|:-:|:-:|
| See everything | ✓ | ✓ | ✓ |
| Queues, exchanges, bindings, messages; close connections | | ✓ | ✓ |
| Register clusters, RabbitMQ users, permissions and policies, maintenance, rabmin users, audit log | | | ✓ |

## Development

Requirements: Go 1.26+, Node 22+, pnpm and Docker.

```bash
make devcluster   # terminal 1: RabbitMQ clusters + demo traffic
make dev          # terminal 2: API server on :8080
cd ui && pnpm install && pnpm dev   # terminal 3: UI with hot reload on :5173
```

`make test` runs the Go tests; integration tests start RabbitMQ clusters in Docker and skip without a daemon. `make check` runs vet, gofmt and the Svelte type checker. After changing the API, `make openapi` regenerates `docs/openapi.json` and the UI's typed client.

See [docs/architecture.md](docs/architecture.md) for how the pieces fit together, [CONTRIBUTING.md](CONTRIBUTING.md) for the workflow, and [RELEASING.md](RELEASING.md) for cutting releases.

## License

Apache License 2.0. RabbitMQ is a trademark of Broadcom. rabmin is an independent project and is not affiliated with Broadcom or the RabbitMQ team.
