package main

import (
	"context"
	"fmt"
	"log"
	"os/exec"
	"time"

	"github.com/openhands/mcp-server-framework/pkg/client"
	"github.com/openhands/mcp-server-framework/pkg/mcp"
)

func main() {
	fmt.Println("🚀 MCP Client-Server Demo")
	fmt.Println("========================")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start the example server
	fmt.Println("📡 Starting MCP server...")
	cmd := exec.CommandContext(ctx, "go", "run", "examples/simple-server.go")
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
			cmd.Process.Kill()
		}
	}()

	// Give the server a moment to start
	time.Sleep(100 * time.Millisecond)

	// Create client
	fmt.Println("🔌 Connecting client to server...")
	transport := client.NewSTDIOTransport(stdout, stdin)
	mcpClient := client.NewClient(transport)
	defer mcpClient.Close()

	if err := mcpClient.Start(ctx); err != nil {
		log.Fatalf("Failed to start client: %v", err)
	}

	// Run the demo
	if err := runDemo(ctx, mcpClient); err != nil {
		log.Fatalf("Demo failed: %v", err)
	}

	fmt.Println("\n🎉 Demo completed successfully!")
}

func runDemo(ctx context.Context, mcpClient *client.Client) error {
	// Step 1: Initialize
	fmt.Println("\n1️⃣  Initializing connection...")
	clientInfo := mcp.ServerInfo{
		Name:    "demo-client",
		Version: "1.0.0",
	}

	initResult, err := mcpClient.Initialize(ctx, clientInfo)
	if err != nil {
		return fmt.Errorf("initialize failed: %w", err)
	}

	fmt.Printf("   ✅ Connected to: %s v%s\n", 
		initResult.ServerInfo.Name, initResult.ServerInfo.Version)

	// Step 2: Discover tools
	fmt.Println("\n2️⃣  Discovering available tools...")
	tools, err := mcpClient.ListTools(ctx)
	if err != nil {
		return fmt.Errorf("list tools failed: %w", err)
	}

	fmt.Printf("   📋 Found %d tools:\n", len(tools))
	for _, tool := range tools {
		fmt.Printf("      • %s - %s\n", tool.Name, tool.Description)
	}

	// Step 3: Use the echo tool
	if len(tools) > 0 {
		echoTool := tools[0] // Assuming first tool is echo
		fmt.Printf("\n3️⃣  Using the '%s' tool...\n", echoTool.Name)
		
		args := map[string]interface{}{
			"message": "Hello from the demo client! 👋",
		}
		
		result, err := mcpClient.CallTool(ctx, echoTool.Name, args)
		if err != nil {
			return fmt.Errorf("tool call failed: %w", err)
		}

		fmt.Println("   📤 Sent: Hello from the demo client! 👋")
		fmt.Printf("   📥 Received: %s\n", result[0].Text)
	}

	// Step 4: Try the math tool (if available)
	mathToolFound := false
	for _, tool := range tools {
		if tool.Name == "math" {
			mathToolFound = true
			break
		}
	}

	if mathToolFound {
		fmt.Println("\n4️⃣  Performing math calculation...")
		
		args := map[string]interface{}{
			"operation": "add",
			"a":         15,
			"b":         27,
		}
		
		result, err := mcpClient.CallTool(ctx, "math", args)
		if err != nil {
			return fmt.Errorf("math tool call failed: %w", err)
		}

		fmt.Println("   🧮 Calculation: 15 + 27")
		fmt.Printf("   📊 Result: %s\n", result[0].Text)
	}

	// Step 5: Send a notification
	fmt.Println("\n5️⃣  Sending notification to server...")
	err = mcpClient.Notify("notifications/message", map[string]interface{}{
		"level":   "info",
		"message": "Demo completed successfully! 🎉",
		"source":  "demo-client",
	})
	if err != nil {
		return fmt.Errorf("notification failed: %w", err)
	}
	fmt.Println("   📢 Notification sent!")

	// Step 6: Test error handling
	fmt.Println("\n6️⃣  Testing error handling...")
	_, err = mcpClient.Call(ctx, "nonexistent/method", nil)
	if err != nil {
		fmt.Printf("   ✅ Error handling works: %v\n", err)
	} else {
		fmt.Println("   ⚠️  Expected error but got success")
	}

	return nil
}