// Copyright 2023 Google LLC
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.26.0
// 	protoc        v3.21.12
// source: internal/tool/ssh/impl/ssh.proto

package impl

import (
	protos "github.com/ServiceWeaver/weaver/runtime/protos"
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

// SshConfig stores the configuration information for one execution of a
// Service Weaver application using the SSH deployer.
type SshConfig struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Information about the application deployment.
	App       *protos.AppConfig                     `protobuf:"bytes,1,opt,name=app,proto3" json:"app,omitempty"`
	DepId     string                                `protobuf:"bytes,2,opt,name=dep_id,json=depId,proto3" json:"dep_id,omitempty"`
	Listeners map[string]*SshConfig_ListenerOptions `protobuf:"bytes,3,rep,name=listeners,proto3" json:"listeners,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	// File that contains the IP addresses of all locations where the application
	// can run.
	Locations string `protobuf:"bytes,4,opt,name=locations,proto3" json:"locations,omitempty"`
}

func (x *SshConfig) Reset() {
	*x = SshConfig{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SshConfig) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SshConfig) ProtoMessage() {}

func (x *SshConfig) ProtoReflect() protoreflect.Message {
	mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SshConfig.ProtoReflect.Descriptor instead.
func (*SshConfig) Descriptor() ([]byte, []int) {
	return file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP(), []int{0}
}

func (x *SshConfig) GetApp() *protos.AppConfig {
	if x != nil {
		return x.App
	}
	return nil
}

func (x *SshConfig) GetDepId() string {
	if x != nil {
		return x.DepId
	}
	return ""
}

func (x *SshConfig) GetListeners() map[string]*SshConfig_ListenerOptions {
	if x != nil {
		return x.Listeners
	}
	return nil
}

func (x *SshConfig) GetLocations() string {
	if x != nil {
		return x.Locations
	}
	return ""
}

// BabysitterInfo contains app deployment information that is needed by a
// babysitter started using SSH to manage a colocation group.
type BabysitterInfo struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	App         *protos.AppConfig `protobuf:"bytes,1,opt,name=app,proto3" json:"app,omitempty"`
	DepId       string            `protobuf:"bytes,2,opt,name=dep_id,json=depId,proto3" json:"dep_id,omitempty"`
	Group       string            `protobuf:"bytes,3,opt,name=group,proto3" json:"group,omitempty"`
	ReplicaId   int32             `protobuf:"varint,4,opt,name=replica_id,json=replicaId,proto3" json:"replica_id,omitempty"`
	ManagerAddr string            `protobuf:"bytes,5,opt,name=manager_addr,json=managerAddr,proto3" json:"manager_addr,omitempty"`
	LogDir      string            `protobuf:"bytes,6,opt,name=logDir,proto3" json:"logDir,omitempty"`
	RunMain     bool              `protobuf:"varint,7,opt,name=run_main,json=runMain,proto3" json:"run_main,omitempty"`
}

func (x *BabysitterInfo) Reset() {
	*x = BabysitterInfo{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BabysitterInfo) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BabysitterInfo) ProtoMessage() {}

func (x *BabysitterInfo) ProtoReflect() protoreflect.Message {
	mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BabysitterInfo.ProtoReflect.Descriptor instead.
func (*BabysitterInfo) Descriptor() ([]byte, []int) {
	return file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP(), []int{1}
}

func (x *BabysitterInfo) GetApp() *protos.AppConfig {
	if x != nil {
		return x.App
	}
	return nil
}

func (x *BabysitterInfo) GetDepId() string {
	if x != nil {
		return x.DepId
	}
	return ""
}

func (x *BabysitterInfo) GetGroup() string {
	if x != nil {
		return x.Group
	}
	return ""
}

func (x *BabysitterInfo) GetReplicaId() int32 {
	if x != nil {
		return x.ReplicaId
	}
	return 0
}

func (x *BabysitterInfo) GetManagerAddr() string {
	if x != nil {
		return x.ManagerAddr
	}
	return ""
}

func (x *BabysitterInfo) GetLogDir() string {
	if x != nil {
		return x.LogDir
	}
	return ""
}

func (x *BabysitterInfo) GetRunMain() bool {
	if x != nil {
		return x.RunMain
	}
	return false
}

// A request from the babysitter to the manager to get the latest set of
// components to run.
type GetComponentsRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Group   string `protobuf:"bytes,1,opt,name=group,proto3" json:"group,omitempty"`
	Version string `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
}

func (x *GetComponentsRequest) Reset() {
	*x = GetComponentsRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetComponentsRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetComponentsRequest) ProtoMessage() {}

func (x *GetComponentsRequest) ProtoReflect() protoreflect.Message {
	mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetComponentsRequest.ProtoReflect.Descriptor instead.
func (*GetComponentsRequest) Descriptor() ([]byte, []int) {
	return file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP(), []int{2}
}

func (x *GetComponentsRequest) GetGroup() string {
	if x != nil {
		return x.Group
	}
	return ""
}

func (x *GetComponentsRequest) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

type GetComponentsReply struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Components []string `protobuf:"bytes,1,rep,name=components,proto3" json:"components,omitempty"`
	Version    string   `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
}

func (x *GetComponentsReply) Reset() {
	*x = GetComponentsReply{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetComponentsReply) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetComponentsReply) ProtoMessage() {}

func (x *GetComponentsReply) ProtoReflect() protoreflect.Message {
	mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetComponentsReply.ProtoReflect.Descriptor instead.
func (*GetComponentsReply) Descriptor() ([]byte, []int) {
	return file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP(), []int{3}
}

func (x *GetComponentsReply) GetComponents() []string {
	if x != nil {
		return x.Components
	}
	return nil
}

func (x *GetComponentsReply) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

// A request from the babysitter to the manager to get the latest routing info
// for a component.
type GetRoutingInfoRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Note that requesting group is the group requesting the routing info, not
	// the group of the component.
	RequestingGroup string `protobuf:"bytes,1,opt,name=requesting_group,json=requestingGroup,proto3" json:"requesting_group,omitempty"`
	Component       string `protobuf:"bytes,2,opt,name=component,proto3" json:"component,omitempty"`
	Routed          bool   `protobuf:"varint,3,opt,name=routed,proto3" json:"routed,omitempty"` // is the component routed?
	Version         string `protobuf:"bytes,4,opt,name=version,proto3" json:"version,omitempty"`
}

func (x *GetRoutingInfoRequest) Reset() {
	*x = GetRoutingInfoRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetRoutingInfoRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetRoutingInfoRequest) ProtoMessage() {}

func (x *GetRoutingInfoRequest) ProtoReflect() protoreflect.Message {
	mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetRoutingInfoRequest.ProtoReflect.Descriptor instead.
func (*GetRoutingInfoRequest) Descriptor() ([]byte, []int) {
	return file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP(), []int{4}
}

func (x *GetRoutingInfoRequest) GetRequestingGroup() string {
	if x != nil {
		return x.RequestingGroup
	}
	return ""
}

func (x *GetRoutingInfoRequest) GetComponent() string {
	if x != nil {
		return x.Component
	}
	return ""
}

func (x *GetRoutingInfoRequest) GetRouted() bool {
	if x != nil {
		return x.Routed
	}
	return false
}

func (x *GetRoutingInfoRequest) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

type GetRoutingInfoReply struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	RoutingInfo *protos.RoutingInfo `protobuf:"bytes,1,opt,name=routing_info,json=routingInfo,proto3" json:"routing_info,omitempty"`
	Version     string              `protobuf:"bytes,2,opt,name=version,proto3" json:"version,omitempty"`
}

func (x *GetRoutingInfoReply) Reset() {
	*x = GetRoutingInfoReply{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *GetRoutingInfoReply) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*GetRoutingInfoReply) ProtoMessage() {}

func (x *GetRoutingInfoReply) ProtoReflect() protoreflect.Message {
	mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use GetRoutingInfoReply.ProtoReflect.Descriptor instead.
func (*GetRoutingInfoReply) Descriptor() ([]byte, []int) {
	return file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP(), []int{5}
}

func (x *GetRoutingInfoReply) GetRoutingInfo() *protos.RoutingInfo {
	if x != nil {
		return x.RoutingInfo
	}
	return nil
}

func (x *GetRoutingInfoReply) GetVersion() string {
	if x != nil {
		return x.Version
	}
	return ""
}

// BabysitterMetrics is a snapshot of a deployment's metrics as collected by a
// babysitter for a given colocation group.
type BabysitterMetrics struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	GroupName string                   `protobuf:"bytes,1,opt,name=group_name,json=groupName,proto3" json:"group_name,omitempty"`
	ReplicaId int32                    `protobuf:"varint,2,opt,name=replica_id,json=replicaId,proto3" json:"replica_id,omitempty"`
	Metrics   []*protos.MetricSnapshot `protobuf:"bytes,3,rep,name=metrics,proto3" json:"metrics,omitempty"`
}

func (x *BabysitterMetrics) Reset() {
	*x = BabysitterMetrics{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *BabysitterMetrics) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*BabysitterMetrics) ProtoMessage() {}

func (x *BabysitterMetrics) ProtoReflect() protoreflect.Message {
	mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use BabysitterMetrics.ProtoReflect.Descriptor instead.
func (*BabysitterMetrics) Descriptor() ([]byte, []int) {
	return file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP(), []int{6}
}

func (x *BabysitterMetrics) GetGroupName() string {
	if x != nil {
		return x.GroupName
	}
	return ""
}

func (x *BabysitterMetrics) GetReplicaId() int32 {
	if x != nil {
		return x.ReplicaId
	}
	return 0
}

func (x *BabysitterMetrics) GetMetrics() []*protos.MetricSnapshot {
	if x != nil {
		return x.Metrics
	}
	return nil
}

// ReplicaToRegister is a request to the manager to register a replica of
// a given colocation group (i.e., a weavelet).
type ReplicaToRegister struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Group   string `protobuf:"bytes,1,opt,name=group,proto3" json:"group,omitempty"`
	Address string `protobuf:"bytes,2,opt,name=address,proto3" json:"address,omitempty"` // Replica internal address.
	Pid     int64  `protobuf:"varint,3,opt,name=pid,proto3" json:"pid,omitempty"`        // Replica pid.
}

func (x *ReplicaToRegister) Reset() {
	*x = ReplicaToRegister{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ReplicaToRegister) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ReplicaToRegister) ProtoMessage() {}

func (x *ReplicaToRegister) ProtoReflect() protoreflect.Message {
	mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ReplicaToRegister.ProtoReflect.Descriptor instead.
func (*ReplicaToRegister) Descriptor() ([]byte, []int) {
	return file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP(), []int{7}
}

func (x *ReplicaToRegister) GetGroup() string {
	if x != nil {
		return x.Group
	}
	return ""
}

func (x *ReplicaToRegister) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

func (x *ReplicaToRegister) GetPid() int64 {
	if x != nil {
		return x.Pid
	}
	return 0
}

// Options for the application listeners, keyed by listener name.
// If a listener isn't specified in the map, default options will be used.
type SshConfig_ListenerOptions struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Address of the listener. The value must have the form :port or
	// host:port, or it may be the empty string, which is treated as ":0".
	Address string `protobuf:"bytes,1,opt,name=address,proto3" json:"address,omitempty"`
}

func (x *SshConfig_ListenerOptions) Reset() {
	*x = SshConfig_ListenerOptions{}
	if protoimpl.UnsafeEnabled {
		mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *SshConfig_ListenerOptions) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*SshConfig_ListenerOptions) ProtoMessage() {}

func (x *SshConfig_ListenerOptions) ProtoReflect() protoreflect.Message {
	mi := &file_internal_tool_ssh_impl_ssh_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use SshConfig_ListenerOptions.ProtoReflect.Descriptor instead.
func (*SshConfig_ListenerOptions) Descriptor() ([]byte, []int) {
	return file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP(), []int{0, 0}
}

func (x *SshConfig_ListenerOptions) GetAddress() string {
	if x != nil {
		return x.Address
	}
	return ""
}

var File_internal_tool_ssh_impl_ssh_proto protoreflect.FileDescriptor

var file_internal_tool_ssh_impl_ssh_proto_rawDesc = []byte{
	0x0a, 0x20, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x74, 0x6f, 0x6f, 0x6c, 0x2f,
	0x73, 0x73, 0x68, 0x2f, 0x69, 0x6d, 0x70, 0x6c, 0x2f, 0x73, 0x73, 0x68, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x12, 0x04, 0x69, 0x6d, 0x70, 0x6c, 0x1a, 0x1b, 0x72, 0x75, 0x6e, 0x74, 0x69, 0x6d,
	0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x73, 0x2f, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x1c, 0x72, 0x75, 0x6e, 0x74, 0x69, 0x6d, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x73, 0x2f, 0x72, 0x75, 0x6e, 0x74, 0x69, 0x6d, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x22, 0xb0, 0x02, 0x0a, 0x09, 0x53, 0x73, 0x68, 0x43, 0x6f, 0x6e, 0x66, 0x69,
	0x67, 0x12, 0x24, 0x0a, 0x03, 0x61, 0x70, 0x70, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12,
	0x2e, 0x72, 0x75, 0x6e, 0x74, 0x69, 0x6d, 0x65, 0x2e, 0x41, 0x70, 0x70, 0x43, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x52, 0x03, 0x61, 0x70, 0x70, 0x12, 0x15, 0x0a, 0x06, 0x64, 0x65, 0x70, 0x5f, 0x69,
	0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x64, 0x65, 0x70, 0x49, 0x64, 0x12, 0x3c,
	0x0a, 0x09, 0x6c, 0x69, 0x73, 0x74, 0x65, 0x6e, 0x65, 0x72, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x1e, 0x2e, 0x69, 0x6d, 0x70, 0x6c, 0x2e, 0x53, 0x73, 0x68, 0x43, 0x6f, 0x6e, 0x66,
	0x69, 0x67, 0x2e, 0x4c, 0x69, 0x73, 0x74, 0x65, 0x6e, 0x65, 0x72, 0x73, 0x45, 0x6e, 0x74, 0x72,
	0x79, 0x52, 0x09, 0x6c, 0x69, 0x73, 0x74, 0x65, 0x6e, 0x65, 0x72, 0x73, 0x12, 0x1c, 0x0a, 0x09,
	0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x09, 0x6c, 0x6f, 0x63, 0x61, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x1a, 0x2b, 0x0a, 0x0f, 0x4c, 0x69,
	0x73, 0x74, 0x65, 0x6e, 0x65, 0x72, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x12, 0x18, 0x0a,
	0x07, 0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07,
	0x61, 0x64, 0x64, 0x72, 0x65, 0x73, 0x73, 0x1a, 0x5d, 0x0a, 0x0e, 0x4c, 0x69, 0x73, 0x74, 0x65,
	0x6e, 0x65, 0x72, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x35, 0x0a, 0x05, 0x76,
	0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1f, 0x2e, 0x69, 0x6d, 0x70,
	0x6c, 0x2e, 0x53, 0x73, 0x68, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x2e, 0x4c, 0x69, 0x73, 0x74,
	0x65, 0x6e, 0x65, 0x72, 0x4f, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x73, 0x52, 0x05, 0x76, 0x61, 0x6c,
	0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22, 0xd8, 0x01, 0x0a, 0x0e, 0x42, 0x61, 0x62, 0x79, 0x73,
	0x69, 0x74, 0x74, 0x65, 0x72, 0x49, 0x6e, 0x66, 0x6f, 0x12, 0x24, 0x0a, 0x03, 0x61, 0x70, 0x70,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x12, 0x2e, 0x72, 0x75, 0x6e, 0x74, 0x69, 0x6d, 0x65,
	0x2e, 0x41, 0x70, 0x70, 0x43, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x03, 0x61, 0x70, 0x70, 0x12,
	0x15, 0x0a, 0x06, 0x64, 0x65, 0x70, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x05, 0x64, 0x65, 0x70, 0x49, 0x64, 0x12, 0x14, 0x0a, 0x05, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x18,
	0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x12, 0x1d, 0x0a, 0x0a,
	0x72, 0x65, 0x70, 0x6c, 0x69, 0x63, 0x61, 0x5f, 0x69, 0x64, 0x18, 0x04, 0x20, 0x01, 0x28, 0x05,
	0x52, 0x09, 0x72, 0x65, 0x70, 0x6c, 0x69, 0x63, 0x61, 0x49, 0x64, 0x12, 0x21, 0x0a, 0x0c, 0x6d,
	0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x5f, 0x61, 0x64, 0x64, 0x72, 0x18, 0x05, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x0b, 0x6d, 0x61, 0x6e, 0x61, 0x67, 0x65, 0x72, 0x41, 0x64, 0x64, 0x72, 0x12, 0x16,
	0x0a, 0x06, 0x6c, 0x6f, 0x67, 0x44, 0x69, 0x72, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06,
	0x6c, 0x6f, 0x67, 0x44, 0x69, 0x72, 0x12, 0x19, 0x0a, 0x08, 0x72, 0x75, 0x6e, 0x5f, 0x6d, 0x61,
	0x69, 0x6e, 0x18, 0x07, 0x20, 0x01, 0x28, 0x08, 0x52, 0x07, 0x72, 0x75, 0x6e, 0x4d, 0x61, 0x69,
	0x6e, 0x22, 0x46, 0x0a, 0x14, 0x47, 0x65, 0x74, 0x43, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e,
	0x74, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x14, 0x0a, 0x05, 0x67, 0x72, 0x6f,
	0x75, 0x70, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x12,
	0x18, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x22, 0x4e, 0x0a, 0x12, 0x47, 0x65, 0x74,
	0x43, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e, 0x74, 0x73, 0x52, 0x65, 0x70, 0x6c, 0x79, 0x12,
	0x1e, 0x0a, 0x0a, 0x63, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e, 0x74, 0x73, 0x18, 0x01, 0x20,
	0x03, 0x28, 0x09, 0x52, 0x0a, 0x63, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e, 0x74, 0x73, 0x12,
	0x18, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x22, 0x92, 0x01, 0x0a, 0x15, 0x47, 0x65,
	0x74, 0x52, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67, 0x49, 0x6e, 0x66, 0x6f, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x29, 0x0a, 0x10, 0x72, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x69, 0x6e,
	0x67, 0x5f, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0f, 0x72,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x69, 0x6e, 0x67, 0x47, 0x72, 0x6f, 0x75, 0x70, 0x12, 0x1c,
	0x0a, 0x09, 0x63, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x09, 0x63, 0x6f, 0x6d, 0x70, 0x6f, 0x6e, 0x65, 0x6e, 0x74, 0x12, 0x16, 0x0a, 0x06,
	0x72, 0x6f, 0x75, 0x74, 0x65, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x08, 0x52, 0x06, 0x72, 0x6f,
	0x75, 0x74, 0x65, 0x64, 0x12, 0x18, 0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x22, 0x68,
	0x0a, 0x13, 0x47, 0x65, 0x74, 0x52, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67, 0x49, 0x6e, 0x66, 0x6f,
	0x52, 0x65, 0x70, 0x6c, 0x79, 0x12, 0x37, 0x0a, 0x0c, 0x72, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67,
	0x5f, 0x69, 0x6e, 0x66, 0x6f, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x72, 0x75,
	0x6e, 0x74, 0x69, 0x6d, 0x65, 0x2e, 0x52, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67, 0x49, 0x6e, 0x66,
	0x6f, 0x52, 0x0b, 0x72, 0x6f, 0x75, 0x74, 0x69, 0x6e, 0x67, 0x49, 0x6e, 0x66, 0x6f, 0x12, 0x18,
	0x0a, 0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x07, 0x76, 0x65, 0x72, 0x73, 0x69, 0x6f, 0x6e, 0x22, 0x84, 0x01, 0x0a, 0x11, 0x42, 0x61, 0x62,
	0x79, 0x73, 0x69, 0x74, 0x74, 0x65, 0x72, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x73, 0x12, 0x1d,
	0x0a, 0x0a, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x5f, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x09, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x4e, 0x61, 0x6d, 0x65, 0x12, 0x1d, 0x0a,
	0x0a, 0x72, 0x65, 0x70, 0x6c, 0x69, 0x63, 0x61, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x05, 0x52, 0x09, 0x72, 0x65, 0x70, 0x6c, 0x69, 0x63, 0x61, 0x49, 0x64, 0x12, 0x31, 0x0a, 0x07,
	0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x73, 0x18, 0x03, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x17, 0x2e,
	0x72, 0x75, 0x6e, 0x74, 0x69, 0x6d, 0x65, 0x2e, 0x4d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x53, 0x6e,
	0x61, 0x70, 0x73, 0x68, 0x6f, 0x74, 0x52, 0x07, 0x6d, 0x65, 0x74, 0x72, 0x69, 0x63, 0x73, 0x22,
	0x55, 0x0a, 0x11, 0x52, 0x65, 0x70, 0x6c, 0x69, 0x63, 0x61, 0x54, 0x6f, 0x52, 0x65, 0x67, 0x69,
	0x73, 0x74, 0x65, 0x72, 0x12, 0x14, 0x0a, 0x05, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x18, 0x01, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x05, 0x67, 0x72, 0x6f, 0x75, 0x70, 0x12, 0x18, 0x0a, 0x07, 0x61, 0x64,
	0x64, 0x72, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x61, 0x64, 0x64,
	0x72, 0x65, 0x73, 0x73, 0x12, 0x10, 0x0a, 0x03, 0x70, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28,
	0x03, 0x52, 0x03, 0x70, 0x69, 0x64, 0x42, 0x38, 0x5a, 0x36, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x53, 0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x57, 0x65, 0x61, 0x76,
	0x65, 0x72, 0x2f, 0x77, 0x65, 0x61, 0x76, 0x65, 0x72, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x2f, 0x74, 0x6f, 0x6f, 0x6c, 0x2f, 0x73, 0x73, 0x68, 0x2f, 0x69, 0x6d, 0x70, 0x6c,
	0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_internal_tool_ssh_impl_ssh_proto_rawDescOnce sync.Once
	file_internal_tool_ssh_impl_ssh_proto_rawDescData = file_internal_tool_ssh_impl_ssh_proto_rawDesc
)

func file_internal_tool_ssh_impl_ssh_proto_rawDescGZIP() []byte {
	file_internal_tool_ssh_impl_ssh_proto_rawDescOnce.Do(func() {
		file_internal_tool_ssh_impl_ssh_proto_rawDescData = protoimpl.X.CompressGZIP(file_internal_tool_ssh_impl_ssh_proto_rawDescData)
	})
	return file_internal_tool_ssh_impl_ssh_proto_rawDescData
}

var file_internal_tool_ssh_impl_ssh_proto_msgTypes = make([]protoimpl.MessageInfo, 10)
var file_internal_tool_ssh_impl_ssh_proto_goTypes = []interface{}{
	(*SshConfig)(nil),                 // 0: impl.SshConfig
	(*BabysitterInfo)(nil),            // 1: impl.BabysitterInfo
	(*GetComponentsRequest)(nil),      // 2: impl.GetComponentsRequest
	(*GetComponentsReply)(nil),        // 3: impl.GetComponentsReply
	(*GetRoutingInfoRequest)(nil),     // 4: impl.GetRoutingInfoRequest
	(*GetRoutingInfoReply)(nil),       // 5: impl.GetRoutingInfoReply
	(*BabysitterMetrics)(nil),         // 6: impl.BabysitterMetrics
	(*ReplicaToRegister)(nil),         // 7: impl.ReplicaToRegister
	(*SshConfig_ListenerOptions)(nil), // 8: impl.SshConfig.ListenerOptions
	nil,                               // 9: impl.SshConfig.ListenersEntry
	(*protos.AppConfig)(nil),          // 10: runtime.AppConfig
	(*protos.RoutingInfo)(nil),        // 11: runtime.RoutingInfo
	(*protos.MetricSnapshot)(nil),     // 12: runtime.MetricSnapshot
}
var file_internal_tool_ssh_impl_ssh_proto_depIdxs = []int32{
	10, // 0: impl.SshConfig.app:type_name -> runtime.AppConfig
	9,  // 1: impl.SshConfig.listeners:type_name -> impl.SshConfig.ListenersEntry
	10, // 2: impl.BabysitterInfo.app:type_name -> runtime.AppConfig
	11, // 3: impl.GetRoutingInfoReply.routing_info:type_name -> runtime.RoutingInfo
	12, // 4: impl.BabysitterMetrics.metrics:type_name -> runtime.MetricSnapshot
	8,  // 5: impl.SshConfig.ListenersEntry.value:type_name -> impl.SshConfig.ListenerOptions
	6,  // [6:6] is the sub-list for method output_type
	6,  // [6:6] is the sub-list for method input_type
	6,  // [6:6] is the sub-list for extension type_name
	6,  // [6:6] is the sub-list for extension extendee
	0,  // [0:6] is the sub-list for field type_name
}

func init() { file_internal_tool_ssh_impl_ssh_proto_init() }
func file_internal_tool_ssh_impl_ssh_proto_init() {
	if File_internal_tool_ssh_impl_ssh_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_internal_tool_ssh_impl_ssh_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SshConfig); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_tool_ssh_impl_ssh_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BabysitterInfo); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_tool_ssh_impl_ssh_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetComponentsRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_tool_ssh_impl_ssh_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetComponentsReply); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_tool_ssh_impl_ssh_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetRoutingInfoRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_tool_ssh_impl_ssh_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*GetRoutingInfoReply); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_tool_ssh_impl_ssh_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*BabysitterMetrics); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_tool_ssh_impl_ssh_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ReplicaToRegister); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_internal_tool_ssh_impl_ssh_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*SshConfig_ListenerOptions); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_internal_tool_ssh_impl_ssh_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   10,
			NumExtensions: 0,
			NumServices:   0,
		},
		GoTypes:           file_internal_tool_ssh_impl_ssh_proto_goTypes,
		DependencyIndexes: file_internal_tool_ssh_impl_ssh_proto_depIdxs,
		MessageInfos:      file_internal_tool_ssh_impl_ssh_proto_msgTypes,
	}.Build()
	File_internal_tool_ssh_impl_ssh_proto = out.File
	file_internal_tool_ssh_impl_ssh_proto_rawDesc = nil
	file_internal_tool_ssh_impl_ssh_proto_goTypes = nil
	file_internal_tool_ssh_impl_ssh_proto_depIdxs = nil
}
