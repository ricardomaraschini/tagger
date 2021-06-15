// Copyright 2020 The Tagger Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//       http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.25.0-devel
// 	protoc        v3.13.0
// source: infra/pb/tagio.proto

package pb

import (
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

// Packet is what goes through the wire. It contains or an Header, or a
// Progress or a Chunk of data.
type Packet struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	// Types that are assignable to TestOneof:
	//	*Packet_Header
	//	*Packet_Progress
	//	*Packet_Chunk
	TestOneof isPacket_TestOneof `protobuf_oneof:"test_oneof"`
}

func (x *Packet) Reset() {
	*x = Packet{}
	if protoimpl.UnsafeEnabled {
		mi := &file_infra_pb_tagio_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Packet) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Packet) ProtoMessage() {}

func (x *Packet) ProtoReflect() protoreflect.Message {
	mi := &file_infra_pb_tagio_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Packet.ProtoReflect.Descriptor instead.
func (*Packet) Descriptor() ([]byte, []int) {
	return file_infra_pb_tagio_proto_rawDescGZIP(), []int{0}
}

func (m *Packet) GetTestOneof() isPacket_TestOneof {
	if m != nil {
		return m.TestOneof
	}
	return nil
}

func (x *Packet) GetHeader() *Header {
	if x, ok := x.GetTestOneof().(*Packet_Header); ok {
		return x.Header
	}
	return nil
}

func (x *Packet) GetProgress() *Progress {
	if x, ok := x.GetTestOneof().(*Packet_Progress); ok {
		return x.Progress
	}
	return nil
}

func (x *Packet) GetChunk() *Chunk {
	if x, ok := x.GetTestOneof().(*Packet_Chunk); ok {
		return x.Chunk
	}
	return nil
}

type isPacket_TestOneof interface {
	isPacket_TestOneof()
}

type Packet_Header struct {
	Header *Header `protobuf:"bytes,1,opt,name=header,proto3,oneof"`
}

type Packet_Progress struct {
	Progress *Progress `protobuf:"bytes,2,opt,name=progress,proto3,oneof"`
}

type Packet_Chunk struct {
	Chunk *Chunk `protobuf:"bytes,3,opt,name=chunk,proto3,oneof"`
}

func (*Packet_Header) isPacket_TestOneof() {}

func (*Packet_Progress) isPacket_TestOneof() {}

func (*Packet_Chunk) isPacket_TestOneof() {}

// Header identifies an user (through a token) and a tag (through namespace
// and name). This is used to when informing which user is requesting wich
// tag during pull and push operations.
type Header struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Namespace string `protobuf:"bytes,1,opt,name=namespace,proto3" json:"namespace,omitempty"`
	Name      string `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Token     string `protobuf:"bytes,3,opt,name=token,proto3" json:"token,omitempty"`
}

func (x *Header) Reset() {
	*x = Header{}
	if protoimpl.UnsafeEnabled {
		mi := &file_infra_pb_tagio_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Header) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Header) ProtoMessage() {}

func (x *Header) ProtoReflect() protoreflect.Message {
	mi := &file_infra_pb_tagio_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Header.ProtoReflect.Descriptor instead.
func (*Header) Descriptor() ([]byte, []int) {
	return file_infra_pb_tagio_proto_rawDescGZIP(), []int{1}
}

func (x *Header) GetNamespace() string {
	if x != nil {
		return x.Namespace
	}
	return ""
}

func (x *Header) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *Header) GetToken() string {
	if x != nil {
		return x.Token
	}
	return ""
}

// Progress is a message informing what is the current offset and what is
// the total size of a file. It is used to inform clients about a file
// transfer status.
type Progress struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Offset int64 `protobuf:"varint,1,opt,name=offset,proto3" json:"offset,omitempty"`
	Size   int64 `protobuf:"varint,2,opt,name=size,proto3" json:"size,omitempty"`
}

func (x *Progress) Reset() {
	*x = Progress{}
	if protoimpl.UnsafeEnabled {
		mi := &file_infra_pb_tagio_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Progress) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Progress) ProtoMessage() {}

func (x *Progress) ProtoReflect() protoreflect.Message {
	mi := &file_infra_pb_tagio_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Progress.ProtoReflect.Descriptor instead.
func (*Progress) Descriptor() ([]byte, []int) {
	return file_infra_pb_tagio_proto_rawDescGZIP(), []int{2}
}

func (x *Progress) GetOffset() int64 {
	if x != nil {
		return x.Offset
	}
	return 0
}

func (x *Progress) GetSize() int64 {
	if x != nil {
		return x.Size
	}
	return 0
}

// Chunk is a message containing part of a file.
type Chunk struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Content []byte `protobuf:"bytes,1,opt,name=content,proto3" json:"content,omitempty"`
}

func (x *Chunk) Reset() {
	*x = Chunk{}
	if protoimpl.UnsafeEnabled {
		mi := &file_infra_pb_tagio_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *Chunk) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*Chunk) ProtoMessage() {}

func (x *Chunk) ProtoReflect() protoreflect.Message {
	mi := &file_infra_pb_tagio_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use Chunk.ProtoReflect.Descriptor instead.
func (*Chunk) Descriptor() ([]byte, []int) {
	return file_infra_pb_tagio_proto_rawDescGZIP(), []int{3}
}

func (x *Chunk) GetContent() []byte {
	if x != nil {
		return x.Content
	}
	return nil
}

var File_infra_pb_tagio_proto protoreflect.FileDescriptor

var file_infra_pb_tagio_proto_rawDesc = []byte{
	0x0a, 0x14, 0x69, 0x6e, 0x66, 0x72, 0x61, 0x2f, 0x70, 0x62, 0x2f, 0x74, 0x61, 0x67, 0x69, 0x6f,
	0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x12, 0x02, 0x70, 0x62, 0x22, 0x8b, 0x01, 0x0a, 0x06, 0x50,
	0x61, 0x63, 0x6b, 0x65, 0x74, 0x12, 0x24, 0x0a, 0x06, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0a, 0x2e, 0x70, 0x62, 0x2e, 0x48, 0x65, 0x61, 0x64, 0x65,
	0x72, 0x48, 0x00, 0x52, 0x06, 0x68, 0x65, 0x61, 0x64, 0x65, 0x72, 0x12, 0x2a, 0x0a, 0x08, 0x70,
	0x72, 0x6f, 0x67, 0x72, 0x65, 0x73, 0x73, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0c, 0x2e,
	0x70, 0x62, 0x2e, 0x50, 0x72, 0x6f, 0x67, 0x72, 0x65, 0x73, 0x73, 0x48, 0x00, 0x52, 0x08, 0x70,
	0x72, 0x6f, 0x67, 0x72, 0x65, 0x73, 0x73, 0x12, 0x21, 0x0a, 0x05, 0x63, 0x68, 0x75, 0x6e, 0x6b,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x09, 0x2e, 0x70, 0x62, 0x2e, 0x43, 0x68, 0x75, 0x6e,
	0x6b, 0x48, 0x00, 0x52, 0x05, 0x63, 0x68, 0x75, 0x6e, 0x6b, 0x42, 0x0c, 0x0a, 0x0a, 0x74, 0x65,
	0x73, 0x74, 0x5f, 0x6f, 0x6e, 0x65, 0x6f, 0x66, 0x22, 0x50, 0x0a, 0x06, 0x48, 0x65, 0x61, 0x64,
	0x65, 0x72, 0x12, 0x1c, 0x0a, 0x09, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09, 0x6e, 0x61, 0x6d, 0x65, 0x73, 0x70, 0x61, 0x63, 0x65,
	0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04,
	0x6e, 0x61, 0x6d, 0x65, 0x12, 0x14, 0x0a, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x05, 0x74, 0x6f, 0x6b, 0x65, 0x6e, 0x22, 0x36, 0x0a, 0x08, 0x50, 0x72,
	0x6f, 0x67, 0x72, 0x65, 0x73, 0x73, 0x12, 0x16, 0x0a, 0x06, 0x6f, 0x66, 0x66, 0x73, 0x65, 0x74,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x03, 0x52, 0x06, 0x6f, 0x66, 0x66, 0x73, 0x65, 0x74, 0x12, 0x12,
	0x0a, 0x04, 0x73, 0x69, 0x7a, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x03, 0x52, 0x04, 0x73, 0x69,
	0x7a, 0x65, 0x22, 0x21, 0x0a, 0x05, 0x43, 0x68, 0x75, 0x6e, 0x6b, 0x12, 0x18, 0x0a, 0x07, 0x63,
	0x6f, 0x6e, 0x74, 0x65, 0x6e, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0c, 0x52, 0x07, 0x63, 0x6f,
	0x6e, 0x74, 0x65, 0x6e, 0x74, 0x32, 0x52, 0x0a, 0x0c, 0x54, 0x61, 0x67, 0x49, 0x4f, 0x53, 0x65,
	0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x20, 0x0a, 0x04, 0x50, 0x75, 0x6c, 0x6c, 0x12, 0x0a, 0x2e,
	0x70, 0x62, 0x2e, 0x50, 0x61, 0x63, 0x6b, 0x65, 0x74, 0x1a, 0x0a, 0x2e, 0x70, 0x62, 0x2e, 0x50,
	0x61, 0x63, 0x6b, 0x65, 0x74, 0x30, 0x01, 0x12, 0x20, 0x0a, 0x04, 0x50, 0x75, 0x73, 0x68, 0x12,
	0x0a, 0x2e, 0x70, 0x62, 0x2e, 0x50, 0x61, 0x63, 0x6b, 0x65, 0x74, 0x1a, 0x0a, 0x2e, 0x70, 0x62,
	0x2e, 0x50, 0x61, 0x63, 0x6b, 0x65, 0x74, 0x28, 0x01, 0x42, 0x32, 0x5a, 0x30, 0x67, 0x69, 0x74,
	0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x72, 0x69, 0x63, 0x61, 0x72, 0x64, 0x6f, 0x6d,
	0x61, 0x72, 0x61, 0x73, 0x63, 0x68, 0x69, 0x6e, 0x69, 0x2f, 0x74, 0x61, 0x67, 0x67, 0x65, 0x72,
	0x2f, 0x69, 0x6d, 0x61, 0x67, 0x65, 0x74, 0x61, 0x67, 0x73, 0x2f, 0x70, 0x62, 0x62, 0x06, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_infra_pb_tagio_proto_rawDescOnce sync.Once
	file_infra_pb_tagio_proto_rawDescData = file_infra_pb_tagio_proto_rawDesc
)

func file_infra_pb_tagio_proto_rawDescGZIP() []byte {
	file_infra_pb_tagio_proto_rawDescOnce.Do(func() {
		file_infra_pb_tagio_proto_rawDescData = protoimpl.X.CompressGZIP(file_infra_pb_tagio_proto_rawDescData)
	})
	return file_infra_pb_tagio_proto_rawDescData
}

var file_infra_pb_tagio_proto_msgTypes = make([]protoimpl.MessageInfo, 4)
var file_infra_pb_tagio_proto_goTypes = []interface{}{
	(*Packet)(nil),   // 0: pb.Packet
	(*Header)(nil),   // 1: pb.Header
	(*Progress)(nil), // 2: pb.Progress
	(*Chunk)(nil),    // 3: pb.Chunk
}
var file_infra_pb_tagio_proto_depIdxs = []int32{
	1, // 0: pb.Packet.header:type_name -> pb.Header
	2, // 1: pb.Packet.progress:type_name -> pb.Progress
	3, // 2: pb.Packet.chunk:type_name -> pb.Chunk
	0, // 3: pb.TagIOService.Pull:input_type -> pb.Packet
	0, // 4: pb.TagIOService.Push:input_type -> pb.Packet
	0, // 5: pb.TagIOService.Pull:output_type -> pb.Packet
	0, // 6: pb.TagIOService.Push:output_type -> pb.Packet
	5, // [5:7] is the sub-list for method output_type
	3, // [3:5] is the sub-list for method input_type
	3, // [3:3] is the sub-list for extension type_name
	3, // [3:3] is the sub-list for extension extendee
	0, // [0:3] is the sub-list for field type_name
}

func init() { file_infra_pb_tagio_proto_init() }
func file_infra_pb_tagio_proto_init() {
	if File_infra_pb_tagio_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_infra_pb_tagio_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Packet); i {
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
		file_infra_pb_tagio_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Header); i {
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
		file_infra_pb_tagio_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Progress); i {
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
		file_infra_pb_tagio_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*Chunk); i {
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
	file_infra_pb_tagio_proto_msgTypes[0].OneofWrappers = []interface{}{
		(*Packet_Header)(nil),
		(*Packet_Progress)(nil),
		(*Packet_Chunk)(nil),
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_infra_pb_tagio_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   4,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_infra_pb_tagio_proto_goTypes,
		DependencyIndexes: file_infra_pb_tagio_proto_depIdxs,
		MessageInfos:      file_infra_pb_tagio_proto_msgTypes,
	}.Build()
	File_infra_pb_tagio_proto = out.File
	file_infra_pb_tagio_proto_rawDesc = nil
	file_infra_pb_tagio_proto_goTypes = nil
	file_infra_pb_tagio_proto_depIdxs = nil
}
