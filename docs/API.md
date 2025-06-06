# MCP Server Framework API Documentation

## Overview

The MCP Server Framework provides a complete implementation of the Model Context Protocol (MCP) with support for multiple transport layers. This document covers all available endpoints, methods, and functionality.

## Transport Layers

### STDIO Transport

The STDIO transport communicates over standard input/output using JSON-RPC 2.0 messages.

**Usage:**
```go
transport := transport.NewSTDIOTransport()
server := mcp.NewServer(transport)
```

**Message Format:**
All messages follow JSON-RPC 2.0 specification:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "methodName",
  "params": {...}
}
```

### SSE (Server-Sent Events) Transport

The SSE transport provides HTTP-based communication with real-time server-to-client messaging.

**Usage:**
```go
transport := transport.NewSSETransport(":8080")
server := mcp.NewServer(transport)
```

#### SSE Endpoints

##### GET /sse
Establishes a Server-Sent Events connection for receiving messages from the server.

**Parameters:**
- `sessionId` (optional): Session identifier. If not provided, a new session ID will be generated.

**Headers:**
- `Content-Type: text/event-stream`
- `Cache-Control: no-cache`
- `Connection: keep-alive`

**JavaScript Example:**
```javascript
const eventSource = new EventSource('http://localhost:8080/sse?sessionId=abc123');
eventSource.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log('Received:', data);
};
```

**curl Example:**
```bash
# Connect to SSE stream (will keep connection open)
curl -N -H "Accept: text/event-stream" http://localhost:8080/sse?sessionId=test-session

# Connect without session ID (server will generate one)
curl -N -H "Accept: text/event-stream" http://localhost:8080/sse
```

##### POST /message
Sends messages to the server.

**Parameters:**
- `sessionId` (required): Session identifier for the client connection.

**Headers:**
- `Content-Type: application/json`

**Body:**
JSON-RPC 2.0 message format.

**JavaScript Example:**
```javascript
fetch('http://localhost:8080/message?sessionId=abc123', {
    method: 'POST',
    headers: { 'Content-Type': 'application/json' },
    body: JSON.stringify({
        jsonrpc: '2.0',
        id: 1,
        method: 'tools/list',
        params: {}
    })
});
```

**curl Examples:**
```bash
# Initialize MCP connection
curl -X POST "http://localhost:8080/message?sessionId=test-session" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "clientInfo": {
        "name": "curl-client",
        "version": "1.0.0"
      }
    }
  }'

# List available tools
curl -X POST "http://localhost:8080/message?sessionId=test-session" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
  }'

# Call echo tool
curl -X POST "http://localhost:8080/message?sessionId=test-session" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "echo",
      "arguments": {
        "message": "Hello from curl!"
      }
    }
  }'
```

##### GET /health
Health check endpoint.

**Response:**
```json
{
  "status": "ok",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

**curl Example:**
```bash
curl http://localhost:8080/health
```

## MCP Protocol Methods

### Core Protocol Methods

#### initialize
Initializes the MCP connection and exchanges capabilities.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "initialize",
  "params": {
    "protocolVersion": "2024-11-05",
    "capabilities": {},
    "clientInfo": {
      "name": "client-name",
      "version": "1.0.0"
    }
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "protocolVersion": "2024-11-05",
    "capabilities": {
      "tools": {
        "listChanged": true
      }
    },
    "serverInfo": {
      "name": "mcp-server-framework",
      "version": "1.0.0"
    }
  }
}
```

**curl Example:**
```bash
curl -X POST "http://localhost:8080/message?sessionId=test-session" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 1,
    "method": "initialize",
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "curl-client",
        "version": "1.0.0"
      }
    }
  }'
```

#### initialized
Notification sent after successful initialization.

**Notification:**
```json
{
  "jsonrpc": "2.0",
  "method": "initialized",
  "params": {}
}
```

**curl Example:**
```bash
curl -X POST "http://localhost:8080/message?sessionId=test-session" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialized",
    "params": {}
  }'
```

### Tools Methods

#### tools/list
Lists all available tools.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/list",
  "params": {}
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 2,
  "result": {
    "tools": [
      {
        "name": "echo",
        "description": "Echo back the provided message",
        "inputSchema": {
          "type": "object",
          "properties": {
            "message": {
              "type": "string",
              "description": "The message to echo back"
            }
          },
          "required": ["message"]
        }
      }
    ]
  }
}
```

**curl Example:**
```bash
curl -X POST "http://localhost:8080/message?sessionId=test-session" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 2,
    "method": "tools/list",
    "params": {}
  }'
```

#### tools/call
Calls a specific tool with provided arguments.

**Request:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "echo",
    "arguments": {
      "message": "Hello, World!"
    }
  }
}
```

**Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "result": {
    "content": [
      {
        "type": "text",
        "text": "Echo: Hello, World!"
      }
    ],
    "isError": false
  }
}
```

**Error Response:**
```json
{
  "jsonrpc": "2.0",
  "id": 3,
  "error": {
    "code": -32602,
    "message": "Unknown tool: nonexistent"
  }
}
```

**curl Examples:**
```bash
# Call echo tool with success
curl -X POST "http://localhost:8080/message?sessionId=test-session" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 3,
    "method": "tools/call",
    "params": {
      "name": "echo",
      "arguments": {
        "message": "Hello from curl!"
      }
    }
  }'

# Call non-existent tool (will return error)
curl -X POST "http://localhost:8080/message?sessionId=test-session" \
  -H "Content-Type: application/json" \
  -d '{
    "jsonrpc": "2.0",
    "id": 4,
    "method": "tools/call",
    "params": {
      "name": "nonexistent",
      "arguments": {}
    }
  }'
```

## Built-in Tools

### echo
Echoes back the provided message.

**Parameters:**
- `message` (string, required): The message to echo back

**Returns:**
- Text content with "Echo: {message}"

**Example:**
```json
{
  "name": "echo",
  "arguments": {
    "message": "Hello, World!"
  }
}
```

## Error Codes

The framework uses standard JSON-RPC 2.0 error codes:

| Code | Name | Description |
|------|------|-------------|
| -32700 | Parse error | Invalid JSON was received |
| -32600 | Invalid Request | The JSON sent is not a valid Request object |
| -32601 | Method not found | The method does not exist / is not available |
| -32602 | Invalid params | Invalid method parameter(s) |
| -32603 | Internal error | Internal JSON-RPC error |

## Server API

### Creating a Server

```go
import (
    "github.com/protobomb/mcp-server-framework/pkg/mcp"
    "github.com/protobomb/mcp-server-framework/pkg/transport"
)

// Create transport
transport := transport.NewSSETransport(":8080")
// or
transport := transport.NewSTDIOTransport()

// Create server
server := mcp.NewServer(transport)
```

### Registering Custom Handlers

```go
// Register a request handler
server.RegisterHandler("custom/method", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
    // Parse params
    var customParams struct {
        Name string `json:"name"`
    }
    if len(params) > 0 {
        if err := json.Unmarshal(params, &customParams); err != nil {
            return nil, &mcp.RPCError{
                Code:    mcp.InvalidParams,
                Message: "Invalid parameters",
            }
        }
    }
    
    // Return result
    return map[string]string{
        "greeting": "Hello, " + customParams.Name,
    }, nil
})

// Register a notification handler
server.RegisterNotificationHandler("custom/notification", func(ctx context.Context, params json.RawMessage) error {
    // Handle notification
    log.Println("Received notification")
    return nil
})
```

### Server Lifecycle

```go
ctx := context.Background()

// Start the server
if err := server.Start(ctx); err != nil {
    log.Fatal(err)
}

// Send notifications to clients
if err := server.SendNotification("custom/event", eventData); err != nil {
    log.Printf("Failed to send notification: %v", err)
}

// Stop the server
if err := server.Stop(); err != nil {
    log.Printf("Failed to stop server: %v", err)
}

// Close the server
if err := server.Close(); err != nil {
    log.Printf("Failed to close server: %v", err)
}
```

## Client API

### Creating a Client

```go
import (
    "github.com/protobomb/mcp-server-framework/pkg/client"
    "github.com/protobomb/mcp-server-framework/pkg/transport"
)

// Create transport
transport := transport.NewHTTPTransport("http://localhost:8080")
// or
transport := transport.NewSTDIOTransport("./mcp-server")

// Create client
client := client.NewClient(transport)
defer client.Close()
```

### Client Methods

```go
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

// List tools
tools, err := client.ListTools(ctx)
if err != nil {
    log.Fatal(err)
}

// Call a tool
params := map[string]interface{}{
    "message": "Hello from client!",
}
response, err := client.CallTool(ctx, "echo", params)
if err != nil {
    log.Fatal(err)
}

// Send custom request
result, err := client.Call(ctx, "custom/method", params)
if err != nil {
    log.Fatal(err)
}

// Send notification
err = client.Notify(ctx, "custom/notification", params)
if err != nil {
    log.Fatal(err)
}
```

## Session Management (SSE Transport)

The SSE transport uses session-based communication:

1. **Session Creation**: When a client connects to `/sse`, a session is created
2. **Session ID**: Each session has a unique identifier
3. **Message Routing**: Messages sent to `/message` are routed to the correct session
4. **Session Cleanup**: Sessions are automatically cleaned up when clients disconnect

### Session ID Generation

Session IDs are automatically generated as 32-character hexadecimal strings:

```go
func generateSessionID() string {
    bytes := make([]byte, 16)
    rand.Read(bytes)
    return hex.EncodeToString(bytes)
}
```

### Custom Session IDs

You can provide your own session ID when connecting:

```javascript
const sessionId = 'my-custom-session-id';
const eventSource = new EventSource(`http://localhost:8080/sse?sessionId=${sessionId}`);
```

## CORS Support

The SSE transport includes built-in CORS support with the following headers:

- `Access-Control-Allow-Origin: *`
- `Access-Control-Allow-Methods: GET, POST, OPTIONS`
- `Access-Control-Allow-Headers: Content-Type`

This allows web applications to connect to the MCP server from any origin.

## Testing

The framework includes comprehensive test coverage for all components:

- **Transport Tests**: STDIO and SSE transport functionality
- **Server Tests**: MCP protocol implementation and tools
- **Client Tests**: Client functionality and error handling
- **Integration Tests**: End-to-end protocol testing

Run tests with:
```bash
go test ./... -v
```

## Examples

See the `/examples` directory for complete working examples:

- `simple-server/`: Basic MCP server implementation
- `client-server-demo/`: Interactive client-server demonstration

## Error Handling

The framework provides comprehensive error handling:

### Transport Errors
- Connection failures
- Invalid JSON
- Network timeouts

### Protocol Errors
- Invalid method names
- Missing parameters
- Type mismatches

### Custom Errors
```go
return nil, &mcp.RPCError{
    Code:    mcp.InvalidParams,
    Message: "Custom error message",
    Data:    additionalErrorData,
}
```

## Performance Considerations

### SSE Transport
- Supports multiple concurrent connections
- Automatic session cleanup
- Configurable timeouts
- CORS-enabled for web clients

### STDIO Transport
- Single connection per process
- Low latency communication
- Suitable for command-line tools

### Memory Management
- Automatic cleanup of closed connections
- Bounded message channels
- Graceful shutdown handling