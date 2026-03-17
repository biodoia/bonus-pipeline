package engine

import "time"

// ── Data structures (compatible with pipeline.json / state.json) ────────────

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
	Deposit       []string `json:"deposit"`
	Withdrawal    []string `json:"withdrawal"`
	SpeedMinutes  int      `json:"withdrawal_speed_minutes"`
	MinWithdrawal float64  `json:"min_withdrawal"`
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
	StopLossPct    float64  `json:"stop_loss_percent"`
	BetSizePct     float64  `json:"bet_size_percent"`
	ReinvestPct    float64  `json:"reinvest_percent"`
	PreferredGames []string `json:"preferred_games_for_wagering"`
	AvoidGames     []string `json:"avoid_games"`
	WithdrawalPrio []string `json:"withdrawal_priority"`
}

type PipelineDB struct {
	Version string   `json:"version"`
	Updated string   `json:"updated"`
	Casinos []Casino `json:"casinos"`
	Banned  []Banned `json:"banned"`
	Rules   Rules    `json:"rules"`
}

type CasinoState struct {
	Status       string    `json:"status"` // pending, active, done, skipped, failed
	StartAt      time.Time `json:"start_at,omitempty"`
	DoneAt       time.Time `json:"done_at,omitempty"`
	InputAmount  float64   `json:"input_amount"`
	OutputAmount float64   `json:"output_amount"`
	EVActual     float64   `json:"ev_actual"`
	Notes        string    `json:"notes,omitempty"`
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
	CasinoID   string    `json:"casino_id"`
	CasinoName string    `json:"casino_name"`
	At         time.Time `json:"at"`
	Input      float64   `json:"input"`
	Output     float64   `json:"output"`
	EV         float64   `json:"ev"`
}

// Stats holds computed pipeline statistics.
type Stats struct {
	TotalCasinos int
	DoneCount    int
	PnL          float64
}
