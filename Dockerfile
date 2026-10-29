# syntax=docker/dockerfile:1
# Build stages run on the build machine's platform and cross-compile, so
# multi-arch images do not need emulation.
FROM --platform=$BUILDPLATFORM node:22-alpine AS ui
WORKDIR /src/ui
RUN corepack enable
COPY ui/package.json ui/pnpm-lock.yaml ./
RUN pnpm install --frozen-lockfile
COPY ui/ ./
RUN mkdir -p ../web/dist && pnpm run build

FROM --platform=$BUILDPLATFORM golang:1.26-alpine AS build
ARG TARGETOS TARGETARCH VERSION=dev
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
COPY --from=ui /src/web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=$TARGETOS GOARCH=$TARGETARCH go build -trimpath \
    -ldflags "-s -w -X github.com/wankdanker/rabmin/internal/version.Version=${VERSION}" \
    -o /out/rabmin ./cmd/rabmin && mkdir -p /out/data

FROM gcr.io/distroless/static-debian12:nonroot
LABEL org.opencontainers.image.source=https://github.com/wankdanker/rabmin
COPY --from=build /out/rabmin /rabmin
# Owned by the nonroot user so a fresh named volume is writable.
COPY --from=build --chown=65532:65532 /out/data /data
ENV RABMIN_DATA=/data RABMIN_ADDR=:8080
VOLUME /data
EXPOSE 8080
USER nonroot
ENTRYPOINT ["/rabmin"]
