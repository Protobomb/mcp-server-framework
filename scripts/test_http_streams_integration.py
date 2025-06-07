#!/usr/bin/env python3
"""
HTTP Streams Integration Test Script for MCP Server Framework
This script tests the HTTP Streams transport with proper SSE stream handling.
"""

import json
import requests
import time
import sys
import threading
import subprocess
import signal
import os
from urllib.parse import urljoin

class HTTPStreamsClient:
    def __init__(self, base_url):
        self.base_url = base_url
        self.mcp_url = f"{base_url}/mcp"
        self.sse_url = f"{base_url}/mcp"  # Same endpoint for SSE streams
        self.session_id = None
        self.session = requests.Session()
        self.responses = {}
        self.running = False
        self.sse_thread = None
        self.stop_event = threading.Event()
        
    def start_sse_stream(self):
        """Start the SSE stream for receiving responses"""
        if not self.session_id:
            print("❌ Cannot start SSE stream without session ID")
            return False
            
        try:
            self.sse_thread = threading.Thread(target=self._listen_sse)
            self.sse_thread.daemon = True
            self.sse_thread.start()
            time.sleep(0.5)  # Give stream time to establish
            return self.running
        except Exception as e:
            print(f"Error starting SSE stream: {e}")
            return False
    
    def _listen_sse(self):
        """Listen for SSE events"""
        try:
            headers = {
                'Accept': 'text/event-stream',
                'Cache-Control': 'no-cache',
                'Connection': 'keep-alive',
                'Mcp-Session-Id': self.session_id
            }
            
            response = self.session.get(self.sse_url, headers=headers, stream=True, timeout=None)
            
            if response.status_code == 200:
                self.running = True
                print(f"✓ SSE stream established for session {self.session_id}")
                
                for line in response.iter_lines(decode_unicode=True):
                    if self.stop_event.is_set():
                        break
                        
                    if line and line.startswith('data: '):
                        data = line[6:]  # Remove 'data: ' prefix
                        if data.strip() and not data.startswith(':'):
                            try:
                                message = json.loads(data)
                                if 'id' in message:
                                    self.responses[message['id']] = message
                                    print(f"← SSE Response: {json.dumps(message, indent=2)}")
                            except json.JSONDecodeError:
                                pass
            else:
                print(f"❌ SSE stream failed with status {response.status_code}")
                                
        except Exception as e:
            print(f"SSE stream error: {e}")
        finally:
            self.running = False
    
    def send_message(self, message, wait_for_response=True):
        """Send a message to the MCP server"""
        try:
            headers = {"Content-Type": "application/json"}
            
            # Add session ID header if we have one
            if self.session_id:
                headers["Mcp-Session-Id"] = self.session_id
            
            response = self.session.post(
                self.mcp_url,
                json=message,
                headers=headers,
                timeout=10
            )
            response.raise_for_status()
            
            # Extract session ID from response headers if present
            if 'Mcp-Session-Id' in response.headers:
                self.session_id = response.headers['Mcp-Session-Id']
                print(f"📝 Session ID: {self.session_id}")
            
            # For initialize, return direct response
            if message.get('method') == 'initialize':
                return response.json()
            
            # For other messages, wait for response via SSE if requested
            if wait_for_response and 'id' in message:
                request_id = message['id']
                # Wait for response via SSE stream
                for _ in range(50):  # Wait up to 5 seconds
                    if request_id in self.responses:
                        return self.responses.pop(request_id)
                    time.sleep(0.1)
                print(f"❌ Timeout waiting for response with ID {request_id}")
                return None
            
            # Handle empty responses (for notifications)
            if response.status_code == 204 or not response.text.strip():
                return None
            
            return response.json()
        except Exception as e:
            print(f"Error sending message: {e}")
            return None
    
    def close(self):
        """Close the client connection"""
        self.stop_event.set()
        if self.sse_thread and self.sse_thread.is_alive():
            self.sse_thread.join(timeout=1)

def test_mcp_workflow(base_url=None):
    """Test complete MCP workflow via HTTP Streams"""
    if base_url is None:
        base_url = "http://localhost:8080"
    
    print(f"🧪 Starting HTTP Streams Integration Test")
    print(f"📡 Base URL: {base_url}")
    print(f"🔗 MCP Endpoint: {base_url}/mcp")
    print(f"📡 SSE Endpoint: {base_url}/mcp (GET)")
    print()
    
    # Test health endpoint first
    try:
        health_response = requests.get(f"{base_url}/health", timeout=5)
        health_response.raise_for_status()
        health_data = health_response.json()
        print(f"✓ Health check: {health_data}")
    except Exception as e:
        print(f"❌ Health check failed: {e}")
        return False
    
    # Create HTTP Streams client
    client = HTTPStreamsClient(base_url)
    
    try:
        # Test 1: Initialize
        print("\n🚀 Test 1: Initialize")
        init_message = {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {
                    "name": "http-streams-integration-test",
                    "version": "1.0.0"
                }
            }
        }
        
        print(f"→ Sending: {json.dumps(init_message, indent=2)}")
        response = client.send_message(init_message)
        
        if response and response.get('result'):
            print(f"← Received: {json.dumps(response, indent=2)}")
            print(f"✓ Initialize successful")
        else:
            print(f"❌ Initialize failed: {response}")
            return False
        
        # Test 2: Start SSE stream
        print("\n📡 Test 2: Start SSE stream")
        if not client.start_sse_stream():
            print("❌ Failed to start SSE stream")
            return False
        
        # Test 3: List tools
        print("\n📋 Test 3: List tools")
        tools_message = {
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/list"
        }
        
        print(f"→ Sending: {json.dumps(tools_message, indent=2)}")
        response = client.send_message(tools_message)
        
        if response and response.get('result', {}).get('tools'):
            tools = response['result']['tools']
            print(f"✓ Tools list successful: {[tool['name'] for tool in tools]}")
        else:
            print(f"❌ Tools list failed: {response}")
            return False
        
        # Test 4: Call echo tool
        print("\n📋 Test 4: Call echo tool")
        call_message = {
            "jsonrpc": "2.0",
            "id": 3,
            "method": "tools/call",
            "params": {
                "name": "echo",
                "arguments": {
                    "message": "HTTP Streams integration test message"
                }
            }
        }
        
        print(f"→ Sending: {json.dumps(call_message, indent=2)}")
        response = client.send_message(call_message)
        
        if response and response.get('result', {}).get('content'):
            content = response['result']['content']
            print(f"✓ Tool call successful: {content}")
            
            # Verify the echo response
            expected_echo = "Echo: HTTP Streams integration test message"
            actual_echo = content[0]['text'] if content and len(content) > 0 else ""
            
            if actual_echo == expected_echo:
                print(f"✓ Echo response verified: '{actual_echo}'")
                return True
            else:
                print(f"❌ Echo response mismatch. Expected: '{expected_echo}', Got: '{actual_echo}'")
                return False
        else:
            print(f"❌ Tool call failed: {response}")
            return False
            
    finally:
        client.close()

def start_server(port=8080):
    """Start the MCP server for testing"""
    try:
        # Build the server first
        print("🔨 Building MCP server...")
        build_result = subprocess.run(['make', 'build'], 
                                    cwd='/workspace/mcp-server-framework',
                                    capture_output=True, text=True, timeout=30)
        if build_result.returncode != 0:
            print(f"❌ Build failed: {build_result.stderr}")
            return None
        
        print("✓ Build successful")
        
        # Start the server
        print(f"🚀 Starting HTTP Streams server on port {port}...")
        server_process = subprocess.Popen([
            './mcp-server', 
            '-transport=http-streams', 
            f'-addr={port}',
            '-debug'
        ], cwd='/workspace/mcp-server-framework')
        
        # Wait for server to start
        time.sleep(2)
        
        # Check if server is running
        try:
            health_response = requests.get(f"http://localhost:{port}/health", timeout=5)
            if health_response.status_code == 200:
                print(f"✓ Server started successfully on port {port}")
                return server_process
            else:
                print(f"❌ Server health check failed")
                server_process.terminate()
                return None
        except Exception as e:
            print(f"❌ Server not responding: {e}")
            server_process.terminate()
            return None
            
    except Exception as e:
        print(f"❌ Failed to start server: {e}")
        return None

def main():
    """Main test function"""
    port = 8080
    if len(sys.argv) > 1:
        try:
            port = int(sys.argv[1])
        except ValueError:
            print("Invalid port number")
            sys.exit(1)
    
    base_url = f"http://localhost:{port}"
    
    # Check if server is already running
    try:
        health_response = requests.get(f"{base_url}/health", timeout=2)
        if health_response.status_code == 200:
            print(f"✓ Server already running on port {port}")
            server_process = None
        else:
            server_process = start_server(port)
            if not server_process:
                sys.exit(1)
    except:
        server_process = start_server(port)
        if not server_process:
            sys.exit(1)
    
    try:
        # Run the test
        success = test_mcp_workflow(base_url)
        
        if success:
            print("\n🎉 HTTP Streams integration test PASSED!")
            sys.exit(0)
        else:
            print("\n❌ HTTP Streams integration test FAILED!")
            sys.exit(1)
            
    finally:
        if server_process:
            print("\n🛑 Stopping server...")
            server_process.terminate()
            try:
                server_process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                server_process.kill()

if __name__ == "__main__":
    main()