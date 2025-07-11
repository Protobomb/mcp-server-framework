package mcp

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

// MockTransport is a mock implementation of Transport for testing
type MockTransport struct {
	sendChan    chan []byte
	receiveChan chan []byte
	started     bool
	stopped     bool
	closed      bool
}

func NewMockTransport() *MockTransport {
	return &MockTransport{
		sendChan:    make(chan []byte, 100),
		receiveChan: make(chan []byte, 100),
	}
}

func (m *MockTransport) Start(ctx context.Context) error {
	m.started = true
	return nil
}

func (m *MockTransport) Stop() error {
	m.stopped = true
	return nil
}

func (m *MockTransport) Send(message []byte) error {
	m.sendChan <- message
	return nil
}

func (m *MockTransport) Receive() <-chan []byte {
	return m.receiveChan
}

func (m *MockTransport) Close() error {
	m.closed = true
	close(m.sendChan)
	close(m.receiveChan)
	return nil
}

func (m *MockTransport) SendMessage(message []byte) {
	m.receiveChan <- message
}

func (m *MockTransport) GetSentMessage() []byte {
	select {
	case msg := <-m.sendChan:
		return msg
	case <-time.After(100 * time.Millisecond):
		return nil
	}
}

func TestNewServer(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	if server == nil {
		t.Fatal("NewServer returned nil")
	}

	if server.transport != transport {
		t.Error("Server transport not set correctly")
	}

	if len(server.handlers) != 0 {
		t.Error("Server should start with no handlers")
	}

	if len(server.notifications) != 0 {
		t.Error("Server should start with no notification handlers")
	}
}

func TestServerRegisterHandler(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	handler := func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return "test", nil
	}

	server.RegisterHandler("test", handler)

	if len(server.handlers) != 1 {
		t.Error("Handler not registered")
	}

	if _, exists := server.handlers["test"]; !exists {
		t.Error("Handler not found with correct key")
	}
}

func TestServerRegisterNotificationHandler(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	handler := func(ctx context.Context, params json.RawMessage) error {
		return nil
	}

	server.RegisterNotificationHandler("test", handler)

	if len(server.notifications) != 1 {
		t.Error("Notification handler not registered")
	}

	if _, exists := server.notifications["test"]; !exists {
		t.Error("Notification handler not found with correct key")
	}
}

func TestServerStart(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	if !transport.started {
		t.Error("Transport not started")
	}

	// Check that default handlers are registered
	if _, exists := server.handlers["initialize"]; !exists {
		t.Error("Initialize handler not registered")
	}

	if _, exists := server.notifications["notifications/initialized"]; !exists {
		t.Error("Initialized notification handler not registered")
	}
}

func TestServerHandleInitialize(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Send initialize request
	request := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
	}

	requestBytes, _ := json.Marshal(request)
	transport.SendMessage(requestBytes)

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check response
	responseBytes := transport.GetSentMessage()
	if responseBytes == nil {
		t.Fatal("No response received")
	}

	var response JSONRPCResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.ID != float64(1) { // JSON unmarshaling converts numbers to float64
		t.Errorf("Response ID mismatch: expected 1, got %v (type %T)", response.ID, response.ID)
	}

	if response.Error != nil {
		t.Errorf("Unexpected error in response: %v", response.Error)
	}

	if response.Result == nil {
		t.Error("No result in response")
	}
}

func TestServerHandleUnknownMethod(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Send request for unknown method
	request := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  "unknownMethod",
	}

	requestBytes, _ := json.Marshal(request)
	transport.SendMessage(requestBytes)

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check response
	responseBytes := transport.GetSentMessage()
	if responseBytes == nil {
		t.Fatal("No response received")
	}

	var response JSONRPCResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error == nil {
		t.Error("Expected error for unknown method")
	}

	if response.Error.Code != MethodNotFound {
		t.Errorf("Expected MethodNotFound error, got %d", response.Error.Code)
	}
}

func TestServerSendNotification(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Send notification
	params := map[string]string{"message": "test"}
	err = server.SendNotification("test", params)
	if err != nil {
		t.Fatalf("Failed to send notification: %v", err)
	}

	// Check sent message
	messageBytes := transport.GetSentMessage()
	if messageBytes == nil {
		t.Fatal("No message sent")
	}

	var notification JSONRPCNotification
	err = json.Unmarshal(messageBytes, &notification)
	if err != nil {
		t.Fatalf("Failed to unmarshal notification: %v", err)
	}

	if notification.Method != "test" {
		t.Errorf("Expected method 'test', got '%s'", notification.Method)
	}

	if notification.JSONRPC != JSONRPCVersion {
		t.Errorf("Expected JSONRPC version '%s', got '%s'", JSONRPCVersion, notification.JSONRPC)
	}
}

func TestRPCError(t *testing.T) {
	err := &RPCError{
		Code:    InvalidParams,
		Message: "Invalid parameters",
		Data:    "test data",
	}

	if err.Error() != "Invalid parameters" {
		t.Errorf("Expected error message 'Invalid parameters', got '%s'", err.Error())
	}
}

func TestNewRPCErrorFunctions(t *testing.T) {
	tests := []struct {
		name     string
		fn       func() *RPCError
		expected int
	}{
		{"ParseError", func() *RPCError { return NewParseError("test") }, ParseError},
		{"InvalidRequest", func() *RPCError { return NewInvalidRequestError("test") }, InvalidRequest},
		{"MethodNotFound", func() *RPCError { return NewMethodNotFoundError("test") }, MethodNotFound},
		{"InvalidParams", func() *RPCError { return NewInvalidParamsError("test") }, InvalidParams},
		{"InternalError", func() *RPCError { return NewInternalError("test") }, InternalError},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.fn()
			if err.Code != tt.expected {
				t.Errorf("Expected code %d, got %d", tt.expected, err.Code)
			}
		})
	}
}

func TestServerGetHandler(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	// Register a test handler
	testHandler := func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return "test result", nil
	}
	server.RegisterHandler("test", testHandler)

	// Test getting existing handler
	handler := server.GetHandler("test")
	if handler == nil {
		t.Error("GetHandler returned nil for existing handler")
	}

	// Test getting non-existent handler
	handler = server.GetHandler("nonexistent")
	if handler != nil {
		t.Error("GetHandler should return nil for non-existent handler")
	}
}

func TestServerToolsListHandler(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Send tools/list request
	request := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  "tools/list",
	}

	requestBytes, _ := json.Marshal(request)
	transport.SendMessage(requestBytes)

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check response
	responseBytes := transport.GetSentMessage()
	if responseBytes == nil {
		t.Fatal("No response received")
	}

	var response JSONRPCResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("Unexpected error in response: %v", response.Error)
	}

	if response.Result == nil {
		t.Error("No result in response")
	}

	// Check that result contains tools
	resultMap, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Error("Result is not a map")
	}

	tools, exists := resultMap["tools"]
	if !exists {
		t.Error("Result does not contain 'tools' field")
	}

	toolsArray, ok := tools.([]interface{})
	if !ok {
		t.Error("Tools field is not an array")
	}

	// Should have at least the echo tool
	if len(toolsArray) == 0 {
		t.Error("No tools returned")
	}
}

func TestServerToolsCallHandler(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Send tools/call request for echo tool
	request := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"echo","arguments":{"message":"test message"}}`),
	}

	requestBytes, _ := json.Marshal(request)
	transport.SendMessage(requestBytes)

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check response
	responseBytes := transport.GetSentMessage()
	if responseBytes == nil {
		t.Fatal("No response received")
	}

	var response JSONRPCResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("Unexpected error in response: %v", response.Error)
	}

	if response.Result == nil {
		t.Error("No result in response")
	}

	// Check that result contains the echoed message
	resultMap, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Error("Result is not a map")
	}

	content, exists := resultMap["content"]
	if !exists {
		t.Error("Result does not contain 'content' field")
	}

	contentArray, ok := content.([]interface{})
	if !ok {
		t.Error("Content field is not an array")
	}

	if len(contentArray) == 0 {
		t.Error("No content returned")
	}

	firstContent, ok := contentArray[0].(map[string]interface{})
	if !ok {
		t.Error("First content item is not a map")
	}

	text, exists := firstContent["text"]
	if !exists {
		t.Error("Content does not contain 'text' field")
	}

	if text != "Echo: test message" {
		t.Errorf("Expected 'Echo: test message', got '%v'", text)
	}
}

func TestServerToolsCallInvalidTool(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Send tools/call request for non-existent tool
	request := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  "tools/call",
		Params:  json.RawMessage(`{"name":"nonexistent","arguments":{}}`),
	}

	requestBytes, _ := json.Marshal(request)
	transport.SendMessage(requestBytes)

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check response
	responseBytes := transport.GetSentMessage()
	if responseBytes == nil {
		t.Fatal("No response received")
	}

	var response JSONRPCResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error == nil {
		t.Error("Expected error for non-existent tool")
	}

	if response.Error.Code != InvalidParams {
		t.Errorf("Expected InvalidParams error, got %d", response.Error.Code)
	}
}

func TestServerInitializeCapabilities(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Send initialize request
	request := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  "initialize",
		Params:  json.RawMessage(`{"protocolVersion":"2024-11-05"}`),
	}

	requestBytes, _ := json.Marshal(request)
	transport.SendMessage(requestBytes)

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check response
	responseBytes := transport.GetSentMessage()
	if responseBytes == nil {
		t.Fatal("No response received")
	}

	var response JSONRPCResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("Unexpected error in response: %v", response.Error)
	}

	// Check capabilities
	resultMap, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Error("Result is not a map")
	}

	capabilities, exists := resultMap["capabilities"]
	if !exists {
		t.Error("Result does not contain 'capabilities' field")
	}

	capMap, ok := capabilities.(map[string]interface{})
	if !ok {
		t.Error("Capabilities field is not a map")
	}

	// Check tools capability
	tools, exists := capMap["tools"]
	if !exists {
		t.Error("Capabilities do not contain 'tools' field")
	}

	toolsMap, ok := tools.(map[string]interface{})
	if !ok {
		t.Error("Tools capability is not a map")
	}

	listChanged, exists := toolsMap["listChanged"]
	if !exists {
		t.Error("Tools capability does not contain 'listChanged' field")
	}

	if listChanged != true {
		t.Error("Tools listChanged should be true")
	}
}

func TestServerCustomHandlersPreserved(t *testing.T) {
	transport := NewMockTransport()
	server := NewServer(transport)

	// Register custom handlers before starting server
	customToolsListCalled := false
	customPromptsListCalled := false
	customResourcesListCalled := false

	customToolsListHandler := func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		customToolsListCalled = true
		return map[string]interface{}{
			"tools": []interface{}{
				map[string]interface{}{
					"name":        "custom_tool",
					"description": "A custom tool",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"message": map[string]interface{}{
								"type":        "string",
								"description": "Message to process",
							},
						},
						"required": []string{"message"},
					},
				},
			},
		}, nil
	}

	customPromptsListHandler := func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		customPromptsListCalled = true
		return map[string]interface{}{
			"prompts": []interface{}{},
		}, nil
	}

	customResourcesListHandler := func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		customResourcesListCalled = true
		return map[string]interface{}{
			"resources": []interface{}{},
		}, nil
	}

	server.RegisterHandler("tools/list", customToolsListHandler)
	server.RegisterHandler("prompts/list", customPromptsListHandler)
	server.RegisterHandler("resources/list", customResourcesListHandler)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := server.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start server: %v", err)
	}

	// Test tools/list uses custom handler
	request := JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      1,
		Method:  "tools/list",
	}

	requestBytes, _ := json.Marshal(request)
	transport.SendMessage(requestBytes)

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check response
	responseBytes := transport.GetSentMessage()
	if responseBytes == nil {
		t.Fatal("No response received for tools/list")
	}

	var response JSONRPCResponse
	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal tools/list response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("Unexpected error in tools/list response: %v", response.Error)
	}

	if !customToolsListCalled {
		t.Error("Custom tools/list handler was not called")
	}

	// Verify the custom tool is returned
	resultMap, ok := response.Result.(map[string]interface{})
	if !ok {
		t.Error("tools/list result is not a map")
	}

	tools, exists := resultMap["tools"]
	if !exists {
		t.Error("tools/list result does not contain 'tools' field")
	}

	toolsArray, ok := tools.([]interface{})
	if !ok {
		t.Error("tools field is not an array")
	}

	if len(toolsArray) != 1 {
		t.Errorf("Expected 1 custom tool, got %d", len(toolsArray))
	}

	tool, ok := toolsArray[0].(map[string]interface{})
	if !ok {
		t.Error("Tool is not a map")
	}

	if tool["name"] != "custom_tool" {
		t.Errorf("Expected custom_tool, got %v", tool["name"])
	}

	// Test prompts/list uses custom handler
	request = JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      2,
		Method:  "prompts/list",
	}

	requestBytes, _ = json.Marshal(request)
	transport.SendMessage(requestBytes)

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check response
	responseBytes = transport.GetSentMessage()
	if responseBytes == nil {
		t.Fatal("No response received for prompts/list")
	}

	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal prompts/list response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("Unexpected error in prompts/list response: %v", response.Error)
	}

	if !customPromptsListCalled {
		t.Error("Custom prompts/list handler was not called")
	}

	// Test resources/list uses custom handler
	request = JSONRPCRequest{
		JSONRPC: JSONRPCVersion,
		ID:      3,
		Method:  "resources/list",
	}

	requestBytes, _ = json.Marshal(request)
	transport.SendMessage(requestBytes)

	// Give some time for processing
	time.Sleep(10 * time.Millisecond)

	// Check response
	responseBytes = transport.GetSentMessage()
	if responseBytes == nil {
		t.Fatal("No response received for resources/list")
	}

	err = json.Unmarshal(responseBytes, &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal resources/list response: %v", err)
	}

	if response.Error != nil {
		t.Errorf("Unexpected error in resources/list response: %v", response.Error)
	}

	if !customResourcesListCalled {
		t.Error("Custom resources/list handler was not called")
	}
}
