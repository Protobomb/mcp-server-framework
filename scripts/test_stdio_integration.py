#!/usr/bin/env python3
"""
STDIO Integration Test Script for MCP Server Framework
This script tests the STDIO transport by launching the server process and communicating via stdin/stdout.
"""

import json
import subprocess
import threading
import time
import sys
import queue
import os

class STDIOClient:
    def __init__(self, command):
        self.command = command
        self.process = None
        self.stdout_queue = queue.Queue()
        self.stderr_queue = queue.Queue()
        self.stdout_thread = None
        self.stderr_thread = None
        
    def start(self):
        """Start the STDIO server process"""
        try:
            self.process = subprocess.Popen(
                self.command,
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=0  # Unbuffered
            )
            
            # Start threads to read stdout and stderr
            self.stdout_thread = threading.Thread(target=self._read_stdout)
            self.stderr_thread = threading.Thread(target=self._read_stderr)
            self.stdout_thread.daemon = True
            self.stderr_thread.daemon = True
            self.stdout_thread.start()
            self.stderr_thread.start()
            
            return True
        except Exception as e:
            print(f"Failed to start process: {e}")
            return False
    
    def _read_stdout(self):
        """Read stdout in a separate thread"""
        try:
            for line in iter(self.process.stdout.readline, ''):
                if line.strip():
                    self.stdout_queue.put(line.strip())
        except Exception as e:
            print(f"Error reading stdout: {e}")
    
    def _read_stderr(self):
        """Read stderr in a separate thread"""
        try:
            for line in iter(self.process.stderr.readline, ''):
                if line.strip():
                    self.stderr_queue.put(line.strip())
        except Exception as e:
            print(f"Error reading stderr: {e}")
    
    def send_message(self, message):
        """Send a JSON-RPC message to the server"""
        try:
            json_str = json.dumps(message)
            print(f"→ Sending: {json_str}")
            self.process.stdin.write(json_str + '\n')
            self.process.stdin.flush()
            return True
        except Exception as e:
            print(f"Error sending message: {e}")
            return False
    
    def wait_for_response(self, timeout=5):
        """Wait for a response from the server"""
        start_time = time.time()
        
        while time.time() - start_time < timeout:
            try:
                # Check for stdout (JSON responses)
                response_line = self.stdout_queue.get(timeout=0.1)
                try:
                    response = json.loads(response_line)
                    print(f"← Received: {json.dumps(response, indent=2)}")
                    return response
                except json.JSONDecodeError:
                    print(f"Non-JSON stdout: {response_line}")
                    continue
            except queue.Empty:
                # Check for stderr (debug/error messages)
                try:
                    stderr_line = self.stderr_queue.get_nowait()
                    print(f"🔍 Server log: {stderr_line}")
                except queue.Empty:
                    pass
                continue
        
        print(f"⏰ Timeout waiting for response")
        return None
    
    def stop(self):
        """Stop the server process"""
        if self.process:
            try:
                self.process.terminate()
                self.process.wait(timeout=5)
            except subprocess.TimeoutExpired:
                self.process.kill()
                self.process.wait()

def test_mcp_workflow(server_binary=None):
    """Test complete MCP workflow via STDIO"""
    if server_binary is None:
        server_binary = "./mcp-server"
    
    # Check if binary exists
    if not os.path.exists(server_binary):
        print(f"❌ Server binary not found: {server_binary}")
        return False
    
    command = [server_binary, "-transport=stdio"]
    
    print(f"🧪 Starting STDIO Integration Test")
    print(f"📡 Command: {' '.join(command)}")
    print()
    
    # Create STDIO client
    client = STDIOClient(command)
    
    try:
        # Start the server process
        print("🚀 Starting STDIO server...")
        if not client.start():
            print("❌ Failed to start STDIO server")
            return False
        
        # Give the server a moment to start
        time.sleep(0.5)
        
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
                    "name": "stdio-integration-test",
                    "version": "1.0.0"
                }
            }
        }
        
        if not client.send_message(init_message):
            print("❌ Failed to send initialize message")
            return False
        
        response = client.wait_for_response(timeout=5)
        if response and response.get('result', {}).get('protocolVersion'):
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
        
        if not client.send_message(initialized_message):
            print("❌ Failed to send initialized notification")
            return False
        
        # For notifications, we don't expect a JSON response, just check logs
        time.sleep(0.5)
        print("✓ Initialized notification sent successfully")
        
        # Test 3: List tools
        print("\n📋 Test 3: List tools")
        tools_message = {
            "jsonrpc": "2.0",
            "id": 2,
            "method": "tools/list"
        }
        
        if not client.send_message(tools_message):
            print("❌ Failed to send tools/list message")
            return False
        
        response = client.wait_for_response(timeout=5)
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
                    "message": "STDIO integration test message"
                }
            }
        }
        
        if not client.send_message(call_message):
            print("❌ Failed to send tools/call message")
            return False
        
        response = client.wait_for_response(timeout=5)
        if response and response.get('result', {}).get('content'):
            content = response['result']['content']
            print(f"✓ Tool call successful: {content}")
            
            # Verify the echo response
            expected_echo = "Echo: STDIO integration test message"
            actual_echo = content[0]['text'] if content and len(content) > 0 else ""
            if actual_echo == expected_echo:
                print("✓ Echo response matches expected output")
            else:
                print(f"❌ Echo mismatch. Expected: '{expected_echo}', Got: '{actual_echo}'")
                return False
        else:
            print(f"❌ Tool call failed: {response}")
            return False
        
        print("\n🎉 All STDIO tests passed!")
        return True
        
    except Exception as e:
        print(f"❌ Test failed with exception: {e}")
        return False
    finally:
        client.stop()

def main():
    """Main test function - runs STDIO transport test"""
    import argparse
    
    parser = argparse.ArgumentParser(description="Test STDIO transport")
    parser.add_argument("--server-binary", default="./mcp-server", 
                       help="Path to the server binary")
    
    args = parser.parse_args()
    
    print("🧪 Starting STDIO Transport Integration Test")
    print(f"📡 Testing with binary: {args.server_binary}")
    
    # Build the server first
    print("🔨 Building MCP server...")
    try:
        build_result = subprocess.run(['make', 'build'], 
                                    cwd='/workspace/mcp-server-framework',
                                    capture_output=True, text=True, timeout=30)
        if build_result.returncode != 0:
            print(f"❌ Build failed: {build_result.stderr}")
            sys.exit(1)
        print("✓ Build successful")
    except Exception as e:
        print(f"❌ Build failed: {e}")
        sys.exit(1)
    
    success = test_mcp_workflow(args.server_binary)
    
    if success:
        print("\n🎉 STDIO integration test PASSED!")
        sys.exit(0)
    else:
        print("\n❌ STDIO integration test FAILED!")
        sys.exit(1)

if __name__ == "__main__":
    main()