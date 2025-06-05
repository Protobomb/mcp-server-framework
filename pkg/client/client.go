// Package client provides a Go client implementation for the Model Context Protocol (MCP).
package client

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/openhands/mcp-server-framework/pkg/mcp"
)

// Client represents an MCP client that can connect to MCP servers
type Client struct {
	transport Transport
	mu        sync.RWMutex
	nextID    int
	pending   map[interface{}]chan *mcp.JSONRPCResponse
	closed    bool
}

// Transport defines the interface for client transports
type Transport interface {
	Send(request *mcp.JSONRPCRequest) error
	Receive() (*mcp.JSONRPCResponse, error)
	Close() error
}

// NewClient creates a new MCP client with the specified transport
func NewClient(transport Transport) *Client {
	return &Client{
		transport: transport,
		pending:   make(map[interface{}]chan *mcp.JSONRPCResponse),
	}
}

// Start begins processing responses from the server
func (c *Client) Start(ctx context.Context) error {
	go c.processResponses(ctx)
	return nil
}

// Call sends a request and waits for a response
func (c *Client) Call(ctx context.Context, method string, params interface{}) (*mcp.JSONRPCResponse, error) {
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, fmt.Errorf("client is closed")
	}

	id := c.nextID
	c.nextID++

	respChan := make(chan *mcp.JSONRPCResponse, 1)
	c.pending[id] = respChan
	c.mu.Unlock()

	// Marshal params to JSON
	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			c.mu.Lock()
			delete(c.pending, id)
			c.mu.Unlock()
			return nil, fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	request := &mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		ID:      id,
		Method:  method,
		Params:  paramsJSON,
	}

	if err := c.transport.Send(request); err != nil {
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, fmt.Errorf("failed to send request: %w", err)
	}

	select {
	case resp := <-respChan:
		return resp, nil
	case <-ctx.Done():
		c.mu.Lock()
		delete(c.pending, id)
		c.mu.Unlock()
		return nil, ctx.Err()
	}
}

// Notify sends a notification (no response expected)
func (c *Client) Notify(method string, params interface{}) error {
	c.mu.RLock()
	if c.closed {
		c.mu.RUnlock()
		return fmt.Errorf("client is closed")
	}
	c.mu.RUnlock()

	// Marshal params to JSON
	var paramsJSON json.RawMessage
	if params != nil {
		var err error
		paramsJSON, err = json.Marshal(params)
		if err != nil {
			return fmt.Errorf("failed to marshal params: %w", err)
		}
	}

	request := &mcp.JSONRPCRequest{
		JSONRPC: "2.0",
		Method:  method,
		Params:  paramsJSON,
	}

	return c.transport.Send(request)
}

// Initialize sends an initialize request to the server
func (c *Client) Initialize(ctx context.Context, clientInfo mcp.ServerInfo) (*mcp.InitializeResult, error) {
	params := mcp.InitializeParams{
		ProtocolVersion: "2024-11-05",
		Capabilities:    mcp.ClientCapabilities{},
		ClientInfo:      clientInfo,
	}

	resp, err := c.Call(ctx, "initialize", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("initialize failed: %s", resp.Error.Message)
	}

	var result mcp.InitializeResult
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal initialize result: %w", err)
	}

	return &result, nil
}

// ListTools requests the list of available tools from the server
func (c *Client) ListTools(ctx context.Context) ([]mcp.Tool, error) {
	resp, err := c.Call(ctx, "tools/list", nil)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("list tools failed: %s", resp.Error.Message)
	}

	var result struct {
		Tools []mcp.Tool `json:"tools"`
	}
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tools list: %w", err)
	}

	return result.Tools, nil
}

// CallTool executes a tool with the given arguments
func (c *Client) CallTool(ctx context.Context, name string, arguments map[string]interface{}) ([]mcp.Content, error) {
	params := mcp.CallToolParams{
		Name:      name,
		Arguments: arguments,
	}

	resp, err := c.Call(ctx, "tools/call", params)
	if err != nil {
		return nil, err
	}

	if resp.Error != nil {
		return nil, fmt.Errorf("call tool failed: %s", resp.Error.Message)
	}

	var result mcp.CallToolResult
	resultBytes, err := json.Marshal(resp.Result)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal result: %w", err)
	}
	if err := json.Unmarshal(resultBytes, &result); err != nil {
		return nil, fmt.Errorf("failed to unmarshal tool result: %w", err)
	}

	return result.Content, nil
}

// Close closes the client and its transport
func (c *Client) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.closed {
		return nil
	}

	c.closed = true

	// Close all pending channels
	for _, ch := range c.pending {
		close(ch)
	}
	c.pending = make(map[interface{}]chan *mcp.JSONRPCResponse)

	return c.transport.Close()
}

// processResponses handles incoming responses from the server
func (c *Client) processResponses(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			return
		default:
			resp, err := c.transport.Receive()
			if err != nil {
				if err == io.EOF {
					return
				}
				// Log error and continue
				continue
			}

			c.mu.RLock()
			if respChan, exists := c.pending[resp.ID]; exists {
				select {
				case respChan <- resp:
				default:
					// Channel is full or closed
				}
			}
			c.mu.RUnlock()

			c.mu.Lock()
			delete(c.pending, resp.ID)
			c.mu.Unlock()
		}
	}
}

// STDIOTransport implements Transport for STDIO communication
type STDIOTransport struct {
	reader  io.Reader
	writer  io.Writer
	scanner *bufio.Scanner
	mu      sync.Mutex
}

// NewSTDIOTransport creates a new STDIO transport
func NewSTDIOTransport(reader io.Reader, writer io.Writer) *STDIOTransport {
	return &STDIOTransport{
		reader:  reader,
		writer:  writer,
		scanner: bufio.NewScanner(reader),
	}
}

// Send sends a request over STDIO
func (t *STDIOTransport) Send(request *mcp.JSONRPCRequest) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	_, err = fmt.Fprintf(t.writer, "%s\n", data)
	return err
}

// Receive receives a response over STDIO
func (t *STDIOTransport) Receive() (*mcp.JSONRPCResponse, error) {
	if !t.scanner.Scan() {
		if err := t.scanner.Err(); err != nil {
			return nil, err
		}
		return nil, io.EOF
	}

	line := t.scanner.Text()
	var response mcp.JSONRPCResponse
	if err := json.Unmarshal([]byte(line), &response); err != nil {
		return nil, fmt.Errorf("failed to unmarshal response: %w", err)
	}

	return &response, nil
}

// Close closes the STDIO transport
func (t *STDIOTransport) Close() error {
	// STDIO doesn't need explicit closing
	return nil
}

// HTTPTransport implements Transport for HTTP/SSE communication
type HTTPTransport struct {
	baseURL    string
	httpClient *http.Client
	mu         sync.Mutex
}

// NewHTTPTransport creates a new HTTP transport
func NewHTTPTransport(baseURL string) *HTTPTransport {
	return &HTTPTransport{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// Send sends a request over HTTP
func (t *HTTPTransport) Send(request *mcp.JSONRPCRequest) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	data, err := json.Marshal(request)
	if err != nil {
		return fmt.Errorf("failed to marshal request: %w", err)
	}

	resp, err := t.httpClient.Post(
		t.baseURL+"/send",
		"application/json",
		strings.NewReader(string(data)),
	)
	if err != nil {
		return fmt.Errorf("failed to send HTTP request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("HTTP request failed with status: %d", resp.StatusCode)
	}

	return nil
}

// Receive receives a response over HTTP (this is a simplified implementation)
// In a real implementation, you would use SSE to receive responses
func (t *HTTPTransport) Receive() (*mcp.JSONRPCResponse, error) {
	// This is a placeholder - in a real implementation, you would
	// establish an SSE connection and listen for events
	return nil, fmt.Errorf("HTTP transport receive not implemented - use SSE client")
}

// Close closes the HTTP transport
func (t *HTTPTransport) Close() error {
	// HTTP client doesn't need explicit closing
	return nil
}
