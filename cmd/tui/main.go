package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/biodoia/framegotui"
	"github.com/biodoia/bonus-pipeline/pkg/engine"
	pb "github.com/biodoia/bonus-pipeline/proto/pipelinepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	daemonAddr = flag.String("addr", "localhost:50051", "daemon gRPC address")
	dataDir    = flag.String("data", ".", "data directory for local strategy lookup")
)

// selectedCasino tracks which casino the advisor panel shows strategy for.
var selectedCasino string

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*daemonAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Cannot connect to daemon at %s: %v", *daemonAddr, err)
	}
	defer conn.Close()

	client := pb.NewPipelineServiceClient(conn)

	// Load local DB for strategy lookup (engine functions need Casino structs)
	db, _ := engine.LoadDB(*dataDir + "/pipeline.json")

	app := framegotui.NewApp("Bonus Pipeline", framegotui.WithTheme(framegotui.CyberpunkTheme))

	// ── Panel 1: Pipeline Status ────────────────────────────────────────
	pipelinePanel := framegotui.NewPanel("Pipeline", framegotui.PanelOpts{
		Position: framegotui.TopLeft,
		Border:   framegotui.DoubleBorder,
	})
	pipelinePanel.SetRenderFunc(func(w, h int) string {
		return renderPipelinePanel(client, w, h)
	})

	// ── Panel 2: EV Calculator ──────────────────────────────────────────
	evPanel := framegotui.NewPanel("EV Calc", framegotui.PanelOpts{
		Position: framegotui.TopRight,
		Border:   framegotui.SingleBorder,
	})
	evPanel.SetRenderFunc(func(w, h int) string {
		return renderEVPanel(client, w, h)
	})

	// ── Panel 3: Live Advisor (BJ/Poker/Slots strategy) ─────────────────
	advisorPanel := framegotui.NewPanel("Live Advisor", framegotui.PanelOpts{
		Position: framegotui.BottomLeft,
		Border:   framegotui.DoubleBorder,
	})
	advisorPanel.SetRenderFunc(func(w, h int) string {
		return renderAdvisorPanel(client, db, w, h)
	})

	// ── Panel 4: Agent Log ──────────────────────────────────────────────
	agentLogPanel := framegotui.NewPanel("Agent Log", framegotui.PanelOpts{
		Position: framegotui.BottomRight,
		Border:   framegotui.SingleBorder,
	})

	app.AddPanel(pipelinePanel)
	app.AddPanel(evPanel)
	app.AddPanel(advisorPanel)
	app.AddPanel(agentLogPanel)

	// ── Keybindings ─────────────────────────────────────────────────────

	app.Bind("n", "Next", func() {
		resp, err := client.GetNext(context.Background(), &pb.GetNextRequest{})
		if err != nil {
			agentLogPanel.Log("ERR: %v", err)
			return
		}
		if resp.PipelineComplete {
			agentLogPanel.Log("Pipeline completata!")
		} else {
			selectedCasino = resp.Casino.Id
			agentLogPanel.Log("NEXT: %s (EV: +%.2f)", resp.Casino.Name, resp.Ev)
		}
		app.Refresh()
	})

	app.Bind("d", "Done", func() {
		app.Prompt("Casino ID:", func(id string) {
			app.Prompt("Importo risultato:", func(amount string) {
				var result float64
				fmt.Sscanf(amount, "%f", &result)
				resp, err := client.MarkDone(context.Background(), &pb.MarkDoneRequest{
					CasinoId: id,
					Result:   result,
				})
				if err != nil {
					agentLogPanel.Log("ERR: %v", err)
					return
				}
				if resp.Ok {
					agentLogPanel.Log("DONE %s | in:%.2f out:%.2f ev:%+.2f | bankroll:%.2f",
						id, resp.InputAmount, resp.OutputAmount, resp.EvActual, resp.NewBankroll)
				} else {
					agentLogPanel.Log("FAIL: %s", resp.Message)
				}
				app.Refresh()
			})
		})
	})

	app.Bind("s", "Skip", func() {
		app.Prompt("Casino ID da skippare:", func(id string) {
			resp, err := client.SkipCasino(context.Background(), &pb.SkipCasinoRequest{
				CasinoId: id,
				Reason:   "skipped via TUI",
			})
			if err != nil {
				agentLogPanel.Log("ERR: %v", err)
				return
			}
			if resp.Ok {
				agentLogPanel.Log("SKIP: %s", id)
			}
			app.Refresh()
		})
	})

	app.Bind("i", "Init", func() {
		app.Prompt("Bankroll iniziale:", func(amount string) {
			var bankroll float64
			fmt.Sscanf(amount, "%f", &bankroll)
			resp, err := client.Init(context.Background(), &pb.InitRequest{Bankroll: bankroll})
			if err != nil {
				agentLogPanel.Log("ERR: %v", err)
				return
			}
			agentLogPanel.Log(resp.Message)
			app.Refresh()
		})
	})

	app.Bind("b", "BJ ref", func() {
		agentLogPanel.Log("--- BJ QUICK REF ---")
		for _, line := range strings.Split(engine.BJQuickRef(), "\n") {
			if line != "" {
				agentLogPanel.Log(line)
			}
		}
		app.Refresh()
	})

	app.Bind("p", "Poker ref", func() {
		agentLogPanel.Log("--- POKER REF ---")
		for _, line := range strings.Split(engine.PokerQuickRef(), "\n") {
			if line != "" {
				agentLogPanel.Log(line)
			}
		}
		app.Refresh()
	})

	app.Bind("q", "Quit", func() {
		app.Quit()
	})

	app.SetRefreshInterval(5 * time.Second)

	go watchUpdates(client, agentLogPanel, app)

	agentLogPanel.Log("Connected to daemon at %s", *daemonAddr)
	agentLogPanel.Log("[n]ext [d]one [s]kip [i]nit [b]j [p]oker [q]uit")

	if err := app.Run(); err != nil {
		log.Fatalf("TUI error: %v", err)
	}
}

// ── Panel renderers ─────────────────────────────────────────────────────────

func renderPipelinePanel(client pb.PipelineServiceClient, w, h int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	resp, err := client.GetStatus(ctx, &pb.GetStatusRequest{})
	if err != nil {
		return fmt.Sprintf(" [error] %v", err)
	}

	var b strings.Builder
	b.WriteString(fmt.Sprintf(" Bankroll: %.2f (start: %.2f)\n", resp.CurrentBankroll, resp.InitialBankroll))

	pnlSign := "+"
	if resp.Pnl < 0 {
		pnlSign = ""
	}
	b.WriteString(fmt.Sprintf(" P&L: %s%.2f | %d/%d done\n", pnlSign, resp.Pnl, resp.DoneCount, resp.TotalCasinos))
	b.WriteString(strings.Repeat("-", min(w-2, 40)) + "\n")

	maxRows := h - 5
	for i, cs := range resp.Casinos {
		if i >= maxRows {
			b.WriteString(fmt.Sprintf(" ... +%d more\n", len(resp.Casinos)-maxRows))
			break
		}
		icon := statusIcon(cs.Status)
		name := truncStr(cs.Name, 16)
		if cs.Status == "done" {
			b.WriteString(fmt.Sprintf(" %s %-16s %+.2f\n", icon, name, cs.EvActual))
		} else {
			b.WriteString(fmt.Sprintf(" %s %-16s ev:+%.2f\n", icon, name, cs.Ev))
		}
	}

	if len(resp.Steps) > 0 {
		b.WriteString("\n Bankroll:\n")
		start := 0
		if len(resp.Steps) > 4 {
			start = len(resp.Steps) - 4
		}
		for _, s := range resp.Steps[start:] {
			name := truncStr(s.CasinoName, 10)
			barLen := int(s.Output / resp.InitialBankroll * 12)
			if barLen < 0 {
				barLen = 0
			}
			b.WriteString(fmt.Sprintf(" %-10s %6.2f %s\n", name, s.Output, strings.Repeat("#", barLen)))
		}
	}

	return b.String()
}

func renderEVPanel(client pb.PipelineServiceClient, w, h int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	next, err := client.GetNext(ctx, &pb.GetNextRequest{})
	if err != nil {
		return fmt.Sprintf(" [error] %v", err)
	}
	if next.PipelineComplete {
		return " Pipeline completata!\n Tutti i casino processati."
	}

	ev, err := client.CalcEV(ctx, &pb.CalcEVRequest{CasinoId: next.Casino.Id})
	if err != nil {
		return fmt.Sprintf(" [error] %v", err)
	}

	sep := strings.Repeat("-", min(w-2, 30))
	var b strings.Builder
	b.WriteString(fmt.Sprintf(" %s\n", ev.CasinoName))
	b.WriteString(" " + sep + "\n")
	b.WriteString(fmt.Sprintf(" Bonus:    %.2f\n", ev.BonusValue))
	b.WriteString(fmt.Sprintf(" Wager:    %.0fx = %.2f\n", ev.WagerReq, ev.WagerAmount))
	b.WriteString(fmt.Sprintf(" Gioco:    %s\n", truncStr(ev.GameName, w-12)))
	b.WriteString(fmt.Sprintf(" RTP:      %.1f%%\n", ev.GameRtp*100))
	b.WriteString(fmt.Sprintf(" Edge:     %.2f%%\n", ev.HouseEdge*100))
	b.WriteString(fmt.Sprintf(" Perdita:  %.2f\n", ev.ExpectedLoss))
	b.WriteString(" " + sep + "\n")
	b.WriteString(fmt.Sprintf(" EV:       %+.2f\n", ev.Ev))
	b.WriteString(fmt.Sprintf(" Output:   %.2f\n", ev.ExpectedOutput))
	b.WriteString(fmt.Sprintf(" Bet:      %.2f\n", ev.BetSize))
	if ev.EstMinutes > 0 {
		b.WriteString(fmt.Sprintf(" Tempo:    ~%.0f min\n", ev.EstMinutes))
	}

	return b.String()
}

func renderAdvisorPanel(client pb.PipelineServiceClient, db *engine.PipelineDB, w, h int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	next, err := client.GetNext(ctx, &pb.GetNextRequest{})
	if err != nil {
		return fmt.Sprintf(" [error] %v", err)
	}
	if next.PipelineComplete {
		return " Pipeline completata!\n Controlla il P&L nel pannello Pipeline."
	}

	c := next.Casino
	sep := strings.Repeat("-", min(w-2, 40))

	var b strings.Builder
	b.WriteString(fmt.Sprintf(" NEXT: %s\n", c.Name))
	b.WriteString(" " + sep + "\n")
	b.WriteString(fmt.Sprintf(" URL:  %s\n", c.Url))
	b.WriteString(fmt.Sprintf(" Tipo: %s | Wager: %.0fx\n", c.BonusType, c.Wager))
	b.WriteString(fmt.Sprintf(" Bet:  %.2f | EV: %+.2f\n", next.BetSize, next.Ev))
	b.WriteString(fmt.Sprintf(" Out:  %s (~%dmin)\n",
		strings.Join(c.WithdrawalMethods, "/"), c.WithdrawalSpeedMinutes))

	if c.KycRequired {
		b.WriteString(" ! KYC richiesto\n")
	}
	if c.SpidRequired {
		b.WriteString(" ! SPID richiesto\n")
	}

	// Game-specific strategy from local engine
	if db != nil {
		casino := engine.FindCasino(db, c.Id)
		if casino != nil {
			strat := engine.GetGameStrategy(*casino)
			b.WriteString(" " + sep + "\n")
			b.WriteString(fmt.Sprintf(" STRATEGIA: %s [risk:%s]\n", strings.ToUpper(strat.GameType), strat.RiskLevel))
			maxTips := h - 12
			if maxTips < 1 {
				maxTips = 1
			}
			for i, tip := range strat.Tips {
				if i >= maxTips {
					break
				}
				b.WriteString(fmt.Sprintf(" > %s\n", truncStr(tip, w-5)))
			}
		}
	}

	b.WriteString(" " + sep + "\n")
	b.WriteString(" [d]one [s]kip [b]j-ref [p]oker-ref\n")

	return b.String()
}

// ── Helpers ─────────────────────────────────────────────────────────────────

func statusIcon(s string) string {
	switch s {
	case "done":
		return "+"
	case "active":
		return ">"
	case "skipped":
		return "-"
	case "failed":
		return "!"
	default:
		return "o"
	}
}

func truncStr(s string, maxLen int) string {
	if len(s) <= maxLen || maxLen <= 3 {
		return s
	}
	return s[:maxLen-3] + "..."
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func watchUpdates(client pb.PipelineServiceClient, logPanel *framegotui.Panel, app *framegotui.App) {
	for {
		stream, err := client.WatchState(context.Background(), &pb.WatchStateRequest{})
		if err != nil {
			logPanel.Log("Watch: disconnected, retry in 3s...")
			time.Sleep(3 * time.Second)
			continue
		}

		for {
			update, err := stream.Recv()
			if err != nil {
				logPanel.Log("Stream lost, reconnecting...")
				break
			}
			logPanel.Log("[%s] %s %s bk=%.2f ev=%.2f %d/%d",
				update.Timestamp[11:19], // HH:MM:SS
				update.Event,
				update.CasinoId,
				update.CurrentBankroll,
				update.TotalEvActual,
				update.DoneCount,
				update.TotalCasinos)
			app.Refresh()
		}
		time.Sleep(2 * time.Second)
	}
}
