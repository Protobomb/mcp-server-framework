package transport

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// SSETransport implements the Transport interface using Server-Sent Events
type SSETransport struct {
	addr           string
	server         *http.Server
	clients        map[string]*SSEClient
	messages       chan []byte
	done           chan struct{}
	mu             sync.RWMutex
	closed         bool
	messageHandler func([]byte) ([]byte, error)
}

// SSEClient represents a connected SSE client
type SSEClient struct {
	id       string
	writer   http.ResponseWriter
	flusher  http.Flusher
	messages chan []byte
	done     chan struct{}
}

// SSEMessage represents a message sent via SSE
type SSEMessage struct {
	Type string      `json:"type"`
	Data interface{} `json:"data"`
}

// generateSessionID generates a random session ID
func generateSessionID() string {
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to a simple timestamp-based ID if random generation fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
	return hex.EncodeToString(bytes)
}

// NewSSETransport creates a new SSE transport
func NewSSETransport(addr string) *SSETransport {
	return &SSETransport{
		addr:     addr,
		clients:  make(map[string]*SSEClient),
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
	}
}

// SetMessageHandler sets the message handler function
func (t *SSETransport) SetMessageHandler(handler func([]byte) ([]byte, error)) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.messageHandler = handler
}

// Start starts the SSE transport
func (t *SSETransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/sse", t.handleSSE)
	mux.HandleFunc("/message", t.handleMessage)
	mux.HandleFunc("/health", t.handleHealth)

	t.server = &http.Server{
		Addr:              t.addr,
		Handler:           t.enableCORS(mux),
		ReadHeaderTimeout: 10 * time.Second,
	}

	// Start the server in a goroutine
	go func() {
		log.Printf("Starting SSE server on %s", t.addr)
		if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Printf("SSE server error: %v", err)
		}
	}()

	return nil
}

// Stop stops the SSE transport
func (t *SSETransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	// Close done channel safely
	select {
	case <-t.done:
		// Already closed
	default:
		close(t.done)
	}

	// Close all clients
	for _, client := range t.clients {
		close(client.done)
	}

	if t.server != nil {
		return t.server.Shutdown(ctx)
	}

	return nil
}

// Send sends a message to all connected clients
func (t *SSETransport) Send(message []byte) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	// Send to all connected clients
	for _, client := range t.clients {
		select {
		case client.messages <- message:
		default:
			// Client buffer is full, skip
			log.Printf("Client %s buffer full, skipping message", client.id)
		}
	}

	return nil
}

// Receive returns a channel for receiving messages
func (t *SSETransport) Receive() <-chan []byte {
	return t.messages
}

// Close closes the SSE transport
func (t *SSETransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true

	// Close channels safely
	select {
	case <-t.done:
		// Already closed
	default:
		close(t.done)
	}

	select {
	case <-t.messages:
		// Already closed
	default:
		close(t.messages)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if t.server != nil {
		return t.server.Shutdown(ctx)
	}

	return nil
}

// handleSSE handles SSE connections
func (t *SSETransport) handleSSE(w http.ResponseWriter, r *http.Request) {
	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache, no-transform")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Get or generate session ID
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	client := &SSEClient{
		id:       sessionID,
		writer:   w,
		flusher:  flusher,
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
	}

	t.mu.Lock()
	t.clients[sessionID] = client
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.clients, sessionID)
		t.mu.Unlock()
		close(client.done)
	}()

	// Send endpoint event as per MCP SSE protocol
	// The client expects an "endpoint" event with the POST URL including sessionId
	endpointURL := fmt.Sprintf("/message?sessionId=%s", sessionID)
	fmt.Fprintf(w, "event: endpoint\ndata: %s\n\n", endpointURL)
	flusher.Flush()

	// Handle client messages
	for {
		select {
		case <-r.Context().Done():
			return
		case <-t.done:
			return
		case <-client.done:
			return
		case message := <-client.messages:
			fmt.Fprintf(w, "event: message\ndata: %s\n\n", string(message))
			flusher.Flush()
		}
	}
}

// handleMessage handles incoming messages from clients
func (t *SSETransport) handleMessage(w http.ResponseWriter, r *http.Request) {
	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Get session ID from query parameter
	sessionID := r.URL.Query().Get("sessionId")
	if sessionID == "" {
		http.Error(w, "Missing sessionId parameter", http.StatusBadRequest)
		return
	}

	// Check if client exists
	t.mu.RLock()
	client, exists := t.clients[sessionID]
	t.mu.RUnlock()

	if !exists {
		http.Error(w, "Invalid session ID", http.StatusBadRequest)
		return
	}

	var message json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Process the message through the handler if available
	if t.messageHandler != nil {
		response, err := t.messageHandler(message)
		if err != nil {
			log.Printf("Error processing message: %v", err)
			// Send error response to client via SSE
			errorResponse := map[string]interface{}{
				"jsonrpc": "2.0",
				"error": map[string]interface{}{
					"code":    -32603,
					"message": "Internal error",
				},
				"id": nil,
			}
			if errorData, err := json.Marshal(errorResponse); err == nil {
				select {
				case client.messages <- errorData:
				default:
					log.Printf("Client %s buffer full, dropping error response", sessionID)
				}
			}
		} else if response != nil {
			// Send response to client via SSE
			select {
			case client.messages <- response:
			default:
				log.Printf("Client %s buffer full, dropping response", sessionID)
			}
		}
	} else {
		// Fallback: put message in the general messages channel
		select {
		case t.messages <- message:
		default:
			log.Printf("Message buffer full, dropping message")
		}
	}

	// Always return 202 Accepted to the HTTP request (per MCP SSE protocol)
	w.WriteHeader(http.StatusAccepted)
	if _, err := w.Write([]byte("Accepted")); err != nil {
		// Log error but don't fail the request since status was already written
		log.Printf("Failed to write response body: %v", err)
	}
}

// handleHealth handles health check requests
func (t *SSETransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	t.mu.RLock()
	clientCount := len(t.clients)
	t.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(map[string]interface{}{
		"status":    "ok",
		"clients":   clientCount,
		"transport": "sse",
		"timestamp": time.Now().Unix(),
	}); err != nil {
		log.Printf("Failed to encode health response: %v", err)
	}
}

// enableCORS enables CORS for all requests
func (t *SSETransport) enableCORS(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Access-Control-Allow-Origin", "*")
		w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
		w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

		if r.Method == http.MethodOptions {
			w.WriteHeader(http.StatusOK)
			return
		}

		next.ServeHTTP(w, r)
	})
}
