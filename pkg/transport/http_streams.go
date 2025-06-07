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

// HTTPStreamsTransport implements the Transport interface using HTTP Streams
type HTTPStreamsTransport struct {
	addr           string
	server         *http.Server
	clients        map[string]*HTTPStreamClient
	messages       chan []byte
	done           chan struct{}
	mu             sync.RWMutex
	closed         bool
	messageHandler func([]byte) ([]byte, error)
	debug          bool
}

// HTTPStreamClient represents a connected HTTP stream client
type HTTPStreamClient struct {
	id       string
	writer   http.ResponseWriter
	flusher  http.Flusher
	messages chan []byte
	done     chan struct{}
}

// NewHTTPStreamsTransport creates a new HTTP Streams transport
func NewHTTPStreamsTransport(addr string) *HTTPStreamsTransport {
	return &HTTPStreamsTransport{
		addr:     addr,
		clients:  make(map[string]*HTTPStreamClient),
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
		debug:    false,
	}
}

// NewHTTPStreamsTransportWithDebug creates a new HTTP Streams transport with debug logging
func NewHTTPStreamsTransportWithDebug(addr string, debug bool) *HTTPStreamsTransport {
	return &HTTPStreamsTransport{
		addr:     addr,
		clients:  make(map[string]*HTTPStreamClient),
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
		debug:    debug,
	}
}

// SetMessageHandler sets the message handler function
func (t *HTTPStreamsTransport) SetMessageHandler(handler func([]byte) ([]byte, error)) {
	t.messageHandler = handler
}

// SetDebug enables or disables debug logging
func (t *HTTPStreamsTransport) SetDebug(debug bool) {
	t.debug = debug
}

// Start starts the HTTP Streams transport
func (t *HTTPStreamsTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.server != nil {
		return fmt.Errorf("transport already started")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/message", t.handleMessage)
	mux.HandleFunc("/stream", t.handleStream)

	t.server = &http.Server{
		Addr:    t.addr,
		Handler: mux,
	}

	go func() {
		if err := t.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			if t.debug {
				log.Printf("[HTTP-STREAMS] Server error: %v", err)
			}
		}
	}()

	// Start message processing
	go t.processMessages(ctx)

	if t.debug {
		log.Printf("[HTTP-STREAMS] Started on %s", t.addr)
	}

	return nil
}

// Stop stops the HTTP Streams transport
func (t *HTTPStreamsTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true
	close(t.done)

	// Close all clients
	for _, client := range t.clients {
		close(client.done)
	}

	if t.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return t.server.Shutdown(ctx)
	}

	return nil
}

// Send sends a message to all connected clients
func (t *HTTPStreamsTransport) Send(message []byte) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	// Send to all connected clients
	for _, client := range t.clients {
		select {
		case client.messages <- message:
		case <-client.done:
			// Client is closed, skip
		default:
			// Client buffer is full, skip to avoid blocking
			if t.debug {
				log.Printf("[HTTP-STREAMS] Client %s buffer full, dropping message", client.id)
			}
		}
	}

	return nil
}

// Receive returns a channel for receiving messages
func (t *HTTPStreamsTransport) Receive() <-chan []byte {
	return t.messages
}

// Close closes the transport
func (t *HTTPStreamsTransport) Close() error {
	return t.Stop()
}

// handleMessage handles incoming HTTP messages
func (t *HTTPStreamsTransport) handleMessage(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusOK)
		return
	}

	var message json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Send message to processing channel
	select {
	case t.messages <- message:
	case <-t.done:
		http.Error(w, "Transport closed", http.StatusServiceUnavailable)
		return
	default:
		http.Error(w, "Message buffer full", http.StatusServiceUnavailable)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "received"})
}

// handleStream handles HTTP stream connections
func (t *HTTPStreamsTransport) handleStream(w http.ResponseWriter, r *http.Request) {
	// Set headers for streaming
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("Access-Control-Allow-Origin", "*")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Generate client ID
	clientID := fmt.Sprintf("client_%d", time.Now().UnixNano())

	client := &HTTPStreamClient{
		id:       clientID,
		writer:   w,
		flusher:  flusher,
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
	}

	// Register client
	t.mu.Lock()
	t.clients[clientID] = client
	t.mu.Unlock()

	// Clean up on disconnect
	defer func() {
		t.mu.Lock()
		delete(t.clients, clientID)
		t.mu.Unlock()
		close(client.done)
		if t.debug {
			log.Printf("[HTTP-STREAMS] Client %s disconnected", clientID)
		}
	}()

	if t.debug {
		log.Printf("[HTTP-STREAMS] Client %s connected", clientID)
	}

	// Send messages to client
	for {
		select {
		case message := <-client.messages:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", message); err != nil {
				return
			}
			flusher.Flush()
		case <-client.done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

// processMessages processes incoming messages
func (t *HTTPStreamsTransport) processMessages(ctx context.Context) {
	for {
		select {
		case message := <-t.messages:
			if t.messageHandler != nil {
				if response, err := t.messageHandler(message); err == nil && response != nil {
					t.Send(response)
				}
			}
		case <-ctx.Done():
			return
		case <-t.done:
			return
		}
	}
}