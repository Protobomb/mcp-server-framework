#!/usr/bin/env python3
"""
Comprehensive test script for all MCP transport protocols
Tests STDIO, SSE, and HTTP Streams transports
"""

import json
import requests
import subprocess
import threading
import time
import sys
import argparse
from typing import Dict, Any, Optional, List
from concurrent.futures import ThreadPoolExecutor, as_completed

class STDIOClient:
    """Client for testing STDIO transport"""
    
    def __init__(self, server_path: str = "./mcp-server"):
        self.server_path = server_path
        self.process = None
        self.responses = {}
        self.running = False
        
    def start(self) -> bool:
        """Start the STDIO server process"""
        try:
            self.process = subprocess.Popen(
                [self.server_path, "--transport", "stdio", "--debug"],
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                bufsize=0
            )
            self.running = True
            
            # Start reading responses
            self.read_thread = threading.Thread(target=self._read_responses)
            self.read_thread.daemon = True
            self.read_thread.start()
            
            time.sleep(0.5)  # Give process time to start
            return True
            
        except Exception as e:
            print(f"Error starting STDIO server: {e}")
            return False
    
    def _read_responses(self):
        """Read responses from stdout"""
        while self.running and self.process:
            try:
                line = self.process.stdout.readline()
                if not line:
                    break
                    
                line = line.strip()
                if line:
                    try:
                        response = json.loads(line)
                        if 'id' in response:
                            self.responses[response['id']] = response
                    except json.JSONDecodeError:
                        pass  # Ignore non-JSON lines
                        
            except Exception as e:
                print(f"Error reading STDIO response: {e}")
                break
    
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
        """Send a JSON-RPC notification"""
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
            if not self.process or not self.running:
                return None
                
            json_str = json.dumps(message)
            self.process.stdin.write(json_str + "\n")
            self.process.stdin.flush()
            
            if wait_for_id is not None:
                # Wait for response
                for _ in range(50):  # Wait up to 5 seconds
                    if wait_for_id in self.responses:
                        return self.responses.pop(wait_for_id)
                    time.sleep(0.1)
                return None
            else:
                return {"status": "ok"}
                
        except Exception as e:
            print(f"Error sending STDIO message: {e}")
            return None
    
    def close(self):
        """Close the client connection"""
        self.running = False
        if self.process:
            self.process.terminate()
            self.process.wait(timeout=5)


class SSEClient:
    """Client for testing SSE transport"""
    
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url
        self.session = requests.Session()
        self.session_id = f"test-session-{int(time.time())}"
        self.sse_url = f"{base_url}/sse?sessionId={self.session_id}"
        self.message_url = f"{base_url}/message?sessionId={self.session_id}"
        self.responses = {}
        self.running = False
        self.sse_thread = None
        self.stop_event = threading.Event()
        
    def start(self) -> bool:
        """Start the SSE connection"""
        try:
            # First check health
            health_response = self.session.get(f"{self.base_url}/health", timeout=5)
            if health_response.status_code != 200:
                return False
                
            # Start SSE stream
            self.sse_thread = threading.Thread(target=self._listen_sse)
            self.sse_thread.daemon = True
            self.sse_thread.start()
            
            time.sleep(0.5)  # Give stream time to establish
            return self.running
                
        except Exception as e:
            print(f"Error starting SSE client: {e}")
            return False
    
    def _listen_sse(self):
        """Listen for SSE events"""
        try:
            headers = {
                'Accept': 'text/event-stream',
                'Cache-Control': 'no-cache',
                'Connection': 'keep-alive'
            }
            
            response = self.session.get(self.sse_url, headers=headers, stream=True, timeout=None)
            
            if response.status_code == 200:
                self.running = True
                
                for line in response.iter_lines(decode_unicode=True):
                    if self.stop_event.is_set():
                        break
                        
                    if line and line.startswith('data: '):
                        data = line[6:]  # Remove 'data: ' prefix
                        if data.strip() and not data.startswith('/message'):
                            try:
                                message = json.loads(data)
                                if 'id' in message:
                                    self.responses[message['id']] = message
                            except json.JSONDecodeError:
                                pass
                                
        except Exception as e:
            print(f"SSE stream reading error: {e}")
        finally:
            self.running = False
    
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
        """Send a JSON-RPC notification"""
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
            response = self.session.post(
                self.message_url,
                json=message,
                timeout=10
            )
            
            if response.status_code not in [200, 202]:
                print(f"POST failed with status {response.status_code}: {response.text}")
                return None
            
            if wait_for_id is not None:
                # Wait for response via SSE stream
                for _ in range(50):  # Wait up to 5 seconds
                    if wait_for_id in self.responses:
                        return self.responses.pop(wait_for_id)
                    time.sleep(0.1)
                print(f"Timeout waiting for response with ID {wait_for_id}")
                return None
            else:
                return {"status": "accepted"}
                
        except Exception as e:
            print(f"Error sending SSE message: {e}")
            return None
    
    def close(self):
        """Close the client connection"""
        self.stop_event.set()
        self.running = False
        if self.sse_thread:
            self.sse_thread.join(timeout=1)


class HTTPStreamsClient:
    """Client for testing HTTP Streams transport"""
    
    def __init__(self, base_url: str = "http://localhost:8080"):
        self.base_url = base_url
        self.session = requests.Session()
        self.stream_response = None
        self.stream_thread = None
        self.responses = {}
        self.running = False
        self.session_id = None
        self.initialized = False
        
    def start(self) -> bool:
        """Start the HTTP Streams connection"""
        try:
            # First, send initialize request to get session ID
            init_message = {
                "jsonrpc": "2.0",
                "method": "initialize",
                "id": 1,
                "params": {
                    "protocolVersion": "2024-11-05",
                    "capabilities": {
                        "roots": {"listChanged": True},
                        "sampling": {}
                    },
                    "clientInfo": {
                        "name": "test-client",
                        "version": "1.0.0"
                    }
                }
            }
            
            response = self.session.post(
                f"{self.base_url}/mcp",
                json=init_message,
                timeout=10
            )
            
            if response.status_code != 200:
                print(f"Initialize failed with status {response.status_code}: {response.text}")
                return False
            
            init_response = response.json()
            
            # Check for session ID in response headers (HTTP Streams) or body (SSE)
            if 'Mcp-Session-Id' in response.headers:
                self.session_id = response.headers['Mcp-Session-Id']
                print(f"Got session ID from header: {self.session_id}")
            elif 'result' in init_response and 'sessionId' in init_response['result']:
                self.session_id = init_response['result']['sessionId']
                print(f"Got session ID from body: {self.session_id}")
            else:
                print(f"No session ID in initialize response: {init_response}")
                return False
                
            # Now start SSE stream with session ID
            headers = {
                'Accept': 'text/event-stream',
                'Cache-Control': 'no-cache',
                'Connection': 'keep-alive',
                'Mcp-Session-Id': self.session_id
            }
            
            stream_response = self.session.get(
                f"{self.base_url}/mcp",
                headers=headers,
                stream=True,
                timeout=None
            )
            
            if stream_response.status_code == 200:
                self.stream_response = stream_response
                self.running = True
                self.stream_thread = threading.Thread(target=self._read_stream)
                self.stream_thread.daemon = True
                self.stream_thread.start()
                
                time.sleep(0.5)  # Give stream time to establish
                self.initialized = True
                return True
            else:
                print(f"SSE stream failed with status {stream_response.status_code}")
                return False
                
        except Exception as e:
            print(f"Error starting HTTP Streams client: {e}")
            return False
    
    def _read_stream(self):
        """Read stream data"""
        try:
            for line in self.stream_response.iter_lines(decode_unicode=True):
                if not self.running:
                    break
                    
                if line and line.startswith('data: '):
                    data = line[6:]  # Remove 'data: ' prefix
                    if data.strip():
                        try:
                            message = json.loads(data)
                            if 'id' in message:
                                self.responses[message['id']] = message
                        except json.JSONDecodeError:
                            pass
                            
        except Exception as e:
            print(f"HTTP Streams reading error: {e}")
    
    def send_request(self, method: str, params: Any = None, request_id: Any = 1) -> Optional[Dict]:
        """Send a JSON-RPC request"""
        # Initialize request is already handled in start(), return mock response
        if method == "initialize" and self.initialized:
            return {"result": {"protocolVersion": "2024-11-05", "capabilities": {}}}
        
        message = {
            "jsonrpc": "2.0",
            "method": method,
            "id": request_id
        }
        
        if params is not None:
            message["params"] = params
            
        return self._send_message(message, request_id)
    
    def send_notification(self, method: str, params: Any = None) -> bool:
        """Send a JSON-RPC notification"""
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
                'Mcp-Session-Id': self.session_id
            }
            
            response = self.session.post(
                f"{self.base_url}/mcp",
                json=message,
                headers=headers,
                timeout=10
            )
            
            if response.status_code not in [200, 202]:
                print(f"POST failed with status {response.status_code}: {response.text}")
                return None
            
            if wait_for_id is not None:
                # Wait for response via stream
                for _ in range(50):  # Wait up to 5 seconds
                    if wait_for_id in self.responses:
                        return self.responses.pop(wait_for_id)
                    time.sleep(0.1)
                print(f"Timeout waiting for response with ID {wait_for_id}")
                return None
            else:
                return {"status": "accepted"}
                
        except Exception as e:
            print(f"Error sending HTTP Streams message: {e}")
            return None
    
    def close(self):
        """Close the client connection"""
        self.running = False
        if self.stream_response:
            try:
                self.stream_response.close()
            except Exception:
                pass  # Ignore cleanup errors
        if self.stream_thread:
            self.stream_thread.join(timeout=1)


def run_transport_tests(transport: str, port: int = 8080) -> bool:
    """Run tests for a specific transport"""
    print(f"\n{'='*60}")
    print(f"🧪 Testing {transport.upper()} Transport")
    print(f"{'='*60}")
    
    # Create appropriate client
    if transport == "stdio":
        client = STDIOClient()
    elif transport == "sse":
        client = SSEClient(f"http://localhost:{port}")
    elif transport == "http-streams":
        client = HTTPStreamsClient(f"http://localhost:{port}")
    else:
        print(f"❌ Unknown transport: {transport}")
        return False
    
    try:
        # Start client
        print(f"🔌 Starting {transport} client...")
        if not client.start():
            print(f"❌ Failed to start {transport} client")
            return False
        
        print(f"✅ {transport} client started successfully")
        
        # Test 1: Initialize
        print(f"\n📋 Test 1: Initialize")
        response = client.send_request("initialize", {
            "protocolVersion": "2024-11-05",
            "capabilities": {},
            "clientInfo": {
                "name": f"test-client-{transport}",
                "version": "1.0.0"
            }
        }, 1)
        
        if response and "result" in response:
            print(f"✅ Initialize successful")
            print(f"   Server: {response['result'].get('serverInfo', {}).get('name', 'unknown')}")
            print(f"   Version: {response['result'].get('serverInfo', {}).get('version', 'unknown')}")
        else:
            print(f"❌ Initialize failed")
            return False
        
        # Test 2: Initialized notification
        print(f"\n📋 Test 2: Initialized notification")
        if client.send_notification("initialized"):
            print(f"✅ Initialized notification sent")
        else:
            print(f"❌ Initialized notification failed")
        
        # Test 3: List tools
        print(f"\n📋 Test 3: List tools")
        response = client.send_request("tools/list", {}, 2)
        if response and "result" in response:
            tools = response['result'].get('tools', [])
            print(f"✅ Tools list successful: {len(tools)} tools found")
            for tool in tools:
                print(f"   - {tool.get('name', 'unknown')}: {tool.get('description', 'no description')}")
        else:
            print(f"❌ Tools list failed")
            return False
        
        # Test 4: Call echo tool
        print(f"\n📋 Test 4: Call echo tool")
        response = client.send_request("tools/call", {
            "name": "echo",
            "arguments": {"message": f"Hello from {transport}!"}
        }, 3)
        
        if response and "result" in response:
            content = response['result'].get('content', [])
            if content and len(content) > 0:
                print(f"✅ Echo tool successful: {content[0].get('text', 'no text')}")
            else:
                print(f"✅ Echo tool successful: {response['result']}")
        else:
            print(f"❌ Echo tool failed")
            return False
        
        print(f"\n🎉 All {transport} tests passed!")
        return True
        
    except Exception as e:
        print(f"❌ Test error for {transport}: {e}")
        return False
    finally:
        client.close()


def start_server(transport: str, port: int) -> subprocess.Popen:
    """Start a server for the given transport"""
    if transport == "stdio":
        return None  # STDIO doesn't need a separate server process
    
    cmd = ["./mcp-server", "--transport", transport, "--addr", f":{port}", "--debug"]
    
    try:
        process = subprocess.Popen(
            cmd,
            stdout=subprocess.PIPE,
            stderr=subprocess.PIPE,
            text=True
        )
        time.sleep(2)  # Give server time to start
        return process
    except Exception as e:
        print(f"Error starting {transport} server: {e}")
        return None


def test_single_transport(transport: str, port: int = 8080) -> Dict[str, Any]:
    """Test a single transport and return results"""
    result = {
        "transport": transport,
        "success": False,
        "error": None,
        "port": port
    }
    
    server_process = None
    
    try:
        # Start server if needed
        if transport != "stdio":
            print(f"🚀 Starting {transport} server on port {port}...")
            server_process = start_server(transport, port)
            if not server_process:
                result["error"] = f"Failed to start {transport} server"
                return result
        
        # Run tests
        result["success"] = run_transport_tests(transport, port)
        
    except Exception as e:
        result["error"] = str(e)
    finally:
        # Clean up server
        if server_process:
            server_process.terminate()
            try:
                server_process.wait(timeout=2)
            except subprocess.TimeoutExpired:
                server_process.kill()
    
    return result


def test_all_transports_parallel():
    """Test all transports in parallel"""
    print("🚀 Starting comprehensive MCP transport testing...")
    print("Testing STDIO, SSE, and HTTP Streams transports in parallel")
    
    # Define transports and their ports
    transports = [
        ("stdio", 0),  # STDIO doesn't use a port
        ("sse", 8081),
        ("http-streams", 8082)
    ]
    
    results = []
    
    # Run tests in parallel
    with ThreadPoolExecutor(max_workers=3) as executor:
        # Submit all test jobs
        future_to_transport = {
            executor.submit(test_single_transport, transport, port): transport
            for transport, port in transports
        }
        
        # Collect results as they complete
        for future in as_completed(future_to_transport):
            transport = future_to_transport[future]
            try:
                result = future.result()
                results.append(result)
            except Exception as e:
                results.append({
                    "transport": transport,
                    "success": False,
                    "error": str(e),
                    "port": 0
                })
    
    # Print summary
    print(f"\n{'='*80}")
    print("📊 TEST SUMMARY")
    print(f"{'='*80}")
    
    passed = 0
    failed = 0
    
    for result in sorted(results, key=lambda x: x["transport"]):
        transport = result["transport"]
        success = result["success"]
        error = result.get("error")
        
        if success:
            print(f"✅ {transport.upper():<12} - PASSED")
            passed += 1
        else:
            print(f"❌ {transport.upper():<12} - FAILED")
            if error:
                print(f"   Error: {error}")
            failed += 1
    
    print(f"\n📈 Results: {passed} passed, {failed} failed")
    
    if failed == 0:
        print("🎉 All transport tests passed!")
        return True
    else:
        print("💥 Some transport tests failed!")
        return False


def main():
    parser = argparse.ArgumentParser(description="Test MCP transport protocols")
    parser.add_argument("--transport", choices=["stdio", "sse", "http-streams", "all"], 
                       default="all", help="Transport to test")
    parser.add_argument("--port", type=int, default=8080, 
                       help="Port for HTTP-based transports")
    
    args = parser.parse_args()
    
    if args.transport == "all":
        success = test_all_transports_parallel()
    else:
        result = test_single_transport(args.transport, args.port)
        success = result["success"]
        if not success and result.get("error"):
            print(f"Error: {result['error']}")
    
    sys.exit(0 if success else 1)


if __name__ == "__main__":
    main()