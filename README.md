# MCP Server Framework

A simple, reusable Model Context Protocol (MCP) server framework written in Go. This framework supports both STDIO and Server-Sent Events (SSE) transports and can be used as both a library and a standalone executable.

## Features

- 🚀 **Multiple Transports**: Support for STDIO and SSE (Server-Sent Events)
- 📦 **Library & Standalone**: Use as a Go library or run as a standalone server
- 🧪 **Well Tested**: Comprehensive test coverage
- 🔄 **JSON-RPC 2.0**: Full JSON-RPC 2.0 protocol support
- 🌐 **CORS Enabled**: Built-in CORS support for web clients
- 🐳 **Containerized**: Docker support with automated builds
- ⚡ **Easy to Use**: Simple API for registering handlers

## Quick Start

### As a Standalone Server

```bash
# STDIO transport (default)
go run cmd/mcp-server/main.go

# SSE transport
go run cmd/mcp-server/main.go -transport=sse -addr=:8080
```

### As a Library

```go
package main

import (
    "context"
    "encoding/json"
    "log"

    "github.com/openhands/mcp-server-framework/internal/transport"
    "github.com/openhands/mcp-server-framework/pkg/mcp"
)

func main() {
    // Create transport (STDIO or SSE)
    transport := transport.NewSTDIOTransport()
    // transport := transport.NewSSETransport(":8080")

    // Create server
    server := mcp.NewServer(transport)

    // Register a custom handler
    server.RegisterHandler("greet", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
        var greetParams struct {
            Name string `json:"name"`
        }
        
        if len(params) > 0 {
            json.Unmarshal(params, &greetParams)
        }
        
        return map[string]string{
            "message": "Hello, " + greetParams.Name + "!",
        }, nil
    })

    // Start server
    ctx := context.Background()
    if err := server.Start(ctx); err != nil {
        log.Fatal(err)
    }

    // Keep running...
    select {}
}
```

## Installation

### Go Module

```bash
go get github.com/openhands/mcp-server-framework
```

### Docker

```bash
docker pull ghcr.io/openhands/mcp-server-framework:latest
docker run -p 8080:8080 ghcr.io/openhands/mcp-server-framework:latest -transport=sse
```

## API Reference

### Server

#### Creating a Server

```go
server := mcp.NewServer(transport)
```

#### Registering Handlers

```go
// Request handler (expects response)
server.RegisterHandler("methodName", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
    // Handle request and return result
    return result, nil
})

// Notification handler (no response expected)
server.RegisterNotificationHandler("notificationName", func(ctx context.Context, params json.RawMessage) error {
    // Handle notification
    return nil
})
```

#### Server Lifecycle

```go
// Start the server
err := server.Start(ctx)

// Send notifications to clients
err := server.SendNotification("eventName", eventData)

// Stop the server
err := server.Stop()

// Close the server
err := server.Close()
```

### Transports

#### STDIO Transport

```go
transport := transport.NewSTDIOTransport()
// or with custom IO
transport := transport.NewSTDIOTransportWithIO(reader, writer)
```

#### SSE Transport

```go
transport := transport.NewSSETransport(":8080")
```

SSE endpoints:
- `GET /events` - SSE event stream
- `POST /send` - Send messages to server
- `GET /health` - Health check

## Built-in Handlers

The framework includes these built-in handlers:

- `initialize` - MCP initialization
- `initialized` - MCP initialization complete notification

## Example Handlers

The standalone server includes example handlers:

- `echo` - Echo back the input message
- `add` - Add two numbers
- `listMethods` - List available methods
- `ping` - Ping notification handler

## Testing

```bash
# Run all tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run tests with verbose output
go test -v ./...
```

## Building

```bash
# Build the standalone server
go build -o mcp-server cmd/mcp-server/main.go

# Build for different platforms
GOOS=linux GOARCH=amd64 go build -o mcp-server-linux cmd/mcp-server/main.go
GOOS=windows GOARCH=amd64 go build -o mcp-server.exe cmd/mcp-server/main.go
GOOS=darwin GOARCH=amd64 go build -o mcp-server-darwin cmd/mcp-server/main.go
```

## Docker

```bash
# Build Docker image
docker build -t mcp-server-framework .

# Run with STDIO
docker run -i mcp-server-framework

# Run with SSE
docker run -p 8080:8080 mcp-server-framework -transport=sse -addr=:8080
```

## Examples

### STDIO Client Example

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./mcp-server
```

### SSE Client Example

```javascript
// Connect to SSE endpoint
const eventSource = new EventSource('http://localhost:8080/events');

eventSource.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log('Received:', data);
};

// Send a message
fetch('http://localhost:8080/send', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
        jsonrpc: '2.0',
        id: 1,
        method: 'echo',
        params: { message: 'Hello, World!' }
    })
});
```

## Contributing

1. Fork the repository
2. Create a feature branch
3. Add tests for your changes
4. Ensure all tests pass
5. Submit a pull request

## License

MIT License - see [LICENSE](LICENSE) file for details.

## Protocol Support

This framework implements the Model Context Protocol (MCP) specification:
- JSON-RPC 2.0 messaging
- Initialization handshake
- Request/response patterns
- Notification support
- Error handling

For more information about MCP, visit the [official specification](https://spec.modelcontextprotocol.io/).