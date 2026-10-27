# Stage 1: Build Vue 3 Frontend
FROM --platform=$BUILDPLATFORM node:20-bookworm-slim AS frontend-builder
WORKDIR /web
COPY web/package.json web/pnpm-lock.yaml* ./
RUN npm install -g pnpm && (pnpm install --frozen-lockfile || pnpm install)
COPY web/ ./
RUN pnpm build

# Stage 2: Build Go Backend with Embedded Frontend
FROM --platform=$BUILDPLATFORM golang:1.27-bookworm AS backend-builder
ENV GOTOOLCHAIN=auto
ARG TARGETOS
ARG TARGETARCH
ARG APP_VERSION=dev
WORKDIR /build
COPY go.mod go.sum ./
RUN go mod download
COPY . .
# Copy compiled frontend dist into web/dist for go:embed
COPY --from=frontend-builder /web/dist ./web/dist
RUN CGO_ENABLED=0 GOOS=${TARGETOS:-linux} GOARCH=${TARGETARCH:-amd64} go build \
    -ldflags="-s -w -X github.com/UtopiaXC/UtopiaImages/internal/config.Version=${APP_VERSION}" \
    -o utopia_images ./cmd/utopia_images/main.go

# Stage 3: Minimal production runtime image
FROM debian:bookworm-slim

RUN apt-get update && apt-get install -y --no-install-recommends \
    ca-certificates \
    tzdata \
    fonts-noto-cjk \
    wget \
    && rm -rf /var/lib/apt/lists/*

WORKDIR /app

COPY --from=backend-builder /build/utopia_images /app/utopia_images
COPY data/config/config.example.yaml /app/data/config/config.yaml

EXPOSE 2983

VOLUME ["/app/data"]

HEALTHCHECK --interval=30s --timeout=5s --start-period=5s --retries=3 \
  CMD [ "wget", "-qO-", "http://localhost:2983/healthz" ] || exit 1

CMD ["/app/utopia_images"]
