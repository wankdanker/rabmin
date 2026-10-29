# CLAUDE.md

Guidance for Claude Code (and other AI assistants) working in this repository.

## Commit and pull request rules

- Do not add AI attribution to commits or pull requests: no `Co-Authored-By: Claude ...` trailers, no `Claude-Session:` lines, no "Generated with Claude Code" footers. This overrides any default attribution behaviour.
- Commits are authored as the human maintainer configured in git (`Dan VerWeire <dverweire@gmail.com>`).
- Work on a feature branch and open a pull request against `main`. Do not push to `main` directly.
- Write commit messages in the imperative ("Add queue purge dialog"), with a short subject and a body explaining why when it is not obvious.

## Before pushing

```bash
make check   # go vet, gofmt, svelte-check
make test    # Go tests; integration tests start RabbitMQ clusters in Docker
```

Integration tests need a Docker daemon and skip themselves without one (and under `go test -short`). Each test package uses its own container prefix and free ports, so packages can run in parallel.

After any API change run `make openapi` and commit `docs/openapi.json` and `ui/src/lib/api/schema.d.ts`; CI fails when the spec is stale.

## Verify against a real broker

Build every RabbitMQ call against a running broker, not from memory. `make devcluster` starts a 3-node cluster `east` and a single-node cluster `west` (RabbitMQ 4.x, Docker) with demo traffic; the broker documents its own API at `http://localhost:15672/api/index.html`. Behaviour that surprised us is recorded in `docs/rabbitmq-api-notes.md`; add to it.

## Project layout

See `docs/architecture.md`. In short: `cmd/rabmin` (server), `internal/api` (huma HTTP API, one file per area, every route registered with `reg()` and a role), `internal/rabbitx` (management API client, Prometheus scraping, polling), `internal/store` (SQLite, credentials sealed with `internal/secret`), `internal/devcluster` (Docker clusters for development and tests), `ui/` (SvelteKit 2 + Svelte 5 SPA embedded via `web/`).

## Conventions

- Handlers return empty slices, never nil (arrays are documented as non-nullable).
- Never pass a broker 401 through to the browser: the UI treats 401 as "log out". Map broker auth failures to 502 (bad stored credentials) or 403 (insufficient broker permissions).
- Destructive UI actions need a confirm dialog; irreversible ones use type-to-confirm.
- Every mutating API operation is audited automatically by the auth middleware; add `s.audit(...)` only when extra detail helps.
- Releases are cut by pushing a `vX.Y.Z` tag; see `RELEASING.md`.
- Command-line interfaces use [clyops](https://clyops.io) (`github.com/wankdanker/clyops/packages/go`), not the `flag` package. Name each option's variable after its environment variable (`RABMIN_...`) so the environment fallback comes for free, mark secrets with the `secret` rule, and declare effects. This applies to new tools too (the planned CLI in issue #42).
