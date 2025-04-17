# 1. Build stage: compile a static Go binary
FROM golang:1.23-alpine AS builder

WORKDIR /src

# Cache dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build with optimizations and static linking
RUN CGO_ENABLED=0 \
    GOOS=linux \
    GOARCH=amd64 \
    go build -ldflags="-s -w" -o /keda-horizon-scaler

# 2. Final stage: minimal scratch image
FROM scratch

# Optional: include CA certificates if you connect over TLS
# COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/ca-certificates.crt

# Copy the binary
COPY --from=builder /keda-horizon-scaler /keda-horizon-scaler

# Expose gRPC port
EXPOSE 6000

# Run the server
ENTRYPOINT ["/keda-horizon-scaler"]
