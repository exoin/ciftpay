# syntax=docker/dockerfile:1.7
# Multi-stage build for the CiftPay Go binaries. docker-compose selects a
# target per service: api, worker, ciftctl, smssink.

FROM golang:1.23-alpine AS build
WORKDIR /src
RUN apk add --no-cache ca-certificates tzdata
COPY go.mod go.sum ./
RUN --mount=type=cache,target=/go/pkg/mod go mod download
COPY . .
ENV CGO_ENABLED=0 GOOS=linux
RUN --mount=type=cache,target=/go/pkg/mod --mount=type=cache,target=/root/.cache/go-build \
    go build -trimpath -ldflags="-s -w" -o /out/api ./cmd/api && \
    go build -trimpath -ldflags="-s -w" -o /out/worker ./cmd/worker && \
    go build -trimpath -ldflags="-s -w" -o /out/ciftctl ./cmd/ciftctl && \
    go build -trimpath -ldflags="-s -w" -o /out/smssink ./cmd/smssink
RUN mkdir -p /out/uploads

FROM gcr.io/distroless/static-debian12:nonroot AS base
WORKDIR /app
COPY --from=build /usr/share/zoneinfo /usr/share/zoneinfo
ENV TZ=Africa/Nairobi
USER nonroot:nonroot

FROM base AS api
COPY --from=build /out/api /out/ciftctl /app/
# UPLOAD_DIR for the signed authorization letters (ADR-0008). Created in the
# image so a fresh named volume inherits nonroot ownership.
COPY --from=build --chown=nonroot:nonroot /out/uploads /var/lib/ciftpay/uploads
EXPOSE 8080
ENTRYPOINT ["/app/api"]

FROM base AS worker
COPY --from=build /out/worker /app/
ENTRYPOINT ["/app/worker"]

FROM base AS ciftctl
COPY --from=build /out/ciftctl /app/
ENTRYPOINT ["/app/ciftctl"]

FROM base AS smssink
COPY --from=build /out/smssink /app/
EXPOSE 8025
ENTRYPOINT ["/app/smssink"]
