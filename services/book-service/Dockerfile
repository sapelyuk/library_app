# syntax=docker/dockerfile:1

# ============================================================
# Stage 1: build the service binary
# ============================================================
FROM golang:1.27-alpine AS builder

ENV GOTOOLCHAIN=local
ENV GOWORK=off

ARG MODULE=services/book-service
ARG BIN_NAME=book-service

# The shared pkg module has no dependencies of its own (no go.sum); its
# replace target must exist before `go mod download` runs in /src.
COPY pkg/ /pkg/

COPY ${MODULE}/go.mod ${MODULE}/go.sum /src/
WORKDIR /src
RUN --mount=type=cache,target=/go/pkg/mod go mod download

COPY ${MODULE}/ /src/
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 go build -ldflags="-s -w" -o /out/${BIN_NAME} ./cmd/server

# ============================================================
# Stage 2: minimal runtime image
# ============================================================
FROM alpine:3.21 AS runtime

RUN apk add --no-cache wget

RUN adduser --disabled-password --home /app --shell /bin/sh --uid 10002 app

COPY --from=builder /out/book-service /app/book-service

WORKDIR /app
USER app

# Container healthcheck: HTTP liveness endpoint.
HEALTHCHECK --interval=30s --timeout=5s --start-period=15s --retries=3 \
    CMD wget -q --spider http://127.0.0.1:8091/healthz || exit 1

EXPOSE 8081 8091

ENTRYPOINT ["/app/book-service"]
