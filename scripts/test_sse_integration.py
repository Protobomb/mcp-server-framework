#!/usr/bin/env python3
"""
SSE Integration Test Script for MCP Server Framework
This script properly handles SSE connections and MCP protocol testing.
"""

import json
import requests
import threading
import time
import sys
from urllib.parse import urljoin

class SSEClient:
    def __init__(self, base_url, session_id):
        self.base_url = base_url
        self.session_id = session_id
        self.sse_url = f"{base_url}/sse?sessionId={session_id}"
        self.message_url = f"{base_url}/message?sessionId={session_id}"
        self.responses = []
        self.connected = False
        self.stop_event = threading.Event()
        self.sse_thread = None
        
    def connect(self):
        """Connect to SSE endpoint and start listening for messages"""
        self.sse_thread = threading.Thread(target=self._listen_sse)
        self.sse_thread.daemon = True
        self.sse_thread.start()
        
        # Wait for connection
        for _ in range(50):  # 5 seconds timeout
            if self.connected:
                return True
            time.sleep(0.1)
        return False
    
    def _listen_sse(self):
        """Listen to SSE stream"""
        try:
            response = requests.get(
                self.sse_url,
                headers={'Accept': 'text/event-stream'},
                stream=True,
                timeout=30
            )
            response.raise_for_status()
            
            for line in response.iter_lines(decode_unicode=True):
                if self.stop_event.is_set():
                    break
                    
                if line.startswith('data: '):
                    data = line[6:]  # Remove 'data: ' prefix
                    try:
                        message = json.loads(data)
                        if message.get('type') == 'connected':
                            self.connected = True
                            print(f"✓ SSE connected with session: {message.get('sessionId')}")
                        else:
                            self.responses.append(message)
                            print(f"← Received: {json.dumps(message, indent=2)}")
                    except json.JSONDecodeError:
                        print(f"Invalid JSON in SSE data: {data}")
                        
        except Exception as e:
            print(f"SSE connection error: {e}")
    
    def send_message(self, message):
        """Send a message via POST and return the HTTP response"""
        try:
            response = requests.post(
                self.message_url,
                json=message,
                headers={'Content-Type': 'application/json'},
                timeout=10
            )
            response.raise_for_status()
            return response.json()
        except Exception as e:
            print(f"Error sending message: {e}")
            return None
    
    def wait_for_response(self, request_id=None, timeout=5):
        """Wait for a response from SSE stream"""
        start_time = time.time()
        
        while time.time() - start_time < timeout:
            # Look for response with matching ID
            for i, response in enumerate(self.responses):
                if request_id is None or response.get('id') == request_id:
                    # Remove the response from the list to avoid reusing it
                    return self.responses.pop(i)
            time.sleep(0.1)
        return None
    
    def disconnect(self):
        """Disconnect from SSE"""
        self.stop_event.set()
        if self.sse_thread:
            self.sse_thread.join(timeout=2)

def test_mcp_workflow():
    """Test complete MCP workflow"""
    port = sys.argv[1] if len(sys.argv) > 1 else "8080"
    base_url = f"http://localhost:{port}"
    session_id = f"test-session-{int(time.time())}"
    
    print(f"🧪 Starting MCP Integration Test")
    print(f"📡 Base URL: {base_url}")
    print(f"🆔 Session ID: {session_id}")
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
    
    # Create SSE client
    client = SSEClient(base_url, session_id)
    
    try:
        # Connect to SSE
        print("🔌 Connecting to SSE...")
        if not client.connect():
            print("❌ Failed to connect to SSE")
            return False
        
        # Test 1: Initialize
        print("\n📋 Test 1: Initialize")
        init_message = {
            "jsonrpc": "2.0",
            "id": 1,
            "method": "initialize",
            "params": {
                "protocolVersion": "2024-11-05",
                "capabilities": {},
                "clientInfo": {
                    "name": "integration-test",
                    "version": "1.0.0"
                }
            }
        }
        
        print(f"→ Sending: {json.dumps(init_message, indent=2)}")
        post_response = client.send_message(init_message)
        print(f"📤 POST response: {post_response}")
        
        # Wait for MCP response via SSE
        mcp_response = client.wait_for_response(request_id=1, timeout=5)
        if mcp_response and mcp_response.get('result', {}).get('protocolVersion'):
            print("✓ Initialize successful")
        else:
            print(f"❌ Initialize failed: {mcp_response}")
            return False
        
        # Test 2: Initialized notification
        print("\n📋 Test 2: Initialized notification")
        initialized_message = {
            "jsonrpc": "2.0",
            "method": "initialized"
        }
        
        print(f"→ Sending: {json.dumps(initialized_message, indent=2)}")
        post_response = client.send_message(initialized_message)
        print(f"📤 POST response: {post_response}")
        
        # For notifications, we don't expect a response, just check if POST was successful
        if post_response and post_response.get('status') == 'ok':
            print("✓ Initialized notification sent successfully")
        else:
            print(f"❌ Initialized notification failed: {post_response}")
        
        # Small delay to let any responses arrive
        time.sleep(0.5)
        
        # Test 3: List tools
        print("\n📋 Test 3: List tools")
        tools_message = {
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/list"
        }
        
        print(f"→ Sending: {json.dumps(tools_message, indent=2)}")
        post_response = client.send_message(tools_message)
        print(f"📤 POST response: {post_response}")
        
        # Wait for MCP response via SSE
        mcp_response = client.wait_for_response(request_id=2, timeout=5)
        if mcp_response and mcp_response.get('result', {}).get('tools'):
            tools = mcp_response['result']['tools']
            print(f"✓ Tools list successful: {[tool['name'] for tool in tools]}")
        else:
            print(f"❌ Tools list failed: {mcp_response}")
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
                    "message": "Integration test message"
                }
            }
        }
        
        print(f"→ Sending: {json.dumps(call_message, indent=2)}")
        post_response = client.send_message(call_message)
        print(f"📤 POST response: {post_response}")
        
        # Wait for MCP response via SSE
        mcp_response = client.wait_for_response(request_id=3, timeout=5)
        if mcp_response and mcp_response.get('result', {}).get('content'):
            content = mcp_response['result']['content']
            print(f"✓ Tool call successful: {content}")
        else:
            print(f"❌ Tool call failed: {mcp_response}")
            return False
        
        print("\n🎉 All tests passed!")
        return True
        
    except Exception as e:
        print(f"❌ Test failed with exception: {e}")
        return False
    finally:
        client.disconnect()

if __name__ == "__main__":
    success = test_mcp_workflow()
    sys.exit(0 if success else 1)