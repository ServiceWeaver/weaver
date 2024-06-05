import grpc
import hellogrpc_pb2
import hellogrpc_pb2_grpc

import http.server
import socketserver
import threading

from http.server import BaseHTTPRequestHandler, HTTPServer

from urllib.parse import urlparse, parse_qs

listening_addr = "50351"

# Choose a port to listen on (make sure it's not used by another process)
port = 8080

class App(BaseHTTPRequestHandler):
  def do_GET(self):
    stub = self.create_stub('localhost:' + listening_addr)

    result = ""
    path = urlparse(self.path)
    if path.path == "/hello":
      params = parse_qs(path.query)
      if 'n' in params:
        n = params['n'][0]
        response = stub.Hello(hellogrpc_pb2.HelloRequest(request=n))
        result = response.response
    self.send_response(200)
    self.send_header('Content-type', 'text/plain')
    self.end_headers()
    self.wfile.write(result.encode("utf-8"))

  def create_stub(self, server_address):
    channel = grpc.insecure_channel(server_address)
    stub = hellogrpc_pb2_grpc.HelloFromNYCStub(channel)
    return stub

def start_server(port):
  with socketserver.TCPServer(("", port), App) as httpd:
    print(f"Serving at port {port}")
    httpd.serve_forever()

def run():
  # Start the server in a separate thread
  server_thread = threading.Thread(target=start_server, args=(port,))
  server_thread.daemon = True
  server_thread.start()

  # (Optionally) Keep the main thread running (if you need to do other things)
  server_thread.join()

if __name__ == '__main__':
  run()