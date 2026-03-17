package engine

import (
	"fmt"
	"math"
	"strings"
	"time"
)

// ── Guided Session ──────────────────────────────────────────────────────────

// SessionMode defines what game the user is playing.
type SessionMode string

const (
	ModeNone       SessionMode = ""
	ModeBlackjack  SessionMode = "blackjack"
	ModePoker      SessionMode = "poker"
	ModeSlots      SessionMode = "slots"
)

// GuidedSession tracks a live wagering session state.
type GuidedSession struct {
	Active      bool
	CasinoID    string
	CasinoName  string
	Mode        SessionMode
	StartedAt   time.Time
	BetSize     float64
	WagerTarget float64
	WageredSoFar float64
	StopLoss    float64
	Balance     float64
	HandsPlayed int

	// Blackjack state
	BJPlayerTotal int
	BJPlayerSoft  bool
	BJPair        bool
	BJPairCard    int
	BJDealerUp    int
	BJLastAction  BJAction
	BJHistory     []BJHandResult

	// Poker state
	PokerHand     string
	PokerPosition PokerPosition
	PotSize       float64
	BetToCall     float64
	PokerOuts     int
	PokerHistory  []PokerHandResult
}

// BJHandResult stores one BJ hand outcome.
type BJHandResult struct {
	PlayerTotal int
	DealerUp    int
	Action      BJAction
	Won         bool
	Amount      float64
}

// PokerHandResult stores one poker hand outcome.
type PokerHandResult struct {
	Hand     string
	Position PokerPosition
	PotOdds  float64
	Won      bool
	Amount   float64
}

// NewGuidedSession starts a guided session for a casino.
func NewGuidedSession(c Casino, bankroll float64) *GuidedSession {
	bonusVal := CalcBonusValue(c)
	betSize := CalcBetSize(c)
	wagerTarget := c.Bonus.Wager * bonusVal
	stopLoss := bonusVal * 0.30

	mode := ModeSlots
	name := strings.ToLower(c.BestGame.Name)
	if strings.Contains(name, "blackjack") || strings.Contains(name, "bj") {
		mode = ModeBlackjack
	} else if strings.Contains(name, "poker") {
		mode = ModePoker
	}

	return &GuidedSession{
		Active:      true,
		CasinoID:    c.ID,
		CasinoName:  c.Name,
		Mode:        mode,
		StartedAt:   time.Now(),
		BetSize:     betSize,
		WagerTarget: wagerTarget,
		StopLoss:    stopLoss,
		Balance:     bankroll,
	}
}

// Progress returns wagering completion percentage.
func (s *GuidedSession) Progress() float64 {
	if s.WagerTarget <= 0 {
		return 100
	}
	return math.Min(s.WageredSoFar/s.WagerTarget*100, 100)
}

// IsStopLoss returns true if balance is below stop-loss.
func (s *GuidedSession) IsStopLoss() bool {
	return s.Balance > 0 && s.Balance < s.StopLoss
}

// BJInput sets the current blackjack hand and returns the recommended action.
func (s *GuidedSession) BJInput(playerTotal int, isSoft, isPair bool, pairCard, dealerUp int) BJAction {
	s.BJPlayerTotal = playerTotal
	s.BJPlayerSoft = isSoft
	s.BJPair = isPair
	s.BJPairCard = pairCard
	s.BJDealerUp = dealerUp

	hand := BJHand{
		PlayerTotal: playerTotal,
		IsSoft:      isSoft,
		IsPair:      isPair,
		PairCard:    pairCard,
		DealerUp:    dealerUp,
	}
	s.BJLastAction = BJAdvise(hand)
	return s.BJLastAction
}

// BJRecordHand records a completed BJ hand and updates wagering.
func (s *GuidedSession) BJRecordHand(won bool, amount float64) {
	s.BJHistory = append(s.BJHistory, BJHandResult{
		PlayerTotal: s.BJPlayerTotal,
		DealerUp:    s.BJDealerUp,
		Action:      s.BJLastAction,
		Won:         won,
		Amount:      amount,
	})
	s.HandsPlayed++
	s.WageredSoFar += s.BetSize
	if won {
		s.Balance += amount
	} else {
		s.Balance -= amount
	}
}

// PokerInput sets the poker hand and returns advice.
func (s *GuidedSession) PokerInput(hand string, position PokerPosition) PokerStartingHand {
	s.PokerHand = hand
	s.PokerPosition = position
	return PokerAdvise(hand, position)
}

// ── Poker Pot Odds Calculator ───────────────────────────────────────────────

// PotOddsResult holds pot odds calculation.
type PotOddsResult struct {
	PotSize    float64
	BetToCall  float64
	PotOdds    float64 // percentage
	Outs       int
	WinPct     float64 // equity based on outs (turn+river)
	IsCall     bool    // should you call?
	Reasoning  string
}

// CalcPotOdds calculates pot odds and equity.
func CalcPotOdds(potSize, betToCall float64, outs int) PotOddsResult {
	// Pot odds = betToCall / (potSize + betToCall)
	potOdds := 0.0
	if potSize+betToCall > 0 {
		potOdds = betToCall / (potSize + betToCall) * 100
	}

	// Equity approximation (rule of 2 and 4)
	// On flop with 2 cards to come: outs * 4
	// On turn with 1 card to come: outs * 2
	// We use "2 cards to come" by default (more common decision point)
	winPct := float64(outs) * 4.0
	if winPct > 100 {
		winPct = 100
	}

	isCall := winPct > potOdds
	reasoning := ""
	if betToCall <= 0 {
		isCall = true
		reasoning = "CHECK gratuito — sempre check/call"
	} else if isCall {
		reasoning = fmt.Sprintf("CALL: equity %.0f%% > pot odds %.0f%%", winPct, potOdds)
	} else {
		reasoning = fmt.Sprintf("FOLD: equity %.0f%% < pot odds %.0f%%", winPct, potOdds)
	}

	return PotOddsResult{
		PotSize:   potSize,
		BetToCall: betToCall,
		PotOdds:   potOdds,
		Outs:      outs,
		WinPct:    winPct,
		IsCall:    isCall,
		Reasoning: reasoning,
	}
}

// CommonOuts returns the number of outs for common poker draws.
func CommonOuts(draw string) (int, string) {
	switch strings.ToLower(strings.TrimSpace(draw)) {
	case "flush", "flush draw":
		return 9, "Flush draw (9 outs)"
	case "oesd", "open-ended", "open ended", "straight draw":
		return 8, "Open-ended straight draw (8 outs)"
	case "gutshot", "inside straight":
		return 4, "Gutshot straight draw (4 outs)"
	case "flush+oesd", "combo":
		return 15, "Flush draw + OESD combo (15 outs)"
	case "overpair", "top pair":
		return 5, "Top pair/overpair miglioramento (5 outs)"
	case "two pair":
		return 4, "Two pair draw (4 outs)"
	case "set", "trips":
		return 2, "Set mining (2 outs)"
	case "overcards", "two overcards":
		return 6, "Two overcards (6 outs)"
	case "backdoor flush":
		return 1, "Backdoor flush (~1.5 effective outs)"
	default:
		return 0, "Draw sconosciuto — specifica gli outs"
	}
}

// ── Render helpers for guided mode ──────────────────────────────────────────

// RenderBJGuided renders the blackjack guided panel content.
func RenderBJGuided(s *GuidedSession, w int) string {
	sep := strings.Repeat("-", minInt(w-2, 40))
	var b strings.Builder

	b.WriteString(fmt.Sprintf(" BJ GUIDED | %s\n", s.CasinoName))
	b.WriteString(" " + sep + "\n")
	b.WriteString(fmt.Sprintf(" Mani: %d | Wagered: %.2f/%.2f (%.0f%%)\n",
		s.HandsPlayed, s.WageredSoFar, s.WagerTarget, s.Progress()))
	b.WriteString(fmt.Sprintf(" Balance: %.2f | Bet: %.2f\n", s.Balance, s.BetSize))

	if s.IsStopLoss() {
		b.WriteString(" !! STOP-LOSS RAGGIUNTO !!\n")
	}

	b.WriteString(" " + sep + "\n")

	if s.BJPlayerTotal > 0 {
		handDesc := ""
		if s.BJPair {
			handDesc = fmt.Sprintf("Pair %d", s.BJPairCard)
		} else if s.BJPlayerSoft {
			handDesc = fmt.Sprintf("Soft %d", s.BJPlayerTotal)
		} else {
			handDesc = fmt.Sprintf("Hard %d", s.BJPlayerTotal)
		}

		b.WriteString(fmt.Sprintf(" Tu: %s | Dealer: %d\n", handDesc, s.BJDealerUp))
		b.WriteString(fmt.Sprintf(" >>> %s <<<\n", s.BJLastAction))
	} else {
		b.WriteString(" Inserisci mano: [h]and\n")
		b.WriteString(" Formato: totale,dealer (es: 16,10)\n")
		b.WriteString("   soft: s17,6  pair: p8,5\n")
	}

	b.WriteString(" " + sep + "\n")

	// Quick reference inline
	b.WriteString(" REGOLE RAPIDE:\n")
	b.WriteString("  17+ STAND | 12-16 STAND vs 2-6\n")
	b.WriteString("  11 DOUBLE | 10 DOUBLE vs 2-9\n")
	b.WriteString("  AA/88 SPLIT | TT STAND\n")

	// Recent hands
	if len(s.BJHistory) > 0 {
		b.WriteString(" " + sep + "\n")
		b.WriteString(" Ultime mani:\n")
		start := 0
		if len(s.BJHistory) > 3 {
			start = len(s.BJHistory) - 3
		}
		for _, h := range s.BJHistory[start:] {
			icon := "-"
			if h.Won {
				icon = "+"
			}
			b.WriteString(fmt.Sprintf("  %s %d vs %d → %s %.2f\n",
				icon, h.PlayerTotal, h.DealerUp, h.Action, h.Amount))
		}
	}

	return b.String()
}

// RenderPokerGuided renders the poker guided panel content.
func RenderPokerGuided(s *GuidedSession, w int) string {
	sep := strings.Repeat("-", minInt(w-2, 40))
	var b strings.Builder

	b.WriteString(fmt.Sprintf(" POKER GUIDED | %s\n", s.CasinoName))
	b.WriteString(" " + sep + "\n")
	b.WriteString(fmt.Sprintf(" Mani: %d | Wagered: %.2f/%.2f (%.0f%%)\n",
		s.HandsPlayed, s.WageredSoFar, s.WagerTarget, s.Progress()))
	b.WriteString(fmt.Sprintf(" Balance: %.2f\n", s.Balance))

	if s.IsStopLoss() {
		b.WriteString(" !! STOP-LOSS RAGGIUNTO !!\n")
	}

	b.WriteString(" " + sep + "\n")

	if s.PokerHand != "" {
		advice := PokerAdvise(s.PokerHand, s.PokerPosition)
		b.WriteString(fmt.Sprintf(" Mano: %s | Pos: %s\n", advice.Hand, s.PokerPosition))
		b.WriteString(fmt.Sprintf(" Tier: %s\n", advice.TierName))
		b.WriteString(fmt.Sprintf(" >>> %s <<<\n", advice.Action))
	} else {
		b.WriteString(" Inserisci mano: [h]and\n")
		b.WriteString(" Formato: AKs,late / QQ,early\n")
	}

	// Pot odds section
	if s.PotSize > 0 || s.BetToCall > 0 {
		odds := CalcPotOdds(s.PotSize, s.BetToCall, s.PokerOuts)
		b.WriteString(" " + sep + "\n")
		b.WriteString(" POT ODDS:\n")
		b.WriteString(fmt.Sprintf("  Pot: %.2f | Call: %.2f\n", odds.PotSize, odds.BetToCall))
		b.WriteString(fmt.Sprintf("  Pot odds: %.0f%% | Equity: %.0f%%\n", odds.PotOdds, odds.WinPct))
		b.WriteString(fmt.Sprintf("  >>> %s <<<\n", odds.Reasoning))
	} else {
		b.WriteString(" " + sep + "\n")
		b.WriteString(" Pot odds: [o]dds (pot,call,outs)\n")
		b.WriteString(" Draw rapidi: flush=9 oesd=8 gut=4\n")
	}

	b.WriteString(" " + sep + "\n")
	b.WriteString(" RANGE RAPIDE:\n")
	b.WriteString("  PREMIUM: AA KK QQ AKs\n")
	b.WriteString("  STRONG:  JJ TT AQs AKo\n")
	b.WriteString("  PLAY:    99-77 AJs KQs\n")

	return b.String()
}

// RenderSlotsGuided renders the slots guided panel content.
func RenderSlotsGuided(s *GuidedSession, w int) string {
	sep := strings.Repeat("-", minInt(w-2, 40))
	var b strings.Builder

	b.WriteString(fmt.Sprintf(" SLOTS GUIDED | %s\n", s.CasinoName))
	b.WriteString(" " + sep + "\n")
	b.WriteString(fmt.Sprintf(" Spins: %d | Wagered: %.2f/%.2f (%.0f%%)\n",
		s.HandsPlayed, s.WageredSoFar, s.WagerTarget, s.Progress()))
	b.WriteString(fmt.Sprintf(" Balance: %.2f | Bet: %.2f\n", s.Balance, s.BetSize))

	if s.IsStopLoss() {
		b.WriteString(" !! STOP-LOSS RAGGIUNTO !!\n")
	}

	b.WriteString(" " + sep + "\n")

	elapsed := time.Since(s.StartedAt)
	if s.HandsPlayed > 0 {
		rate := float64(s.HandsPlayed) / elapsed.Minutes()
		remaining := (s.WagerTarget - s.WageredSoFar) / s.BetSize
		etaMins := remaining / rate
		b.WriteString(fmt.Sprintf(" Velocita: %.0f spin/min\n", rate))
		b.WriteString(fmt.Sprintf(" Spin rimasti: ~%.0f\n", remaining))
		b.WriteString(fmt.Sprintf(" Tempo stimato: ~%.0f min\n", etaMins))
	}

	b.WriteString(" " + sep + "\n")
	b.WriteString(" REGOLE:\n")
	b.WriteString(fmt.Sprintf("  Bet: %.2f (MAI di piu)\n", s.BetSize))
	b.WriteString("  Velocita LENTA\n")
	b.WriteString("  NO autoplay veloce\n")
	b.WriteString(fmt.Sprintf("  Stop-loss: %.2f\n", s.StopLoss))

	return b.String()
}

func minInt(a, b int) int {
	if a < b {
		return a
	}
	return b
}
