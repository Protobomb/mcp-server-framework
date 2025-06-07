// Package transport provides HTTP Streams transport implementation for MCP.
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
		Addr:              t.addr,
		Handler:           mux,
		ReadHeaderTimeout: 30 * time.Second,
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
	if _, err := rand.Read(bytes); err != nil {
		// Fallback to timestamp-based ID if random generation fails
		return fmt.Sprintf("%d", time.Now().UnixNano())
	}
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

	switch r.Method {
	case http.MethodGet:
		t.handleSSEStream(w, r)
	case http.MethodPost:
		t.handleMessage(w, r)
	default:
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

	sessionID, session, err := t.validateSSESession(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if session == nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	t.setupSSEStream(w, flusher, session, sessionID)
	t.streamMessages(w, flusher, r, session, sessionID)
}

// validateSSESession validates the session for SSE connection
func (t *HTTPStreamsTransport) validateSSESession(r *http.Request) (string, *HTTPStreamSession, error) {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		if t.debug {
			log.Printf("[HTTP-STREAMS] Missing Mcp-Session-Id header")
		}
		return "", nil, fmt.Errorf("missing Mcp-Session-Id header")
	}

	t.mu.Lock()
	session, exists := t.sessions[sessionID]
	if !exists {
		if t.debug {
			log.Printf("[HTTP-STREAMS] Session %s not found", sessionID)
		}
		t.mu.Unlock()
		return sessionID, nil, nil
	}
	t.mu.Unlock()

	return sessionID, session, nil
}

// setupSSEStream sets up the SSE stream for a session
func (t *HTTPStreamsTransport) setupSSEStream(
	w http.ResponseWriter, flusher http.Flusher, session *HTTPStreamSession, sessionID string,
) {
	t.mu.Lock()
	session.writer = w
	session.flusher = flusher
	session.active = true
	t.mu.Unlock()

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.WriteHeader(http.StatusOK)

	if t.debug {
		log.Printf("[HTTP-STREAMS] SSE stream established for session %s", sessionID)
	}

	if _, err := fmt.Fprintf(w, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()
}

// streamMessages handles the message streaming loop
func (t *HTTPStreamsTransport) streamMessages(
	w http.ResponseWriter, flusher http.Flusher, r *http.Request, session *HTTPStreamSession, sessionID string,
) {
	defer t.cleanupSSEStream(session, sessionID)

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

// cleanupSSEStream cleans up the SSE stream on disconnect
func (t *HTTPStreamsTransport) cleanupSSEStream(session *HTTPStreamSession, sessionID string) {
	t.mu.Lock()
	if session.active {
		session.active = false
	}
	t.mu.Unlock()
	if t.debug {
		log.Printf("[HTTP-STREAMS] SSE stream closed for session %s", sessionID)
	}
}

// handleMessage handles incoming HTTP POST messages
func (t *HTTPStreamsTransport) handleMessage(w http.ResponseWriter, r *http.Request) {
	if t.debug {
		log.Printf("[HTTP-STREAMS] Message request from %s", r.RemoteAddr)
	}

	message, parsedMessage, err := t.parseMessage(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	method, ok := parsedMessage["method"].(string)
	if !ok {
		http.Error(w, "Missing or invalid method", http.StatusBadRequest)
		return
	}

	if method == "initialize" {
		t.handleInitializeMessage(w, message)
		return
	}

	t.handleRegularMessage(w, r, message, parsedMessage)
}

// parseMessage parses the incoming JSON message
func (t *HTTPStreamsTransport) parseMessage(r *http.Request) (json.RawMessage, map[string]interface{}, error) {
	var message json.RawMessage
	if err := json.NewDecoder(r.Body).Decode(&message); err != nil {
		if t.debug {
			log.Printf("[HTTP-STREAMS] Error decoding JSON: %v", err)
		}
		return nil, nil, fmt.Errorf("invalid JSON")
	}

	var parsedMessage map[string]interface{}
	if err := json.Unmarshal(message, &parsedMessage); err != nil {
		return nil, nil, fmt.Errorf("invalid JSON")
	}

	return message, parsedMessage, nil
}

// handleInitializeMessage handles initialize requests
func (t *HTTPStreamsTransport) handleInitializeMessage(w http.ResponseWriter, message json.RawMessage) {
	if t.messageHandler == nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	response, err := t.messageHandler(message)
	if err != nil {
		http.Error(w, "Internal error", http.StatusInternalServerError)
		return
	}

	sessionID := t.generateSessionID()
	session := &HTTPStreamSession{
		id:       sessionID,
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
		active:   false,
	}

	t.mu.Lock()
	t.sessions[sessionID] = session
	t.mu.Unlock()

	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Mcp-Session-Id", sessionID)
	if _, err := w.Write(response); err != nil {
		log.Printf("[HTTP-STREAMS] Failed to write response: %v", err)
	}

	if t.debug {
		log.Printf("[HTTP-STREAMS] Initialize response sent with session ID %s", sessionID)
	}
}

// handleRegularMessage handles non-initialize requests
func (t *HTTPStreamsTransport) handleRegularMessage(
	w http.ResponseWriter, r *http.Request, message json.RawMessage, parsedMessage map[string]interface{},
) {
	sessionID := r.Header.Get("Mcp-Session-Id")
	if sessionID == "" {
		http.Error(w, "Missing Mcp-Session-Id header", http.StatusBadRequest)
		return
	}

	t.mu.RLock()
	session, exists := t.sessions[sessionID]
	t.mu.RUnlock()

	if !exists {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	t.processMessageWithSession(session, sessionID, message, parsedMessage)
	w.WriteHeader(http.StatusAccepted)
}

// processMessageWithSession processes a message for a specific session
func (t *HTTPStreamsTransport) processMessageWithSession(
	session *HTTPStreamSession, sessionID string, message json.RawMessage, parsedMessage map[string]interface{},
) {
	if t.messageHandler != nil {
		response, err := t.messageHandler(message)
		if err != nil {
			t.sendErrorResponse(session, sessionID, parsedMessage, err)
		} else if response != nil {
			t.sendResponse(session, sessionID, response)
		}
	} else {
		t.sendToGeneralChannel(message)
	}
}

// sendErrorResponse sends an error response via SSE stream
func (t *HTTPStreamsTransport) sendErrorResponse(
	session *HTTPStreamSession, sessionID string, parsedMessage map[string]interface{}, err error,
) {
	log.Printf("[HTTP-STREAMS] Error processing message: %v", err)
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
}

// sendResponse sends a response via SSE stream
func (t *HTTPStreamsTransport) sendResponse(session *HTTPStreamSession, sessionID string, response []byte) {
	select {
	case session.messages <- response:
	default:
		log.Printf("[HTTP-STREAMS] Session %s buffer full, dropping response", sessionID)
	}
}

// sendToGeneralChannel sends message to the general messages channel
func (t *HTTPStreamsTransport) sendToGeneralChannel(message json.RawMessage) {
	select {
	case t.messages <- message:
	default:
		log.Printf("[HTTP-STREAMS] Message buffer full, dropping message")
	}
}

// handleHealth handles health check requests
func (t *HTTPStreamsTransport) handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if err := json.NewEncoder(w).Encode(map[string]string{"status": "ok"}); err != nil {
		log.Printf("[HTTP-STREAMS] Failed to encode health response: %v", err)
	}
}

// processMessages processes incoming messages
func (t *HTTPStreamsTransport) processMessages(ctx context.Context) {
	for {
		select {
		case message := <-t.messages:
			if t.messageHandler != nil {
				if response, err := t.messageHandler(message); err == nil && response != nil {
					if err := t.Send(response); err != nil {
						log.Printf("[HTTP-STREAMS] Failed to send response: %v", err)
					}
				}
			}
		case <-ctx.Done():
			return
		case <-t.done:
			return
		}
	}
}
