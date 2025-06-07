# Build stage
FROM golang:1.21-alpine AS builder

# Set working directory
WORKDIR /app

# Install git (needed for go mod download)
RUN apk add --no-cache git

# Copy go mod files
COPY go.mod ./
COPY go.su[m] ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 GOOS=linux go build -a -installsuffix cgo -o mcp-server cmd/mcp-server/main.go

# Final stage
FROM alpine:latest

# Install ca-certificates for HTTPS requests
RUN apk --no-cache add ca-certificates

# Create non-root user
RUN addgroup -g 1001 -S appgroup && \
    adduser -u 1001 -S appuser -G appgroup

WORKDIR /root/

# Copy the binary from builder stage
COPY --from=builder /app/mcp-server .

# Change ownership to non-root user
RUN chown appuser:appgroup mcp-server

# Switch to non-root user
USER appuser

# Expose port for HTTP Streams transport
EXPOSE 8080

# Default command
ENTRYPOINT ["./mcp-server"]

# Default arguments (can be overridden)
CMD ["-transport=http-streams", "-addr=8080"]