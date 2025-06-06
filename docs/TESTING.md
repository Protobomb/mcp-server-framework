# Testing Guide

## Overview

The MCP Server Framework includes comprehensive test coverage for all components, ensuring reliability and correctness of the implementation.

## Test Structure

```
pkg/
├── client/
│   └── client_test.go      # Client functionality tests
├── mcp/
│   └── server_test.go      # MCP server and protocol tests
└── transport/
    └── sse_test.go         # SSE transport tests
    └── stdio_test.go       # STDIO transport tests
```

## Running Tests

### Quick Testing with Makefile

```bash
# Run all tests (unit + integration)
make test-all

# Run only unit tests
make test

# Run only integration tests
make test-integration

# Run tests with coverage report
make test-coverage

# Run all checks (format, lint, test)
make check
```

### Manual Testing

```bash
# Run all unit tests
go test ./pkg/... -v

# Run with race detection
go test ./pkg/... -race

# Run with coverage
go test ./pkg/... -cover

# Generate detailed coverage report
go test ./pkg/... -coverprofile=coverage.out
go tool cover -html=coverage.out
```

### Specific Package Tests
```bash
# Test only transport layer
go test ./pkg/transport/... -v

# Test only MCP server
go test ./pkg/mcp/... -v

# Test only client
go test ./pkg/client/... -v
```

### Integration Testing

Integration tests verify the complete MCP protocol flow:

```bash
# Run integration tests (requires Python 3 and requests library)
make test-integration

# Manual integration testing
./mcp-server -transport=sse -addr=8081 &
python3 scripts/test_sse_integration.py 8081
kill %1  # Stop the server
```

## Test Coverage

### Transport Layer Tests

#### SSE Transport (`sse_test.go`)

**Basic Functionality:**
- `TestNewSSETransport` - Transport creation
- `TestSSETransportHealthHandler` - Health endpoint
- `TestSSETransportSend` - Message sending
- `TestSSETransportClose` - Transport cleanup
- `TestSSETransportCORS` - CORS headers

**Message Handling:**
- `TestSSETransportMessageHandler` - Basic message processing
- `TestSSETransportMessageHandlerInvalidMethod` - Invalid HTTP methods
- `TestSSETransportMessageHandlerInvalidJSON` - Malformed JSON handling
- `TestSSETransportMessageHandlerWithCallback` - Callback functionality

**Session Management:**
- `TestSSETransportSessionManagement` - Session creation and cleanup
- `TestSSETransportSSEHandler` - SSE endpoint functionality
- `TestSSETransportReceive` - Message receiving

**Protocol Integration:**
- `TestSSETransportMCPProtocolIntegration` - Full MCP protocol flow
- `TestSSETransportErrorHandling` - Error scenarios

#### STDIO Transport (`stdio_test.go`)

**Basic Functionality:**
- `TestNewSTDIOTransport` - Transport creation
- `TestNewSTDIOTransportWithIO` - Custom IO configuration
- `TestSTDIOTransportStart` - Transport startup
- `TestSTDIOTransportSend` - Message sending
- `TestSTDIOTransportReceive` - Message receiving
- `TestSTDIOTransportStop` - Transport shutdown
- `TestSTDIOTransportClose` - Transport cleanup

**Error Handling:**
- `TestSTDIOTransportSendAfterClose` - Send after close
- `TestSTDIOTransportStartAfterClose` - Start after close

### MCP Server Tests (`server_test.go`)

**Core Functionality:**
- `TestNewServer` - Server creation
- `TestServerRegisterHandler` - Handler registration
- `TestServerRegisterNotificationHandler` - Notification handler registration
- `TestServerStart` - Server startup
- `TestServerGetHandler` - Handler retrieval

**Protocol Implementation:**
- `TestServerHandleInitialize` - MCP initialization
- `TestServerHandleUnknownMethod` - Unknown method handling
- `TestServerSendNotification` - Notification sending

**Tools Functionality:**
- `TestServerToolsListHandler` - Tools listing
- `TestServerToolsCallHandler` - Tool execution
- `TestServerToolsCallInvalidTool` - Invalid tool handling
- `TestServerInitializeCapabilities` - Capability negotiation

**Error Handling:**
- `TestRPCError` - RPC error structure
- `TestNewRPCErrorFunctions` - Error creation functions

### Client Tests (`client_test.go`)

**Core Functionality:**
- `TestNewClient` - Client creation
- `TestClientCall` - Method calls
- `TestClientCallError` - Error handling
- `TestClientCallTimeout` - Timeout handling
- `TestClientNotify` - Notifications
- `TestClientClose` - Client cleanup

**MCP Protocol:**
- `TestClientInitialize` - MCP initialization
- `TestClientListTools` - Tools listing
- `TestClientCallTool` - Tool execution

**Transport Integration:**
- `TestSTDIOTransport` - STDIO transport
- `TestSTDIOTransportEOF` - EOF handling
- `TestHTTPTransport` - HTTP transport

**Advanced Features:**
- `TestClientSendError` - Send error handling
- `TestClientMultipleCalls` - Concurrent calls

## Test Scenarios

### Happy Path Testing

1. **Server Startup and Initialization**
   - Transport creation
   - Server startup
   - MCP initialization handshake
   - Capability negotiation

2. **Tools Workflow**
   - List available tools
   - Call tools with valid parameters
   - Receive proper responses

3. **Client-Server Communication**
   - Establish connection
   - Send requests
   - Receive responses
   - Handle notifications

### Error Handling Testing

1. **Transport Errors**
   - Invalid JSON messages
   - Network connection failures
   - Session management errors
   - CORS preflight handling

2. **Protocol Errors**
   - Unknown methods
   - Invalid parameters
   - Missing required fields
   - Type mismatches

3. **Tool Errors**
   - Unknown tool names
   - Invalid tool parameters
   - Tool execution failures

### Edge Cases

1. **Concurrent Operations**
   - Multiple simultaneous requests
   - Session cleanup during active connections
   - Server shutdown during active requests

2. **Resource Management**
   - Memory cleanup
   - Connection cleanup
   - Channel cleanup

3. **Timeout Scenarios**
   - Request timeouts
   - Connection timeouts
   - Server shutdown timeouts

## Mock Objects and Test Utilities

### Mock Transport
```go
type MockTransport struct {
    messages [][]byte
    closed   bool
}

func (m *MockTransport) Start(ctx context.Context) error { return nil }
func (m *MockTransport) Send(data []byte) error { 
    m.messages = append(m.messages, data)
    return nil 
}
// ... other methods
```

### Test Helpers
```go
// Create test server with mock transport
func createTestServer() (*mcp.Server, *MockTransport) {
    transport := &MockTransport{}
    server := mcp.NewServer(transport)
    return server, transport
}

// Create test JSON-RPC request
func createTestRequest(method string, params interface{}) mcp.JSONRPCRequest {
    paramsBytes, _ := json.Marshal(params)
    return mcp.JSONRPCRequest{
        JSONRPC: "2.0",
        ID:      1,
        Method:  method,
        Params:  json.RawMessage(paramsBytes),
    }
}
```

## Integration Testing

### SSE Integration Test
```go
func TestSSEIntegration(t *testing.T) {
    // Start real HTTP server
    transport := transport.NewSSETransport(":0")
    server := mcp.NewServer(transport)
    
    ctx := context.Background()
    if err := server.Start(ctx); err != nil {
        t.Fatal(err)
    }
    defer server.Close()
    
    // Test with real HTTP client
    // ... test implementation
}
```

### STDIO Integration Test
```go
func TestSTDIOIntegration(t *testing.T) {
    // Create pipes for communication
    r1, w1 := io.Pipe()
    r2, w2 := io.Pipe()
    
    // Server transport
    serverTransport := transport.NewSTDIOTransportWithIO(r1, w2)
    server := mcp.NewServer(serverTransport)
    
    // Client transport  
    clientTransport := transport.NewSTDIOTransportWithIO(r2, w1)
    client := client.NewClient(clientTransport)
    
    // Test full communication flow
    // ... test implementation
}
```

## Performance Testing

### Concurrent Connections
```go
func TestConcurrentConnections(t *testing.T) {
    const numClients = 100
    
    // Start server
    transport := transport.NewSSETransport(":0")
    server := mcp.NewServer(transport)
    server.Start(context.Background())
    defer server.Close()
    
    // Create multiple clients
    var wg sync.WaitGroup
    for i := 0; i < numClients; i++ {
        wg.Add(1)
        go func() {
            defer wg.Done()
            // Test client operations
        }()
    }
    wg.Wait()
}
```

### Memory Usage
```go
func TestMemoryUsage(t *testing.T) {
    var m1, m2 runtime.MemStats
    runtime.GC()
    runtime.ReadMemStats(&m1)
    
    // Perform operations
    // ...
    
    runtime.GC()
    runtime.ReadMemStats(&m2)
    
    // Check memory usage
    if m2.Alloc > m1.Alloc*2 {
        t.Errorf("Memory usage increased significantly")
    }
}
```

## Test Data

### Sample JSON-RPC Messages
```go
var testMessages = map[string]string{
    "initialize": `{
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2024-11-05",
            "clientInfo": {
                "name": "test-client",
                "version": "1.0.0"
            }
        }
    }`,
    
    "tools/list": `{
        "jsonrpc": "2.0",
        "id": 2,
        "method": "tools/list",
        "params": {}
    }`,
    
    "tools/call": `{
        "jsonrpc": "2.0",
        "id": 3,
        "method": "tools/call",
        "params": {
            "name": "echo",
            "arguments": {
                "message": "test message"
            }
        }
    }`
}
```

## Continuous Integration

### GitHub Actions Workflow

The project includes a comprehensive CI/CD pipeline that runs:

1. **Unit Tests**: All 42 unit tests with race detection
2. **Integration Tests**: Complete MCP protocol flow testing
3. **Linting**: golangci-lint with multiple linters
4. **Coverage**: Code coverage reporting with Codecov
5. **Multi-platform Builds**: Linux, macOS, Windows (amd64, arm64)
6. **Docker Images**: Automated container builds

```yaml
name: CI/CD Pipeline
on:
  push:
    branches: [ main, develop ]
  pull_request:
    branches: [ main ]

jobs:
  test:
    runs-on: ubuntu-latest
    steps:
    - uses: actions/checkout@v4
    - uses: actions/setup-go@v4
      with:
        go-version: '1.21'
    - uses: actions/setup-python@v4
      with:
        python-version: '3.x'
    - run: pip install requests
    - run: make deps
    - run: make test           # Unit tests
    - run: make test-integration  # Integration tests
    - run: make test-coverage  # Coverage report
```

### Test Coverage Requirements
- **Current Coverage**: 42 comprehensive test cases
- **Unit Tests**: All transport, server, and client functionality
- **Integration Tests**: Complete MCP protocol workflows
- **Error Handling**: All error paths and edge cases tested
- **Performance**: Race detection and concurrent testing

## Debugging Tests

### Verbose Output
```bash
# Run with verbose output
go test ./... -v

# Run specific test
go test ./pkg/transport -run TestSSETransport -v

# Run with race detection
go test ./... -race
```

### Test Debugging
```go
func TestDebugExample(t *testing.T) {
    // Enable debug logging
    log.SetLevel(log.DebugLevel)
    
    // Use t.Logf for test-specific logging
    t.Logf("Starting test with parameter: %v", param)
    
    // Use testify for better assertions
    assert.Equal(t, expected, actual, "Values should be equal")
    require.NoError(t, err, "Should not return error")
}
```

## Best Practices

1. **Test Isolation**: Each test should be independent
2. **Resource Cleanup**: Always clean up resources in defer statements
3. **Error Testing**: Test both success and failure cases
4. **Realistic Data**: Use realistic test data and scenarios
5. **Performance**: Include performance tests for critical paths
6. **Documentation**: Document complex test scenarios
7. **Maintainability**: Keep tests simple and readable

## Running Specific Test Suites

```bash
# Run only SSE transport tests
go test ./pkg/transport -run TestSSE

# Run only error handling tests
go test ./... -run Error

# Run only integration tests
go test ./... -run Integration

# Run tests with timeout
go test ./... -timeout 30s

# Run tests with custom flags
go test ./... -short  # Skip long-running tests
```