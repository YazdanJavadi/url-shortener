# syntax=docker/dockerfile:1

# ---- Builder stage: compile the Go binary ----
FROM golang:1.26-alpine AS builder

WORKDIR /src

# Cache deps: copy manifests first so layer is reused when only code changes.
COPY go.mod go.sum ./
RUN go mod download

# Copy source and build a static binary.
COPY . .
# CGO disabled + musl -> fully static binary suitable for scratch/distroless.
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o /out/urlshort ./cmd/urlshort

# ---- Runtime stage: minimal image with the binary + config ----
FROM alpine:3.20

# ca-certificates needed so TLS handshakes to https:// long URLs work on redirect.
RUN apk add --no-cache ca-certificates tzdata

WORKDIR /app
COPY --from=builder /out/urlshort /app/urlshort
COPY config.yaml /app/config.yaml

EXPOSE 8080

# Default: run the server. docker-compose overrides this to migrate-then-serve.
CMD ["/app/urlshort", "serve"]
