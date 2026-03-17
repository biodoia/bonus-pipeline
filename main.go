package main

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"
)

// ── Data structures ──────────────────────────────────────────────────────────

type Bonus struct {
	Type        string  `json:"type"`
	Amount      float64 `json:"amount"`
	Currency    string  `json:"currency"`
	SpinValue   float64 `json:"spin_value,omitempty"`
	Wager       float64 `json:"wager"`
	MatchPct    float64 `json:"match_percent,omitempty"`
	MaxCashout  float64 `json:"max_cashout"`
	ExpiryDays  int     `json:"expiry_days"`
	TrancheSize float64 `json:"tranche_size,omitempty"`
	Notes       string  `json:"notes,omitempty"`
	Frequency   string  `json:"frequency,omitempty"`
}

type Payment struct {
	Deposit          []string `json:"deposit"`
	Withdrawal       []string `json:"withdrawal"`
	SpeedMinutes     int      `json:"withdrawal_speed_minutes"`
	MinWithdrawal    float64  `json:"min_withdrawal"`
}

type Game struct {
	Name string  `json:"name"`
	RTP  float64 `json:"rtp"`
}

type Casino struct {
	ID           string  `json:"id"`
	Name         string  `json:"name"`
	Priority     int     `json:"priority"`
	Tier         string  `json:"tier"`
	URL          string  `json:"url"`
	Bonus        Bonus   `json:"bonus"`
	Payment      Payment `json:"payment"`
	BestGame     Game    `json:"best_game"`
	BestGameAlt  *Game   `json:"best_game_alt,omitempty"`
	KYCRequired  bool    `json:"kyc_required"`
	SPIDRequired bool    `json:"spid_required"`
	Jurisdiction string  `json:"jurisdiction"`
	Notes        string  `json:"notes"`
}

type Banned struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

type Rules struct {
	StopLossPct      float64  `json:"stop_loss_percent"`
	BetSizePct       float64  `json:"bet_size_percent"`
	ReinvestPct      float64  `json:"reinvest_percent"`
	PreferredGames   []string `json:"preferred_games_for_wagering"`
	AvoidGames       []string `json:"avoid_games"`
	WithdrawalPrio   []string `json:"withdrawal_priority"`
}

type PipelineDB struct {
	Version  string   `json:"version"`
	Updated  string   `json:"updated"`
	Casinos  []Casino `json:"casinos"`
	Banned   []Banned `json:"banned"`
	Rules    Rules    `json:"rules"`
}

type CasinoState struct {
	Status      string    `json:"status"` // pending, active, done, skipped, failed
	StartAt     time.Time `json:"start_at,omitempty"`
	DoneAt      time.Time `json:"done_at,omitempty"`
	InputAmount float64   `json:"input_amount"`
	OutputAmount float64  `json:"output_amount"`
	EVActual    float64   `json:"ev_actual"`
	Notes       string    `json:"notes,omitempty"`
}

type PipelineState struct {
	InitialBankroll float64                `json:"initial_bankroll"`
	CurrentBankroll float64                `json:"current_bankroll"`
	StartedAt       time.Time              `json:"started_at"`
	UpdatedAt       time.Time              `json:"updated_at"`
	Casinos         map[string]CasinoState `json:"casinos"`
	TotalEVActual   float64                `json:"total_ev_actual"`
	Steps           []StepLog              `json:"steps"`
}

type StepLog struct {
	CasinoID    string    `json:"casino_id"`
	CasinoName  string    `json:"casino_name"`
	At          time.Time `json:"at"`
	Input       float64   `json:"input"`
	Output      float64   `json:"output"`
	EV          float64   `json:"ev"`
}

// ── EV Calculator ─────────────────────────────────────────────────────────────

func calcBonusValue(c Casino) float64 {
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
		return 0 // cashback EV depends on losses, skip for now
	}
	return 0
}

func calcEV(c Casino, bankroll float64) float64 {
	bonus := calcBonusValue(c)
	if bonus <= 0 {
		return 0
	}

	// Use best game RTP for wagering
	rtp := c.BestGame.RTP
	if rtp == 0 {
		rtp = 0.97
	}
	houseEdge := 1.0 - rtp
	wager := c.Bonus.Wager

	if wager == 0 {
		// 0x wager: pure EV
		ev := bonus
		if c.Bonus.MaxCashout > 0 && ev > c.Bonus.MaxCashout {
			ev = c.Bonus.MaxCashout
		}
		return ev
	}

	// EV = Bonus - (WagerMultiplier × Bonus × HouseEdge)
	wagerAmount := wager * bonus
	expectedLoss := wagerAmount * houseEdge
	ev := bonus - expectedLoss

	if c.Bonus.MaxCashout > 0 && ev > c.Bonus.MaxCashout {
		ev = c.Bonus.MaxCashout
	}

	return ev
}

func calcExpectedOutput(c Casino, bankroll float64) float64 {
	ev := calcEV(c, bankroll)
	// For deposit bonuses, we lose the deposit amount in worst case,
	// but gain the bonus EV. Net output = bankroll + ev
	if c.Bonus.Type == "deposit" || c.Bonus.Type == "deposit-tranches" {
		depositAmt := math.Min(bankroll, c.Bonus.Amount)
		return bankroll - depositAmt + depositAmt + ev
	}
	return bankroll + ev
}

// ── File I/O ──────────────────────────────────────────────────────────────────

const dbFile = "pipeline.json"
const stateFile = "state.json"

func loadDB() (*PipelineDB, error) {
	data, err := os.ReadFile(dbFile)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", dbFile, err)
	}
	var db PipelineDB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", dbFile, err)
	}
	return &db, nil
}

func loadState() (*PipelineState, error) {
	data, err := os.ReadFile(stateFile)
	if os.IsNotExist(err) {
		return &PipelineState{
			Casinos:   make(map[string]CasinoState),
			StartedAt: time.Now(),
			UpdatedAt: time.Now(),
		}, nil
	}
	if err != nil {
		return nil, err
	}
	var s PipelineState
	if err := json.Unmarshal(data, &s); err != nil {
		return nil, err
	}
	return &s, nil
}

func saveState(s *PipelineState) error {
	s.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(stateFile, data, 0644)
}

// ── Display helpers ────────────────────────────────────────────────────────────

const (
	reset  = "\033[0m"
	bold   = "\033[1m"
	cyan   = "\033[36m"
	green  = "\033[32m"
	yellow = "\033[33m"
	red    = "\033[31m"
	gray   = "\033[90m"
	purple = "\033[35m"
)

func colorStatus(s string) string {
	switch s {
	case "done":
		return green + "✓ done" + reset
	case "active":
		return cyan + "► active" + reset
	case "skipped":
		return gray + "⊘ skipped" + reset
	case "failed":
		return red + "✗ failed" + reset
	default:
		return yellow + "○ pending" + reset
	}
}

func evColor(ev float64) string {
	if ev > 5 {
		return green + fmt.Sprintf("+€%.2f", ev) + reset
	} else if ev > 0 {
		return yellow + fmt.Sprintf("+€%.2f", ev) + reset
	}
	return red + fmt.Sprintf("€%.2f", ev) + reset
}

func header(title string) {
	fmt.Printf("\n%s%s══════════════════════════════════════════%s\n", bold, cyan, reset)
	fmt.Printf("%s%s  %s%s\n", bold, cyan, title, reset)
	fmt.Printf("%s%s══════════════════════════════════════════%s\n\n", bold, cyan, reset)
}

func tier(t string) string {
	switch t {
	case "crypto-nokyo":
		return purple + "◈ Crypto No-KYC" + reset
	case "adm-italy":
		return cyan + "🇮🇹 ADM Italia" + reset
	default:
		return gray + t + reset
	}
}

// ── Commands ───────────────────────────────────────────────────────────────────

func cmdNext(db *PipelineDB, state *PipelineState) {
	header("NEXT CASINO")

	// Find next pending casino
	sorted := make([]Casino, len(db.Casinos))
	copy(sorted, db.Casinos)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	for _, c := range sorted {
		cs := state.Casinos[c.ID]
		if cs.Status == "done" || cs.Status == "skipped" || cs.Status == "failed" {
			continue
		}

		ev := calcEV(c, state.CurrentBankroll)
		expectedOut := calcExpectedOutput(c, state.CurrentBankroll)

		fmt.Printf("%s %s\n\n", bold+c.Name+reset, tier(c.Tier))
		fmt.Printf("  %sURL:%s           %s\n", gray, reset, c.URL)
		fmt.Printf("  %sBonus type:%s    %s\n", gray, reset, c.Bonus.Type)

		bonusVal := calcBonusValue(c)
		if bonusVal > 0 {
			fmt.Printf("  %sBonus value:%s   €%.2f\n", gray, reset, bonusVal)
		}
		fmt.Printf("  %sWager:%s         %.0fx\n", gray, reset, c.Bonus.Wager)
		fmt.Printf("  %sEV esperato:%s   %s\n", gray, reset, evColor(ev))
		fmt.Printf("  %sBankroll in:%s   €%.2f\n", gray, reset, state.CurrentBankroll)
		fmt.Printf("  %sOutput atteso:%s €%.2f\n", gray, reset, expectedOut)
		fmt.Printf("  %sMiglior gioco:%s %s (RTP %.1f%%)\n", gray, reset, c.BestGame.Name, c.BestGame.RTP*100)
		fmt.Printf("  %sPrelievo:%s      %s (~%d min)\n", gray, reset, strings.Join(c.Payment.Withdrawal, ", "), c.Payment.SpeedMinutes)

		if c.KYCRequired {
			fmt.Printf("  %s⚠ KYC richiesto%s\n", yellow, reset)
		}
		if c.SPIDRequired {
			fmt.Printf("  %s⚠ SPID richiesto%s\n", yellow, reset)
		}

		fmt.Printf("\n  %sNote:%s %s\n", gray, reset, c.Notes)

		fmt.Printf("\n%s📋 STEP BY STEP:%s\n", bold, reset)
		printSteps(c, state.CurrentBankroll)

		fmt.Printf("\n%sQuando finito:%s  pipeline done %s --result <importo_prelevato>\n\n", gray, reset, c.ID)
		return
	}

	fmt.Printf("%s🎉 Pipeline completata! Tutti i casino eseguiti.%s\n\n", green, reset)
	fmt.Printf("Bankroll finale: %s€%.2f%s (iniziale: €%.2f)\n",
		bold, state.CurrentBankroll, reset, state.InitialBankroll)
	fmt.Printf("EV totale reale: %s\n\n", evColor(state.TotalEVActual))
}

func printSteps(c Casino, bankroll float64) {
	step := 1
	p := func(s string, args ...interface{}) {
		fmt.Printf("  %d. %s\n", step, fmt.Sprintf(s, args...))
		step++
	}

	p("%sRegistrati su %s%s", bold, c.URL, reset)

	if c.SPIDRequired {
		p("Accedi con SPID (necessario per bonus ADM)")
	}

	if c.Bonus.Type == "no-deposit" || c.Bonus.Type == "no-deposit-freespins" {
		p("Riscatta il bonus NO-DEPOSIT (nessun deposito richiesto)")
	} else if c.Bonus.Type == "deposit" {
		depositAmt := math.Min(bankroll*0.5, c.Bonus.Amount)
		p("Effettua deposito di %s€%.2f%s per attivare il bonus", bold, depositAmt, reset)
		if c.Bonus.MatchPct > 0 {
			bonusReceived := depositAmt * c.Bonus.MatchPct / 100
			p("Ricevi bonus di €%.2f (%.0f%% match)", bonusReceived, c.Bonus.MatchPct)
		}
	}

	bonusVal := calcBonusValue(c)
	betSize := bonusVal * 0.01
	if betSize < 0.10 {
		betSize = 0.10
	}

	p("Gioca su: %s%s%s (RTP %.1f%%)", bold, c.BestGame.Name, reset, c.BestGame.RTP*100)
	p("Bet size: %s€%.2f per spin/mano%s (1%% del bonus — mai di più!)", bold, betSize, reset)

	if c.Bonus.Wager > 0 {
		wagerTarget := c.Bonus.Wager * bonusVal
		p("Obiettivo wagering: €%.2f giocate totali", wagerTarget)
		p("Stop-loss: se il saldo scende sotto €%.2f → abbandona", bonusVal*0.30)
	}

	p("Preleva su: %s%s%s (~%d minuti)", bold,
		strings.Join(c.Payment.Withdrawal, " o "), reset, c.Payment.SpeedMinutes)
	p("Registra il risultato: %spipeline done %s --result <importo>%s", cyan, c.ID, reset)
}

func cmdStatus(db *PipelineDB, state *PipelineState) {
	header("PIPELINE STATUS")

	total := len(db.Casinos)
	done := 0
	for _, cs := range state.Casinos {
		if cs.Status == "done" {
			done++
		}
	}

	fmt.Printf("  Bankroll iniziale: €%.2f\n", state.InitialBankroll)
	fmt.Printf("  Bankroll attuale:  %s€%.2f%s\n", bold, state.CurrentBankroll, reset)

	pnl := state.CurrentBankroll - state.InitialBankroll
	fmt.Printf("  P&L attuale:       %s\n", evColor(pnl))
	fmt.Printf("  Progressione:      %d/%d casino completati\n\n", done, total)

	sorted := make([]Casino, len(db.Casinos))
	copy(sorted, db.Casinos)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	for _, c := range sorted {
		cs := state.Casinos[c.ID]
		status := cs.Status
		if status == "" {
			status = "pending"
		}
		ev := calcEV(c, state.CurrentBankroll)

		fmt.Printf("  %2d. %-22s %s  EV=%s",
			c.Priority,
			c.Name,
			colorStatus(status),
			evColor(ev),
		)

		if cs.Status == "done" {
			actual := cs.OutputAmount - cs.InputAmount
			fmt.Printf("  actual=%s", evColor(actual))
		}
		fmt.Println()
	}

	// Bankroll evolution chart
	if len(state.Steps) > 0 {
		fmt.Printf("\n%sBankroll evolution:%s\n", bold, reset)
		for _, s := range state.Steps {
			bar := strings.Repeat("█", int(s.Output/state.InitialBankroll*20))
			fmt.Printf("  %-20s €%6.2f  %s%s%s\n",
				s.CasinoName, s.Output,
				green, bar, reset)
		}
	}

	fmt.Println()
}

func cmdDone(db *PipelineDB, state *PipelineState, casinoID string, result float64) {
	var found *Casino
	for i := range db.Casinos {
		if db.Casinos[i].ID == casinoID {
			found = &db.Casinos[i]
			break
		}
	}
	if found == nil {
		fmt.Printf("%sErrore: casino '%s' non trovato%s\n", red, casinoID, reset)
		os.Exit(1)
	}

	cs := state.Casinos[casinoID]
	cs.Status = "done"
	cs.DoneAt = time.Now()
	cs.InputAmount = state.CurrentBankroll
	cs.OutputAmount = result
	cs.EVActual = result - state.CurrentBankroll
	state.Casinos[casinoID] = cs

	state.Steps = append(state.Steps, StepLog{
		CasinoID:   casinoID,
		CasinoName: found.Name,
		At:         time.Now(),
		Input:      state.CurrentBankroll,
		Output:     result,
		EV:         cs.EVActual,
	})

	state.TotalEVActual += cs.EVActual
	state.CurrentBankroll = result

	if err := saveState(state); err != nil {
		fmt.Printf("%sErrore nel salvare lo stato: %v%s\n", red, err, reset)
		os.Exit(1)
	}

	fmt.Printf("\n%s✓ %s completato!%s\n", green, found.Name, reset)
	fmt.Printf("  Input:   €%.2f\n", cs.InputAmount)
	fmt.Printf("  Output:  €%.2f\n", result)
	fmt.Printf("  EV reale: %s\n", evColor(cs.EVActual))
	fmt.Printf("  Bankroll aggiornato: %s€%.2f%s\n\n", bold, state.CurrentBankroll, reset)
	fmt.Printf("Prossimo step: %spipeline next%s\n\n", cyan, reset)
}

func cmdEV(db *PipelineDB, state *PipelineState, casinoID string) {
	var found *Casino
	for i := range db.Casinos {
		if db.Casinos[i].ID == casinoID {
			found = &db.Casinos[i]
			break
		}
	}
	if found == nil {
		fmt.Printf("%sErrore: casino '%s' non trovato%s\n", red, casinoID, reset)
		os.Exit(1)
	}

	header("EV CALCULATOR — " + found.Name)

	bonusVal := calcBonusValue(*found)
	ev := calcEV(*found, state.CurrentBankroll)
	expectedOut := calcExpectedOutput(*found, state.CurrentBankroll)
	houseEdge := 1.0 - found.BestGame.RTP
	wagerAmount := found.Bonus.Wager * bonusVal
	expectedLoss := wagerAmount * houseEdge

	fmt.Printf("  Bonus value:      €%.2f\n", bonusVal)
	fmt.Printf("  Wager req:        %.0fx = €%.2f giocate\n", found.Bonus.Wager, wagerAmount)
	fmt.Printf("  Gioco:            %s (RTP %.1f%%)\n", found.BestGame.Name, found.BestGame.RTP*100)
	fmt.Printf("  House edge:       %.1f%%\n", houseEdge*100)
	fmt.Printf("  Perdita attesa:   €%.2f\n", expectedLoss)
	fmt.Printf("  %s─────────────────────────────%s\n", cyan, reset)
	fmt.Printf("  EV netto:         %s\n", evColor(ev))
	fmt.Printf("  Bankroll attuale: €%.2f\n", state.CurrentBankroll)
	fmt.Printf("  Output atteso:    €%.2f\n\n", expectedOut)

	// Bet size recommendation
	betSize := bonusVal * 0.01
	if betSize < 0.10 {
		betSize = 0.10
	}
	fmt.Printf("  Bet size consigliato: €%.2f (1%% del bonus)\n", betSize)
	if wagerAmount > 0 {
		fmt.Printf("  Tempo stimato (50 spin/ora): %.0f min\n\n", (wagerAmount/betSize)/50*60)
	}
}

func cmdReset(state *PipelineState) {
	fmt.Printf("%sReset dello stato... sei sicuro? (yes/no): %s", yellow, reset)
	var confirm string
	fmt.Scanln(&confirm)
	if confirm != "yes" {
		fmt.Println("Annullato.")
		return
	}
	*state = PipelineState{
		Casinos:   make(map[string]CasinoState),
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
	if err := saveState(state); err != nil {
		fmt.Printf("%sErrore: %v%s\n", red, err, reset)
		return
	}
	fmt.Printf("%sStato resettato.%s\n", green, reset)
}

func cmdInit(state *PipelineState, bankroll float64) {
	if state.InitialBankroll > 0 {
		fmt.Printf("%sAttenzione: pipeline già inizializzata (bankroll: €%.2f). Usa 'reset' prima.%s\n",
			yellow, state.InitialBankroll, reset)
		return
	}
	state.InitialBankroll = bankroll
	state.CurrentBankroll = bankroll
	state.StartedAt = time.Now()
	if err := saveState(state); err != nil {
		fmt.Printf("%sErrore: %v%s\n", red, err, reset)
		return
	}
	fmt.Printf("\n%s🚀 Pipeline inizializzata con €%.2f%s\n", green, bankroll, reset)
	fmt.Printf("Prossimo step: %spipeline next%s\n\n", cyan, reset)
}

func cmdList(db *PipelineDB) {
	header("CASINO LIST")
	sorted := make([]Casino, len(db.Casinos))
	copy(sorted, db.Casinos)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	for _, c := range sorted {
		fmt.Printf("  %2d. %-20s %s  wager=%.0fx  RTP=%.1f%%  prelievo=%dmin\n",
			c.Priority, c.Name, tier(c.Tier),
			c.Bonus.Wager, c.BestGame.RTP*100, c.Payment.SpeedMinutes)
	}
	fmt.Printf("\n%sBannati:%s\n", red, reset)
	for _, b := range db.Banned {
		fmt.Printf("  ✗ %-20s %s\n", b.Name, b.Reason)
	}
	fmt.Println()
}

func usage() {
	fmt.Printf(`
%sBonus Pipeline — Casino Bonus Hunting Optimizer%s

USAGE:
  pipeline <command> [options]

COMMANDS:
  init <bankroll>          Inizializza la pipeline con il bankroll iniziale (es: pipeline init 100)
  next                     Mostra il prossimo casino da giocare + step by step
  status                   Mostra lo stato completo della pipeline
  done <id> --result <n>   Segna un casino come completato con il risultato finale
  ev <id>                  Calcola l'EV per un casino specifico
  list                     Lista tutti i casino disponibili con priorità
  reset                    Resetta lo stato della pipeline

EXAMPLES:
  pipeline init 100
  pipeline next
  pipeline done bc-game --result 12.50
  pipeline ev leovegas-it
  pipeline status

%sCASINO IDs:%s
  bc-game, bc-poker, bitstarz, winzio, leovegas-it,
  betsson-it, william-hill-it, betpanda, jetton,
  vave, mbit, betflag-it, rollino, donbet, mystake

`, bold, reset, bold, reset)
}

// ── Main ───────────────────────────────────────────────────────────────────────

func main() {
	db, err := loadDB()
	if err != nil {
		fmt.Printf("%sErrore caricamento DB: %v%s\n", red, err, reset)
		os.Exit(1)
	}

	state, err := loadState()
	if err != nil {
		fmt.Printf("%sErrore caricamento stato: %v%s\n", red, err, reset)
		os.Exit(1)
	}

	args := os.Args[1:]
	if len(args) == 0 {
		usage()
		return
	}

	switch args[0] {
	case "init":
		if len(args) < 2 {
			fmt.Printf("%sUso: pipeline init <bankroll>%s\n", red, reset)
			os.Exit(1)
		}
		bankroll, err := strconv.ParseFloat(args[1], 64)
		if err != nil {
			fmt.Printf("%sValore bankroll non valido: %s%s\n", red, args[1], reset)
			os.Exit(1)
		}
		cmdInit(state, bankroll)

	case "next":
		if state.InitialBankroll == 0 {
			fmt.Printf("%sPipeline non inizializzata. Usa: pipeline init <bankroll>%s\n", yellow, reset)
			os.Exit(1)
		}
		cmdNext(db, state)

	case "status":
		if state.InitialBankroll == 0 {
			fmt.Printf("%sPipeline non inizializzata. Usa: pipeline init <bankroll>%s\n", yellow, reset)
			os.Exit(1)
		}
		cmdStatus(db, state)

	case "done":
		if len(args) < 4 || args[2] != "--result" {
			fmt.Printf("%sUso: pipeline done <casino-id> --result <importo>%s\n", red, reset)
			os.Exit(1)
		}
		result, err := strconv.ParseFloat(args[3], 64)
		if err != nil {
			fmt.Printf("%sImporto non valido: %s%s\n", red, args[3], reset)
			os.Exit(1)
		}
		cmdDone(db, state, args[1], result)

	case "ev":
		if len(args) < 2 {
			fmt.Printf("%sUso: pipeline ev <casino-id>%s\n", red, reset)
			os.Exit(1)
		}
		cmdEV(db, state, args[1])

	case "list":
		cmdList(db)

	case "reset":
		cmdReset(state)

	default:
		fmt.Printf("%sComando sconosciuto: %s%s\n", red, args[0], reset)
		usage()
		os.Exit(1)
	}
}
