package engine

import (
	"fmt"
	"math"
	"strings"
)

// CalcBonusValue returns the raw bonus value before wagering losses.
func CalcBonusValue(c Casino) float64 {
	switch c.Bonus.Type {
	case "no-deposit":
		return c.Bonus.Amount
	case "no-deposit-freespins":
		spinVal := c.Bonus.SpinValue
		if spinVal == 0 {
			spinVal = 0.10
		}
		return c.Bonus.Amount * spinVal
	case "deposit", "deposit-tranches":
		return c.Bonus.Amount
	case "cashback":
		return 0
	}
	return 0
}

// CalcEV returns the expected value for a casino given current bankroll.
func CalcEV(c Casino, bankroll float64) float64 {
	bonus := CalcBonusValue(c)
	if bonus <= 0 {
		return 0
	}

	rtp := c.BestGame.RTP
	if rtp == 0 {
		rtp = 0.97
	}
	houseEdge := 1.0 - rtp
	wager := c.Bonus.Wager

	if wager == 0 {
		ev := bonus
		if c.Bonus.MaxCashout > 0 && ev > c.Bonus.MaxCashout {
			ev = c.Bonus.MaxCashout
		}
		return ev
	}

	wagerAmount := wager * bonus
	expectedLoss := wagerAmount * houseEdge
	ev := bonus - expectedLoss

	if c.Bonus.MaxCashout > 0 && ev > c.Bonus.MaxCashout {
		ev = c.Bonus.MaxCashout
	}

	return ev
}

// CalcExpectedOutput returns expected bankroll after completing this casino.
func CalcExpectedOutput(c Casino, bankroll float64) float64 {
	ev := CalcEV(c, bankroll)
	if c.Bonus.Type == "deposit" || c.Bonus.Type == "deposit-tranches" {
		depositAmt := math.Min(bankroll, c.Bonus.Amount)
		return bankroll - depositAmt + depositAmt + ev
	}
	return bankroll + ev
}

// CalcBetSize returns the recommended bet size for a casino bonus.
func CalcBetSize(c Casino) float64 {
	bonusVal := CalcBonusValue(c)
	betSize := bonusVal * 0.01
	if betSize < 0.10 {
		betSize = 0.10
	}
	return betSize
}

// EVBreakdown holds all EV calculation details for display.
type EVBreakdown struct {
	BonusValue   float64
	WagerReq     float64
	WagerAmount  float64
	HouseEdge    float64
	ExpectedLoss float64
	EV           float64
	ExpectedOut  float64
	BetSize      float64
	EstMinutes   float64
}

// CalcEVBreakdown returns a full EV breakdown for a casino.
func CalcEVBreakdown(c Casino, bankroll float64) EVBreakdown {
	bonusVal := CalcBonusValue(c)
	rtp := c.BestGame.RTP
	if rtp == 0 {
		rtp = 0.97
	}
	houseEdge := 1.0 - rtp
	wagerAmount := c.Bonus.Wager * bonusVal
	expectedLoss := wagerAmount * houseEdge
	betSize := CalcBetSize(c)

	var estMin float64
	if wagerAmount > 0 && betSize > 0 {
		estMin = (wagerAmount / betSize) / 50 * 60
	}

	return EVBreakdown{
		BonusValue:   bonusVal,
		WagerReq:     c.Bonus.Wager,
		WagerAmount:  wagerAmount,
		HouseEdge:    houseEdge,
		ExpectedLoss: expectedLoss,
		EV:           CalcEV(c, bankroll),
		ExpectedOut:  CalcExpectedOutput(c, bankroll),
		BetSize:      betSize,
		EstMinutes:   estMin,
	}
}

// ── Blackjack Basic Strategy Advisor ────────────────────────────────────────

// BJHand represents a blackjack hand situation.
type BJHand struct {
	PlayerTotal int
	IsSoft      bool // has usable ace
	IsPair      bool
	PairCard    int  // card value if pair (2-11)
	DealerUp    int  // dealer upcard (2-11)
}

// BJAction is the recommended action.
type BJAction string

const (
	BJHit       BJAction = "HIT"
	BJStand     BJAction = "STAND"
	BJDouble    BJAction = "DOUBLE"
	BJSplit     BJAction = "SPLIT"
	BJSurrender BJAction = "SURRENDER"
)

// BJAdvise returns basic strategy recommendation for a hand.
func BJAdvise(hand BJHand) BJAction {
	d := hand.DealerUp

	// Pairs
	if hand.IsPair {
		return bjPairStrategy(hand.PairCard, d)
	}

	// Soft totals (with usable ace)
	if hand.IsSoft {
		return bjSoftStrategy(hand.PlayerTotal, d)
	}

	// Hard totals
	return bjHardStrategy(hand.PlayerTotal, d)
}

func bjHardStrategy(total, dealer int) BJAction {
	switch {
	case total >= 17:
		return BJStand
	case total == 16:
		if dealer >= 9 {
			return BJSurrender
		}
		if dealer <= 6 {
			return BJStand
		}
		return BJHit
	case total == 15:
		if dealer == 10 {
			return BJSurrender
		}
		if dealer <= 6 {
			return BJStand
		}
		return BJHit
	case total == 13 || total == 14:
		if dealer <= 6 {
			return BJStand
		}
		return BJHit
	case total == 12:
		if dealer >= 4 && dealer <= 6 {
			return BJStand
		}
		return BJHit
	case total == 11:
		return BJDouble
	case total == 10:
		if dealer <= 9 {
			return BJDouble
		}
		return BJHit
	case total == 9:
		if dealer >= 3 && dealer <= 6 {
			return BJDouble
		}
		return BJHit
	default: // 8 or less
		return BJHit
	}
}

func bjSoftStrategy(total, dealer int) BJAction {
	switch {
	case total >= 19:
		return BJStand
	case total == 18:
		if dealer >= 3 && dealer <= 6 {
			return BJDouble
		}
		if dealer >= 9 {
			return BJHit
		}
		return BJStand
	case total == 17:
		if dealer >= 3 && dealer <= 6 {
			return BJDouble
		}
		return BJHit
	case total == 16 || total == 15:
		if dealer >= 4 && dealer <= 6 {
			return BJDouble
		}
		return BJHit
	case total == 14 || total == 13:
		if dealer >= 5 && dealer <= 6 {
			return BJDouble
		}
		return BJHit
	default:
		return BJHit
	}
}

func bjPairStrategy(card, dealer int) BJAction {
	switch card {
	case 11: // Aces
		return BJSplit
	case 10:
		return BJStand
	case 9:
		if dealer == 7 || dealer >= 10 {
			return BJStand
		}
		return BJSplit
	case 8:
		return BJSplit
	case 7:
		if dealer <= 7 {
			return BJSplit
		}
		return BJHit
	case 6:
		if dealer <= 6 {
			return BJSplit
		}
		return BJHit
	case 5:
		if dealer <= 9 {
			return BJDouble
		}
		return BJHit
	case 4:
		if dealer == 5 || dealer == 6 {
			return BJSplit
		}
		return BJHit
	case 3, 2:
		if dealer <= 7 {
			return BJSplit
		}
		return BJHit
	}
	return BJHit
}

// BJQuickRef returns a compact basic strategy reference card as text.
func BJQuickRef() string {
	var b strings.Builder
	b.WriteString("BLACKJACK BASIC STRATEGY\n")
	b.WriteString("========================\n")
	b.WriteString("HARD TOTALS:\n")
	b.WriteString("  17+     → STAND sempre\n")
	b.WriteString("  13-16   → STAND vs 2-6, HIT vs 7+\n")
	b.WriteString("  12      → STAND vs 4-6, HIT resto\n")
	b.WriteString("  11      → DOUBLE sempre\n")
	b.WriteString("  10      → DOUBLE vs 2-9, HIT vs 10/A\n")
	b.WriteString("  9       → DOUBLE vs 3-6, HIT resto\n")
	b.WriteString("  8-      → HIT sempre\n")
	b.WriteString("SOFT TOTALS:\n")
	b.WriteString("  A8+     → STAND\n")
	b.WriteString("  A7      → DOUBLE 3-6, STAND 2/7/8, HIT 9+\n")
	b.WriteString("  A6      → DOUBLE 3-6, HIT resto\n")
	b.WriteString("  A4-A5   → DOUBLE 4-6, HIT resto\n")
	b.WriteString("  A2-A3   → DOUBLE 5-6, HIT resto\n")
	b.WriteString("PAIRS:\n")
	b.WriteString("  AA/88   → SPLIT sempre\n")
	b.WriteString("  TT      → STAND sempre\n")
	b.WriteString("  99      → SPLIT no 7/10/A\n")
	b.WriteString("  77      → SPLIT vs 2-7\n")
	b.WriteString("  66      → SPLIT vs 2-6\n")
	b.WriteString("  55      → DOUBLE vs 2-9 (mai split)\n")
	b.WriteString("  44      → SPLIT vs 5-6\n")
	b.WriteString("  22/33   → SPLIT vs 2-7\n")
	return b.String()
}

// ── Poker EV Advisor ────────────────────────────────────────────────────────

// PokerPosition represents player position at the table.
type PokerPosition string

const (
	PokerEarly  PokerPosition = "early"
	PokerMiddle PokerPosition = "middle"
	PokerLate   PokerPosition = "late"
	PokerBlinds PokerPosition = "blinds"
)

// PokerHandTier categorizes starting hand strength.
type PokerHandTier int

const (
	PokerPremium   PokerHandTier = iota // AA, KK, QQ, AKs
	PokerStrong                         // JJ, TT, AQs, AKo
	PokerPlayable                       // 99-77, AJs-ATs, KQs, AQo
	PokerSpeculate                      // 66-22, suited connectors, suited aces
	PokerFold                           // everything else
)

// PokerStartingHand holds pre-flop advice.
type PokerStartingHand struct {
	Hand     string
	Tier     PokerHandTier
	TierName string
	Action   string
}

// PokerAdvise returns starting hand advice for bonus grinding.
func PokerAdvise(hand string, position PokerPosition) PokerStartingHand {
	tier, tierName := classifyPokerHand(hand)
	action := pokerAction(tier, position)
	return PokerStartingHand{
		Hand:     hand,
		Tier:     tier,
		TierName: tierName,
		Action:   action,
	}
}

func classifyPokerHand(hand string) (PokerHandTier, string) {
	h := strings.ToUpper(strings.TrimSpace(hand))
	switch h {
	case "AA", "KK", "QQ", "AKS":
		return PokerPremium, "PREMIUM"
	case "JJ", "TT", "AQS", "AKO":
		return PokerStrong, "STRONG"
	case "99", "88", "77", "AJS", "ATS", "KQS", "AQO":
		return PokerPlayable, "PLAYABLE"
	case "66", "55", "44", "33", "22",
		"KJS", "QJS", "JTS", "T9S", "98S", "87S", "76S",
		"A9S", "A8S", "A7S", "A6S", "A5S", "A4S", "A3S", "A2S":
		return PokerSpeculate, "SPECULATIVE"
	default:
		return PokerFold, "FOLD"
	}
}

func pokerAction(tier PokerHandTier, pos PokerPosition) string {
	switch tier {
	case PokerPremium:
		return "RAISE/RE-RAISE da qualsiasi posizione"
	case PokerStrong:
		if pos == PokerBlinds || pos == PokerEarly {
			return "RAISE, fold vs 3-bet (tranne JJ)"
		}
		return "RAISE, call 3-bet"
	case PokerPlayable:
		if pos == PokerEarly {
			return "FOLD o limp (tight per bonus grinding)"
		}
		return "RAISE da middle/late, fold vs re-raise"
	case PokerSpeculate:
		if pos == PokerLate || pos == PokerBlinds {
			return "CALL se cheap, fold vs raise"
		}
		return "FOLD (non sprecare stack per bonus)"
	default:
		return "FOLD"
	}
}

// PokerQuickRef returns a compact poker starting hands reference.
func PokerQuickRef() string {
	var b strings.Builder
	b.WriteString("POKER STARTING HANDS (bonus grinding)\n")
	b.WriteString("=====================================\n")
	b.WriteString("PREMIUM (raise/3bet sempre):\n")
	b.WriteString("  AA  KK  QQ  AKs\n")
	b.WriteString("STRONG (raise, cautela vs 3bet):\n")
	b.WriteString("  JJ  TT  AQs  AKo\n")
	b.WriteString("PLAYABLE (raise middle/late):\n")
	b.WriteString("  99-77  AJs ATs  KQs  AQo\n")
	b.WriteString("SPECULATIVE (solo late/cheap):\n")
	b.WriteString("  66-22  suited connectors  suited aces\n")
	b.WriteString("FOLD: tutto il resto\n")
	b.WriteString("\nBONUS GRINDING TIPS:\n")
	b.WriteString("  • Gioca TIGHT — solo premium/strong\n")
	b.WriteString("  • Evita bluff costosi\n")
	b.WriteString("  • Obiettivo: accumulare rake/mani\n")
	b.WriteString("  • Stop-loss: -30% dello stack\n")
	return b.String()
}

// ── Game Strategy Selector ──────────────────────────────────────────────────

// GameStrategy holds the recommended approach for a casino's best game.
type GameStrategy struct {
	GameType    string // "blackjack", "poker", "slots", "provably-fair"
	StrategyRef string // compact strategy text
	Tips        []string
	RiskLevel   string // "low", "medium", "high"
}

// GetGameStrategy returns the right strategy based on the casino's best game.
func GetGameStrategy(c Casino) GameStrategy {
	name := strings.ToLower(c.BestGame.Name)

	switch {
	case strings.Contains(name, "blackjack") || strings.Contains(name, "bj"):
		return GameStrategy{
			GameType:    "blackjack",
			StrategyRef: BJQuickRef(),
			Tips: []string{
				fmt.Sprintf("Bet size: €%.2f (1%% del bonus)", CalcBetSize(c)),
				"Segui SEMPRE la basic strategy — zero deviazioni",
				"Non prendere insurance MAI",
				"Conta le mani per il wagering, non i soldi",
				fmt.Sprintf("RTP con BS: %.1f%% → house edge %.2f%%", c.BestGame.RTP*100, (1-c.BestGame.RTP)*100),
			},
			RiskLevel: "low",
		}

	case strings.Contains(name, "poker"):
		return GameStrategy{
			GameType:    "poker",
			StrategyRef: PokerQuickRef(),
			Tips: []string{
				"Gioca TIGHT pre-flop, soprattutto per bonus",
				"Premium hands: AA KK QQ AKs → raise SEMPRE",
				"Fold tutto il resto da early position",
				"Il rake conta come wagering — accumula mani",
				"Stop-loss: se perdi 30% dello stack, fermati",
			},
			RiskLevel: "medium",
		}

	case strings.Contains(name, "provably fair") || strings.Contains(name, "originals"):
		return GameStrategy{
			GameType:    "provably-fair",
			StrategyRef: "",
			Tips: []string{
				fmt.Sprintf("Bet size: €%.2f (1%% del bonus)", CalcBetSize(c)),
				fmt.Sprintf("RTP dichiarato: %.1f%% — verificabile on-chain", c.BestGame.RTP*100),
				"Usa auto-bet con stop-loss impostato",
				"Preferisci giochi con volatilità bassa (Dice, Limbo)",
				"Verifica il seed prima di iniziare la sessione",
			},
			RiskLevel: "low",
		}

	default: // slots
		return GameStrategy{
			GameType:    "slots",
			StrategyRef: "",
			Tips: []string{
				fmt.Sprintf("Bet size: €%.2f (1%% del bonus — MAI di più!)", CalcBetSize(c)),
				fmt.Sprintf("RTP: %.1f%% → perdita attesa per wagering: %.1f%%", c.BestGame.RTP*100, (1-c.BestGame.RTP)*100),
				"Gioca a velocità LENTA — non autoplay veloce",
				"Stop-loss: se il saldo scende sotto il 30% del bonus → abbandona",
				"Blood Suckers / 1429 Uncharted Seas → RTP più alto",
			},
			RiskLevel: "low",
		}
	}
}
