# Build Stage
FROM golang:alpine AS builder

WORKDIR /app

# Install necessary runtime deps (ca-certificates and tzdata)
RUN apk --no-cache add ca-certificates tzdata

# Install dependencies
COPY go.mod go.sum ./
RUN go mod download

# Copy source
COPY cmd/ cmd/
COPY internal/ internal/
COPY config.json ./

# Build with optimizations (strip debug symbols)
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-s -w" -o agent cmd/agent/main.go

# Run Stage
FROM scratch

WORKDIR /app

# Copy CA certificates and Timezone data from builder
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/
COPY --from=builder /usr/share/zoneinfo /usr/share/zoneinfo

# Copy binary from builder
COPY --from=builder /app/agent .
# Copy default config
COPY --from=builder /app/config.json .

# Expose port
EXPOSE 8080

# Environment variables
ENV CONFIG_PATH=/app/config.json
ENV LISTEN_ADDR=:8080

# Run
CMD ["./agent"]
