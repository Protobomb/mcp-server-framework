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
- 🔧 **MCP Client**: Full-featured client implementation for testing and development

## Quick Start

### As a Standalone Server

```bash
# STDIO transport (default)
go run cmd/mcp-server/main.go

# SSE transport
go run cmd/mcp-server/main.go -transport=sse -addr=8080
```

### As a Library

```go
package main

import (
    "context"
    "encoding/json"
    "log"

    "github.com/protobomb/mcp-server-framework/pkg/transport"
    "github.com/protobomb/mcp-server-framework/pkg/mcp"
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

### As a Client

```go
package main

import (
    "context"
    "log"

    "github.com/openhands/mcp-server-framework/internal/transport"
    "github.com/openhands/mcp-server-framework/pkg/client"
    "github.com/openhands/mcp-server-framework/pkg/mcp"
)

func main() {
    // Create transport (STDIO or HTTP)
    transport := transport.NewSTDIOTransport("./mcp-server")
    // transport := transport.NewHTTPTransport("http://localhost:8080")

    // Create client
    client := client.NewClient(transport)
    defer client.Close()

    ctx := context.Background()

    // Initialize connection
    clientInfo := mcp.ServerInfo{
        Name:    "my-client",
        Version: "1.0.0",
    }
    
    result, err := client.Initialize(ctx, clientInfo)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Server: %s v%s", result.ServerInfo.Name, result.ServerInfo.Version)

    // List available tools
    tools, err := client.ListTools(ctx)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Available tools: %d", len(tools.Tools))

    // Call a tool
    params := map[string]interface{}{
        "message": "Hello from client!",
    }
    
    response, err := client.CallTool(ctx, "echo", params)
    if err != nil {
        log.Fatal(err)
    }
    log.Printf("Tool response: %v", response)
}
```

## Quick Commands

```bash
# Build everything
make build-both

# Test the client
make run-client

# Try the interactive demo
make demo

# Run all tests
make test
```

## Installation

### Go Module

```bash
go get github.com/openhands/mcp-server-framework
```

### Docker

```bash
docker pull ghcr.io/openhands/mcp-server-framework:latest

# Run with SSE transport (default)
docker run -p 8080:8080 ghcr.io/openhands/mcp-server-framework:latest

# Run with STDIO transport
docker run -i ghcr.io/openhands/mcp-server-framework:latest -transport=stdio
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

### Client

#### Creating a Client

```go
// With STDIO transport
transport := transport.NewSTDIOTransport("./mcp-server")
client := client.NewClient(transport)

// With HTTP transport
transport := transport.NewHTTPTransport("http://localhost:8080")
client := client.NewClient(transport)
```

#### Client Methods

```go
// Initialize connection
clientInfo := mcp.ServerInfo{
    Name:    "my-client",
    Version: "1.0.0",
}
result, err := client.Initialize(ctx, clientInfo)

// List available tools
tools, err := client.ListTools(ctx)

// Call a tool
params := map[string]interface{}{
    "param1": "value1",
    "param2": 42,
}
response, err := client.CallTool(ctx, "toolName", params)

// Send notifications
err := client.Notify(ctx, "notificationName", params)
```

#### Client Lifecycle

```go
// Close the client
err := client.Close()
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
- `GET /sse` - SSE event stream (with optional ?sessionId parameter)
- `POST /message` - Send messages to server (requires ?sessionId parameter)
- `GET /health` - Health check

## Built-in Handlers

The framework includes these built-in MCP handlers:

- `initialize` - MCP initialization handshake
- `initialized` - MCP initialization complete notification
- `tools/list` - List available tools
- `tools/call` - Call a specific tool

## Built-in Tools

The framework includes these example tools:

- `echo` - Echo back the provided message
  - Parameters: `message` (string) - The message to echo back
  - Returns: Text content with "Echo: {message}"

## Testing

The framework includes comprehensive testing with both unit and integration tests.

### Quick Testing

```bash
# Run all tests (unit + integration)
make test-all

# Run only unit tests
make test

# Run only integration tests
make test-integration

# Run tests with coverage
make test-coverage

# Run all checks (format, lint, test)
make check
```

### Test Coverage

The framework has **42 comprehensive test cases** covering:

- **SSE Transport**: 14 tests covering session management, message handling, CORS, error handling
- **STDIO Transport**: 9 tests covering transport lifecycle and message handling  
- **MCP Server**: 13 tests covering handlers, tools, capabilities, and protocol compliance
- **MCP Client**: 14 tests covering client operations, timeouts, and error handling

### Integration Testing

Integration tests automatically:
1. Start the MCP server with SSE transport
2. Test complete MCP protocol flow (initialize → initialized → tools/list → tools/call)
3. Verify request/response matching and notification handling
4. Clean up server process

### Manual Testing

```bash
# Manual unit testing
go test ./pkg/...
go test -v -race ./pkg/...
go test -cover ./pkg/...

# Test with mcp-cli (requires Node.js)
./mcp-server -transport=sse -addr=8080 &
npx @modelcontextprotocol/cli connect sse http://localhost:8080/sse

# Test client with STDIO server
make test-client-stdio

# Test client with HTTP server  
make test-client-http
```

### Test Scripts

- `scripts/test_sse_integration.py` - Python-based SSE integration test
- `scripts/test-examples.sh` - Bash-based endpoint testing script

## Building

```bash
# Build everything (server + client)
make build-both

# Build just the server
make build

# Build just the client
make build-client

# Run the client with help
make run-client

# Try the interactive demo
make demo

# Manual building
go build -o mcp-server cmd/mcp-server/main.go
go build -o mcp-client cmd/mcp-client/main.go

# Build for different platforms
GOOS=linux GOARCH=amd64 go build -o mcp-server-linux cmd/mcp-server/main.go
GOOS=windows GOARCH=amd64 go build -o mcp-server.exe cmd/mcp-server/main.go
GOOS=darwin GOARCH=amd64 go build -o mcp-server-darwin cmd/mcp-server/main.go
```

## Docker

```bash
# Build Docker image
docker build -t mcp-server-framework .

# Run with SSE transport (default)
docker run -p 8080:8080 mcp-server-framework

# Run with STDIO transport
docker run -i mcp-server-framework -transport=stdio

# Run with custom SSE address
docker run -p 9090:9090 mcp-server-framework -addr=9090
```

## Examples

### Using the Test Client

```bash
# Test with STDIO server
./mcp-client -transport=stdio -command='./mcp-server'

# Test with HTTP server (start server first)
./mcp-server -transport=sse -addr=8080 &
./mcp-client -transport=http -addr=http://localhost:8080

# Interactive demo
make demo
```

### Raw STDIO Client Example

```bash
echo '{"jsonrpc":"2.0","id":1,"method":"initialize","params":{}}' | ./mcp-server
```

### SSE Client Example

```javascript
// Generate or get session ID
const sessionId = 'your-session-id'; // or generate one

// Connect to SSE endpoint
const eventSource = new EventSource(`http://localhost:8080/sse?sessionId=${sessionId}`);

eventSource.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log('Received:', data);
};

// Send a message
fetch(`http://localhost:8080/message?sessionId=${sessionId}`, {
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