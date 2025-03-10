// Code generated by protoc-gen-go-grpc. DO NOT EDIT.
// versions:
// - protoc-gen-go-grpc v1.5.1
// - protoc             v3.21.12
// source: ark/v1/admin.proto

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
	AdminService_GetScheduledSweep_FullMethodName      = "/ark.v1.AdminService/GetScheduledSweep"
	AdminService_GetRoundDetails_FullMethodName        = "/ark.v1.AdminService/GetRoundDetails"
	AdminService_GetRounds_FullMethodName              = "/ark.v1.AdminService/GetRounds"
	AdminService_CreateNote_FullMethodName             = "/ark.v1.AdminService/CreateNote"
	AdminService_GetMarketHourConfig_FullMethodName    = "/ark.v1.AdminService/GetMarketHourConfig"
	AdminService_UpdateMarketHourConfig_FullMethodName = "/ark.v1.AdminService/UpdateMarketHourConfig"
	AdminService_GetTxRequestQueue_FullMethodName      = "/ark.v1.AdminService/GetTxRequestQueue"
	AdminService_DeleteTxRequests_FullMethodName       = "/ark.v1.AdminService/DeleteTxRequests"
	AdminService_Withdraw_FullMethodName               = "/ark.v1.AdminService/Withdraw"
)

// AdminServiceClient is the client API for AdminService service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type AdminServiceClient interface {
	GetScheduledSweep(ctx context.Context, in *GetScheduledSweepRequest, opts ...grpc.CallOption) (*GetScheduledSweepResponse, error)
	GetRoundDetails(ctx context.Context, in *GetRoundDetailsRequest, opts ...grpc.CallOption) (*GetRoundDetailsResponse, error)
	GetRounds(ctx context.Context, in *GetRoundsRequest, opts ...grpc.CallOption) (*GetRoundsResponse, error)
	CreateNote(ctx context.Context, in *CreateNoteRequest, opts ...grpc.CallOption) (*CreateNoteResponse, error)
	GetMarketHourConfig(ctx context.Context, in *GetMarketHourConfigRequest, opts ...grpc.CallOption) (*GetMarketHourConfigResponse, error)
	UpdateMarketHourConfig(ctx context.Context, in *UpdateMarketHourConfigRequest, opts ...grpc.CallOption) (*UpdateMarketHourConfigResponse, error)
	GetTxRequestQueue(ctx context.Context, in *GetTxRequestQueueRequest, opts ...grpc.CallOption) (*GetTxRequestQueueResponse, error)
	DeleteTxRequests(ctx context.Context, in *DeleteTxRequestsRequest, opts ...grpc.CallOption) (*DeleteTxRequestsResponse, error)
	Withdraw(ctx context.Context, in *WithdrawRequest, opts ...grpc.CallOption) (*WithdrawResponse, error)
}

type adminServiceClient struct {
	cc grpc.ClientConnInterface
}

func NewAdminServiceClient(cc grpc.ClientConnInterface) AdminServiceClient {
	return &adminServiceClient{cc}
}

func (c *adminServiceClient) GetScheduledSweep(ctx context.Context, in *GetScheduledSweepRequest, opts ...grpc.CallOption) (*GetScheduledSweepResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetScheduledSweepResponse)
	err := c.cc.Invoke(ctx, AdminService_GetScheduledSweep_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetRoundDetails(ctx context.Context, in *GetRoundDetailsRequest, opts ...grpc.CallOption) (*GetRoundDetailsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetRoundDetailsResponse)
	err := c.cc.Invoke(ctx, AdminService_GetRoundDetails_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetRounds(ctx context.Context, in *GetRoundsRequest, opts ...grpc.CallOption) (*GetRoundsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetRoundsResponse)
	err := c.cc.Invoke(ctx, AdminService_GetRounds_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) CreateNote(ctx context.Context, in *CreateNoteRequest, opts ...grpc.CallOption) (*CreateNoteResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(CreateNoteResponse)
	err := c.cc.Invoke(ctx, AdminService_CreateNote_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetMarketHourConfig(ctx context.Context, in *GetMarketHourConfigRequest, opts ...grpc.CallOption) (*GetMarketHourConfigResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetMarketHourConfigResponse)
	err := c.cc.Invoke(ctx, AdminService_GetMarketHourConfig_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) UpdateMarketHourConfig(ctx context.Context, in *UpdateMarketHourConfigRequest, opts ...grpc.CallOption) (*UpdateMarketHourConfigResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(UpdateMarketHourConfigResponse)
	err := c.cc.Invoke(ctx, AdminService_UpdateMarketHourConfig_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) GetTxRequestQueue(ctx context.Context, in *GetTxRequestQueueRequest, opts ...grpc.CallOption) (*GetTxRequestQueueResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(GetTxRequestQueueResponse)
	err := c.cc.Invoke(ctx, AdminService_GetTxRequestQueue_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) DeleteTxRequests(ctx context.Context, in *DeleteTxRequestsRequest, opts ...grpc.CallOption) (*DeleteTxRequestsResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(DeleteTxRequestsResponse)
	err := c.cc.Invoke(ctx, AdminService_DeleteTxRequests_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *adminServiceClient) Withdraw(ctx context.Context, in *WithdrawRequest, opts ...grpc.CallOption) (*WithdrawResponse, error) {
	cOpts := append([]grpc.CallOption{grpc.StaticMethod()}, opts...)
	out := new(WithdrawResponse)
	err := c.cc.Invoke(ctx, AdminService_Withdraw_FullMethodName, in, out, cOpts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// AdminServiceServer is the server API for AdminService service.
// All implementations must embed UnimplementedAdminServiceServer
// for forward compatibility.
type AdminServiceServer interface {
	GetScheduledSweep(context.Context, *GetScheduledSweepRequest) (*GetScheduledSweepResponse, error)
	GetRoundDetails(context.Context, *GetRoundDetailsRequest) (*GetRoundDetailsResponse, error)
	GetRounds(context.Context, *GetRoundsRequest) (*GetRoundsResponse, error)
	CreateNote(context.Context, *CreateNoteRequest) (*CreateNoteResponse, error)
	GetMarketHourConfig(context.Context, *GetMarketHourConfigRequest) (*GetMarketHourConfigResponse, error)
	UpdateMarketHourConfig(context.Context, *UpdateMarketHourConfigRequest) (*UpdateMarketHourConfigResponse, error)
	GetTxRequestQueue(context.Context, *GetTxRequestQueueRequest) (*GetTxRequestQueueResponse, error)
	DeleteTxRequests(context.Context, *DeleteTxRequestsRequest) (*DeleteTxRequestsResponse, error)
	Withdraw(context.Context, *WithdrawRequest) (*WithdrawResponse, error)
	mustEmbedUnimplementedAdminServiceServer()
}

// UnimplementedAdminServiceServer must be embedded to have
// forward compatible implementations.
//
// NOTE: this should be embedded by value instead of pointer to avoid a nil
// pointer dereference when methods are called.
type UnimplementedAdminServiceServer struct{}

func (UnimplementedAdminServiceServer) GetScheduledSweep(context.Context, *GetScheduledSweepRequest) (*GetScheduledSweepResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetScheduledSweep not implemented")
}
func (UnimplementedAdminServiceServer) GetRoundDetails(context.Context, *GetRoundDetailsRequest) (*GetRoundDetailsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRoundDetails not implemented")
}
func (UnimplementedAdminServiceServer) GetRounds(context.Context, *GetRoundsRequest) (*GetRoundsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetRounds not implemented")
}
func (UnimplementedAdminServiceServer) CreateNote(context.Context, *CreateNoteRequest) (*CreateNoteResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method CreateNote not implemented")
}
func (UnimplementedAdminServiceServer) GetMarketHourConfig(context.Context, *GetMarketHourConfigRequest) (*GetMarketHourConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetMarketHourConfig not implemented")
}
func (UnimplementedAdminServiceServer) UpdateMarketHourConfig(context.Context, *UpdateMarketHourConfigRequest) (*UpdateMarketHourConfigResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method UpdateMarketHourConfig not implemented")
}
func (UnimplementedAdminServiceServer) GetTxRequestQueue(context.Context, *GetTxRequestQueueRequest) (*GetTxRequestQueueResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetTxRequestQueue not implemented")
}
func (UnimplementedAdminServiceServer) DeleteTxRequests(context.Context, *DeleteTxRequestsRequest) (*DeleteTxRequestsResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method DeleteTxRequests not implemented")
}
func (UnimplementedAdminServiceServer) Withdraw(context.Context, *WithdrawRequest) (*WithdrawResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Withdraw not implemented")
}
func (UnimplementedAdminServiceServer) mustEmbedUnimplementedAdminServiceServer() {}
func (UnimplementedAdminServiceServer) testEmbeddedByValue()                      {}

// UnsafeAdminServiceServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to AdminServiceServer will
// result in compilation errors.
type UnsafeAdminServiceServer interface {
	mustEmbedUnimplementedAdminServiceServer()
}

func RegisterAdminServiceServer(s grpc.ServiceRegistrar, srv AdminServiceServer) {
	// If the following call pancis, it indicates UnimplementedAdminServiceServer was
	// embedded by pointer and is nil.  This will cause panics if an
	// unimplemented method is ever invoked, so we test this at initialization
	// time to prevent it from happening at runtime later due to I/O.
	if t, ok := srv.(interface{ testEmbeddedByValue() }); ok {
		t.testEmbeddedByValue()
	}
	s.RegisterService(&AdminService_ServiceDesc, srv)
}

func _AdminService_GetScheduledSweep_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetScheduledSweepRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetScheduledSweep(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdminService_GetScheduledSweep_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetScheduledSweep(ctx, req.(*GetScheduledSweepRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetRoundDetails_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRoundDetailsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetRoundDetails(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdminService_GetRoundDetails_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetRoundDetails(ctx, req.(*GetRoundDetailsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetRounds_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetRoundsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetRounds(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdminService_GetRounds_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetRounds(ctx, req.(*GetRoundsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_CreateNote_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CreateNoteRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).CreateNote(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdminService_CreateNote_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).CreateNote(ctx, req.(*CreateNoteRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetMarketHourConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetMarketHourConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetMarketHourConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdminService_GetMarketHourConfig_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetMarketHourConfig(ctx, req.(*GetMarketHourConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_UpdateMarketHourConfig_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(UpdateMarketHourConfigRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).UpdateMarketHourConfig(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdminService_UpdateMarketHourConfig_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).UpdateMarketHourConfig(ctx, req.(*UpdateMarketHourConfigRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_GetTxRequestQueue_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetTxRequestQueueRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).GetTxRequestQueue(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdminService_GetTxRequestQueue_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).GetTxRequestQueue(ctx, req.(*GetTxRequestQueueRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_DeleteTxRequests_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(DeleteTxRequestsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).DeleteTxRequests(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdminService_DeleteTxRequests_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).DeleteTxRequests(ctx, req.(*DeleteTxRequestsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _AdminService_Withdraw_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(WithdrawRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(AdminServiceServer).Withdraw(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: AdminService_Withdraw_FullMethodName,
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(AdminServiceServer).Withdraw(ctx, req.(*WithdrawRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// AdminService_ServiceDesc is the grpc.ServiceDesc for AdminService service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var AdminService_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "ark.v1.AdminService",
	HandlerType: (*AdminServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "GetScheduledSweep",
			Handler:    _AdminService_GetScheduledSweep_Handler,
		},
		{
			MethodName: "GetRoundDetails",
			Handler:    _AdminService_GetRoundDetails_Handler,
		},
		{
			MethodName: "GetRounds",
			Handler:    _AdminService_GetRounds_Handler,
		},
		{
			MethodName: "CreateNote",
			Handler:    _AdminService_CreateNote_Handler,
		},
		{
			MethodName: "GetMarketHourConfig",
			Handler:    _AdminService_GetMarketHourConfig_Handler,
		},
		{
			MethodName: "UpdateMarketHourConfig",
			Handler:    _AdminService_UpdateMarketHourConfig_Handler,
		},
		{
			MethodName: "GetTxRequestQueue",
			Handler:    _AdminService_GetTxRequestQueue_Handler,
		},
		{
			MethodName: "DeleteTxRequests",
			Handler:    _AdminService_DeleteTxRequests_Handler,
		},
		{
			MethodName: "Withdraw",
			Handler:    _AdminService_Withdraw_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "ark/v1/admin.proto",
}
