package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/openhands/mcp-server-framework/internal/transport"
	"github.com/openhands/mcp-server-framework/pkg/mcp"
)

func main() {
	var (
		transportType = flag.String("transport", "stdio", "Transport type: stdio or sse")
		addr          = flag.String("addr", ":8080", "Address for SSE transport")
		help          = flag.Bool("help", false, "Show help")
	)
	flag.Parse()

	if *help {
		fmt.Println("MCP Server Framework")
		fmt.Println("Usage:")
		flag.PrintDefaults()
		os.Exit(0)
	}

	// Create transport based on type
	var t mcp.Transport
	switch *transportType {
	case "stdio":
		t = transport.NewSTDIOTransport()
	case "sse":
		t = transport.NewSSETransport(*addr)
	default:
		log.Fatalf("Unknown transport type: %s", *transportType)
	}

	// Create server
	server := mcp.NewServer(t)

	// Register example handlers
	registerExampleHandlers(server)

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

	log.Printf("MCP server started with %s transport", *transportType)
	if *transportType == "sse" {
		log.Printf("SSE endpoints available at:")
		log.Printf("  Events: http://%s/events", *addr)
		log.Printf("  Send: http://%s/send", *addr)
		log.Printf("  Health: http://%s/health", *addr)
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
	// Echo handler
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

	// Add handler
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
}