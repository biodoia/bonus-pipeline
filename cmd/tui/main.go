package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"strconv"
	"strings"
	"time"

	"github.com/biodoia/bonus-pipeline/pkg/engine"
	"github.com/biodoia/framegotui"
	pb "github.com/biodoia/bonus-pipeline/proto/pipelinepb"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

var (
	daemonAddr = flag.String("addr", "localhost:50051", "daemon gRPC address")
	dataDir    = flag.String("data", ".", "data directory for local strategy lookup")
)

// ── Global state ────────────────────────────────────────────────────────────

var (
	session *engine.GuidedSession // nil = no active session
	db      *engine.PipelineDB
)

func main() {
	flag.Parse()

	conn, err := grpc.Dial(*daemonAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		log.Fatalf("Cannot connect to daemon at %s: %v", *daemonAddr, err)
	}
	defer conn.Close()

	client := pb.NewPipelineServiceClient(conn)
	var dbErr error
	db, dbErr = engine.LoadDB(*dataDir + "/pipeline.json")
	if dbErr != nil {
		log.Printf("WARNING: cannot load pipeline.json: %v (guided mode will be unavailable)", dbErr)
	}

	app := framegotui.NewApp("Bonus Pipeline", framegotui.WithTheme(framegotui.CyberpunkTheme))

	// ── Panels ──────────────────────────────────────────────────────────
	pipelinePanel := framegotui.NewPanel("Pipeline", framegotui.PanelOpts{
		Position: framegotui.TopLeft,
		Border:   framegotui.DoubleBorder,
	})
	pipelinePanel.SetRenderFunc(func(w, h int) string {
		return renderPipelinePanel(client, w, h)
	})

	evPanel := framegotui.NewPanel("EV Calc", framegotui.PanelOpts{
		Position: framegotui.TopRight,
		Border:   framegotui.SingleBorder,
	})
	evPanel.SetRenderFunc(func(w, h int) string {
		return renderEVPanel(client, w, h)
	})

	advisorPanel := framegotui.NewPanel("Live Advisor", framegotui.PanelOpts{
		Position: framegotui.BottomLeft,
		Border:   framegotui.DoubleBorder,
	})
	advisorPanel.SetRenderFunc(func(w, h int) string {
		if session != nil && session.Active {
			return renderGuidedPanel(w, h)
		}
		return renderAdvisorPanel(client, w, h)
	})

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
			agentLogPanel.Log("NEXT: %s (EV: +%.2f)", resp.Casino.Name, resp.Ev)
		}
		app.Refresh()
	})

	app.Bind("g", "Guided", func() {
		if session != nil && session.Active {
			// End current session
			session.Active = false
			agentLogPanel.Log("Sessione guidata terminata: %s | mani:%d wagered:%.2f",
				session.CasinoName, session.HandsPlayed, session.WageredSoFar)
			session = nil
			app.Refresh()
			return
		}
		// Start guided session on next casino
		if db == nil {
			agentLogPanel.Log("ERR: pipeline.json non caricato")
			return
		}
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		next, err := client.GetNext(ctx, &pb.GetNextRequest{})
		if err != nil || next.PipelineComplete {
			agentLogPanel.Log("ERR: nessun casino disponibile")
			return
		}
		casino := engine.FindCasino(db, next.Casino.Id)
		if casino == nil {
			agentLogPanel.Log("ERR: casino %s non trovato in DB locale", next.Casino.Id)
			return
		}
		session = engine.NewGuidedSession(*casino, next.CurrentBankroll)
		agentLogPanel.Log("GUIDED: sessione %s avviata [%s]", session.CasinoName, session.Mode)
		agentLogPanel.Log("  [h]and [w]in [l]oss [o]dds [g] per terminare")
		app.Refresh()
	})

	app.Bind("h", "Hand", func() {
		if session == nil || !session.Active {
			agentLogPanel.Log("Avvia una sessione guidata prima con [g]")
			app.Refresh()
			return
		}

		switch session.Mode {
		case engine.ModeBlackjack:
			app.Prompt("BJ mano (tot,dealer | s=soft p=pair es: 16,10 s17,6 p8,5):", func(input string) {
				action := parseBJHand(input, session)
				if action != "" {
					agentLogPanel.Log("BJ: %s → %s", input, action)
				} else {
					agentLogPanel.Log("ERR: formato non valido. Usa: 16,10 o s17,6 o p8,5")
				}
				app.Refresh()
			})

		case engine.ModePoker:
			app.Prompt("Poker mano,posizione (es: AKs,late | QQ,early):", func(input string) {
				advice := parsePokerHand(input, session)
				if advice != "" {
					agentLogPanel.Log("POKER: %s → %s", input, advice)
				} else {
					agentLogPanel.Log("ERR: formato non valido. Usa: AKs,late")
				}
				app.Refresh()
			})

		default:
			// Slots: just record a spin
			session.HandsPlayed++
			session.WageredSoFar += session.BetSize
			agentLogPanel.Log("Spin #%d registrato (wagered: %.2f)", session.HandsPlayed, session.WageredSoFar)
			app.Refresh()
		}
	})

	app.Bind("o", "Odds", func() {
		if session == nil || !session.Active || session.Mode != engine.ModePoker {
			agentLogPanel.Log("Pot odds disponibili solo in sessione poker")
			app.Refresh()
			return
		}
		app.Prompt("Pot odds (pot,call,outs | pot,call,draw es: 100,20,9 o 100,20,flush):", func(input string) {
			result := parsePotOdds(input, session)
			if result != "" {
				agentLogPanel.Log("ODDS: %s", result)
			} else {
				agentLogPanel.Log("ERR: formato: pot,call,outs (o pot,call,flush/oesd/gutshot)")
			}
			app.Refresh()
		})
	})

	app.Bind("w", "Win", func() {
		if session == nil || !session.Active {
			return
		}
		app.Prompt("Importo vinto:", func(input string) {
			amount, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
			if err != nil {
				agentLogPanel.Log("ERR: importo non valido")
				app.Refresh()
				return
			}
			if session.Mode == engine.ModeBlackjack {
				session.BJRecordHand(true, amount)
			} else {
				session.HandsPlayed++
				session.WageredSoFar += session.BetSize
				session.Balance += amount
			}
			agentLogPanel.Log("WIN +%.2f | balance:%.2f | progress:%.0f%%",
				amount, session.Balance, session.Progress())
			app.Refresh()
		})
	})

	app.Bind("l", "Loss", func() {
		if session == nil || !session.Active {
			return
		}
		app.Prompt("Importo perso:", func(input string) {
			amount, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
			if err != nil {
				agentLogPanel.Log("ERR: importo non valido")
				app.Refresh()
				return
			}
			if session.Mode == engine.ModeBlackjack {
				session.BJRecordHand(false, amount)
			} else {
				session.HandsPlayed++
				session.WageredSoFar += session.BetSize
				session.Balance -= amount
			}
			agentLogPanel.Log("LOSS -%.2f | balance:%.2f | progress:%.0f%%",
				amount, session.Balance, session.Progress())
			if session.IsStopLoss() {
				agentLogPanel.Log("!! STOP-LOSS RAGGIUNTO — fermati !!")
			}
			app.Refresh()
		})
	})

	app.Bind("d", "Done", func() {
		if session != nil && session.Active {
			// Done = end session and mark casino done
			app.Prompt("Importo finale prelevato:", func(input string) {
				amount, err := strconv.ParseFloat(strings.TrimSpace(input), 64)
				if err != nil {
					agentLogPanel.Log("ERR: importo non valido")
					app.Refresh()
					return
				}
				resp, err := client.MarkDone(context.Background(), &pb.MarkDoneRequest{
					CasinoId: session.CasinoID,
					Result:   amount,
				})
				if err != nil {
					agentLogPanel.Log("ERR: %v", err)
				} else if resp.Ok {
					agentLogPanel.Log("DONE %s | ev:%+.2f | mani:%d | bankroll:%.2f",
						session.CasinoName, resp.EvActual, session.HandsPlayed, resp.NewBankroll)
				}
				session.Active = false
				session = nil
				app.Refresh()
			})
			return
		}
		app.Prompt("Casino ID:", func(id string) {
			app.Prompt("Importo risultato:", func(amount string) {
				result, parseErr := strconv.ParseFloat(strings.TrimSpace(amount), 64)
				if parseErr != nil {
					agentLogPanel.Log("ERR: importo non valido: %s", amount)
					app.Refresh()
					return
				}
				resp, err := client.MarkDone(context.Background(), &pb.MarkDoneRequest{
					CasinoId: id,
					Result:   result,
				})
				if err != nil {
					agentLogPanel.Log("ERR: %v", err)
					return
				}
				if resp.Ok {
					agentLogPanel.Log("DONE %s | ev:%+.2f | bankroll:%.2f",
						id, resp.EvActual, resp.NewBankroll)
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
			bankroll, parseErr := strconv.ParseFloat(strings.TrimSpace(amount), 64)
			if parseErr != nil || bankroll <= 0 {
				agentLogPanel.Log("ERR: bankroll non valido: %s", amount)
				app.Refresh()
				return
			}
			resp, err := client.Init(context.Background(), &pb.InitRequest{Bankroll: bankroll})
			if err != nil {
				agentLogPanel.Log("ERR: %v", err)
				return
			}
			agentLogPanel.Log(resp.Message)
			app.Refresh()
		})
	})

	app.Bind("q", "Quit", func() {
		app.Quit()
	})

	app.SetRefreshInterval(5 * time.Second)
	go watchUpdates(client, agentLogPanel, app)

	agentLogPanel.Log("Bonus Pipeline TUI v2 — GUIDED mode")
	agentLogPanel.Log("[n]ext [g]uided [d]one [s]kip [i]nit [q]uit")
	agentLogPanel.Log("Guided: [h]and [w]in [l]oss [o]dds")

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

	if session != nil && session.Active {
		b.WriteString(fmt.Sprintf(" SESSION: %s [%s]\n", session.CasinoName, session.Mode))
		b.WriteString(fmt.Sprintf(" Progress: %.0f%% | Balance: %.2f\n", session.Progress(), session.Balance))
	}

	b.WriteString(strings.Repeat("-", clamp(w-2, 1, 40)) + "\n")

	maxRows := h - 7
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
		start := len(resp.Steps) - 3
		if start < 0 {
			start = 0
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
		return " Pipeline completata!"
	}

	ev, err := client.CalcEV(ctx, &pb.CalcEVRequest{CasinoId: next.Casino.Id})
	if err != nil {
		return fmt.Sprintf(" [error] %v", err)
	}

	sep := strings.Repeat("-", clamp(w-2, 1, 30))
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

func renderGuidedPanel(w, h int) string {
	if session == nil {
		return ""
	}
	switch session.Mode {
	case engine.ModeBlackjack:
		return engine.RenderBJGuided(session, w)
	case engine.ModePoker:
		return engine.RenderPokerGuided(session, w)
	default:
		return engine.RenderSlotsGuided(session, w)
	}
}

func renderAdvisorPanel(client pb.PipelineServiceClient, w, h int) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	next, err := client.GetNext(ctx, &pb.GetNextRequest{})
	if err != nil {
		return fmt.Sprintf(" [error] %v", err)
	}
	if next.PipelineComplete {
		return " Pipeline completata!"
	}

	c := next.Casino
	sep := strings.Repeat("-", clamp(w-2, 1, 40))

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

	if db != nil {
		casino := engine.FindCasino(db, c.Id)
		if casino != nil {
			strat := engine.GetGameStrategy(*casino)
			b.WriteString(" " + sep + "\n")
			b.WriteString(fmt.Sprintf(" STRATEGIA: %s [%s]\n", strings.ToUpper(strat.GameType), strat.RiskLevel))
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
	b.WriteString(" [g] avvia sessione GUIDED\n")

	return b.String()
}

// ── Input parsers ───────────────────────────────────────────────────────────

// parseBJHand parses "16,10" or "s17,6" or "p8,5" and calls session.BJInput.
func parseBJHand(input string, s *engine.GuidedSession) string {
	input = strings.TrimSpace(input)
	parts := strings.Split(input, ",")
	if len(parts) != 2 {
		return ""
	}

	handStr := strings.TrimSpace(parts[0])
	dealerStr := strings.TrimSpace(parts[1])

	dealer, err := strconv.Atoi(dealerStr)
	if err != nil || dealer < 2 || dealer > 11 {
		return ""
	}

	isSoft := false
	isPair := false
	pairCard := 0

	if strings.HasPrefix(handStr, "s") || strings.HasPrefix(handStr, "S") {
		isSoft = true
		handStr = handStr[1:]
	} else if strings.HasPrefix(handStr, "p") || strings.HasPrefix(handStr, "P") {
		isPair = true
		handStr = handStr[1:]
	}

	total, err := strconv.Atoi(handStr)
	if err != nil {
		return ""
	}

	if isPair {
		pairCard = total
		total = total * 2
		if pairCard == 11 {
			total = 12 // pair of aces = soft 12
			isSoft = true
		}
	}

	action := s.BJInput(total, isSoft, isPair, pairCard, dealer)
	return string(action)
}

// parsePokerHand parses "AKs,late" and calls session.PokerInput.
func parsePokerHand(input string, s *engine.GuidedSession) string {
	input = strings.TrimSpace(input)
	parts := strings.Split(input, ",")
	if len(parts) != 2 {
		return ""
	}

	hand := strings.TrimSpace(parts[0])
	posStr := strings.ToLower(strings.TrimSpace(parts[1]))

	var pos engine.PokerPosition
	switch posStr {
	case "early", "e", "utg":
		pos = engine.PokerEarly
	case "middle", "m", "mp":
		pos = engine.PokerMiddle
	case "late", "l", "co", "btn", "button":
		pos = engine.PokerLate
	case "blinds", "bb", "sb", "blind":
		pos = engine.PokerBlinds
	default:
		pos = engine.PokerMiddle
	}

	advice := s.PokerInput(hand, pos)
	return fmt.Sprintf("[%s] %s", advice.TierName, advice.Action)
}

// parsePotOdds parses "100,20,9" or "100,20,flush" and updates session.
func parsePotOdds(input string, s *engine.GuidedSession) string {
	input = strings.TrimSpace(input)
	parts := strings.Split(input, ",")
	if len(parts) != 3 {
		return ""
	}

	pot, err1 := strconv.ParseFloat(strings.TrimSpace(parts[0]), 64)
	call, err2 := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64)
	if err1 != nil || err2 != nil {
		return ""
	}

	outsStr := strings.TrimSpace(parts[2])
	outs, err := strconv.Atoi(outsStr)
	if err != nil {
		// Try named draw
		namedOuts, desc := engine.CommonOuts(outsStr)
		if namedOuts == 0 {
			return ""
		}
		outs = namedOuts
		_ = desc
	}

	s.PotSize = pot
	s.BetToCall = call
	s.PokerOuts = outs

	result := engine.CalcPotOdds(pot, call, outs)
	return result.Reasoning
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
	if maxLen <= 3 || len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}

func clamp(v, lo, hi int) int {
	if v < lo {
		return lo
	}
	if v > hi {
		return hi
	}
	return v
}

func watchUpdates(client pb.PipelineServiceClient, logPanel *framegotui.Panel, app *framegotui.App) {
	for {
		stream, err := client.WatchState(context.Background(), &pb.WatchStateRequest{})
		if err != nil {
			logPanel.Log("Watch: disconnected, retry 3s...")
			time.Sleep(3 * time.Second)
			continue
		}
		for {
			update, err := stream.Recv()
			if err != nil {
				logPanel.Log("Stream lost, reconnecting...")
				break
			}
			ts := update.Timestamp
			if len(ts) > 19 {
				ts = ts[11:19]
			}
			logPanel.Log("[%s] %s %s bk=%.2f %d/%d",
				ts, update.Event, update.CasinoId,
				update.CurrentBankroll, update.DoneCount, update.TotalCasinos)
			app.Refresh()
		}
		time.Sleep(2 * time.Second)
	}
}
