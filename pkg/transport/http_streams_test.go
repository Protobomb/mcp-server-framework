package transport

import (
	"context"
	"testing"
	"time"
)

func TestNewHTTPStreamsTransport(t *testing.T) {
	transport := NewHTTPStreamsTransport(":8080")
	if transport == nil {
		t.Fatal("Expected transport to be created")
	}
	if transport.addr != ":8080" {
		t.Errorf("Expected addr to be :8080, got %s", transport.addr)
	}
}

func TestHTTPStreamsTransportStartStop(t *testing.T) {
	transport := NewHTTPStreamsTransport(":0") // Use random port
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
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

func TestHTTPStreamsTransportSend(t *testing.T) {
	transport := NewHTTPStreamsTransport(":0")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}
	defer transport.Stop()
	
	message := []byte(`{"jsonrpc":"2.0","method":"test","id":1}`)
	err = transport.Send(message)
	if err != nil {
		t.Errorf("Failed to send message: %v", err)
	}
}

func TestHTTPStreamsTransportReceive(t *testing.T) {
	transport := NewHTTPStreamsTransport(":0")
	
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	err := transport.Start(ctx)
	if err != nil {
		t.Fatalf("Failed to start transport: %v", err)
	}
	defer transport.Stop()
	
	receiveChan := transport.Receive()
	if receiveChan == nil {
		t.Error("Expected receive channel to be non-nil")
	}
}