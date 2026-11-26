# syntax=docker/dockerfile:1

# ---- Stage 1: build a static binary ----
FROM golang:1.25-alpine AS build
WORKDIR /src

# Cache module downloads separately from source changes.
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY . .
ARG VERSION=dev
ARG TARGETOS=linux
ARG TARGETARCH
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=${TARGETOS} GOARCH=${TARGETARCH} \
    go build -trimpath -ldflags="-s -w -X main.version=${VERSION}" -o /out/skylight-mcp .

# Writable data dir for the token cache, owned by the distroless nonroot user (65532).
RUN mkdir -p /out/data && chown 65532:65532 /out/data

# ---- Stage 2: minimal runtime (CA certs + tzdata, no shell, non-root) ----
FROM gcr.io/distroless/static-debian12:nonroot
COPY --from=build /out/skylight-mcp /usr/local/bin/skylight-mcp
COPY --from=build --chown=65532:65532 /out/data /data

# The token cache defaults to a relative path, so it lands in /data.
WORKDIR /data
VOLUME ["/data"]
EXPOSE 8080
USER nonroot:nonroot

HEALTHCHECK --interval=30s --timeout=5s --start-period=10s --retries=3 \
    CMD ["/usr/local/bin/skylight-mcp", "-healthcheck", "http://127.0.0.1:8080/healthz"]

# Mount your config at /config/config.yaml (read-only).
ENTRYPOINT ["/usr/local/bin/skylight-mcp"]
CMD ["-config", "/config/config.yaml"]
