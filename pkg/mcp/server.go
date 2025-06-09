package mcp

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
)

// NewServer creates a new MCP server
func NewServer(transport Transport) *Server {
	return &Server{
		handlers:      make(map[string]Handler),
		notifications: make(map[string]NotificationHandler),
		transport:     transport,
	}
}

// RegisterHandler registers a method handler
func (s *Server) RegisterHandler(method string, handler Handler) {
	s.handlers[method] = handler
}

// RegisterNotificationHandler registers a notification handler
func (s *Server) RegisterNotificationHandler(method string, handler NotificationHandler) {
	s.notifications[method] = handler
}

// GetHandler returns the handler for a given method
func (s *Server) GetHandler(method string) Handler {
	return s.handlers[method]
}

// GetNotificationHandler returns the notification handler for a given method
func (s *Server) GetNotificationHandler(method string) NotificationHandler {
	return s.notifications[method]
}

// Start starts the MCP server
func (s *Server) Start(ctx context.Context) error {
	// Register default handlers
	s.registerDefaultHandlers()

	// Start the transport
	if err := s.transport.Start(ctx); err != nil {
		return fmt.Errorf("failed to start transport: %w", err)
	}

	// Start message processing
	go s.processMessages(ctx)

	return nil
}

// Stop stops the MCP server
func (s *Server) Stop() error {
	return s.transport.Stop()
}

// Close closes the MCP server
func (s *Server) Close() error {
	return s.transport.Close()
}

// SendNotification sends a notification to the client
func (s *Server) SendNotification(method string, params interface{}) error {
	notification := JSONRPCNotification{
		JSONRPC: JSONRPCVersion,
		Method:  method,
	}

	if params != nil {
		paramsBytes, err := json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
		notification.Params = paramsBytes
	}

	message, err := json.Marshal(notification)
	if err != nil {
		return fmt.Errorf("failed to marshal notification: %w", err)
	}

	return s.transport.Send(message)
}

// processMessages processes incoming messages
func (s *Server) processMessages(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		case message := <-s.transport.Receive():
			if len(message) == 0 {
				continue
			}
			s.handleMessage(ctx, message)
		}
	}
}

// handleMessage handles a single message
func (s *Server) handleMessage(ctx context.Context, message []byte) {
	// Try to parse as request first
	var request JSONRPCRequest
	if err := json.Unmarshal(message, &request); err == nil && request.Method != "" {
		log.Printf("Parsed as request: method=%s, id=%v", request.Method, request.ID)
		if request.ID != nil {
			// This is a request
			log.Printf("Handling as request: %s", request.Method)
			s.handleRequest(ctx, request)
		} else {
			// This is a notification
			log.Printf("Handling as notification: %s", request.Method)
			s.handleNotification(ctx, JSONRPCNotification{
				JSONRPC: request.JSONRPC,
				Method:  request.Method,
				Params:  request.Params,
			})
		}
		return
	}

	// Try to parse as notification
	var notification JSONRPCNotification
	if err := json.Unmarshal(message, &notification); err == nil && notification.Method != "" {
		log.Printf("Parsed as notification: %s", notification.Method)
		s.handleNotification(ctx, notification)
		return
	}

	log.Printf("Failed to parse message: %s", string(message))
}

// handleRequest handles a JSON-RPC request
func (s *Server) handleRequest(ctx context.Context, request JSONRPCRequest) {
	response := JSONRPCResponse{
		JSONRPC: JSONRPCVersion,
		ID:      request.ID,
	}

	handler, exists := s.handlers[request.Method]
	if !exists {
		response.Error = &RPCError{
			Code:    MethodNotFound,
			Message: fmt.Sprintf("Method not found: %s", request.Method),
		}
	} else {
		result, err := handler(ctx, request.Params)
		if err != nil {
			if rpcErr, ok := err.(*RPCError); ok {
				response.Error = rpcErr
			} else {
				response.Error = &RPCError{
					Code:    InternalError,
					Message: err.Error(),
				}
			}
		} else {
			response.Result = result
		}
	}

	responseBytes, err := json.Marshal(response)
	if err != nil {
		log.Printf("Failed to marshal response: %v", err)
		return
	}

	if err := s.transport.Send(responseBytes); err != nil {
		log.Printf("Failed to send response: %v", err)
	}
}

// handleNotification handles a JSON-RPC notification
func (s *Server) handleNotification(ctx context.Context, notification JSONRPCNotification) {
	handler, exists := s.notifications[notification.Method]
	if !exists {
		log.Printf("No handler for notification: %s", notification.Method)
		return
	}

	if err := handler(ctx, notification.Params); err != nil {
		log.Printf("Error handling notification %s: %v", notification.Method, err)
	}
}

// registerDefaultHandlers registers the default MCP handlers
// Only registers handlers if they don't already exist (allows custom overrides)
func (s *Server) registerDefaultHandlers() {
	// Only register if not already registered (allows custom handlers to override)
	if s.GetHandler("initialize") == nil {
		s.RegisterHandler("initialize", s.handleInitialize)
	}
	if s.GetNotificationHandler("initialized") == nil {
		s.RegisterNotificationHandler("initialized", s.handleInitialized)
	}
	if s.GetHandler("tools/list") == nil {
		s.RegisterHandler("tools/list", s.handleToolsList)
	}
	if s.GetHandler("tools/call") == nil {
		s.RegisterHandler("tools/call", s.handleToolsCall)
	}
}

// handleInitialize handles the initialize request
func (s *Server) handleInitialize(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var initParams InitializeParams
	if len(params) > 0 {
		if err := json.Unmarshal(params, &initParams); err != nil {
			return nil, &RPCError{
				Code:    InvalidParams,
				Message: "Invalid initialization parameters",
			}
		}
	}

	return InitializeResult{
		ProtocolVersion: "2024-11-05",
		Capabilities: ServerCapabilities{
			Tools: &ToolsCapability{
				ListChanged: true,
			},
		},
		ServerInfo: ServerInfo{
			Name:    "mcp-server-framework",
			Version: "1.0.0",
		},
	}, nil
}

// handleInitialized handles the initialized notification
func (s *Server) handleInitialized(ctx context.Context, params json.RawMessage) error {
	log.Println("Server initialized")
	return nil
}

// handleToolsList handles the tools/list request
func (s *Server) handleToolsList(ctx context.Context, params json.RawMessage) (interface{}, error) {
	// Return a list of available tools
	tools := []Tool{
		{
			Name:        "echo",
			Description: "Echo back the provided message",
			InputSchema: map[string]interface{}{
				"type": "object",
				"properties": map[string]interface{}{
					"message": map[string]interface{}{
						"type":        "string",
						"description": "The message to echo back",
					},
				},
				"required": []string{"message"},
			},
		},
	}

	return ToolsListResult{
		Tools: tools,
	}, nil
}

// handleToolsCall handles the tools/call request
func (s *Server) handleToolsCall(ctx context.Context, params json.RawMessage) (interface{}, error) {
	var callParams ToolsCallParams
	if err := json.Unmarshal(params, &callParams); err != nil {
		return nil, &RPCError{
			Code:    InvalidParams,
			Message: "Invalid tool call parameters",
		}
	}

	switch callParams.Name {
	case "echo":
		// Handle echo tool
		message, ok := callParams.Arguments["message"].(string)
		if !ok {
			return nil, &RPCError{
				Code:    InvalidParams,
				Message: "Missing or invalid 'message' argument",
			}
		}

		return ToolsCallResult{
			Content: []Content{
				{
					Type: "text",
					Text: fmt.Sprintf("Echo: %s", message),
				},
			},
		}, nil

	default:
		return nil, &RPCError{
			Code:    InvalidParams,
			Message: fmt.Sprintf("Unknown tool: %s", callParams.Name),
		}
	}
}

// Utility functions for creating common responses

// NewRPCError creates a new RPC error
func NewRPCError(code int, message string, data interface{}) *RPCError {
	return &RPCError{
		Code:    code,
		Message: message,
		Data:    data,
	}
}

// NewParseError creates a parse error
func NewParseError(message string) *RPCError {
	return NewRPCError(ParseError, message, nil)
}

// NewInvalidRequestError creates an invalid request error
func NewInvalidRequestError(message string) *RPCError {
	return NewRPCError(InvalidRequest, message, nil)
}

// NewMethodNotFoundError creates a method not found error
func NewMethodNotFoundError(method string) *RPCError {
	return NewRPCError(MethodNotFound, fmt.Sprintf("Method not found: %s", method), nil)
}

// NewInvalidParamsError creates an invalid params error
func NewInvalidParamsError(message string) *RPCError {
	return NewRPCError(InvalidParams, message, nil)
}

// NewInternalError creates an internal error
func NewInternalError(message string) *RPCError {
	return NewRPCError(InternalError, message, nil)
}
