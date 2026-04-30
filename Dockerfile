FROM golang:1.21-alpine AS builder

WORKDIR /build

# Build arguments for version information
ARG VERSION=dev
ARG COMMIT=unknown
ARG BUILD_DATE=unknown

# Copy go mod files
COPY go.mod go.sum* ./
RUN go mod download

# Copy source
COPY . .

# Build with version information
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo \
    -ldflags "-X github.com/netspec/netspec/internal/version.Version=${VERSION} \
              -X github.com/netspec/netspec/internal/version.Commit=${COMMIT} \
              -X github.com/netspec/netspec/internal/version.BuildDate=${BUILD_DATE}" \
    -o netspec ./cmd/netspec

# Final stage
FROM alpine:latest

# Install wget for downloading gnmic, and keep ca-certificates for HTTPS
RUN apk --no-cache add ca-certificates tzdata wget

WORKDIR /app

COPY --from=builder /build/netspec .

# Download and install gnmic (gNMI CLI client)
# Supports both amd64 and arm64 architectures
ARG TARGETARCH
ARG GNMIC_VERSION=0.26.0
RUN case ${TARGETARCH} in \
        amd64) ARCH="Linux_x86_64" ;; \
        arm64) ARCH="Linux_aarch64" ;; \
        *) echo "Unsupported architecture: ${TARGETARCH}" && exit 1 ;; \
    esac && \
    wget -q -O /tmp/gnmic.tar.gz \
        "https://github.com/karimra/gnmic/releases/download/v${GNMIC_VERSION}/gnmic_${GNMIC_VERSION}_${ARCH}.tar.gz" && \
    tar -xzf /tmp/gnmic.tar.gz -C /tmp && \
    mv /tmp/gnmic /usr/local/bin/gnmic && \
    chmod +x /usr/local/bin/gnmic && \
    rm -rf /tmp/gnmic.tar.gz /tmp/gnmic_* && \
    gnmic version

# Create config and data directories
RUN mkdir -p /config /data

EXPOSE 8088

ENTRYPOINT ["./netspec"]
