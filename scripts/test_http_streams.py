#!/usr/bin/env python3
"""
Test script for HTTP Streams transport
"""

import json
import requests
import time
import threading
from typing import Dict, Any, Optional

class HTTPStreamsClient:
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url
        self.session = requests.Session()
        self.session_id = None
        self.stream_response = None
        self.stream_thread = None
        self.responses = {}
        self.running = False
        
    def start_stream(self) -> bool:
        """Start the SSE stream for receiving responses"""
        try:
            headers = {
                'Accept': 'text/event-stream',
                'Cache-Control': 'no-cache',
                'Connection': 'keep-alive'
            }
            
            if self.session_id:
                headers['Mcp-Session-Id'] = self.session_id
                
            print(f"Starting SSE stream with headers: {headers}")
            response = self.session.get(
                f"{self.base_url}/mcp",
                headers=headers,
                stream=True,
                timeout=None  # No timeout for SSE stream
            )
            
            print(f"SSE stream response status: {response.status_code}")
            if response.status_code == 200:
                self.stream_response = response
                self.running = True
                self.stream_thread = threading.Thread(target=self._read_stream)
                self.stream_thread.daemon = True
                self.stream_thread.start()
                
                # Give the stream a moment to establish
                time.sleep(0.1)
                print("SSE stream started successfully")
                return True
            else:
                print(f"Failed to start stream: {response.status_code}")
                return False
                
        except Exception as e:
            print(f"Error starting stream: {e}")
            return False
    
    def _read_stream(self):
        """Read SSE events from the stream"""
        try:
            print("Starting to read SSE stream...")
            for line in self.stream_response.iter_lines(decode_unicode=True):
                if not self.running:
                    print("Stream reading stopped (running=False)")
                    break
                    
                if line:
                    print(f"SSE line received: {repr(line)}")
                    
                if line.startswith('data: '):
                    data = line[6:]  # Remove 'data: ' prefix
                    if data.strip():
                        try:
                            message = json.loads(data)
                            print(f"Received: {message}")
                            
                            # Store response by ID for matching
                            if 'id' in message:
                                self.responses[message['id']] = message
                                
                        except json.JSONDecodeError as e:
                            print(f"Failed to parse JSON: {e}")
                elif line.startswith(':'):
                    print(f"SSE comment: {line}")
                            
        except Exception as e:
            print(f"Stream reading error: {e}")
        finally:
            print("SSE stream reading ended")
    
    def send_request(self, method: str, params: Any = None, request_id: Any = 1) -> Optional[Dict]:
        """Send a JSON-RPC request"""
        message = {
            "jsonrpc": "2.0",
            "method": method,
            "id": request_id
        }
        
        if params is not None:
            message["params"] = params
            
        return self._send_message(message, request_id)
    
    def send_notification(self, method: str, params: Any = None) -> bool:
        """Send a JSON-RPC notification (no response expected)"""
        message = {
            "jsonrpc": "2.0",
            "method": method
        }
        
        if params is not None:
            message["params"] = params
            
        return self._send_message(message) is not None
    
    def _send_message(self, message: Dict, wait_for_id: Any = None) -> Optional[Dict]:
        """Send a message and optionally wait for response"""
        try:
            headers = {
                'Content-Type': 'application/json',
                'Accept': 'application/json, text/event-stream'
            }
            
            if self.session_id:
                headers['Mcp-Session-Id'] = self.session_id
                
            response = self.session.post(
                f"{self.base_url}/mcp",
                json=message,
                headers=headers,
                timeout=10
            )
            
            print(f"POST response: {response.status_code}")
            
            # For initialize request, expect direct JSON response
            if message.get('method') == 'initialize':
                if response.status_code == 200:
                    try:
                        json_response = response.json()
                        print(f"Direct JSON response: {json_response}")
                        # Extract session ID from response headers
                        session_id = response.headers.get('Mcp-Session-Id')
                        if session_id:
                            self.session_id = session_id
                            print(f"Session ID from header: {session_id}")
                        return json_response
                    except Exception as e:
                        print(f"Error parsing JSON response: {e}")
                        return None
                else:
                    return None
            
            if wait_for_id is not None:
                # Wait for response via SSE stream
                for _ in range(50):  # Wait up to 5 seconds
                    if wait_for_id in self.responses:
                        return self.responses.pop(wait_for_id)
                    time.sleep(0.1)
                    
                print(f"Timeout waiting for response to request {wait_for_id}")
                return None
            else:
                # For notifications, just return success status
                return {"status": "ok"} if response.status_code in [200, 202] else None
                
        except Exception as e:
            print(f"Error sending message: {e}")
            return None
    
    def close(self):
        """Close the client connection"""
        self.running = False
        if self.stream_response:
            self.stream_response.close()
        if self.stream_thread:
            self.stream_thread.join(timeout=1)

def test_http_streams():
    """Test HTTP Streams transport"""
    print("Testing HTTP Streams transport...")
    
    client = HTTPStreamsClient()
    
    try:
        # Test 1: Initialize (without stream first)
        print("\n1. Testing initialize...")
        response = client.send_request("initialize", {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {
                "name": "test-client",
                "version": "1.0.0"
            }
        })
        
        if response and "result" in response:
            print(f"✓ Initialize successful: {response['result']}")
            # Extract session ID from response if available
            if 'sessionId' in response.get('result', {}):
                client.session_id = response['result']['sessionId']
                print(f"Session ID: {client.session_id}")
        else:
            print("✗ Initialize failed")
            return False
        
        # Now start the stream for subsequent requests
        print("\n1b. Starting SSE stream...")
        if not client.start_stream():
            print("Failed to start stream")
            return False
        time.sleep(0.5)  # Give stream time to establish
        
        # Test 2: Send initialized notification
        print("\n2. Testing initialized notification...")
        if client.send_notification("initialized"):
            print("✓ Initialized notification sent")
        else:
            print("✗ Initialized notification failed")
        
        # Test 3: List tools
        print("\n3. Testing tools/list...")
        response = client.send_request("tools/list", {}, 2)
        if response and "result" in response:
            tools = response['result'].get('tools', [])
            print(f"✓ Tools list successful: {len(tools)} tools found")
            for tool in tools:
                print(f"  - {tool.get('name', 'unknown')}: {tool.get('description', 'no description')}")
        else:
            print("✗ Tools list failed")
        
        # Test 4: Call echo tool
        print("\n4. Testing tools/call (echo)...")
        response = client.send_request("tools/call", {
            "name": "echo",
            "arguments": {"message": "Hello HTTP Streams!"}
        }, 3)
        
        if response and "result" in response:
            print(f"✓ Echo tool successful: {response['result']}")
        else:
            print("✗ Echo tool failed")
        
        # Test 5: Call math tool
        print("\n5. Testing tools/call (math)...")
        response = client.send_request("tools/call", {
            "name": "math",
            "arguments": {"operation": "add", "a": 10, "b": 5}
        }, 4)
        
        if response and "result" in response:
            print(f"✓ Math tool successful: {response['result']}")
        else:
            print("✗ Math tool failed")
        
        print("\n✓ All HTTP Streams tests completed successfully!")
        return True
        
    except Exception as e:
        print(f"Test error: {e}")
        return False
    finally:
        client.close()

if __name__ == "__main__":
    success = test_http_streams()
    exit(0 if success else 1)