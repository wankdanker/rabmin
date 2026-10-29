# Contributing

Thanks for helping. Issues and pull requests are welcome.

## Setup

Go 1.26+, Node 22+, pnpm and Docker are needed. RabbitMQ is Erlang and cannot be embedded, so the development clusters and the integration tests run RabbitMQ in Docker.

```bash
make devcluster   # 3-node cluster east + single-node west, with demo traffic
```

```bash
make dev
```

```bash
cd ui && pnpm install && pnpm dev
```

## Workflow

- Pick an issue (the [roadmap issues](https://github.com/wankdanker/rabmin/issues?q=is%3Aissue+is%3Aopen+label%3Aroadmap) are the backlog) or open one first for larger changes.
- Branch from `main` (`feature/oidc-login`, `fix/connection-paging`) and open a pull request. `main` only changes through pull requests.
- CI must pass: Go tests and race detector, golangci-lint, svelte-check, a snapshot release build and the Docker build.
- Releases are cut from `main` by tagging; see [RELEASING.md](RELEASING.md).

## Before you open a pull request

```bash
make check
```

```bash
make test
```

- API changes need a test in `internal/api/api_test.go` and a regenerated spec (`make openapi`). CI fails if `docs/openapi.json` is stale.
- Verify RabbitMQ API calls against a running broker (`make devcluster`, then `http://localhost:15672/api/index.html`) rather than from memory, and note surprises in `docs/rabbitmq-api-notes.md`.
- Destructive actions in the UI need a confirmation; irreversible ones (deleting queues, vhosts, users) use type-to-confirm.
- Follow the style of the surrounding code. Go is `gofmt`ed; Svelte uses runes.

## Reporting security issues

Please do not open public issues for vulnerabilities. Email the maintainer listed on the GitHub profile instead.
