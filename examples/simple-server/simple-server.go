package main

import (
	"context"
	"encoding/json"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/openhands/mcp-server-framework/internal/transport"
	"github.com/openhands/mcp-server-framework/pkg/mcp"
)

func main() {
	// Create STDIO transport
	t := transport.NewSTDIOTransport()

	// Create server
	server := mcp.NewServer(t)

	// Register custom handlers
	registerHandlers(server)

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
		log.Fatalf("Failed to start server: %v", err)
	}

	log.Println("Simple MCP server started")

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

func registerHandlers(server *mcp.Server) {
	// Calculator handler
	server.RegisterHandler("calculate", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		var calcParams struct {
			Operation string  `json:"operation"`
			A         float64 `json:"a"`
			B         float64 `json:"b"`
		}

		if len(params) > 0 {
			if err := json.Unmarshal(params, &calcParams); err != nil {
				return nil, mcp.NewInvalidParamsError("Invalid calculation parameters")
			}
		}

		var result float64
		switch calcParams.Operation {
		case "add":
			result = calcParams.A + calcParams.B
		case "subtract":
			result = calcParams.A - calcParams.B
		case "multiply":
			result = calcParams.A * calcParams.B
		case "divide":
			if calcParams.B == 0 {
				return nil, mcp.NewInvalidParamsError("Division by zero")
			}
			result = calcParams.A / calcParams.B
		default:
			return nil, mcp.NewInvalidParamsError("Unknown operation: " + calcParams.Operation)
		}

		return map[string]interface{}{
			"result":    result,
			"operation": calcParams.Operation,
			"a":         calcParams.A,
			"b":         calcParams.B,
		}, nil
	})

	// Status handler
	server.RegisterHandler("status", func(ctx context.Context, params json.RawMessage) (interface{}, error) {
		return map[string]interface{}{
			"status":  "running",
			"version": "1.0.0",
			"uptime":  "unknown", // In a real implementation, you'd track this
		}, nil
	})

	// Log notification handler
	server.RegisterNotificationHandler("log", func(ctx context.Context, params json.RawMessage) error {
		var logParams struct {
			Level   string `json:"level"`
			Message string `json:"message"`
		}

		if len(params) > 0 {
			if err := json.Unmarshal(params, &logParams); err != nil {
				log.Printf("Invalid log parameters: %v", err)
				return nil
			}
		}

		log.Printf("[%s] %s", logParams.Level, logParams.Message)
		return nil
	})
}
