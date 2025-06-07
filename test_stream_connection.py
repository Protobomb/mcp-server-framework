#!/usr/bin/env python3

import requests
import time

def test_stream_connection():
    print("Testing HTTP Streams connection...")
    
    try:
        # Test stream endpoint
        response = requests.get('http://localhost:8082/stream', stream=True, timeout=5)
        print(f"Status code: {response.status_code}")
        print(f"Headers: {response.headers}")
        
        # Read first few lines
        lines_read = 0
        for line in response.iter_lines(decode_unicode=True):
            if line:
                print(f"Received: {line}")
                lines_read += 1
                if lines_read >= 3:  # Read first 3 lines then exit
                    break
                    
    except requests.exceptions.Timeout:
        print("Connection timed out")
    except Exception as e:
        print(f"Error: {e}")

if __name__ == "__main__":
    test_stream_connection()