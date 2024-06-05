import grpc
class Registry:
  def __init__(self):
    self.components = {}

  def register(self, reg):
    name = reg.name

    channel = grpc.insecure_channel("localhost:53051")
    client = reg.client(channel)
    self.components[name] = Component(name)



globalRegistry = Registry()

class Registration:
  def __init__(self, name, client, server):
    self.name = name
    self.client = client
    self.server = server

class Component:
  def __init__(self, name):
    self.name = name
    #self.handle = handle
    #self.impl = impl