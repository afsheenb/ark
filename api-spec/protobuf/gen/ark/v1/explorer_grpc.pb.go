// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v3.21.12
// source: ark/v1/explorer.proto

package arkv1

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.64.0 or later.
const _ = grpc.SupportPackageIsVersion9

const (
	ExplorerService_GetRound_FullMethodName     = "/ark.v1.ExplorerService/GetRound"
	ExplorerService_GetRoundById_FullMethodName = "/ark.v1.ExplorerService/GetRoundById"
	ExplorerService_ListVtxos_FullMethodName    = "/ark.v1.ExplorerService/ListVtxos"
)

// ExplorerServiceClient is the client API for ExplorerService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type ExplorerServiceClient interface {
	GetRound(ctx context.Context, in *GetRoundRequest, opts ...grpc.CallOption) (*GetRoundResponse, error)
	GetRoundById(ctx context.Context, in *GetRoundByIdRequest, opts ...grpc.CallOption) (*GetRoundByIdResponse, error)
	ListVtxos(ctx context.Context, in *ListVtxosRequest, opts ...grpc.CallOption) (*ListVtxosResponse, error)
}

type explorerServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewExplorerServiceClient(cc grpc.ClientConnInterface) ExplorerServiceClient {
	return &explorerServiceClient{cc}
}

func (c *explorerServiceClient) GetRound(ctx context.Context, in *GetRoundRequest, opts ...grpc.CallOption) (*GetRoundResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetRoundResponse)
	err := c.cc.Invoke(ctx, ExplorerService_GetRound_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *explorerServiceClient) GetRoundById(ctx context.Context, in *GetRoundByIdRequest, opts ...grpc.CallOption) (*GetRoundByIdResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetRoundByIdResponse)
	err := c.cc.Invoke(ctx, ExplorerService_GetRoundById_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *explorerServiceClient) ListVtxos(ctx context.Context, in *ListVtxosRequest, opts ...grpc.CallOption) (*ListVtxosResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(ListVtxosResponse)
	err := c.cc.Invoke(ctx, ExplorerService_ListVtxos_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// ExplorerServiceServer is the server API for ExplorerService service.
// All implementations must embed UnimplementedExplorerServiceServer
// for forward compatibility.
type ExplorerServiceServer interface {
	GetRound(context.Context, *GetRoundRequest) (*GetRoundResponse, error)
	GetRoundById(context.Context, *GetRoundByIdRequest) (*GetRoundByIdResponse, error)
	ListVtxos(context.Context, *ListVtxosRequest) (*ListVtxosResponse, error)
	mustEmbedUnimplementedExplorerServiceServer()
}

// UnimplementedExplorerServiceServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedExplorerServiceServer struct{}

func (UnimplementedExplorerServiceServer) GetRound(context.Context, *GetRoundRequest) (*GetRoundResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRound not implemented")
}
func (UnimplementedExplorerServiceServer) GetRoundById(context.Context, *GetRoundByIdRequest) (*GetRoundByIdResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRoundById not implemented")
}
func (UnimplementedExplorerServiceServer) ListVtxos(context.Context, *ListVtxosRequest) (*ListVtxosResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method ListVtxos not implemented")
}
func (UnimplementedExplorerServiceServer) mustEmbedUnimplementedExplorerServiceServer() {}
func (UnimplementedExplorerServiceServer) testEmbeddedByValue()                         {}

// UnsafeExplorerServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to ExplorerServiceServer will
// result in compilation errors.
type UnsafeExplorerServiceServer interface {
	mustEmbedUnimplementedExplorerServiceServer()
}

func RegisterExplorerServiceServer(s grpc.ServiceRegistrar, srv ExplorerServiceServer) {
	// If the following call pancis, it indicates UnimplementedExplorerServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&ExplorerService_ServiceDesc, srv)
}

func _ExplorerService_GetRound_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRoundRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExplorerServiceServer).GetRound(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ExplorerService_GetRound_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExplorerServiceServer).GetRound(ctx, req.(*GetRoundRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ExplorerService_GetRoundById_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRoundByIdRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExplorerServiceServer).GetRoundById(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ExplorerService_GetRoundById_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExplorerServiceServer).GetRoundById(ctx, req.(*GetRoundByIdRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _ExplorerService_ListVtxos_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListVtxosRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(ExplorerServiceServer).ListVtxos(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: ExplorerService_ListVtxos_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(ExplorerServiceServer).ListVtxos(ctx, req.(*ListVtxosRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// ExplorerService_ServiceDesc is the grpc.ServiceDesc for ExplorerService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var ExplorerService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "ark.v1.ExplorerService",
	HandlerType: (*ExplorerServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetRound",
			Handler:    _ExplorerService_GetRound_Handler,
		},
		{
			MethodName: "GetRoundById",
			Handler:    _ExplorerService_GetRoundById_Handler,
		},
		{
			MethodName: "ListVtxos",
			Handler:    _ExplorerService_ListVtxos_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "ark/v1/explorer.proto",
}
