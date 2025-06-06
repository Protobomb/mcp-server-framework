package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/protobomb/mcp-server-framework/pkg/mcp"
	"github.com/protobomb/mcp-server-framework/pkg/transport"
)

const (
	transportSSE = "sse"
)

var debugMode bool

func main() {
	var (
		transportType = flag.String("transport", "stdio", "Transport type: stdio or sse")
		addr          = flag.String("addr", "8080", "Port for SSE transport (e.g., 8080)")
		debug         = flag.Bool("debug", false, "Enable debug logging")
		help          = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	// Set global debug mode
	debugMode = *debug

	if *help {
		fmt.Println("MCP Server Framework")
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(0)
	}

	// Format address for SSE transport
	var formattedAddr string
	if *transportType == transportSSE {
		// If addr doesn't start with ":", add it
		if !strings.HasPrefix(*addr, ":") {
			formattedAddr = ":" + *addr
		} else {
			formattedAddr = *addr
		}
	}

	// Create transport based on type
	var t mcp.Transport
	switch *transportType {
	case "stdio":
		t = transport.NewSTDIOTransport()
	case transportSSE:
		t = transport.NewSSETransportWithDebug(formattedAddr, debugMode)
	default:
		log.Fatalf("Unknown transport type: %s", *transportType)
	}

	// Create server
	server := mcp.NewServer(t)

	// Register example handlers
	registerExampleHandlers(server)

	// Set up message handler for SSE transport
	if sseTransport, ok := t.(*transport.SSETransport); ok {
		sseTransport.SetMessageHandler(func(message []byte) ([]byte, error) {
			// Create a temporary context for message processing
			msgCtx := context.Background()

			// Parse the JSON-RPC message to check if it's a request or notification
			var request mcp.JSONRPCRequest
			if err := json.Unmarshal(message, &request); err != nil {
				return nil, fmt.Errorf("invalid JSON-RPC message: %w", err)
			}

			// Check if this is a notification (no ID field)
			if request.ID == nil {
				// This is a notification - handle it and don't send a response
				if handler := server.GetNotificationHandler(request.Method); handler != nil {
					if err := handler(msgCtx, request.Params); err != nil {
						log.Printf("Error handling notification %s: %v", request.Method, err)
					}
				} else {
					log.Printf("No handler for notification: %s", request.Method)
				}
				// Return nil for notifications (no response expected)
				return nil, nil
			}

			// This is a request - handle it and send a response
			response := mcp.JSONRPCResponse{
				JSONRPC: mcp.JSONRPCVersion,
				ID:      request.ID,
			}

			// Get the handler for this method
			if handler := server.GetHandler(request.Method); handler != nil {
				result, err := handler(msgCtx, request.Params)
				if err != nil {
					if rpcErr, ok := err.(*mcp.RPCError); ok {
						response.Error = rpcErr
					} else {
						response.Error = &mcp.RPCError{
							Code:    mcp.InternalError,
							Message: err.Error(),
						}
					}
				} else {
					response.Result = result
				}
			} else {
				response.Error = &mcp.RPCError{
					Code:    mcp.MethodNotFound,
					Message: fmt.Sprintf("Method not found: %s", request.Method),
				}
			}

			// Marshal the response
			return json.Marshal(response)
		})
	}

	// Setup context with cancellation
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Handle shutdown signals
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		log.Println("Shutting down server...")
		cancel()
	}()

	// Start server
	if err := server.Start(ctx); err != nil {
		cancel()
		log.Fatalf("Failed to start server: %v", err) //nolint:gocritic // cancel() called manually
	}

	log.Printf("MCP server started with %s transport", *transportType)
	if *transportType == transportSSE {
		log.Printf("SSE endpoints available at:")
		log.Printf("  Events: http://localhost%s/sse", formattedAddr)
		log.Printf("  Message: http://localhost%s/message", formattedAddr)
		log.Printf("  Health: http://localhost%s/health", formattedAddr)
	}

	// Wait for context cancellation
	<-ctx.Done()

	// Cleanup
	if err := server.Stop(); err != nil {
		log.Printf("Error stopping server: %v", err)
	}

	if err := server.Close(); err != nil {
		log.Printf("Error closing server: %v", err)
	}

	log.Println("Server stopped")
}

// registerExampleHandlers registers example handlers for demonstration
func registerExampleHandlers(server *mcp.Server) {
	// MCP tools/list handler
	server.RegisterHandler("tools/list", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return map[string]interface{}{
			"tools": []map[string]interface{}{
				{
					"name":        "echo",
					"description": "Echo back a message",
					"inputSchema": map[string]interface{}{
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
				{
					"name":        "math",
					"description": "Perform basic mathematical operations",
					"inputSchema": map[string]interface{}{
						"type": "object",
						"properties": map[string]interface{}{
							"operation": map[string]interface{}{
								"type":        "string",
								"description": "The operation to perform (add, subtract, multiply, divide)",
								"enum":        []string{"add", "subtract", "multiply", "divide"},
							},
							"a": map[string]interface{}{
								"type":        "number",
								"description": "First number",
							},
							"b": map[string]interface{}{
								"type":        "number",
								"description": "Second number",
							},
						},
						"required": []string{"operation", "a", "b"},
					},
				},
			},
		}, nil
	})

	// MCP tools/call handler
	server.RegisterHandler("tools/call", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		var callParams struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}

		if len(params) > 0 {
			if err := json.Unmarshal(params, &callParams); err != nil {
				return nil, mcp.NewInvalidParamsError("Invalid tool call parameters")
			}
		}

		switch callParams.Name {
		case "echo":
			message, ok := callParams.Arguments["message"].(string)
			if !ok {
				return nil, mcp.NewInvalidParamsError("Missing or invalid 'message' parameter")
			}
			return map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": message,
					},
				},
			}, nil

		case "math":
			operation, ok := callParams.Arguments["operation"].(string)
			if !ok {
				return nil, mcp.NewInvalidParamsError("Missing or invalid 'operation' parameter")
			}

			a, ok := callParams.Arguments["a"].(float64)
			if !ok {
				return nil, mcp.NewInvalidParamsError("Missing or invalid 'a' parameter")
			}

			b, ok := callParams.Arguments["b"].(float64)
			if !ok {
				return nil, mcp.NewInvalidParamsError("Missing or invalid 'b' parameter")
			}

			var result float64
			switch operation {
			case "add":
				result = a + b
			case "subtract":
				result = a - b
			case "multiply":
				result = a * b
			case "divide":
				if b == 0 {
					return nil, mcp.NewInvalidParamsError("Division by zero")
				}
				result = a / b
			default:
				return nil, mcp.NewInvalidParamsError("Unknown operation: " + operation)
			}

			return map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": fmt.Sprintf("%.2f", result),
					},
				},
			}, nil

		default:
			return nil, mcp.NewMethodNotFoundError("Unknown tool: " + callParams.Name)
		}
	})

	// Echo handler (legacy)
	server.RegisterHandler("echo", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		var echoParams struct {
			Message string `json:"message"`
		}

		if len(params) > 0 {
			if err := json.Unmarshal(params, &echoParams); err != nil {
				return nil, mcp.NewInvalidParamsError("Invalid echo parameters")
			}
		}

		return map[string]interface{}{
			"echo": echoParams.Message,
		}, nil
	})

	// Add handler (legacy)
	server.RegisterHandler("add", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		var addParams struct {
			A float64 `json:"a"`
			B float64 `json:"b"`
		}

		if len(params) > 0 {
			if err := json.Unmarshal(params, &addParams); err != nil {
				return nil, mcp.NewInvalidParamsError("Invalid add parameters")
			}
		}

		return map[string]interface{}{
			"result": addParams.A + addParams.B,
		}, nil
	})

	// List methods handler
	server.RegisterHandler("listMethods", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return map[string]interface{}{
			"methods": []string{
				"initialize",
				"tools/list",
				"tools/call",
				"echo",
				"add",
				"listMethods",
			},
		}, nil
	})

	// Ping notification handler
	server.RegisterNotificationHandler("ping", func(ctx context.Context, params json.RawMessage) error {
		log.Println("Received ping notification")
		return nil
	})

	// Notifications/message handler for demo
	server.RegisterNotificationHandler("notifications/message", func(ctx context.Context, params json.RawMessage) error {
		var msgParams struct {
			Level   string `json:"level"`
			Message string `json:"message"`
			Source  string `json:"source"`
		}

		if len(params) > 0 {
			if err := json.Unmarshal(params, &msgParams); err != nil {
				log.Printf("Invalid notification parameters: %v", err)
				return nil
			}
		}

		log.Printf("[%s] %s (from %s)", msgParams.Level, msgParams.Message, msgParams.Source)
		return nil
	})
}
