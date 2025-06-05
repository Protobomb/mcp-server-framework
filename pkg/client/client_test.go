package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/openhands/mcp-server-framework/pkg/mcp"
)

// MockTransport implements Transport for testing
type MockTransport struct {
	sendCalls    []mcp.JSONRPCRequest
	responses    chan mcp.JSONRPCResponse
	closed       bool
	mu           sync.RWMutex
	sendError    error
	receiveError error
	autoRespond  bool
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		responses: make(chan mcp.JSONRPCResponse, 10),
	}
}

func (m *MockTransport) Send(request *mcp.JSONRPCRequest) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	
	if m.sendError != nil {
		return m.sendError
	}
	
	if m.closed {
		return fmt.Errorf("transport closed")
	}
	
	m.sendCalls = append(m.sendCalls, *request)
	
	// Auto-respond if enabled
	if m.autoRespond {
		go func() {
			// Extract call ID from request params if it exists
			var callID int
			if request.Params != nil {
				var params map[string]interface{}
				if err := json.Unmarshal(request.Params, &params); err == nil {
					if id, ok := params["id"].(float64); ok {
						callID = int(id)
					}
				}
			}
			
			response := mcp.JSONRPCResponse{
				JSONRPC: "2.0",
				ID:      request.ID,
				Result:  json.RawMessage(fmt.Sprintf(`{"call": %d}`, callID)),
			}
			
			m.mu.RLock()
			if !m.closed {
				select {
				case m.responses <- response:
				default:
				}
			}
			m.mu.RUnlock()
		}()
	}
	
	return nil
}

func (m *MockTransport) Receive() (*mcp.JSONRPCResponse, error) {
	m.mu.RLock()
	if m.receiveError != nil {
		m.mu.RUnlock()
		return nil, m.receiveError
	}
	if m.closed {
		m.mu.RUnlock()
		return nil, io.EOF
	}
	m.mu.RUnlock()

	select {
	case resp := <-m.responses:
		return &resp, nil
	case <-time.After(100 * time.Millisecond):
		return nil, fmt.Errorf("timeout")
	}
}

func (m *MockTransport) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.closed = true
	close(m.responses)
	return nil
}

func (m *MockTransport) AddResponse(resp mcp.JSONRPCResponse) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if !m.closed {
		select {
		case m.responses <- resp:
		default:
		}
	}
}

func (m *MockTransport) SetAutoRespond(enabled bool) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.autoRespond = enabled
}

func (m *MockTransport) GetSendCalls() []mcp.JSONRPCRequest {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return append([]mcp.JSONRPCRequest{}, m.sendCalls...)
}

func (m *MockTransport) SetSendError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.sendError = err
}

func (m *MockTransport) SetReceiveError(err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.receiveError = err
}

func TestNewClient(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)
	
	if client == nil {
		t.Fatal("NewClient returned nil")
	}
	
	if client.transport != transport {
		t.Error("Client transport not set correctly")
	}
	
	if client.pending == nil {
		t.Error("Client pending map not initialized")
	}
}

func TestClientCall(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	
	client.Start(ctx)
	defer client.Close()
	
	// Add expected response
	expectedResult := json.RawMessage(`{"test": "result"}`)
	transport.AddResponse(mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      0,
		Result:  expectedResult,
	})
	
	resp, err := client.Call(ctx, "test/method", map[string]string{"param": "value"})
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	
	if resp.ID != 0 {
		t.Errorf("Expected ID 0, got %v", resp.ID)
	}
	
	if string(resp.Result.(json.RawMessage)) != string(expectedResult) {
		t.Errorf("Expected result %s, got %s", expectedResult, resp.Result)
	}
	
	// Check that request was sent
	calls := transport.GetSendCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 send call, got %d", len(calls))
	}
	
	if calls[0].Method != "test/method" {
		t.Errorf("Expected method 'test/method', got '%s'", calls[0].Method)
	}
}

func TestClientCallError(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	
	client.Start(ctx)
	defer client.Close()
	
	// Add error response
	transport.AddResponse(mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      0,
		Error: &mcp.RPCError{
			Code:    -32601,
			Message: "Method not found",
		},
	})
	
	resp, err := client.Call(ctx, "unknown/method", nil)
	if err != nil {
		t.Fatalf("Call failed: %v", err)
	}
	
	if resp.Error == nil {
		t.Error("Expected error response, got nil")
	}
	
	if resp.Error.Message != "Method not found" {
		t.Errorf("Expected error message 'Method not found', got '%s'", resp.Error.Message)
	}
}

func TestClientCallTimeout(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)
	
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	
	client.Start(ctx)
	defer client.Close()
	
	// Don't add any response - should timeout
	_, err := client.Call(ctx, "test/method", nil)
	if err == nil {
		t.Error("Expected timeout error, got nil")
	}
	
	if err != context.DeadlineExceeded {
		t.Errorf("Expected context.DeadlineExceeded, got %v", err)
	}
}

func TestClientNotify(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)
	defer client.Close()
	
	err := client.Notify("test/notification", map[string]string{"param": "value"})
	if err != nil {
		t.Fatalf("Notify failed: %v", err)
	}
	
	calls := transport.GetSendCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 send call, got %d", len(calls))
	}
	
	if calls[0].ID != nil {
		t.Error("Expected notification to have nil ID")
	}
	
	if calls[0].Method != "test/notification" {
		t.Errorf("Expected method 'test/notification', got '%s'", calls[0].Method)
	}
}

func TestClientInitialize(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	
	client.Start(ctx)
	defer client.Close()
	
	// Add expected response
	result := mcp.InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: mcp.ServerCapabilities{
			Tools: &mcp.ToolsCapability{},
		},
		ServerInfo: mcp.ServerInfo{
			Name:    "test-server",
			Version: "1.0.0",
		},
	}
	resultBytes, _ := json.Marshal(result)
	
	transport.AddResponse(mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      0,
		Result:  json.RawMessage(resultBytes),
	})
	
	clientInfo := mcp.ServerInfo{
		Name:    "test-client",
		Version: "1.0.0",
	}
	
	initResult, err := client.Initialize(ctx, clientInfo)
	if err != nil {
		t.Fatalf("Initialize failed: %v", err)
	}
	
	if initResult.ServerInfo.Name != "test-server" {
		t.Errorf("Expected server name 'test-server', got '%s'", initResult.ServerInfo.Name)
	}
	
	// Check that initialize request was sent
	calls := transport.GetSendCalls()
	if len(calls) != 1 {
		t.Fatalf("Expected 1 send call, got %d", len(calls))
	}
	
	if calls[0].Method != "initialize" {
		t.Errorf("Expected method 'initialize', got '%s'", calls[0].Method)
	}
}

func TestClientListTools(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	
	client.Start(ctx)
	defer client.Close()
	
	// Add expected response
	tools := []mcp.Tool{
		{
			Name:        "test-tool",
			Description: "A test tool",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"input": map[string]interface{}{
						"type": "string",
					},
				},
			},
		},
	}
	result := struct {
		Tools []mcp.Tool `json:"tools"`
	}{Tools: tools}
	resultBytes, _ := json.Marshal(result)
	
	transport.AddResponse(mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      0,
		Result:  json.RawMessage(resultBytes),
	})
	
	toolsList, err := client.ListTools(ctx)
	if err != nil {
		t.Fatalf("ListTools failed: %v", err)
	}
	
	if len(toolsList) != 1 {
		t.Fatalf("Expected 1 tool, got %d", len(toolsList))
	}
	
	if toolsList[0].Name != "test-tool" {
		t.Errorf("Expected tool name 'test-tool', got '%s'", toolsList[0].Name)
	}
}

func TestClientCallTool(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	
	client.Start(ctx)
	defer client.Close()
	
	// Add expected response
	content := []mcp.Content{
		{
			Type: "text",
			Text: "Tool result",
		},
	}
	result := mcp.CallToolResult{Content: content}
	resultBytes, _ := json.Marshal(result)
	
	transport.AddResponse(mcp.JSONRPCResponse{
		JSONRPC: "2.0",
		ID:      0,
		Result:  json.RawMessage(resultBytes),
	})
	
	args := map[string]interface{}{"input": "test"}
	toolResult, err := client.CallTool(ctx, "test-tool", args)
	if err != nil {
		t.Fatalf("CallTool failed: %v", err)
	}
	
	if len(toolResult) != 1 {
		t.Fatalf("Expected 1 content item, got %d", len(toolResult))
	}
	
	if toolResult[0].Text != "Tool result" {
		t.Errorf("Expected text 'Tool result', got '%s'", toolResult[0].Text)
	}
}

func TestClientClose(t *testing.T) {
	transport := NewMockTransport()
	client := NewClient(transport)
	
	err := client.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	
	// Test that operations fail after close
	err = client.Notify("test", nil)
	if err == nil {
		t.Error("Expected error after close, got nil")
	}
	
	ctx := context.Background()
	_, err = client.Call(ctx, "test", nil)
	if err == nil {
		t.Error("Expected error after close, got nil")
	}
}

func TestSTDIOTransport(t *testing.T) {
	var buf bytes.Buffer
	reader := strings.NewReader(`{"jsonrpc":"2.0","id":1,"result":{"test":"value"}}` + "\n")
	
	transport := NewSTDIOTransport(reader, &buf)
	
	// Test Send
	params, _ := json.Marshal(map[string]string{"key": "value"})
	request := &mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      1,
		Method:  "test",
		Params:  params,
	}
	
	err := transport.Send(request)
	if err != nil {
		t.Fatalf("Send failed: %v", err)
	}
	
	// Check that request was written
	written := buf.String()
	if !strings.Contains(written, `"method":"test"`) {
		t.Error("Request not written correctly")
	}
	
	// Test Receive
	response, err := transport.Receive()
	if err != nil {
		t.Fatalf("Receive failed: %v", err)
	}
	
	if response.ID != float64(1) { // JSON unmarshals numbers as float64
		t.Errorf("Expected ID 1, got %v (type %T)", response.ID, response.ID)
	}
	
	// Test Close
	err = transport.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
}

func TestSTDIOTransportEOF(t *testing.T) {
	reader := strings.NewReader("")
	var buf bytes.Buffer
	
	transport := NewSTDIOTransport(reader, &buf)
	
	_, err := transport.Receive()
	if err != io.EOF {
		t.Errorf("Expected EOF, got %v", err)
	}
}

func TestHTTPTransport(t *testing.T) {
	transport := NewHTTPTransport("http://localhost:8080")
	
	if transport.baseURL != "http://localhost:8080" {
		t.Errorf("Expected baseURL 'http://localhost:8080', got '%s'", transport.baseURL)
	}
	
	// Test that baseURL trailing slash is removed
	transport2 := NewHTTPTransport("http://localhost:8080/")
	if transport2.baseURL != "http://localhost:8080" {
		t.Errorf("Expected baseURL 'http://localhost:8080', got '%s'", transport2.baseURL)
	}
	
	// Test Close
	err := transport.Close()
	if err != nil {
		t.Fatalf("Close failed: %v", err)
	}
	
	// Test Receive (should return error)
	_, err = transport.Receive()
	if err == nil {
		t.Error("Expected error from HTTP Receive, got nil")
	}
}

func TestClientSendError(t *testing.T) {
	transport := NewMockTransport()
	transport.SetSendError(fmt.Errorf("send error"))
	
	client := NewClient(transport)
	defer client.Close()
	
	ctx := context.Background()
	_, err := client.Call(ctx, "test", nil)
	if err == nil {
		t.Error("Expected send error, got nil")
	}
	
	if !strings.Contains(err.Error(), "failed to send request") {
		t.Errorf("Expected 'failed to send request' error, got: %v", err)
	}
}

func TestClientMultipleCalls(t *testing.T) {
	transport := NewMockTransport()
	transport.SetAutoRespond(true)
	client := NewClient(transport)
	
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	
	client.Start(ctx)
	defer client.Close()
	
	// Make multiple concurrent calls
	var wg sync.WaitGroup
	results := make([]string, 3)
	
	for i := 0; i < 3; i++ {
		wg.Add(1)
		go func(callID int) {
			defer wg.Done()
			resp, err := client.Call(ctx, "test", map[string]int{"id": callID})
			if err != nil {
				t.Errorf("Call %d failed: %v", callID, err)
				return
			}
			results[callID] = string(resp.Result.(json.RawMessage))
		}(i)
	}
	
	wg.Wait()
	
	// Verify all calls completed
	for i, result := range results {
		expected := fmt.Sprintf(`{"call": %d}`, i)
		if result != expected {
			t.Errorf("Call %d: expected %s, got %s", i, expected, result)
		}
	}
}