package transport

import (
	"bytes"
	"context"
	"strings"
	"testing"
	"time"
)

func TestNewSTDIOTransport(t *testing.T) {
	transport := NewSTDIOTransport()
	if transport == nil {
		t.Fatal("NewSTDIOTransport returned nil")
	}

	if transport.input == nil {
		t.Error("Input not set")
	}

	if transport.output == nil {
		t.Error("Output not set")
	}

	if transport.messages == nil {
		t.Error("Messages channel not initialized")
	}

	if transport.done == nil {
		t.Error("Done channel not initialized")
	}
}

func TestNewSTDIOTransportWithIO(t *testing.T) {
	input := strings.NewReader("test input")
	output := &bytes.Buffer{}

	transport := NewSTDIOTransportWithIO(input, output)
	if transport == nil {
		t.Fatal("NewSTDIOTransportWithIO returned nil")
	}

	if transport.input != input {
		t.Error("Input not set correctly")
	}

	if transport.output != output {
		t.Error("Output not set correctly")
	}
}

func TestSTDIOTransportStart(t *testing.T) {
	input := strings.NewReader("test message\n")
	output := &bytes.Buffer{}

	transport := NewSTDIOTransportWithIO(input, output)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}

	if transport.scanner == nil {
		t.Error("Scanner not initialized")
	}
}

func TestSTDIOTransportSend(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	transport := NewSTDIOTransportWithIO(input, output)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}

	message := []byte(`{"test": "message"}`)
	err = transport.Send(message)
	if err != nil {
		t.Fatalf("Failed to send message: %v", err)
	}

	expected := `{"test": "message"}` + "\n"
	if output.String() != expected {
		t.Errorf("Expected output '%s', got '%s'", expected, output.String())
	}
}

func TestSTDIOTransportReceive(t *testing.T) {
	input := strings.NewReader("test message\nsecond message\n")
	output := &bytes.Buffer{}

	transport := NewSTDIOTransportWithIO(input, output)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}

	// Give some time for reading
	time.Sleep(10 * time.Millisecond)

	messages := transport.Receive()

	// Read first message
	select {
	case msg := <-messages:
		if string(msg) != "test message" {
			t.Errorf("Expected 'test message', got '%s'", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for first message")
	}

	// Read second message
	select {
	case msg := <-messages:
		if string(msg) != "second message" {
			t.Errorf("Expected 'second message', got '%s'", string(msg))
		}
	case <-time.After(100 * time.Millisecond):
		t.Error("Timeout waiting for second message")
	}
}

func TestSTDIOTransportStop(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	transport := NewSTDIOTransportWithIO(input, output)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}

	err = transport.Stop()
	if err != nil {
		t.Fatalf("Failed to stop transport: %v", err)
	}
}

func TestSTDIOTransportClose(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	transport := NewSTDIOTransportWithIO(input, output)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}

	err = transport.Close()
	if err != nil {
		t.Fatalf("Failed to close transport: %v", err)
	}

	if !transport.closed {
		t.Error("Transport not marked as closed")
	}

	// Test that sending after close returns error
	err = transport.Send([]byte("test"))
	if err == nil {
		t.Error("Expected error when sending after close")
	}
}

func TestSTDIOTransportSendAfterClose(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	transport := NewSTDIOTransportWithIO(input, output)

	err := transport.Close()
	if err != nil {
		t.Fatalf("Failed to close transport: %v", err)
	}

	err = transport.Send([]byte("test"))
	if err == nil {
		t.Error("Expected error when sending after close")
	}
}

func TestSTDIOTransportStartAfterClose(t *testing.T) {
	input := strings.NewReader("")
	output := &bytes.Buffer{}

	transport := NewSTDIOTransportWithIO(input, output)

	err := transport.Close()
	if err != nil {
		t.Fatalf("Failed to close transport: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	err = transport.Start(ctx)
	if err == nil {
		t.Error("Expected error when starting after close")
	}
}