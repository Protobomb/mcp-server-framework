# Server-Sent Events (SSE) Transport

## Overview

The SSE transport provides HTTP-based communication with real-time server-to-client messaging using Server-Sent Events. This transport is ideal for web applications and scenarios where bidirectional HTTP communication is preferred over STDIO.

## Architecture

```
Client (Browser/HTTP)  ←→  SSE Transport  ←→  MCP Server
                      HTTP/SSE              JSON-RPC 2.0
```

## Endpoints

The SSE transport exposes three HTTP endpoints:

### GET /sse
**Purpose**: Establishes a Server-Sent Events connection for receiving messages from the server.

**Parameters**:
- `sessionId` (optional): Session identifier. If not provided, a new session ID will be generated.

**Response Headers**:
```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
Access-Control-Allow-Origin: *
```

**Response Format**:
```
data: {"jsonrpc":"2.0","id":1,"result":{"message":"response"}}

```

**Example Usage**:
```javascript
// Connect with auto-generated session ID
const eventSource = new EventSource('http://localhost:8080/sse');

// Connect with custom session ID
const sessionId = 'my-session-123';
const eventSource = new EventSource(`http://localhost:8080/sse?sessionId=${sessionId}`);

eventSource.onmessage = function(event) {
    const data = JSON.parse(event.data);
    console.log('Received:', data);
};

eventSource.onerror = function(event) {
    console.error('SSE error:', event);
};
```

### POST /message
**Purpose**: Sends messages to the server.

**Parameters**:
- `sessionId` (required): Session identifier for the client connection.

**Request Headers**:
```
Content-Type: application/json
```

**Request Body**:
JSON-RPC 2.0 message format.

**Response**:
- `200 OK`: Message accepted
- `400 Bad Request`: Invalid session ID or malformed JSON
- `405 Method Not Allowed`: Invalid HTTP method

**Example Usage**:
```javascript
const sessionId = 'my-session-123';

fetch(`http://localhost:8080/message?sessionId=${sessionId}`, {
    method: 'POST',
    headers: {
        'Content-Type': 'application/json'
    },
    body: JSON.stringify({
        jsonrpc: '2.0',
        id: 1,
        method: 'tools/list',
        params: {}
    })
});
```

### GET /health
**Purpose**: Health check endpoint for monitoring and load balancing.

**Response**:
```json
{
  "status": "ok",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

**Example Usage**:
```bash
curl http://localhost:8080/health
```

## Message Handling

The SSE transport handles both requests and notifications:

### Request vs Notification Handling

- **Requests** (with `id` field): Expect a response via SSE stream
- **Notifications** (without `id` field): No response expected, processed silently

**Request Example**:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list",
  "params": {}
}
```

**Notification Example**:
```json
{
  "jsonrpc": "2.0",
  "method": "initialized"
}
```

### Message Flow

1. **Send Request/Notification**: POST to `/message?sessionId=<session-id>`
2. **Receive Response**: Listen to SSE stream from `/sse?sessionId=<session-id>` (requests only)
3. **Response Correlation**: Responses include the same `id` as the original request

## Session Management

### Session Lifecycle

1. **Session Creation**: When a client connects to `/sse`, a session is created
2. **Session Registration**: The session is registered with a unique ID
3. **Message Routing**: Messages sent to `/message` are routed to the correct session
4. **Session Cleanup**: Sessions are automatically cleaned up when clients disconnect

### Session ID Format

Session IDs are 32-character hexadecimal strings generated using cryptographically secure random bytes:

```go
func generateSessionID() string {
    bytes := make([]byte, 16)
    rand.Read(bytes)
    return hex.EncodeToString(bytes)
}
```

Example: `a1b2c3d4e5f6789012345678901234567890abcd`

### Custom Session IDs

You can provide your own session ID when connecting:

```javascript
const customSessionId = 'my-custom-session-id';
const eventSource = new EventSource(`http://localhost:8080/sse?sessionId=${customSessionId}`);
```

**Requirements for custom session IDs**:
- Must be unique across all active sessions
- Should be URL-safe
- Recommended length: 8-64 characters

## CORS Support

The SSE transport includes comprehensive CORS support for web applications:

**Preflight Requests (OPTIONS)**:
```
Access-Control-Allow-Origin: *
Access-Control-Allow-Methods: GET, POST, OPTIONS
Access-Control-Allow-Headers: Content-Type
Access-Control-Max-Age: 86400
```

**All Responses**:
```
Access-Control-Allow-Origin: *
```

This allows web applications from any origin to connect to the MCP server.

## Complete Client Example

### JavaScript/Browser Client

```html
<!DOCTYPE html>
<html>
<head>
    <title>MCP SSE Client</title>
</head>
<body>
    <div id="output"></div>
    <button onclick="listTools()">List Tools</button>
    <button onclick="callEcho()">Call Echo Tool</button>

    <script>
        let sessionId = null;
        let eventSource = null;
        let requestId = 1;

        function log(message) {
            document.getElementById('output').innerHTML += message + '<br>';
        }

        function connect() {
            // Generate session ID or use existing one
            sessionId = sessionId || 'client-' + Date.now();
            
            // Connect to SSE endpoint
            eventSource = new EventSource(`http://localhost:8080/sse?sessionId=${sessionId}`);
            
            eventSource.onopen = function(event) {
                log('Connected to server');
                initialize();
            };
            
            eventSource.onmessage = function(event) {
                const data = JSON.parse(event.data);
                log('Received: ' + JSON.stringify(data, null, 2));
            };
            
            eventSource.onerror = function(event) {
                log('Error: ' + event);
            };
        }

        function sendMessage(method, params = {}) {
            const message = {
                jsonrpc: '2.0',
                id: requestId++,
                method: method,
                params: params
            };

            fetch(`http://localhost:8080/message?sessionId=${sessionId}`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(message)
            }).catch(error => {
                log('Send error: ' + error);
            });
        }

        function initialize() {
            sendMessage('initialize', {
                protocolVersion: '2024-11-05',
                clientInfo: {
                    name: 'web-client',
                    version: '1.0.0'
                }
            });
        }

        function listTools() {
            sendMessage('tools/list');
        }

        function callEcho() {
            sendMessage('tools/call', {
                name: 'echo',
                arguments: {
                    message: 'Hello from web client!'
                }
            });
        }

        // Connect when page loads
        window.onload = connect;
    </script>
</body>
</html>
```

### Node.js Client

```javascript
const EventSource = require('eventsource');
const fetch = require('node-fetch');

class MCPSSEClient {
    constructor(baseUrl) {
        this.baseUrl = baseUrl;
        this.sessionId = 'node-client-' + Date.now();
        this.requestId = 1;
        this.eventSource = null;
    }

    connect() {
        return new Promise((resolve, reject) => {
            this.eventSource = new EventSource(`${this.baseUrl}/sse?sessionId=${this.sessionId}`);
            
            this.eventSource.onopen = () => {
                console.log('Connected to server');
                resolve();
            };
            
            this.eventSource.onmessage = (event) => {
                const data = JSON.parse(event.data);
                console.log('Received:', data);
            };
            
            this.eventSource.onerror = (error) => {
                console.error('SSE error:', error);
                reject(error);
            };
        });
    }

    async sendMessage(method, params = {}) {
        const message = {
            jsonrpc: '2.0',
            id: this.requestId++,
            method: method,
            params: params
        };

        try {
            const response = await fetch(`${this.baseUrl}/message?sessionId=${this.sessionId}`, {
                method: 'POST',
                headers: {
                    'Content-Type': 'application/json'
                },
                body: JSON.stringify(message)
            });

            if (!response.ok) {
                throw new Error(`HTTP ${response.status}: ${response.statusText}`);
            }
        } catch (error) {
            console.error('Send error:', error);
            throw error;
        }
    }

    async initialize() {
        await this.sendMessage('initialize', {
            protocolVersion: '2024-11-05',
            clientInfo: {
                name: 'node-client',
                version: '1.0.0'
            }
        });
    }

    async listTools() {
        await this.sendMessage('tools/list');
    }

    async callTool(name, arguments) {
        await this.sendMessage('tools/call', {
            name: name,
            arguments: arguments
        });
    }

    close() {
        if (this.eventSource) {
            this.eventSource.close();
        }
    }
}

// Usage example
async function main() {
    const client = new MCPSSEClient('http://localhost:8080');
    
    try {
        await client.connect();
        await client.initialize();
        await client.listTools();
        await client.callTool('echo', { message: 'Hello from Node.js!' });
    } catch (error) {
        console.error('Client error:', error);
    } finally {
        client.close();
    }
}

main();
```

## Server Configuration

### Basic Server Setup

```go
package main

import (
    "context"
    "log"
    "os"
    "os/signal"
    "syscall"

    "github.com/protobomb/mcp-server-framework/pkg/mcp"
    "github.com/protobomb/mcp-server-framework/pkg/transport"
)

func main() {
    // Create SSE transport
    transport := transport.NewSSETransport(":8080")
    
    // Create MCP server
    server := mcp.NewServer(transport)
    
    // Register custom handlers
    server.RegisterHandler("custom/greet", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
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
        log.Fatal("Failed to start server:", err)
    }
    
    log.Println("MCP server started on :8080")
    log.Println("SSE endpoint: http://localhost:8080/sse")
    log.Println("Message endpoint: http://localhost:8080/message")
    log.Println("Health endpoint: http://localhost:8080/health")
    
    // Wait for interrupt signal
    c := make(chan os.Signal, 1)
    signal.Notify(c, os.Interrupt, syscall.SIGTERM)
    <-c
    
    log.Println("Shutting down server...")
    server.Close()
}
```

### Advanced Configuration

```go
// Custom SSE transport with configuration
type SSEConfig struct {
    Addr           string
    ReadTimeout    time.Duration
    WriteTimeout   time.Duration
    MaxConnections int
}

func NewConfiguredSSETransport(config SSEConfig) *SSETransport {
    transport := NewSSETransport(config.Addr)
    
    // Configure HTTP server
    transport.server.ReadTimeout = config.ReadTimeout
    transport.server.WriteTimeout = config.WriteTimeout
    
    // Add middleware for connection limiting
    // ... implementation
    
    return transport
}
```

## Error Handling

### Client-Side Error Handling

```javascript
eventSource.onerror = function(event) {
    console.error('SSE connection error:', event);
    
    // Implement reconnection logic
    setTimeout(() => {
        console.log('Attempting to reconnect...');
        connect();
    }, 5000);
};

// Handle fetch errors
async function sendMessage(method, params) {
    try {
        const response = await fetch(`/message?sessionId=${sessionId}`, {
            method: 'POST',
            headers: { 'Content-Type': 'application/json' },
            body: JSON.stringify({ jsonrpc: '2.0', id: 1, method, params })
        });
        
        if (!response.ok) {
            throw new Error(`HTTP ${response.status}: ${response.statusText}`);
        }
    } catch (error) {
        console.error('Failed to send message:', error);
        // Handle error (retry, show user message, etc.)
    }
}
```

### Server-Side Error Handling

The SSE transport handles various error conditions:

1. **Invalid Session ID**: Returns 400 Bad Request
2. **Malformed JSON**: Returns 400 Bad Request  
3. **Invalid HTTP Method**: Returns 405 Method Not Allowed
4. **Connection Errors**: Automatic cleanup of disconnected clients

## Performance Considerations

### Connection Management

- **Concurrent Connections**: The server supports multiple simultaneous SSE connections
- **Memory Usage**: Each connection uses approximately 1KB of memory for buffering
- **CPU Usage**: Minimal CPU overhead for message routing

### Optimization Tips

1. **Connection Pooling**: Reuse connections when possible
2. **Message Batching**: Batch multiple requests when appropriate
3. **Compression**: Enable gzip compression for large messages
4. **Keep-Alive**: Use HTTP keep-alive for the message endpoint

### Monitoring

```go
// Add metrics collection
var (
    activeConnections = prometheus.NewGauge(prometheus.GaugeOpts{
        Name: "sse_active_connections",
        Help: "Number of active SSE connections",
    })
    
    messagesReceived = prometheus.NewCounter(prometheus.CounterOpts{
        Name: "sse_messages_received_total",
        Help: "Total number of messages received",
    })
)

// Update metrics in handlers
func (t *SSETransport) handleSSE(w http.ResponseWriter, r *http.Request) {
    activeConnections.Inc()
    defer activeConnections.Dec()
    // ... handler implementation
}
```

## Security Considerations

### Authentication

The basic SSE transport doesn't include authentication. For production use, consider:

```go
// Add authentication middleware
func authMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        token := r.Header.Get("Authorization")
        if !validateToken(token) {
            http.Error(w, "Unauthorized", http.StatusUnauthorized)
            return
        }
        next(w, r)
    }
}

// Apply to endpoints
mux.HandleFunc("/sse", authMiddleware(t.handleSSE))
mux.HandleFunc("/message", authMiddleware(t.handleMessage))
```

### Rate Limiting

```go
// Add rate limiting
import "golang.org/x/time/rate"

type rateLimitedTransport struct {
    *SSETransport
    limiter *rate.Limiter
}

func (t *rateLimitedTransport) handleMessage(w http.ResponseWriter, r *http.Request) {
    if !t.limiter.Allow() {
        http.Error(w, "Rate limit exceeded", http.StatusTooManyRequests)
        return
    }
    t.SSETransport.handleMessage(w, r)
}
```

### Input Validation

```go
// Validate session IDs
func validateSessionID(sessionID string) bool {
    if len(sessionID) < 8 || len(sessionID) > 64 {
        return false
    }
    
    // Check for valid characters
    for _, char := range sessionID {
        if !isValidSessionChar(char) {
            return false
        }
    }
    
    return true
}
```

## Troubleshooting

### Common Issues

1. **Connection Refused**: Check if server is running and port is accessible
2. **CORS Errors**: Ensure CORS headers are properly set
3. **Session Not Found**: Verify session ID is correctly passed
4. **Message Not Received**: Check SSE connection status

### Debug Mode

```go
// Enable debug logging
log.SetLevel(log.DebugLevel)

// Add request logging middleware
func loggingMiddleware(next http.HandlerFunc) http.HandlerFunc {
    return func(w http.ResponseWriter, r *http.Request) {
        log.Printf("Request: %s %s from %s", r.Method, r.URL.Path, r.RemoteAddr)
        next(w, r)
    }
}
```

### Testing Tools

```bash
# Test SSE endpoint
curl -N http://localhost:8080/sse

# Test message endpoint
curl -X POST http://localhost:8080/message?sessionId=test \
  -H "Content-Type: application/json" \
  -d '{"jsonrpc":"2.0","id":1,"method":"tools/list","params":{}}'

# Test health endpoint
curl http://localhost:8080/health
```