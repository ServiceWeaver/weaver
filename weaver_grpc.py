import grpc
def RegisterComponent(func, server):
  func(server)
  return

