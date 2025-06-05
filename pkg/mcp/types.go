// Package mcp provides types and interfaces for the Model Context Protocol (MCP).
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
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ClientCapabilities `json:"capabilities"`
	ClientInfo      ServerInfo         `json:"clientInfo"`
}

// InitializeResult represents initialization result
type InitializeResult struct {
	ProtocolVersion string             `json:"protocolVersion"`
	Capabilities    ServerCapabilities `json:"capabilities"`
	ServerInfo      ServerInfo         `json:"serverInfo"`
}

// Tool represents an MCP tool
type Tool struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	InputSchema map[string]interface{} `json:"inputSchema"`
}

// Content represents content in MCP responses
type Content struct {
	Type string `json:"type"`
	Text string `json:"text,omitempty"`
}

// CallToolResult represents the result of calling a tool
type CallToolResult struct {
	Content []Content `json:"content"`
	IsError bool      `json:"isError,omitempty"`
}

// ToolsCapability represents tools capability
type ToolsCapability struct {
	ListChanged bool `json:"listChanged,omitempty"`
}

// ServerCapabilities represents server capabilities
type ServerCapabilities struct {
	Tools *ToolsCapability `json:"tools,omitempty"`
}

// ClientCapabilities represents client capabilities
type ClientCapabilities struct {
	// Add client-specific capabilities here as needed
}

// CallToolParams represents call tool request parameters
type CallToolParams struct {
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments,omitempty"`
}

// TextContent represents text content
type TextContent struct {
	Type string `json:"type"`
	Text string `json:"text"`
}
