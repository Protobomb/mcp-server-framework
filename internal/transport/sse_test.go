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

	req := httptest.NewRequest("GET", "/health", nil)
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

func TestSSETransportSendHandler(t *testing.T) {
	transport := NewSSETransport(":8080")

	// Test valid JSON
	message := map[string]string{"test": "message"}
	messageBytes, _ := json.Marshal(message)

	req := httptest.NewRequest("POST", "/send", bytes.NewReader(messageBytes))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.handleSend(w, req)

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

func TestSSETransportSendHandlerInvalidMethod(t *testing.T) {
	transport := NewSSETransport(":8080")

	req := httptest.NewRequest("GET", "/send", nil)
	w := httptest.NewRecorder()

	transport.handleSend(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405, got %d", w.Code)
	}
}

func TestSSETransportSendHandlerInvalidJSON(t *testing.T) {
	transport := NewSSETransport(":8080")

	req := httptest.NewRequest("POST", "/send", strings.NewReader("invalid json"))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	transport.handleSend(w, req)

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
		if string(msg) != string(message) {
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
	req := httptest.NewRequest("OPTIONS", "/test", nil)
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