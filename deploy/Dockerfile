# UtopiaImages Dockerfile
# Stage 1: Build binary
FROM golang:1.24-bookworm AS builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips-dev \
    gcc \
    pkg-config \
    ca-certificates

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .

RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o utopiaimages ./cmd/utopiaimages/main.go

# Stage 2: Final runtime image
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips42 \
    ca-certificates \
    tzdata \
    fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=builder /build/utopiaimages /app/utopiaimages
COPY config/config.example.yaml /app/config/config.yaml

EXPOSE 8080

VOLUME ["/app/config", "/app/data", "/app/storage", "/app/cache"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD [ "wget", "-qO-", "http://localhost:8080/healthz" ] || exit 1

CMD ["/app/utopiaimages"]
