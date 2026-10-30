# syntax=docker/dockerfile:1

# ---------------------------------------------------------------------------
# Build stage.
#
# The frontend is embedded with go:embed, so the image needs nothing but the
# Go toolchain at build time and a tiny runtime afterwards.
# ---------------------------------------------------------------------------
ARG GO_VERSION=1.26
FROM golang:${GO_VERSION}-alpine AS build

WORKDIR /src

# Dependencies first: this layer is cached until go.mod/go.sum change.
COPY go.mod go.sum ./
RUN go mod download

COPY . .

# CGO is off because the SQLite driver is pure Go (glebarez/sqlite). The
# resulting binary is static, which is why the runtime stage can be so small.
RUN CGO_ENABLED=0 GOOS=linux go build \
    -trimpath \
    -ldflags="-s -w" \
    -o /out/genepath ./cmd/server

# ---------------------------------------------------------------------------
# Runtime stage.
# ---------------------------------------------------------------------------
FROM alpine:3.21

# ca-certificates: calling g:Profiler over HTTPS needs a root store.
# wget: used by the HEALTHCHECK below.
RUN apk add --no-cache ca-certificates tzdata wget \
    && addgroup -S app \
    && adduser -S -G app -h /app app

WORKDIR /app
COPY --from=build /out/genepath /usr/local/bin/genepath

# State lives in a volume so `docker run -v genepath-data:/data` survives
# container replacement. The process runs as an unprivileged user.
RUN mkdir -p /data && chown -R app:app /data /app
VOLUME ["/data"]

ENV GIN_MODE=release \
    GENEPATH_ADDR=0.0.0.0:8080 \
    GENEPATH_DB=/data/genepath.db \
    GENEPATH_ARTIFACTS=/data/artifacts \
    GENEPATH_SEED=false

USER app
EXPOSE 8080

HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget -qO- http://127.0.0.1:8080/healthz >/dev/null || exit 1

ENTRYPOINT ["genepath"]
CMD ["-addr", "0.0.0.0:8080", "-db", "/data/genepath.db", "-artifacts", "/data/artifacts", "-seed=false"]
