# HTTP Streams Transport

The HTTP Streams transport implements the MCP (Model Context Protocol) Streamable HTTP specification (2024-11-05), providing a modern alternative to SSE transport with improved performance and standards compliance.

## Overview

HTTP Streams transport combines HTTP POST requests for sending messages with Server-Sent Events (SSE) for receiving responses, providing a bidirectional communication channel that's compatible with modern web standards and proxy servers.

## Key Features

- **MCP Specification Compliance**: Fully implements MCP Streamable HTTP (2024-11-05)
- **Session Management**: Automatic session ID generation and validation
- **Persistent SSE Streams**: Long-lived connections for real-time responses
- **Request/Response Mapping**: Intelligent routing of responses to correct SSE streams
- **Batch Request Support**: Handle multiple requests in a single HTTP call
- **Error Handling**: Comprehensive error responses with proper HTTP status codes
- **Debug Logging**: Detailed logging for troubleshooting and monitoring

## Architecture

### Transport Flow

1. **Initialize**: Client sends `initialize` request via POST, receives direct JSON response with session ID
2. **SSE Stream**: Client establishes SSE stream via GET request with session ID header
3. **Communication**: All subsequent requests sent via POST, responses received via SSE stream
4. **Session Validation**: All requests validated against established session

### HTTP Endpoints

- **POST /mcp**: Send MCP requests and notifications
- **GET /mcp**: Establish SSE stream for receiving responses
- **GET /health**: Health check endpoint

## Usage

### Starting the Server

```bash
# Start with HTTP Streams transport (default)
./mcp-server -addr=8080

# Start with debug logging
./mcp-server -addr=8080 -debug
```

### Client Implementation

#### 1. Initialize Connection

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialize",
    "id": 1,
    "params": {
      "protocolVersion": "2024-11-05",
      "capabilities": {},
      "clientInfo": {
        "name": "test-client",
        "version": "1.0.0"
      }
    }
  }'
```

Response includes session ID:
```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "capabilities": {"tools": {}},
    "protocolVersion": "2024-11-05",
    "serverInfo": {
      "name": "mcp-server-framework",
      "version": "1.0.0"
    },
    "sessionId": "abc123..."
  }
}
```

#### 2. Establish SSE Stream

```bash
curl -N -H "Accept: text/event-stream" \
     -H "Cache-Control: no-cache" \
     -H "Connection: keep-alive" \
     -H "Mcp-Session-Id: abc123..." \
     http://localhost:8080/mcp
```

#### 3. Send Initialized Notification

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: abc123..." \
  -d '{
    "jsonrpc": "2.0",
    "method": "initialized"
  }'
```

#### 4. List Available Tools

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: abc123..." \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/list",
    "id": 2,
    "params": {}
  }'
```

Response via SSE stream:
```
event: message
data: {"jsonrpc":"2.0","id":2,"result":{"tools":[...]}}
```

#### 5. Call a Tool

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: abc123..." \
  -d '{
    "jsonrpc": "2.0",
    "method": "tools/call",
    "id": 3,
    "params": {
      "name": "echo",
      "arguments": {
        "message": "Hello HTTP Streams!"
      }
    }
  }'
```

Response via SSE stream:
```
event: message
data: {"jsonrpc":"2.0","id":3,"result":{"content":[{"text":"Hello HTTP Streams!","type":"text"}]}}
```

## HTTP Status Codes

- **200 OK**: Successful request processing
- **202 Accepted**: Notification received (no response expected)
- **400 Bad Request**: Invalid request format or missing headers
- **409 Conflict**: Session conflicts or initialization errors
- **415 Unsupported Media Type**: Invalid Content-Type header
- **500 Internal Server Error**: Server processing errors

## Session Management

### Session ID Generation

Session IDs are automatically generated using cryptographically secure random bytes:

```go
func generateSessionID() string {
    bytes := make([]byte, 16)
    if _, err := rand.Read(bytes); err != nil {
        // Fallback to timestamp-based ID
        return fmt.Sprintf("session_%d", time.Now().UnixNano())
    }
    return hex.EncodeToString(bytes)
}
```

### Session Validation

All requests (except `initialize`) must include the `Mcp-Session-Id` header with a valid session ID.

## Error Handling

### Request Errors

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32601,
    "message": "Method not found",
    "data": "unknown/method"
  }
}
```

### Tool Errors

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "error": {
    "code": -32000,
    "message": "Tool not found",
    "data": "nonexistent_tool"
  }
}
```

## Batch Requests

HTTP Streams supports batch requests for improved efficiency:

```bash
curl -X POST http://localhost:8080/mcp \
  -H "Content-Type: application/json" \
  -H "Accept: application/json, text/event-stream" \
  -H "Mcp-Session-Id: abc123..." \
  -d '[
    {
      "jsonrpc": "2.0",
      "method": "tools/list",
      "id": 1,
      "params": {}
    },
    {
      "jsonrpc": "2.0",
      "method": "tools/call",
      "id": 2,
      "params": {
        "name": "echo",
        "arguments": {"message": "Hello"}
      }
    }
  ]'
```

Responses are sent individually via SSE stream with matching request IDs.

## Debug Logging

Enable debug logging to monitor HTTP Streams transport activity:

```bash
./mcp-server -debug
```

Debug output includes:
- Request/response processing
- Session management
- SSE stream lifecycle
- Message routing
- Error conditions

## Integration Testing

Use the provided Python integration test script:

```bash
python3 scripts/test_http_streams.py
```

This script tests:
- Connection initialization
- SSE stream establishment
- Tool listing and calling
- Error handling
- Session management

## Comparison with SSE Transport

| Feature | HTTP Streams | SSE Transport |
|---------|-------------|---------------|
| MCP Compliance | ✅ Full (2024-11-05) | ⚠️ Custom implementation |
| Session Management | ✅ Built-in | ⚠️ Optional |
| Request Routing | ✅ ID-based mapping | ⚠️ Simple forwarding |
| Batch Requests | ✅ Supported | ❌ Not supported |
| Standards Compliance | ✅ HTTP + SSE | ⚠️ Custom SSE |
| Proxy Compatibility | ✅ Excellent | ⚠️ Limited |

## Best Practices

1. **Always establish SSE stream before sending requests** (except `initialize`)
2. **Include session ID in all requests** after initialization
3. **Handle SSE reconnection** for robust client implementations
4. **Use batch requests** for multiple operations to reduce overhead
5. **Implement proper error handling** for all response types
6. **Monitor debug logs** for troubleshooting connection issues

## Troubleshooting

### Common Issues

1. **Session ID missing**: Ensure `Mcp-Session-Id` header is included
2. **SSE stream not established**: Check Accept headers and connection
3. **Responses not received**: Verify SSE stream is active before sending requests
4. **Connection timeouts**: Implement SSE reconnection logic in clients

### Debug Commands

```bash
# Check server health
curl http://localhost:8080/health

# Test SSE stream manually
curl -N -H "Accept: text/event-stream" http://localhost:8080/mcp

# Monitor server logs
./mcp-server -debug 2>&1 | grep "HTTP-STREAMS"
```

## Performance Considerations

- **Keep SSE streams alive** to avoid reconnection overhead
- **Use batch requests** for multiple operations
- **Implement connection pooling** for high-throughput scenarios
- **Monitor memory usage** for long-lived SSE connections
- **Consider request timeouts** for client implementations

## Security Considerations

- **Session IDs are cryptographically secure** (16 random bytes)
- **No authentication implemented** - add authentication layer as needed
- **CORS headers included** for web browser compatibility
- **Input validation** performed on all requests
- **Error messages sanitized** to prevent information leakage