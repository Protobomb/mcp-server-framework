#!/usr/bin/env python3
"""
HTTP Streams Integration Test Script for MCP Server Framework
This script tests the HTTP Streams transport with proper session management.
"""

import json
import requests
import time
import sys
from urllib.parse import urljoin

class HTTPStreamsClient:
    def __init__(self, base_url):
        self.base_url = base_url
        self.mcp_url = f"{base_url}/mcp"
        self.session_id = None
        self.session = requests.Session()
        
    def send_message(self, message):
        """Send a message via HTTP Streams and return the response"""
        try:
            headers = {'Content-Type': 'application/json'}
            if self.session_id:
                headers['Mcp-Session-Id'] = self.session_id
                
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
            
            # Handle empty responses (for notifications)
            if response.status_code == 204 or not response.text.strip():
                return None
            
            return response.json()
        except Exception as e:
            print(f"Error sending message: {e}")
            return None

def test_mcp_workflow(base_url=None):
    """Test complete MCP workflow via HTTP Streams"""
    if base_url is None:
        base_url = "http://localhost:8080"
    
    print(f"🧪 Starting HTTP Streams Integration Test")
    print(f"📡 Base URL: {base_url}")
    print(f"🔗 MCP Endpoint: {base_url}/mcp")
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
        print("\n📋 Test 1: Initialize")
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
        
        if response and response.get('result', {}).get('protocolVersion'):
            print(f"← Received: {json.dumps(response, indent=2)}")
            print("✓ Initialize successful")
        else:
            print(f"❌ Initialize failed: {response}")
            return False
        
        # Test 2: Initialized notification
        print("\n📋 Test 2: Initialized notification")
        initialized_message = {
            "jsonrpc": "2.0",
            "method": "notifications/initialized"
        }
        
        print(f"→ Sending: {json.dumps(initialized_message, indent=2)}")
        response = client.send_message(initialized_message)
        
        # For notifications, we expect an empty response or acknowledgment
        if response is None:
            print("← Received: No response (expected for notification)")
            print("✓ Initialized notification sent successfully")
        else:
            print(f"← Received: {json.dumps(response, indent=2)}")
            print("✓ Initialized notification sent successfully")
        
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
            print(f"← Received: {json.dumps(response, indent=2)}")
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
            print(f"← Received: {json.dumps(response, indent=2)}")
            print(f"✓ Tool call successful: {content}")
            
            # Verify the echo response
            expected_echo = "Echo: HTTP Streams integration test message"
            actual_echo = content[0]['text'] if content and len(content) > 0 else ""
            if actual_echo == expected_echo:
                print("✓ Echo response matches expected output")
            else:
                print(f"❌ Echo mismatch. Expected: '{expected_echo}', Got: '{actual_echo}'")
                return False
        else:
            print(f"❌ Tool call failed: {response}")
            return False
        
        print("\n🎉 All HTTP Streams tests passed!")
        return True
        
    except Exception as e:
        print(f"❌ Test failed with exception: {e}")
        return False

if __name__ == "__main__":
    import argparse
    
    parser = argparse.ArgumentParser(description="Test HTTP Streams transport")
    parser.add_argument("--base-url", default="http://localhost:8080", 
                       help="Base URL for the server")
    
    args = parser.parse_args()
    
    success = test_mcp_workflow(args.base_url)
    sys.exit(0 if success else 1)