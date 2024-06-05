import grpc
import os
import hellogrpc_pb2
import hellogrpc_pb2_grpc

from datetime import datetime
from concurrent import futures

listening_addr = "50351"

class HelloGrpcNYC(hellogrpc_pb2_grpc.HelloFromNYCServicer):
  def Hello(self, request, context):
    now = datetime.now()
    current_time = now.time()
    response = "[NYC][" + str(os.getpid()) + "] Hello, " + request.request + " at " + current_time.strftime("%H:%M:%S") + "!\n"
    return hellogrpc_pb2.HelloResponse(response=response)

def serve():
  server = grpc.server(futures.ThreadPoolExecutor(max_workers=10))
  hellogrpc_pb2_grpc.add_HelloFromNYCServicer_to_server(HelloGrpcNYC(), server)
  server.add_insecure_port('[::]:' + listening_addr)
  print("Server listening on port " + listening_addr)
  server.start()
  server.wait_for_termination()

if __name__ == '__main__':
  serve()
