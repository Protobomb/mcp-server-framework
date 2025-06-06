#!/bin/bash

# Test Examples Script
# This script tests all the curl examples from the documentation

set -e

# Colors for output
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
NC='\033[0m' # No Color

# Configuration
SERVER_HOST="localhost"
SERVER_PORT="8080"
BASE_URL="http://${SERVER_HOST}:${SERVER_PORT}"
SESSION_ID="test-session-$(date +%s)"
SERVER_PID=""

# Function to print colored output
print_status() {
    echo -e "${BLUE}[INFO]${NC} $1"
}

print_success() {
    echo -e "${GREEN}[SUCCESS]${NC} $1"
}

print_error() {
    echo -e "${RED}[ERROR]${NC} $1"
}

print_warning() {
    echo -e "${YELLOW}[WARNING]${NC} $1"
}

# Function to check if server is running
check_server() {
    if curl -s "${BASE_URL}/health" > /dev/null 2>&1; then
        return 0
    else
        return 1
    fi
}

# Function to start the server
start_server() {
    print_status "Starting MCP server..."
    
    # Build the server if it doesn't exist
    if [ ! -f "./mcp-server" ]; then
        print_status "Building MCP server..."
        go build -o mcp-server cmd/mcp-server/main.go
    fi
    
    # Start server in background
    ./mcp-server -transport=sse -addr="${SERVER_PORT}" > server.log 2>&1 &
    SERVER_PID=$!
    
    # Wait for server to start
    print_status "Waiting for server to start..."
    for i in {1..30}; do
        if check_server; then
            print_success "Server started successfully (PID: ${SERVER_PID})"
            return 0
        fi
        sleep 1
    done
    
    print_error "Server failed to start within 30 seconds"
    if [ -f "server.log" ]; then
        print_error "Server log:"
        cat server.log
    fi
    return 1
}

# Function to stop the server
stop_server() {
    if [ -n "$SERVER_PID" ]; then
        print_status "Stopping server (PID: ${SERVER_PID})..."
        kill $SERVER_PID 2>/dev/null || true
        wait $SERVER_PID 2>/dev/null || true
        print_success "Server stopped"
    fi
}

# Function to test an endpoint
test_endpoint() {
    local name="$1"
    local method="$2"
    local url="$3"
    local data="$4"
    local expected_status="$5"
    
    print_status "Testing: $name"
    
    if [ "$method" = "GET" ]; then
        response=$(curl -s -w "\n%{http_code}" "$url")
    else
        response=$(curl -s -w "\n%{http_code}" -X "$method" -H "Content-Type: application/json" -d "$data" "$url")
    fi
    
    # Split response and status code
    status_code=$(echo "$response" | tail -n1)
    body=$(echo "$response" | head -n -1)
    
    if [ "$status_code" = "$expected_status" ]; then
        print_success "$name - Status: $status_code"
        if [ -n "$body" ] && [ "$body" != "" ]; then
            echo "Response: $body" | jq . 2>/dev/null || echo "Response: $body"
        fi
        echo
        return 0
    else
        print_error "$name - Expected: $expected_status, Got: $status_code"
        echo "Response: $body"
        echo
        return 1
    fi
}

# Function to establish SSE connection and keep it alive
establish_sse_connection() {
    print_status "Establishing SSE connection..."
    
    # Start SSE connection in background
    curl -N -H "Accept: text/event-stream" "${BASE_URL}/sse?sessionId=${SESSION_ID}" > sse_output.txt 2>&1 &
    SSE_PID=$!
    
    # Give it a moment to connect
    sleep 2
    
    # Check if connection is established
    if ps -p $SSE_PID > /dev/null 2>&1; then
        print_success "SSE connection established (PID: $SSE_PID)"
        return 0
    else
        print_error "Failed to establish SSE connection"
        return 1
    fi
}

# Function to close SSE connection
close_sse_connection() {
    if [ -n "$SSE_PID" ] && ps -p $SSE_PID > /dev/null 2>&1; then
        print_status "Closing SSE connection..."
        kill $SSE_PID 2>/dev/null || true
        wait $SSE_PID 2>/dev/null || true
        print_success "SSE connection closed"
    fi
    rm -f sse_output.txt
}

# Main test function
run_tests() {
    print_status "Starting MCP Server Framework API Tests"
    print_status "Session ID: $SESSION_ID"
    echo
    
    # Test 1: Health Check
    test_endpoint "Health Check" "GET" "${BASE_URL}/health" "" "200"
    
    # Test 2: Establish SSE Connection
    if ! establish_sse_connection; then
        print_error "Cannot continue without SSE connection"
        return 1
    fi
    
    # Test 3: Initialize
    test_endpoint "Initialize" "POST" "${BASE_URL}/message?sessionId=${SESSION_ID}" '{
        "jsonrpc": "2.0",
        "id": 1,
        "method": "initialize",
        "params": {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {
                "name": "curl-client",
                "version": "1.0.0"
            }
        }
    }' "200"
    
    # Test 4: Initialized Notification
    test_endpoint "Initialized Notification" "POST" "${BASE_URL}/message?sessionId=${SESSION_ID}" '{
        "jsonrpc": "2.0",
        "method": "initialized",
        "params": {}
    }' "200"
    
    # Test 5: Tools List
    test_endpoint "Tools List" "POST" "${BASE_URL}/message?sessionId=${SESSION_ID}" '{
        "jsonrpc": "2.0",
        "id": 2,
        "method": "tools/list",
        "params": {}
    }' "200"
    
    # Test 6: Echo Tool Call
    test_endpoint "Echo Tool Call" "POST" "${BASE_URL}/message?sessionId=${SESSION_ID}" '{
        "jsonrpc": "2.0",
        "id": 3,
        "method": "tools/call",
        "params": {
            "name": "echo",
            "arguments": {
                "message": "Hello from curl!"
            }
        }
    }' "200"
    
    # Test 7: Invalid Tool Call (should return error)
    test_endpoint "Invalid Tool Call" "POST" "${BASE_URL}/message?sessionId=${SESSION_ID}" '{
        "jsonrpc": "2.0",
        "id": 4,
        "method": "tools/call",
        "params": {
            "name": "nonexistent",
            "arguments": {}
        }
    }' "200"
    
    # Test 8: Invalid Session ID
    test_endpoint "Invalid Session ID" "POST" "${BASE_URL}/message?sessionId=invalid" '{
        "jsonrpc": "2.0",
        "id": 5,
        "method": "tools/list",
        "params": {}
    }' "400"
    
    # Test 9: Invalid JSON
    test_endpoint "Invalid JSON" "POST" "${BASE_URL}/message?sessionId=${SESSION_ID}" '{invalid json}' "400"
    
    # Test 10: Invalid HTTP Method
    test_endpoint "Invalid HTTP Method" "PUT" "${BASE_URL}/message?sessionId=${SESSION_ID}" '{}' "405"
    
    # Close SSE connection
    close_sse_connection
}

# Function to run integration tests
run_integration_tests() {
    print_status "Running Integration Tests"
    echo
    
    # Check if Python SSE integration test exists
    if [ ! -f "scripts/test_sse_integration.py" ]; then
        print_error "Python SSE integration test not found"
        return 1
    fi
    
    # Run the Python SSE integration test
    print_status "Running Python SSE integration test..."
    if python3 scripts/test_sse_integration.py "$SERVER_PORT"; then
        print_success "All integration tests passed!"
        return 0
    else
        print_error "Integration tests failed"
        return 1
    fi

}

# Function to test concurrent connections
test_concurrent_connections() {
    print_status "Testing concurrent connections..."
    
    # Create multiple sessions and test them concurrently
    pids=()
    for i in {1..5}; do
        (
            session_id="concurrent-session-$i"
            response=$(curl -s -X POST "${BASE_URL}/message?sessionId=${session_id}" \
                -H "Content-Type: application/json" \
                -d '{
                    "jsonrpc": "2.0",
                    "id": 1,
                    "method": "tools/list",
                    "params": {}
                }')
            
            if echo "$response" | jq -e '.result.tools' > /dev/null 2>&1; then
                echo "Session $i: SUCCESS"
            else
                echo "Session $i: FAILED"
            fi
        ) &
        pids+=($!)
    done
    
    # Wait for all background processes
    for pid in "${pids[@]}"; do
        wait $pid
    done
    
    print_success "Concurrent connections test completed"
    echo
}

# Cleanup function
cleanup() {
    print_status "Cleaning up..."
    stop_server
    rm -f server.log sse_output.txt
}

# Trap to ensure cleanup on exit
trap cleanup EXIT

# Main execution
main() {
    # Check if jq is available
    if ! command -v jq &> /dev/null; then
        print_warning "jq is not installed. JSON responses will not be formatted."
    fi
    
    # Check if we're in the right directory
    if [ ! -f "go.mod" ]; then
        print_error "Please run this script from the project root directory"
        exit 1
    fi
    
    # Start server
    if ! start_server; then
        print_error "Failed to start server"
        exit 1
    fi
    
    # Run tests
    print_status "Running API endpoint tests..."
    echo
    run_tests
    
    print_status "Running integration tests..."
    echo
    run_integration_tests
    
    print_status "Running concurrent connection tests..."
    echo
    test_concurrent_connections
    
    print_success "All tests completed successfully!"
    print_status "Check server.log for server output"
}

# Cleanup function
cleanup() {
    print_status "Cleaning up..."
    
    # Close any remaining SSE connections
    if [ -n "$SSE_PID" ] && ps -p $SSE_PID > /dev/null 2>&1; then
        kill $SSE_PID 2>/dev/null || true
        wait $SSE_PID 2>/dev/null || true
    fi
    
    # Stop server
    stop_server
    
    # Clean up temp files
    rm -f sse_output.txt
}

# Set up trap for cleanup on exit
trap cleanup EXIT

# Run main function
main "$@"