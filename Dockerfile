# syntax=docker/dockerfile:1.7
FROM golang:1.21-alpine AS builder

WORKDIR /build

# Build arguments for version information
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Copy go mod files
COPY go.mod go.sum* ./
RUN --mount=type=cache,target=/go/pkg/mod \
    go mod download

# Copy only build-relevant source
COPY cmd ./cmd
COPY internal ./internal

# Build with version information
RUN --mount=type=cache,target=/go/pkg/mod \
    --mount=type=cache,target=/root/.cache/go-build \
    CGO_ENABLED=0 GOOS=linux go build -trimpath \
    -ldflags "-X github.com/netspec/netspec/internal/version.Version=${VERSION} \
              -X github.com/netspec/netspec/internal/version.Commit=${COMMIT} \
              -X github.com/netspec/netspec/internal/version.BuildDate=${BUILD_DATE}" \
    -o netspec ./cmd/netspec

# Final stage
FROM alpine:latest

# Keep runtime certs/timezone data
RUN apk --no-cache add ca-certificates tzdata

WORKDIR /app

COPY --from=builder /build/netspec .

# Create config and data directories
RUN mkdir -p /config /data

EXPOSE 8088

ENTRYPOINT ["./netspec"]
