package transport

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"
)

// SSETransport implements the Transport interface using Server-Sent Events
type SSETransport struct {
	addr     string
	server   *http.Server
	clients  map[string]*SSEClient
	messages chan []byte
	done     chan struct{}
	mu       sync.RWMutex
	closed   bool
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

// NewSSETransport creates a new SSE transport
func NewSSETransport(addr string) *SSETransport {
	return &SSETransport{
		addr:     addr,
		clients:  make(map[string]*SSEClient),
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
	}
}

// Start starts the SSE transport
func (t *SSETransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/events", t.handleSSE)
	mux.HandleFunc("/send", t.handleSend)
	mux.HandleFunc("/health", t.handleHealth)

	t.server = &http.Server{
		Addr:    t.addr,
		Handler: t.enableCORS(mux),
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
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	clientID := r.URL.Query().Get("client_id")
	if clientID == "" {
		clientID = fmt.Sprintf("client_%d", time.Now().UnixNano())
	}

	client := &SSEClient{
		id:       clientID,
		writer:   w,
		flusher:  flusher,
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
	}

	t.mu.Lock()
	t.clients[clientID] = client
	t.mu.Unlock()

	defer func() {
		t.mu.Lock()
		delete(t.clients, clientID)
		t.mu.Unlock()
		close(client.done)
	}()

	// Send initial connection message
	fmt.Fprintf(w, "data: {\"type\":\"connected\",\"client_id\":\"%s\"}\n\n", clientID)
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
			fmt.Fprintf(w, "data: %s\n\n", string(message))
			flusher.Flush()
		}
	}
}

// handleSend handles incoming messages from clients
func (t *SSETransport) handleSend(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var message json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	select {
	case t.messages <- message:
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	default:
		http.Error(w, "Message buffer full", http.StatusServiceUnavailable)
	}
}

// handleHealth handles health check requests
func (t *SSETransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	t.mu.RLock()
	clientCount := len(t.clients)
	t.mu.RUnlock()

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]interface{}{
		"status":       "ok",
		"clients":      clientCount,
		"transport":    "sse",
		"timestamp":    time.Now().Unix(),
	})
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