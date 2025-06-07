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

// HTTPStreamsTransport implements the Transport interface using HTTP Streams
type HTTPStreamsTransport struct {
	addr           string
	server         *http.Server
	sessions       map[string]*HTTPStreamSession
	messages       chan []byte
	done           chan struct{}
	mu             sync.RWMutex
	closed         bool
	messageHandler func([]byte) ([]byte, error)
	debug          bool
}

// HTTPStreamSession represents a client session with SSE stream
type HTTPStreamSession struct {
	id       string
	writer   http.ResponseWriter
	flusher  http.Flusher
	messages chan []byte
	done     chan struct{}
	active   bool
}

// NewHTTPStreamsTransport creates a new HTTP Streams transport
func NewHTTPStreamsTransport(addr string) *HTTPStreamsTransport {
	return &HTTPStreamsTransport{
		addr:     addr,
		sessions: make(map[string]*HTTPStreamSession),
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
		debug:    false,
	}
}

// NewHTTPStreamsTransportWithDebug creates a new HTTP Streams transport with debug logging
func NewHTTPStreamsTransportWithDebug(addr string, debug bool) *HTTPStreamsTransport {
	return &HTTPStreamsTransport{
		addr:     addr,
		sessions: make(map[string]*HTTPStreamSession),
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
	mux.HandleFunc("/mcp", t.handleMCP)
	mux.HandleFunc("/health", t.handleHealth)

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

	// Close all sessions
	for _, session := range t.sessions {
		close(session.done)
	}

	if t.server != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		return t.server.Shutdown(ctx)
	}

	return nil
}

// Send sends a message to all connected sessions
func (t *HTTPStreamsTransport) Send(message []byte) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	// Send to all connected sessions
	for _, session := range t.sessions {
		if session.active {
			select {
			case session.messages <- message:
			case <-session.done:
				// Session is closed, skip
			default:
				// Session buffer is full, skip to avoid blocking
				if t.debug {
					log.Printf("[HTTP-STREAMS] Session %s buffer full, dropping message", session.id)
				}
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

// generateSessionID generates a random session ID
func (t *HTTPStreamsTransport) generateSessionID() string {
	bytes := make([]byte, 16)
	rand.Read(bytes)
	return hex.EncodeToString(bytes)
}

// handleMCP handles both GET (SSE stream) and POST (messages) requests
func (t *HTTPStreamsTransport) handleMCP(w http.ResponseWriter, r *http.Request) {
	if t.debug {
		log.Printf("[HTTP-STREAMS] MCP request from %s: %s %s", r.RemoteAddr, r.Method, r.URL.String())
	}

	// Set CORS headers
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, Mcp-Session-Id")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")

	if r.Method == http.MethodOptions {
		if t.debug {
			log.Printf("[HTTP-STREAMS] OPTIONS request handled")
		}
		w.WriteHeader(http.StatusOK)
		return
	}

	if r.Method == http.MethodGet {
		t.handleSSEStream(w, r)
	} else if r.Method == http.MethodPost {
		t.handleMessage(w, r)
	} else {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
	}
}

// handleSSEStream handles SSE stream connections
func (t *HTTPStreamsTransport) handleSSEStream(w http.ResponseWriter, r *http.Request) {
	if t.debug {
		log.Printf("[HTTP-STREAMS] SSE stream request from %s", r.RemoteAddr)
	}

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "Streaming unsupported", http.StatusInternalServerError)
		return
	}

	// Get session ID from header
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		if t.debug {
			log.Printf("[HTTP-STREAMS] Missing Mcp-Session-Id header")
		}
		http.Error(w, "Missing Mcp-Session-Id header", http.StatusBadRequest)
		return
	}

	// Check if session exists
	t.mu.Lock()
	session, exists := t.sessions[sessionID]
	if !exists {
		if t.debug {
			log.Printf("[HTTP-STREAMS] Session %s not found", sessionID)
		}
		t.mu.Unlock()
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Set up SSE stream
	session.writer = w
	session.flusher = flusher
	session.active = true
	t.mu.Unlock()

	// Set SSE headers
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	if t.debug {
		log.Printf("[HTTP-STREAMS] SSE stream established for session %s", sessionID)
	}

	// Send initial connection message
	if _, err := fmt.Fprintf(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	// Clean up on disconnect
	defer func() {
		t.mu.Lock()
		if session.active {
			session.active = false
		}
		t.mu.Unlock()
		if t.debug {
			log.Printf("[HTTP-STREAMS] SSE stream closed for session %s", sessionID)
		}
	}()

	// Send messages to client via SSE
	for {
		select {
		case message := <-session.messages:
			if _, err := fmt.Fprintf(w, "data: %s\n\n", message); err != nil {
				return
			}
			flusher.Flush()
		case <-session.done:
			return
		case <-r.Context().Done():
			return
		}
	}
}

// handleMessage handles incoming HTTP POST messages
func (t *HTTPStreamsTransport) handleMessage(w http.ResponseWriter, r *http.Request) {
	if t.debug {
		log.Printf("[HTTP-STREAMS] Message request from %s", r.RemoteAddr)
	}

	// Read the message
	var message json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		if t.debug {
			log.Printf("[HTTP-STREAMS] Error decoding JSON: %v", err)
		}
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	// Parse message to check if it's initialize
	var parsedMessage map[string]interface{}
	if err := json.Unmarshal(message, &parsedMessage); err != nil {
		http.Error(w, "Invalid JSON", http.StatusBadRequest)
		return
	}

	method, _ := parsedMessage["method"].(string)
	
	// Handle initialize request specially - return direct JSON response with session ID
	if method == "initialize" {
		if t.messageHandler != nil {
			response, err := t.messageHandler(message)
			if err != nil {
				http.Error(w, "Internal error", http.StatusInternalServerError)
				return
			}

			// Generate session ID and create session
			sessionID := t.generateSessionID()
			
			// Create session
			session := &HTTPStreamSession{
				id:       sessionID,
				messages: make(chan []byte, 100),
				done:     make(chan struct{}),
				active:   false,
			}

			t.mu.Lock()
			t.sessions[sessionID] = session
			t.mu.Unlock()

			// Send direct JSON response with session ID in header
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("Mcp-Session-Id", sessionID)
			w.Write(response)
			
			if t.debug {
				log.Printf("[HTTP-STREAMS] Initialize response sent with session ID %s", sessionID)
			}
			return
		}
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	// For non-initialize requests, get session ID from header
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		http.Error(w, "Missing Mcp-Session-Id header", http.StatusBadRequest)
		return
	}

	// Check if session exists
	t.mu.RLock()
	session, exists := t.sessions[sessionID]
	t.mu.RUnlock()

	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Process the message through the handler
	if t.messageHandler != nil {
		response, err := t.messageHandler(message)
		if err != nil {
			log.Printf("[HTTP-STREAMS] Error processing message: %v", err)
			// Send error response via SSE stream
			errorResponse := map[string]interface{}{
				"jsonrpc": "2.0",
				"error": map[string]interface{}{
					"code":    -32603,
					"message": "Internal error",
				},
				"id": parsedMessage["id"],
			}
			if errorData, err := json.Marshal(errorResponse); err == nil {
				select {
				case session.messages <- errorData:
				default:
					log.Printf("[HTTP-STREAMS] Session %s buffer full, dropping error response", sessionID)
				}
			}
		} else if response != nil {
			// Send response via SSE stream
			select {
			case session.messages <- response:
			default:
				log.Printf("[HTTP-STREAMS] Session %s buffer full, dropping response", sessionID)
			}
		}
	} else {
		// Fallback: put message in the general messages channel
		select {
		case t.messages <- message:
		default:
			log.Printf("[HTTP-STREAMS] Message buffer full, dropping message")
		}
	}

	// Send acknowledgment
	w.WriteHeader(http.StatusAccepted)
}

// handleHealth handles health check requests
func (t *HTTPStreamsTransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
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