// Package pipelinepb contains hand-written stubs matching pipeline.proto.
// Replace with protoc-generated code when protoc + grpc plugin are available:
//   protoc --go_out=. --go-grpc_out=. proto/pipeline.proto
package pipelinepb

import (
	"context"

	"google.golang.org/grpc"
)

// ── Request / Response types ────────────────────────────────────────────────

type InitRequest struct {
	Bankroll float64
}
type InitResponse struct {
	Ok       bool
	Message  string
	Bankroll float64
}

type ResetRequest struct{}
type ResetResponse struct {
	Ok      bool
	Message string
}

type GetNextRequest struct{}
type GetNextResponse struct {
	PipelineComplete bool
	Casino           *CasinoInfo
	Ev               float64
	ExpectedOutput   float64
	CurrentBankroll  float64
	BetSize          float64
}

type MarkDoneRequest struct {
	CasinoId string
	Result   float64
}
type MarkDoneResponse struct {
	Ok           bool
	Message      string
	InputAmount  float64
	OutputAmount float64
	EvActual     float64
	NewBankroll  float64
}

type SkipCasinoRequest struct {
	CasinoId string
	Reason   string
}
type SkipCasinoResponse struct {
	Ok      bool
	Message string
}

type GetStatusRequest struct{}
type GetStatusResponse struct {
	InitialBankroll float64
	CurrentBankroll float64
	Pnl             float64
	TotalCasinos    int32
	DoneCount       int32
	TotalEvActual   float64
	Casinos         []*CasinoStatus
	Steps           []*StepLogEntry
}

type ListCasinosRequest struct{}
type ListCasinosResponse struct {
	Casinos []*CasinoInfo
	Banned  []*BannedInfo
}

type CalcEVRequest struct {
	CasinoId string
}
type CalcEVResponse struct {
	CasinoName      string
	BonusValue      float64
	WagerReq        float64
	WagerAmount     float64
	GameName        string
	GameRtp         float64
	HouseEdge       float64
	ExpectedLoss    float64
	Ev              float64
	CurrentBankroll float64
	ExpectedOutput  float64
	BetSize         float64
	EstMinutes      float64
}

type GetStrategyRequest struct {
	CasinoId string
}
type GetStrategyResponse struct {
	CasinoName  string
	GameType    string
	RiskLevel   string
	StrategyRef string
	Tips        []string
}

type WatchStateRequest struct{}
type StateUpdate struct {
	Event           string
	CasinoId        string
	CurrentBankroll float64
	TotalEvActual   float64
	DoneCount       int32
	TotalCasinos    int32
	Timestamp       string
}

// ── Shared types ────────────────────────────────────────────────────────────

type CasinoInfo struct {
	Id                     string
	Name                   string
	Priority               int32
	Tier                   string
	Url                    string
	BonusType              string
	BonusAmount            float64
	Wager                  float64
	MaxCashout             float64
	ExpiryDays             int32
	BestGameName           string
	BestGameRtp            float64
	DepositMethods         []string
	WithdrawalMethods      []string
	WithdrawalSpeedMinutes int32
	KycRequired            bool
	SpidRequired           bool
	Jurisdiction           string
	Notes                  string
	Ev                     float64
}

type CasinoStatus struct {
	Id           string
	Name         string
	Priority     int32
	Tier         string
	Status       string
	Ev           float64
	EvActual     float64
	InputAmount  float64
	OutputAmount float64
}

type BannedInfo struct {
	Id     string
	Name   string
	Reason string
}

type StepLogEntry struct {
	CasinoId   string
	CasinoName string
	Timestamp  string
	Input      float64
	Output     float64
	Ev         float64
}

// ── Service interfaces ──────────────────────────────────────────────────────

type PipelineServiceServer interface {
	Init(context.Context, *InitRequest) (*InitResponse, error)
	Reset(context.Context, *ResetRequest) (*ResetResponse, error)
	GetNext(context.Context, *GetNextRequest) (*GetNextResponse, error)
	MarkDone(context.Context, *MarkDoneRequest) (*MarkDoneResponse, error)
	SkipCasino(context.Context, *SkipCasinoRequest) (*SkipCasinoResponse, error)
	GetStatus(context.Context, *GetStatusRequest) (*GetStatusResponse, error)
	ListCasinos(context.Context, *ListCasinosRequest) (*ListCasinosResponse, error)
	CalcEV(context.Context, *CalcEVRequest) (*CalcEVResponse, error)
	GetStrategy(context.Context, *GetStrategyRequest) (*GetStrategyResponse, error)
	WatchState(*WatchStateRequest, PipelineService_WatchStateServer) error
}

type UnimplementedPipelineServiceServer struct{}

func (UnimplementedPipelineServiceServer) Init(context.Context, *InitRequest) (*InitResponse, error) {
	return nil, grpc.ErrServerStopped
}
func (UnimplementedPipelineServiceServer) Reset(context.Context, *ResetRequest) (*ResetResponse, error) {
	return nil, grpc.ErrServerStopped
}
func (UnimplementedPipelineServiceServer) GetNext(context.Context, *GetNextRequest) (*GetNextResponse, error) {
	return nil, grpc.ErrServerStopped
}
func (UnimplementedPipelineServiceServer) MarkDone(context.Context, *MarkDoneRequest) (*MarkDoneResponse, error) {
	return nil, grpc.ErrServerStopped
}
func (UnimplementedPipelineServiceServer) SkipCasino(context.Context, *SkipCasinoRequest) (*SkipCasinoResponse, error) {
	return nil, grpc.ErrServerStopped
}
func (UnimplementedPipelineServiceServer) GetStatus(context.Context, *GetStatusRequest) (*GetStatusResponse, error) {
	return nil, grpc.ErrServerStopped
}
func (UnimplementedPipelineServiceServer) ListCasinos(context.Context, *ListCasinosRequest) (*ListCasinosResponse, error) {
	return nil, grpc.ErrServerStopped
}
func (UnimplementedPipelineServiceServer) CalcEV(context.Context, *CalcEVRequest) (*CalcEVResponse, error) {
	return nil, grpc.ErrServerStopped
}
func (UnimplementedPipelineServiceServer) GetStrategy(context.Context, *GetStrategyRequest) (*GetStrategyResponse, error) {
	return nil, grpc.ErrServerStopped
}
func (UnimplementedPipelineServiceServer) WatchState(*WatchStateRequest, PipelineService_WatchStateServer) error {
	return grpc.ErrServerStopped
}

// ── Stream interfaces ───────────────────────────────────────────────────────

type PipelineService_WatchStateServer interface {
	Send(*StateUpdate) error
	grpc.ServerStream
}

type PipelineService_WatchStateClient interface {
	Recv() (*StateUpdate, error)
	grpc.ClientStream
}

// ── Client ──────────────────────────────────────────────────────────────────

type PipelineServiceClient interface {
	Init(ctx context.Context, in *InitRequest, opts ...grpc.CallOption) (*InitResponse, error)
	Reset(ctx context.Context, in *ResetRequest, opts ...grpc.CallOption) (*ResetResponse, error)
	GetNext(ctx context.Context, in *GetNextRequest, opts ...grpc.CallOption) (*GetNextResponse, error)
	MarkDone(ctx context.Context, in *MarkDoneRequest, opts ...grpc.CallOption) (*MarkDoneResponse, error)
	SkipCasino(ctx context.Context, in *SkipCasinoRequest, opts ...grpc.CallOption) (*SkipCasinoResponse, error)
	GetStatus(ctx context.Context, in *GetStatusRequest, opts ...grpc.CallOption) (*GetStatusResponse, error)
	ListCasinos(ctx context.Context, in *ListCasinosRequest, opts ...grpc.CallOption) (*ListCasinosResponse, error)
	CalcEV(ctx context.Context, in *CalcEVRequest, opts ...grpc.CallOption) (*CalcEVResponse, error)
	GetStrategy(ctx context.Context, in *GetStrategyRequest, opts ...grpc.CallOption) (*GetStrategyResponse, error)
	WatchState(ctx context.Context, in *WatchStateRequest, opts ...grpc.CallOption) (PipelineService_WatchStateClient, error)
}

// ── Registration ────────────────────────────────────────────────────────────

func RegisterPipelineServiceServer(s *grpc.Server, srv PipelineServiceServer) {
	s.RegisterService(&_PipelineService_serviceDesc, srv)
}

func NewPipelineServiceClient(cc grpc.ClientConnInterface) PipelineServiceClient {
	return &pipelineServiceClient{cc}
}

type pipelineServiceClient struct {
	cc grpc.ClientConnInterface
}

func (c *pipelineServiceClient) Init(ctx context.Context, in *InitRequest, opts ...grpc.CallOption) (*InitResponse, error) {
	out := new(InitResponse)
	err := c.cc.Invoke(ctx, "/pipeline.PipelineService/Init", in, out, opts...)
	return out, err
}

func (c *pipelineServiceClient) Reset(ctx context.Context, in *ResetRequest, opts ...grpc.CallOption) (*ResetResponse, error) {
	out := new(ResetResponse)
	err := c.cc.Invoke(ctx, "/pipeline.PipelineService/Reset", in, out, opts...)
	return out, err
}

func (c *pipelineServiceClient) GetNext(ctx context.Context, in *GetNextRequest, opts ...grpc.CallOption) (*GetNextResponse, error) {
	out := new(GetNextResponse)
	err := c.cc.Invoke(ctx, "/pipeline.PipelineService/GetNext", in, out, opts...)
	return out, err
}

func (c *pipelineServiceClient) MarkDone(ctx context.Context, in *MarkDoneRequest, opts ...grpc.CallOption) (*MarkDoneResponse, error) {
	out := new(MarkDoneResponse)
	err := c.cc.Invoke(ctx, "/pipeline.PipelineService/MarkDone", in, out, opts...)
	return out, err
}

func (c *pipelineServiceClient) SkipCasino(ctx context.Context, in *SkipCasinoRequest, opts ...grpc.CallOption) (*SkipCasinoResponse, error) {
	out := new(SkipCasinoResponse)
	err := c.cc.Invoke(ctx, "/pipeline.PipelineService/SkipCasino", in, out, opts...)
	return out, err
}

func (c *pipelineServiceClient) GetStatus(ctx context.Context, in *GetStatusRequest, opts ...grpc.CallOption) (*GetStatusResponse, error) {
	out := new(GetStatusResponse)
	err := c.cc.Invoke(ctx, "/pipeline.PipelineService/GetStatus", in, out, opts...)
	return out, err
}

func (c *pipelineServiceClient) ListCasinos(ctx context.Context, in *ListCasinosRequest, opts ...grpc.CallOption) (*ListCasinosResponse, error) {
	out := new(ListCasinosResponse)
	err := c.cc.Invoke(ctx, "/pipeline.PipelineService/ListCasinos", in, out, opts...)
	return out, err
}

func (c *pipelineServiceClient) CalcEV(ctx context.Context, in *CalcEVRequest, opts ...grpc.CallOption) (*CalcEVResponse, error) {
	out := new(CalcEVResponse)
	err := c.cc.Invoke(ctx, "/pipeline.PipelineService/CalcEV", in, out, opts...)
	return out, err
}

func (c *pipelineServiceClient) GetStrategy(ctx context.Context, in *GetStrategyRequest, opts ...grpc.CallOption) (*GetStrategyResponse, error) {
	out := new(GetStrategyResponse)
	err := c.cc.Invoke(ctx, "/pipeline.PipelineService/GetStrategy", in, out, opts...)
	return out, err
}

func (c *pipelineServiceClient) WatchState(ctx context.Context, in *WatchStateRequest, opts ...grpc.CallOption) (PipelineService_WatchStateClient, error) {
	stream, err := c.cc.NewStream(ctx, &_PipelineService_serviceDesc.Streams[0], "/pipeline.PipelineService/WatchState", opts...)
	if err != nil {
		return nil, err
	}
	x := &watchStateClient{stream}
	if err := x.ClientStream.SendMsg(in); err != nil {
		return nil, err
	}
	if err := x.ClientStream.CloseSend(); err != nil {
		return nil, err
	}
	return x, nil
}

type watchStateClient struct {
	grpc.ClientStream
}

func (x *watchStateClient) Recv() (*StateUpdate, error) {
	m := new(StateUpdate)
	if err := x.ClientStream.RecvMsg(m); err != nil {
		return nil, err
	}
	return m, nil
}

// ── gRPC service descriptors ────────────────────────────────────────────────

var _PipelineService_serviceDesc = grpc.ServiceDesc{
	ServiceName: "pipeline.PipelineService",
	HandlerType: (*PipelineServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Init", Handler: _PipelineService_Init_Handler},
		{MethodName: "Reset", Handler: _PipelineService_Reset_Handler},
		{MethodName: "GetNext", Handler: _PipelineService_GetNext_Handler},
		{MethodName: "MarkDone", Handler: _PipelineService_MarkDone_Handler},
		{MethodName: "SkipCasino", Handler: _PipelineService_SkipCasino_Handler},
		{MethodName: "GetStatus", Handler: _PipelineService_GetStatus_Handler},
		{MethodName: "ListCasinos", Handler: _PipelineService_ListCasinos_Handler},
		{MethodName: "CalcEV", Handler: _PipelineService_CalcEV_Handler},
		{MethodName: "GetStrategy", Handler: _PipelineService_GetStrategy_Handler},
	},
	Streams: []grpc.StreamDesc{
		{
			StreamName:   "WatchState",
			Handler:      _PipelineService_WatchState_Handler,
			ServerStreams: true,
		},
	},
	Metadata: "proto/pipeline.proto",
}

// ── Method handlers ─────────────────────────────────────────────────────────

func _PipelineService_Init_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InitRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PipelineServiceServer).Init(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/pipeline.PipelineService/Init"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PipelineServiceServer).Init(ctx, req.(*InitRequest))
	})
}

func _PipelineService_Reset_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ResetRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PipelineServiceServer).Reset(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/pipeline.PipelineService/Reset"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PipelineServiceServer).Reset(ctx, req.(*ResetRequest))
	})
}

func _PipelineService_GetNext_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetNextRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PipelineServiceServer).GetNext(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/pipeline.PipelineService/GetNext"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PipelineServiceServer).GetNext(ctx, req.(*GetNextRequest))
	})
}

func _PipelineService_MarkDone_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(MarkDoneRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PipelineServiceServer).MarkDone(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/pipeline.PipelineService/MarkDone"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PipelineServiceServer).MarkDone(ctx, req.(*MarkDoneRequest))
	})
}

func _PipelineService_SkipCasino_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(SkipCasinoRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PipelineServiceServer).SkipCasino(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/pipeline.PipelineService/SkipCasino"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PipelineServiceServer).SkipCasino(ctx, req.(*SkipCasinoRequest))
	})
}

func _PipelineService_GetStatus_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetStatusRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PipelineServiceServer).GetStatus(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/pipeline.PipelineService/GetStatus"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PipelineServiceServer).GetStatus(ctx, req.(*GetStatusRequest))
	})
}

func _PipelineService_ListCasinos_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListCasinosRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PipelineServiceServer).ListCasinos(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/pipeline.PipelineService/ListCasinos"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PipelineServiceServer).ListCasinos(ctx, req.(*ListCasinosRequest))
	})
}

func _PipelineService_CalcEV_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(CalcEVRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PipelineServiceServer).CalcEV(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/pipeline.PipelineService/CalcEV"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PipelineServiceServer).CalcEV(ctx, req.(*CalcEVRequest))
	})
}

func _PipelineService_GetStrategy_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetStrategyRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(PipelineServiceServer).GetStrategy(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/pipeline.PipelineService/GetStrategy"}
	return interceptor(ctx, in, info, func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(PipelineServiceServer).GetStrategy(ctx, req.(*GetStrategyRequest))
	})
}

func _PipelineService_WatchState_Handler(srv interface{}, stream grpc.ServerStream) error {
	m := new(WatchStateRequest)
	if err := stream.RecvMsg(m); err != nil {
		return err
	}
	return srv.(PipelineServiceServer).WatchState(m, &watchStateServer{stream})
}

type watchStateServer struct {
	grpc.ServerStream
}

func (x *watchStateServer) Send(m *StateUpdate) error {
	return x.ServerStream.SendMsg(m)
}
