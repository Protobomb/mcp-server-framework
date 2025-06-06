package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/exec"
	"time"

	"github.com/protobomb/mcp-server-framework/pkg/client"
	"github.com/protobomb/mcp-server-framework/pkg/mcp"
)

var (
	version   = "dev"
	buildTime = "unknown"
)

func main() {
	var (
		transport = flag.String("transport", "stdio", "Transport type: stdio or http")
		addr      = flag.String("addr", "http://localhost:8080", "Address for HTTP transport")
		command   = flag.String("command", "", "Command to run for STDIO transport (e.g., './mcp-server')")
		help      = flag.Bool("help", false, "Show help")
		showVer   = flag.Bool("version", false, "Show version")
	)
	flag.Parse()

	if *help {
		fmt.Printf("MCP Test Client\n")
		fmt.Printf("Usage:\n")
		flag.PrintDefaults()
		fmt.Printf("\nExamples:\n")
		fmt.Printf("  # Connect to STDIO server\n")
		fmt.Printf("  %s -transport=stdio -command='./mcp-server'\n", os.Args[0])
		fmt.Printf("  # Connect to HTTP server\n")
		fmt.Printf("  %s -transport=http -addr=http://localhost:8080\n", os.Args[0])
		return
	}

	if *showVer {
		fmt.Printf("MCP Test Client %s (built %s)\n", version, buildTime)
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	var mcpClient *client.Client
	var cmd *exec.Cmd

	switch *transport {
	case "stdio":
		if *command == "" {
			cancel()
			log.Fatal("Command is required for STDIO transport") //nolint:gocritic // cancel() called manually
		}

		// Start the server process
		cmd = exec.CommandContext(ctx, "sh", "-c", *command) //nolint:gosec // Command comes from user flag
		stdin, err := cmd.StdinPipe()
		if err != nil {
			log.Fatalf("Failed to create stdin pipe: %v", err)
		}
		stdout, err := cmd.StdoutPipe()
		if err != nil {
			log.Fatalf("Failed to create stdout pipe: %v", err)
		}

		if err := cmd.Start(); err != nil {
			log.Fatalf("Failed to start server: %v", err)
		}
		defer func() {
			if cmd.Process != nil {
				if err := cmd.Process.Kill(); err != nil {
					log.Printf("Failed to kill process: %v", err)
				}
			}
		}()

		transport := client.NewSTDIOTransport(stdout, stdin)
		mcpClient = client.NewClient(transport)

	case "http":
		transport := client.NewHTTPTransport(*addr)
		mcpClient = client.NewClient(transport)

	default:
		log.Fatalf("Unknown transport: %s", *transport)
	}

	defer mcpClient.Close()

	if err := mcpClient.Start(ctx); err != nil {
		log.Fatalf("Failed to start client: %v", err)
	}

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Test the MCP client
	if err := testMCPClient(ctx, mcpClient); err != nil {
		log.Fatalf("Test failed: %v", err)
	}

	fmt.Println("✅ All tests passed!")
}

func testMCPClient(ctx context.Context, mcpClient *client.Client) error {
	fmt.Println("🔄 Testing MCP client...")

	// Test 1: Initialize
	fmt.Println("📡 Testing initialize...")
	clientInfo := mcp.ServerInfo{
		Name:    "mcp-test-client",
		Version: version,
	}

	initResult, err := mcpClient.Initialize(ctx, clientInfo)
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	fmt.Printf("✅ Initialize successful! Server: %s v%s\n",
		initResult.ServerInfo.Name, initResult.ServerInfo.Version)
	fmt.Printf("   Protocol version: %s\n", initResult.ProtocolVersion)

	// Test 2: List tools
	fmt.Println("🔧 Testing tools/list...")
	tools, err := mcpClient.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("list tools failed: %w", err)
	}

	fmt.Printf("✅ Found %d tools:\n", len(tools))
	for _, tool := range tools {
		fmt.Printf("   - %s: %s\n", tool.Name, tool.Description)
	}

	// Test 3: Call a tool (if available)
	if len(tools) > 0 {
		fmt.Printf("🚀 Testing tools/call with '%s'...\n", tools[0].Name)

		// Try to call the first tool with empty arguments
		result, callErr := mcpClient.CallTool(ctx, tools[0].Name, map[string]interface{}{})
		if callErr != nil {
			// Tool call might fail due to missing arguments, but that's expected
			fmt.Printf("⚠️  Tool call failed (expected): %v\n", callErr)
		} else {
			fmt.Printf("✅ Tool call successful! Result:\n")
			for i, content := range result {
				fmt.Printf("   [%d] %s: %s\n", i, content.Type, content.Text)
			}
		}
	}

	// Test 4: Send a notification
	fmt.Println("📢 Testing notification...")
	err = mcpClient.Notify("notifications/message", map[string]interface{}{
		"level":   "info",
		"message": "Test notification from client",
	})
	if err != nil {
		return fmt.Errorf("notification failed: %w", err)
	}
	fmt.Println("✅ Notification sent successfully!")

	// Test 5: Test unknown method (should fail gracefully)
	fmt.Println("❌ Testing unknown method (should fail)...")
	_, err = mcpClient.Call(ctx, "unknown/method", nil)
	if err != nil {
		fmt.Printf("✅ Unknown method correctly failed: %v\n", err)
	} else {
		fmt.Println("⚠️  Unknown method unexpectedly succeeded")
	}

	return nil
}
