# Releasing

Releases are built by GitHub Actions when a version tag is pushed.

```bash
git checkout main && git pull
```

```bash
git tag -a v0.2.0 -m "v0.2.0"
```

```bash
git push origin v0.2.0
```

The `Release` workflow then:

- builds `rabmin` for Linux, macOS, Windows and FreeBSD (amd64, arm64, and armv7 on Linux) with the UI embedded, plus `rabmin-devcluster`
- publishes a GitHub release with archives, `checksums.txt` and generated notes
- pushes multi-arch images to `ghcr.io/wankdanker/rabmin` (`X.Y.Z`, `X.Y`, and `latest` for non-prereleases)
- attaches build provenance attestations to the archives and the image

Tags with a suffix (`v0.3.0-rc.1`) are published as prereleases and do not move `latest`.

To try the release build locally:

```bash
goreleaser release --snapshot --clean
```
