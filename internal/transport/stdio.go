package transport

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os"
	"sync"
)

// STDIOTransport implements the Transport interface using STDIO
type STDIOTransport struct {
	input    io.Reader
	output   io.Writer
	scanner  *bufio.Scanner
	messages chan []byte
	done     chan struct{}
	mu       sync.RWMutex
	closed   bool
}

// NewSTDIOTransport creates a new STDIO transport
func NewSTDIOTransport() *STDIOTransport {
	return &STDIOTransport{
		input:    os.Stdin,
		output:   os.Stdout,
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
	}
}

// NewSTDIOTransportWithIO creates a new STDIO transport with custom IO
func NewSTDIOTransportWithIO(input io.Reader, output io.Writer) *STDIOTransport {
	return &STDIOTransport{
		input:    input,
		output:   output,
		messages: make(chan []byte, 100),
		done:     make(chan struct{}),
	}
}

// Start starts the STDIO transport
func (t *STDIOTransport) Start(ctx context.Context) error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	t.scanner = bufio.NewScanner(t.input)
	
	// Start reading from input in a goroutine
	go t.readLoop(ctx)

	return nil
}

// Stop stops the STDIO transport
func (t *STDIOTransport) Stop() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	close(t.done)
	return nil
}

// Send sends a message through STDIO
func (t *STDIOTransport) Send(message []byte) error {
	t.mu.RLock()
	defer t.mu.RUnlock()

	if t.closed {
		return fmt.Errorf("transport is closed")
	}

	// Write the message followed by a newline
	if _, err := t.output.Write(message); err != nil {
		return fmt.Errorf("failed to write message: %w", err)
	}
	
	if _, err := t.output.Write([]byte("\n")); err != nil {
		return fmt.Errorf("failed to write newline: %w", err)
	}

	// Flush if the output supports it
	if flusher, ok := t.output.(interface{ Flush() error }); ok {
		if err := flusher.Flush(); err != nil {
			return fmt.Errorf("failed to flush output: %w", err)
		}
	}

	return nil
}

// Receive returns a channel for receiving messages
func (t *STDIOTransport) Receive() <-chan []byte {
	return t.messages
}

// Close closes the STDIO transport
func (t *STDIOTransport) Close() error {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.closed {
		return nil
	}

	t.closed = true
	close(t.done)
	close(t.messages)

	return nil
}

// readLoop reads messages from input
func (t *STDIOTransport) readLoop(ctx context.Context) {
	defer func() {
		t.mu.Lock()
		if !t.closed {
			close(t.messages)
		}
		t.mu.Unlock()
	}()

	for {
		select {
		case <-ctx.Done():
			return
		case <-t.done:
			return
		default:
			if t.scanner.Scan() {
				line := t.scanner.Bytes()
				if len(line) > 0 {
					// Make a copy of the line since scanner reuses the buffer
					message := make([]byte, len(line))
					copy(message, line)
					
					select {
					case t.messages <- message:
					case <-ctx.Done():
						return
					case <-t.done:
						return
					}
				}
			} else {
				// Check for scanner error
				if err := t.scanner.Err(); err != nil {
					// Log error but continue
					fmt.Fprintf(os.Stderr, "Scanner error: %v\n", err)
				}
				return
			}
		}
	}
}