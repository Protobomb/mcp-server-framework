package mcp

import (
	"context"
	"encoding/json"
)

// JSONRPCVersion represents the JSON-RPC version
const JSONRPCVersion = "2.0"

// JSONRPCRequest represents a JSON-RPC 2.0 request
type JSONRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// JSONRPCResponse represents a JSON-RPC 2.0 response
type JSONRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id,omitempty"`
	Result  interface{} `json:"result,omitempty"`
	Error   *RPCError   `json:"error,omitempty"`
}

// JSONRPCNotification represents a JSON-RPC 2.0 notification
type JSONRPCNotification struct {
	JSONRPC string          `json:"jsonrpc"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

// RPCError represents a JSON-RPC error
type RPCError struct {
	Code    int         `json:"code"`
	Message string      `json:"message"`
	Data    interface{} `json:"data,omitempty"`
}

// Error implements the error interface
func (e *RPCError) Error() string {
	return e.Message
}

// Common RPC error codes
const (
	ParseError     = -32700
	InvalidRequest = -32600
	MethodNotFound = -32601
	InvalidParams  = -32602
	InternalError  = -32603
)

// Handler represents a method handler function
type Handler func(ctx context.Context, params json.RawMessage) (interface{}, error)

// NotificationHandler represents a notification handler function
type NotificationHandler func(ctx context.Context, params json.RawMessage) error

// Transport represents the communication transport interface
type Transport interface {
	// Start starts the transport
	Start(ctx context.Context) error
	// Stop stops the transport
	Stop() error
	// Send sends a message
	Send(message []byte) error
	// Receive returns a channel for receiving messages
	Receive() <-chan []byte
	// Close closes the transport
	Close() error
}

// Server represents the MCP server
type Server struct {
	handlers      map[string]Handler
	notifications map[string]NotificationHandler
	transport     Transport
}

// ServerInfo represents server information
type ServerInfo struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

// InitializeParams represents initialization parameters
type InitializeParams struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ClientInfo      ServerInfo             `json:"clientInfo"`
}

// InitializeResult represents initialization result
type InitializeResult struct {
	ProtocolVersion string                 `json:"protocolVersion"`
	Capabilities    map[string]interface{} `json:"capabilities"`
	ServerInfo      ServerInfo             `json:"serverInfo"`
}