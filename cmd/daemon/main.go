package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"time"

	"github.com/biodoia/bonus-pipeline/pkg/engine"
	pb "github.com/biodoia/bonus-pipeline/proto/pipelinepb"
	"google.golang.org/grpc"
)

var (
	listenAddr = flag.String("addr", ":50051", "gRPC listen address")
	dataDir    = flag.String("data", ".", "directory containing pipeline.json and state.json")
)

type server struct {
	pb.UnimplementedPipelineServiceServer
	mu       sync.RWMutex
	db       *engine.PipelineDB
	state    *engine.PipelineState
	dataDir  string
	watchers []chan *pb.StateUpdate
}

func newServer(dataDir string) (*server, error) {
	dbPath := filepath.Join(dataDir, "pipeline.json")
	statePath := filepath.Join(dataDir, "state.json")

	db, err := engine.LoadDB(dbPath)
	if err != nil {
		return nil, fmt.Errorf("load DB: %w", err)
	}

	state, err := engine.LoadState(statePath)
	if err != nil {
		return nil, fmt.Errorf("load state: %w", err)
	}

	return &server{
		db:      db,
		state:   state,
		dataDir: dataDir,
	}, nil
}

func (s *server) save() error {
	return engine.SaveState(filepath.Join(s.dataDir, "state.json"), s.state)
}

func (s *server) broadcast(event, casinoID string) {
	stats := engine.GetStats(s.db, s.state)
	update := &pb.StateUpdate{
		Event:           event,
		CasinoId:        casinoID,
		CurrentBankroll: s.state.CurrentBankroll,
		TotalEvActual:   s.state.TotalEVActual,
		DoneCount:       int32(stats.DoneCount),
		TotalCasinos:    int32(stats.TotalCasinos),
		Timestamp:       time.Now().Format(time.RFC3339),
	}
	for _, ch := range s.watchers {
		select {
		case ch <- update:
		default:
		}
	}
}

// ── gRPC methods ────────────────────────────────────────────────────────────

func (s *server) Init(_ context.Context, req *pb.InitRequest) (*pb.InitResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	if err := engine.InitPipeline(s.state, req.Bankroll); err != nil {
		return &pb.InitResponse{Ok: false, Message: err.Error()}, nil
	}
	if err := s.save(); err != nil {
		return &pb.InitResponse{Ok: false, Message: err.Error()}, nil
	}
	s.broadcast("init", "")
	return &pb.InitResponse{
		Ok:       true,
		Message:  fmt.Sprintf("Pipeline inizializzata con €%.2f", req.Bankroll),
		Bankroll: req.Bankroll,
	}, nil
}

func (s *server) Reset(_ context.Context, _ *pb.ResetRequest) (*pb.ResetResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	engine.ResetPipeline(s.state)
	if err := s.save(); err != nil {
		return &pb.ResetResponse{Ok: false, Message: err.Error()}, nil
	}
	s.broadcast("reset", "")
	return &pb.ResetResponse{Ok: true, Message: "Pipeline resettata"}, nil
}

func (s *server) GetNext(_ context.Context, _ *pb.GetNextRequest) (*pb.GetNextResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c := engine.NextPendingCasino(s.db, s.state)
	if c == nil {
		return &pb.GetNextResponse{PipelineComplete: true}, nil
	}

	ev := engine.CalcEV(*c, s.state.CurrentBankroll)
	expectedOut := engine.CalcExpectedOutput(*c, s.state.CurrentBankroll)
	betSize := engine.CalcBetSize(*c)

	return &pb.GetNextResponse{
		PipelineComplete: false,
		Casino:           casinoToProto(*c, s.state.CurrentBankroll),
		Ev:               ev,
		ExpectedOutput:   expectedOut,
		CurrentBankroll:  s.state.CurrentBankroll,
		BetSize:          betSize,
	}, nil
}

func (s *server) MarkDone(_ context.Context, req *pb.MarkDoneRequest) (*pb.MarkDoneResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	inputAmount := s.state.CurrentBankroll
	if err := engine.MarkDone(s.db, s.state, req.CasinoId, req.Result); err != nil {
		return &pb.MarkDoneResponse{Ok: false, Message: err.Error()}, nil
	}
	if err := s.save(); err != nil {
		return &pb.MarkDoneResponse{Ok: false, Message: err.Error()}, nil
	}

	evActual := req.Result - inputAmount
	s.broadcast("done", req.CasinoId)

	return &pb.MarkDoneResponse{
		Ok:           true,
		Message:      fmt.Sprintf("Casino %s completato", req.CasinoId),
		InputAmount:  inputAmount,
		OutputAmount: req.Result,
		EvActual:     evActual,
		NewBankroll:  s.state.CurrentBankroll,
	}, nil
}

func (s *server) SkipCasino(_ context.Context, req *pb.SkipCasinoRequest) (*pb.SkipCasinoResponse, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	c := engine.FindCasino(s.db, req.CasinoId)
	if c == nil {
		return &pb.SkipCasinoResponse{Ok: false, Message: "casino not found"}, nil
	}

	cs := s.state.Casinos[req.CasinoId]
	cs.Status = "skipped"
	cs.Notes = req.Reason
	s.state.Casinos[req.CasinoId] = cs

	if err := s.save(); err != nil {
		return &pb.SkipCasinoResponse{Ok: false, Message: err.Error()}, nil
	}

	s.broadcast("skip", req.CasinoId)
	return &pb.SkipCasinoResponse{
		Ok:      true,
		Message: fmt.Sprintf("Casino %s skippato", req.CasinoId),
	}, nil
}

func (s *server) GetStatus(_ context.Context, _ *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := engine.GetStats(s.db, s.state)
	sorted := engine.SortedCasinos(s.db.Casinos)

	var casinoStatuses []*pb.CasinoStatus
	for _, c := range sorted {
		cs := s.state.Casinos[c.ID]
		status := cs.Status
		if status == "" {
			status = "pending"
		}
		casinoStatuses = append(casinoStatuses, &pb.CasinoStatus{
			Id:           c.ID,
			Name:         c.Name,
			Priority:     int32(c.Priority),
			Tier:         c.Tier,
			Status:       status,
			Ev:           engine.CalcEV(c, s.state.CurrentBankroll),
			EvActual:     cs.EVActual,
			InputAmount:  cs.InputAmount,
			OutputAmount: cs.OutputAmount,
		})
	}

	var steps []*pb.StepLogEntry
	for _, sl := range s.state.Steps {
		steps = append(steps, &pb.StepLogEntry{
			CasinoId:   sl.CasinoID,
			CasinoName: sl.CasinoName,
			Timestamp:  sl.At.Format(time.RFC3339),
			Input:      sl.Input,
			Output:     sl.Output,
			Ev:         sl.EV,
		})
	}

	return &pb.GetStatusResponse{
		InitialBankroll: s.state.InitialBankroll,
		CurrentBankroll: s.state.CurrentBankroll,
		Pnl:            stats.PnL,
		TotalCasinos:   int32(stats.TotalCasinos),
		DoneCount:      int32(stats.DoneCount),
		TotalEvActual:  s.state.TotalEVActual,
		Casinos:        casinoStatuses,
		Steps:          steps,
	}, nil
}

func (s *server) ListCasinos(_ context.Context, _ *pb.ListCasinosRequest) (*pb.ListCasinosResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	sorted := engine.SortedCasinos(s.db.Casinos)
	var casinos []*pb.CasinoInfo
	for _, c := range sorted {
		casinos = append(casinos, casinoToProto(c, s.state.CurrentBankroll))
	}

	var banned []*pb.BannedInfo
	for _, b := range s.db.Banned {
		banned = append(banned, &pb.BannedInfo{
			Id:     b.ID,
			Name:   b.Name,
			Reason: b.Reason,
		})
	}

	return &pb.ListCasinosResponse{Casinos: casinos, Banned: banned}, nil
}

func (s *server) CalcEV(_ context.Context, req *pb.CalcEVRequest) (*pb.CalcEVResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c := engine.FindCasino(s.db, req.CasinoId)
	if c == nil {
		return nil, fmt.Errorf("casino '%s' not found", req.CasinoId)
	}

	bd := engine.CalcEVBreakdown(*c, s.state.CurrentBankroll)

	return &pb.CalcEVResponse{
		CasinoName:      c.Name,
		BonusValue:      bd.BonusValue,
		WagerReq:        bd.WagerReq,
		WagerAmount:     bd.WagerAmount,
		GameName:        c.BestGame.Name,
		GameRtp:         c.BestGame.RTP,
		HouseEdge:       bd.HouseEdge,
		ExpectedLoss:    bd.ExpectedLoss,
		Ev:              bd.EV,
		CurrentBankroll: s.state.CurrentBankroll,
		ExpectedOutput:  bd.ExpectedOut,
		BetSize:         bd.BetSize,
		EstMinutes:      bd.EstMinutes,
	}, nil
}

func (s *server) GetStrategy(_ context.Context, req *pb.GetStrategyRequest) (*pb.GetStrategyResponse, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	c := engine.FindCasino(s.db, req.CasinoId)
	if c == nil {
		return nil, fmt.Errorf("casino '%s' not found", req.CasinoId)
	}

	strat := engine.GetGameStrategy(*c)
	return &pb.GetStrategyResponse{
		CasinoName:  c.Name,
		GameType:    strat.GameType,
		RiskLevel:   strat.RiskLevel,
		StrategyRef: strat.StrategyRef,
		Tips:        strat.Tips,
	}, nil
}

func (s *server) WatchState(_ *pb.WatchStateRequest, stream pb.PipelineService_WatchStateServer) error {
	ch := make(chan *pb.StateUpdate, 16)

	s.mu.Lock()
	s.watchers = append(s.watchers, ch)
	s.mu.Unlock()

	defer func() {
		s.mu.Lock()
		for i, w := range s.watchers {
			if w == ch {
				s.watchers = append(s.watchers[:i], s.watchers[i+1:]...)
				break
			}
		}
		s.mu.Unlock()
		close(ch)
	}()

	for {
		select {
		case update := <-ch:
			if err := stream.Send(update); err != nil {
				return err
			}
		case <-stream.Context().Done():
			return nil
		}
	}
}

// ── AgentService implementation ──────────────────────────────────────────────

type agentServer struct {
	pb.UnimplementedAgentServiceServer
	pipeline *server
	logCh    chan *pb.AgentLogEntry
}

func (a *agentServer) GetTask(_ context.Context, req *pb.GetTaskRequest) (*pb.AgentTask, error) {
	a.pipeline.mu.RLock()
	defer a.pipeline.mu.RUnlock()

	c := engine.NextPendingCasino(a.pipeline.db, a.pipeline.state)
	if c == nil {
		return &pb.AgentTask{TaskId: "", TaskType: "none"}, nil
	}

	strat := engine.GetGameStrategy(*c)
	bonusVal := engine.CalcBonusValue(*c)
	betSize := engine.CalcBetSize(*c)
	wagerTarget := c.Bonus.Wager * bonusVal
	stopLoss := bonusVal * a.pipeline.db.Rules.StopLossPct

	// Determine task type based on casino state
	cs := a.pipeline.state.Casinos[c.ID]
	taskType := "register"
	switch cs.Status {
	case "active":
		taskType = "do_wagering"
	}

	return &pb.AgentTask{
		TaskId:      fmt.Sprintf("%s-%s-%d", c.ID, taskType, time.Now().Unix()),
		TaskType:    taskType,
		CasinoId:    c.ID,
		Casino:      casinoToProto(*c, a.pipeline.state.CurrentBankroll),
		StrategyRef: strat.StrategyRef,
		Tips:        strat.Tips,
		BetSize:     betSize,
		StopLoss:    stopLoss,
		WagerTarget: wagerTarget,
	}, nil
}

func (a *agentServer) ReportTaskStatus(_ context.Context, req *pb.TaskStatusReport) (*pb.TaskStatusResponse, error) {
	log.Printf("[agent:%s] task=%s status=%s msg=%s balance=%.2f wagered=%.2f",
		req.AgentId, req.TaskId, req.Status, req.Message,
		req.CurrentBalance, req.WageredSoFar)

	// Forward to watchers as a state update
	a.pipeline.broadcast("agent_"+req.Status, req.TaskId)

	instruction := "continue"
	if req.Status == "failed" {
		instruction = "stop"
	}

	return &pb.TaskStatusResponse{Ok: true, Instruction: instruction}, nil
}

func (a *agentServer) StreamAgentLog(stream pb.AgentService_StreamAgentLogServer) error {
	for {
		entry, err := stream.Recv()
		if err != nil {
			return stream.SendAndClose(&pb.AgentLogAck{Ok: true})
		}
		log.Printf("[agent-log:%s] [%s] %s", entry.AgentId, entry.Level, entry.Message)
		if a.logCh != nil {
			select {
			case a.logCh <- entry:
			default:
			}
		}
	}
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func casinoToProto(c engine.Casino, bankroll float64) *pb.CasinoInfo {
	return &pb.CasinoInfo{
		Id:                    c.ID,
		Name:                  c.Name,
		Priority:              int32(c.Priority),
		Tier:                  c.Tier,
		Url:                   c.URL,
		BonusType:             c.Bonus.Type,
		BonusAmount:           c.Bonus.Amount,
		Wager:                 c.Bonus.Wager,
		MaxCashout:            c.Bonus.MaxCashout,
		ExpiryDays:            int32(c.Bonus.ExpiryDays),
		BestGameName:          c.BestGame.Name,
		BestGameRtp:           c.BestGame.RTP,
		DepositMethods:        c.Payment.Deposit,
		WithdrawalMethods:     c.Payment.Withdrawal,
		WithdrawalSpeedMinutes: int32(c.Payment.SpeedMinutes),
		KycRequired:           c.KYCRequired,
		SpidRequired:          c.SPIDRequired,
		Jurisdiction:          c.Jurisdiction,
		Notes:                 c.Notes,
		Ev:                    engine.CalcEV(c, bankroll),
	}
}

// ── Main ────────────────────────────────────────────────────────────────────

func main() {
	flag.Parse()

	srv, err := newServer(*dataDir)
	if err != nil {
		log.Fatalf("Failed to initialize server: %v", err)
	}

	lis, err := net.Listen("tcp", *listenAddr)
	if err != nil {
		log.Fatalf("Failed to listen on %s: %v", *listenAddr, err)
	}

	grpcServer := grpc.NewServer()
	pb.RegisterPipelineServiceServer(grpcServer, srv)
	// H8: AgentService disabled until protoc-generated stubs with proper
	// Marshal/Unmarshal are available. Hand-written stubs lack encoding,
	// causing gRPC serialization failures at runtime.
	// TODO: re-enable after running: make proto
	// pb.RegisterAgentServiceServer(grpcServer, &agentServer{
	// 	pipeline: srv,
	// 	logCh:    make(chan *pb.AgentLogEntry, 64),
	// })

	// Graceful shutdown
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigCh
		log.Println("Shutting down gRPC server...")
		grpcServer.GracefulStop()
	}()

	log.Printf("bonus-pipeline daemon listening on %s (data: %s)", *listenAddr, *dataDir)
	if err := grpcServer.Serve(lis); err != nil {
		log.Fatalf("gRPC server error: %v", err)
	}
}
