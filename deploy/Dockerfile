# Stage 1: Build Vue 3 Frontend
FROM node:20-bookworm-slim AS frontend-builder
WORKDIR /web
COPY web/package.json web/pnpm-lock.yaml* ./
RUN npm install -g pnpm && pnpm install --frozen-lockfile || pnpm install
COPY web/ ./
RUN pnpm build

# Stage 2: Build Go Backend with Embedded Frontend
FROM golang:1.24-bookworm AS backend-builder

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips-dev \
    gcc \
    pkg-config \
    ca-certificates

WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy compiled frontend dist into web/dist for go:embed
COPY --from=frontend-builder /web/dist ./web/dist

RUN CGO_ENABLED=1 GOOS=linux go build -ldflags="-s -w" -o utopiaimages ./cmd/utopiaimages/main.go

# Stage 3: Minimal production runtime image
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    libvips42 \
    ca-certificates \
    tzdata \
    fonts-noto-cjk \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=backend-builder /build/utopiaimages /app/utopiaimages
COPY config/config.example.yaml /app/config/config.yaml

EXPOSE 8080

VOLUME ["/app/config", "/app/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD [ "wget", "-qO-", "http://localhost:8080/healthz" ] || exit 1

CMD ["/app/utopiaimages"]
