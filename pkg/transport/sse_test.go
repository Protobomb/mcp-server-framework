package transport

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestNewSSETransport(t *testing.T) {
	transport := NewSSETransport(":8080")
	if transport == nil {
		t.Fatal("NewSSETransport returned nil")
	}

	if transport.addr != ":8080" {
		t.Errorf("Expected addr ':8080', got '%s'", transport.addr)
	}

	if transport.clients == nil {
		t.Error("Clients map not initialized")
	}

	if transport.messages == nil {
		t.Error("Messages channel not initialized")
	}

	if transport.done == nil {
		t.Error("Done channel not initialized")
	}
}

func TestSSETransportHealthHandler(t *testing.T) {
	transport := NewSSETransport(":8080")

	req := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()

	transport.handleHealth(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["status"] != "ok" {
		t.Errorf("Expected status 'ok', got '%v'", response["status"])
	}

	if response["transport"] != "sse" {
		t.Errorf("Expected transport 'sse', got '%v'", response["transport"])
	}
}

func TestSSETransportMessageHandler(t *testing.T) {
	transport := NewSSETransport(":8080")

	// Create a mock client
	sessionID := generateSessionID()
	client := &SSEClient{
		id:       sessionID,
		messages: make(chan []byte, 10),
		done:     make(chan struct{}),
	}

	transport.mu.Lock()
	transport.clients[sessionID] = client
	transport.mu.Unlock()

	// Test valid JSON
	message := map[string]string{"test": "message"}
	messageBytes, _ := json.Marshal(message)

	req := httptest.NewRequest("POST", "/message?sessionId="+sessionID, bytes.NewReader(messageBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.handleMessage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check that message was received
	select {
	case msg := <-transport.Receive():
		var received map[string]string
		err := json.Unmarshal(msg, &received)
		if err != nil {
			t.Fatalf("Failed to unmarshal received message: %v", err)
		}
		if received["test"] != "message" {
			t.Errorf("Expected 'message', got '%s'", received["test"])
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for message")
	}
}

func TestSSETransportMessageHandlerInvalidMethod(t *testing.T) {
	transport := NewSSETransport(":8080")

	req := httptest.NewRequest("GET", "/message", http.NoBody)
	w := httptest.NewRecorder()

	transport.handleMessage(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestSSETransportMessageHandlerInvalidJSON(t *testing.T) {
	transport := NewSSETransport(":8080")

	req := httptest.NewRequest("POST", "/message", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.handleMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400, got %d", w.Code)
	}
}

func TestSSETransportSend(t *testing.T) {
	transport := NewSSETransport(":8080")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}

	// Create a mock client
	client := &SSEClient{
		id:       "test-client",
		messages: make(chan []byte, 10),
		done:     make(chan struct{}),
	}

	transport.mu.Lock()
	transport.clients["test-client"] = client
	transport.mu.Unlock()

	message := []byte(`{"test": "message"}`)
	err = transport.Send(message)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	// Check that client received the message
	select {
	case msg := <-client.messages:
		if !bytes.Equal(msg, message) {
			t.Errorf("Expected '%s', got '%s'", string(message), string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for message")
	}

	transport.Stop()
}

func TestSSETransportClose(t *testing.T) {
	transport := NewSSETransport(":8080")

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}

	err = transport.Close()
	if err != nil {
		t.Fatalf("Failed to close transport: %v", err)
	}

	if !transport.closed {
		t.Error("Transport not marked as closed")
	}

	// Test that sending after close returns error
	err = transport.Send([]byte("test"))
	if err == nil {
		t.Error("Expected error when sending after close")
	}
}

func TestSSETransportCORS(t *testing.T) {
	transport := NewSSETransport(":8080")

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	corsHandler := transport.enableCORS(handler)

	// Test OPTIONS request
	req := httptest.NewRequest("OPTIONS", "/test", http.NoBody)
	w := httptest.NewRecorder()

	corsHandler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for OPTIONS, got %d", w.Code)
	}

	// Check CORS headers
	if w.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Error("CORS Allow-Origin header not set correctly")
	}

	if w.Header().Get("Access-Control-Allow-Methods") == "" {
		t.Error("CORS Allow-Methods header not set")
	}

	if w.Header().Get("Access-Control-Allow-Headers") == "" {
		t.Error("CORS Allow-Headers header not set")
	}
}

func TestSSETransportReceive(t *testing.T) {
	transport := NewSSETransport(":8080")

	messages := transport.Receive()
	if messages == nil {
		t.Error("Receive returned nil channel")
	}

	// Test that we can receive from the channel
	go func() {
		transport.messages <- []byte("test message")
	}()

	select {
	case msg := <-messages:
		if string(msg) != "test message" {
			t.Errorf("Expected 'test message', got '%s'", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for message")
	}
}

func TestSSETransportSessionManagement(t *testing.T) {
	transport := NewSSETransport(":8080")

	// Test session ID generation
	sessionID := generateSessionID()
	if len(sessionID) == 0 {
		t.Error("Generated session ID is empty")
	}

	// Test adding client
	client := &SSEClient{
		id:       sessionID,
		messages: make(chan []byte, 10),
		done:     make(chan struct{}),
	}

	transport.mu.Lock()
	transport.clients[sessionID] = client
	transport.mu.Unlock()

	// Test client count in health endpoint
	req := httptest.NewRequest("GET", "/health", http.NoBody)
	w := httptest.NewRecorder()

	transport.handleHealth(w, req)

	var response map[string]interface{}
	err := json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["clients"] != float64(1) {
		t.Errorf("Expected 1 client, got %v", response["clients"])
	}

	// Test removing client
	transport.mu.Lock()
	delete(transport.clients, sessionID)
	transport.mu.Unlock()

	req = httptest.NewRequest("GET", "/health", http.NoBody)
	w = httptest.NewRecorder()

	transport.handleHealth(w, req)

	err = json.Unmarshal(w.Body.Bytes(), &response)
	if err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	if response["clients"] != float64(0) {
		t.Errorf("Expected 0 clients, got %v", response["clients"])
	}
}

func TestSSETransportMessageHandlerWithCallback(t *testing.T) {
	transport := NewSSETransport(":8080")

	// Create a mock client
	sessionID := generateSessionID()
	client := &SSEClient{
		id:       sessionID,
		messages: make(chan []byte, 10),
		done:     make(chan struct{}),
	}

	transport.mu.Lock()
	transport.clients[sessionID] = client
	transport.mu.Unlock()

	// Set up a mock message handler
	var receivedMessage []byte
	transport.SetMessageHandler(func(message []byte) ([]byte, error) {
		receivedMessage = message
		return []byte(`{"result": "processed"}`), nil
	})

	// Test message handling
	message := map[string]string{"test": "message"}
	messageBytes, _ := json.Marshal(message)

	req := httptest.NewRequest("POST", "/message?sessionId="+sessionID, bytes.NewReader(messageBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.handleMessage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check that message handler was called
	if !bytes.Equal(receivedMessage, messageBytes) {
		t.Errorf("Message not passed correctly to handler")
	}
}

func TestSSETransportSSEHandler(t *testing.T) {
	transport := NewSSETransport(":8080")

	// Test SSE endpoint with a context that will be cancelled
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	req := httptest.NewRequest("GET", "/sse", http.NoBody)
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()

	// Cancel the context immediately to prevent hanging
	cancel()

	// This is a basic test - full SSE testing would require more complex setup
	// We'll test that the handler exists and responds appropriately
	transport.handleSSE(w, req)

	// The SSE handler should set appropriate headers
	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Error("SSE handler should set Content-Type to text/event-stream")
	}

	if w.Header().Get("Cache-Control") != "no-cache" {
		t.Error("SSE handler should set Cache-Control to no-cache")
	}
}

func TestSSETransportMCPProtocolIntegration(t *testing.T) {
	transport := NewSSETransport(":8080")

	// Create a mock client
	sessionID := generateSessionID()
	client := &SSEClient{
		id:       sessionID,
		messages: make(chan []byte, 10),
		done:     make(chan struct{}),
	}

	transport.mu.Lock()
	transport.clients[sessionID] = client
	transport.mu.Unlock()

	// Test tools/list request
	toolsListRequest := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      1,
		"method":  "tools/list",
	}
	requestBytes, _ := json.Marshal(toolsListRequest)

	req := httptest.NewRequest("POST", "/message?sessionId="+sessionID, bytes.NewReader(requestBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.handleMessage(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}

	// Check that message was received
	select {
	case msg := <-transport.Receive():
		var received map[string]interface{}
		err := json.Unmarshal(msg, &received)
		if err != nil {
			t.Fatalf("Failed to unmarshal received message: %v", err)
		}
		if received["method"] != "tools/list" {
			t.Errorf("Expected method 'tools/list', got '%v'", received["method"])
		}
		if received["jsonrpc"] != "2.0" {
			t.Errorf("Expected jsonrpc '2.0', got '%v'", received["jsonrpc"])
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for message")
	}
}

func TestSSETransportErrorHandling(t *testing.T) {
	transport := NewSSETransport(":8080")

	// Test missing session ID
	req := httptest.NewRequest("POST", "/message", http.NoBody)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.handleMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for missing session ID, got %d", w.Code)
	}

	// Test invalid session ID
	req = httptest.NewRequest("POST", "/message?sessionId=invalid", strings.NewReader(`{"test": "message"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	transport.handleMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid session ID, got %d", w.Code)
	}

	// Test invalid JSON with valid session
	sessionID := generateSessionID()
	client := &SSEClient{
		id:       sessionID,
		messages: make(chan []byte, 10),
		done:     make(chan struct{}),
	}

	transport.mu.Lock()
	transport.clients[sessionID] = client
	transport.mu.Unlock()

	req = httptest.NewRequest("POST", "/message?sessionId="+sessionID, strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()

	transport.handleMessage(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected status 400 for invalid JSON, got %d", w.Code)
	}
}
