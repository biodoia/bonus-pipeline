package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"time"
)

// LoadDB reads and parses the pipeline.json database file.
func LoadDB(path string) (*PipelineDB, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("cannot read %s: %w", path, err)
	}
	var db PipelineDB
	if err := json.Unmarshal(data, &db); err != nil {
		return nil, fmt.Errorf("cannot parse %s: %w", path, err)
	}
	return &db, nil
}

// LoadState reads and parses the state.json file, creating a fresh state if missing.
func LoadState(path string) (*PipelineState, error) {
	data, err := os.ReadFile(path)
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

// SaveState writes the pipeline state to the given file path.
func SaveState(path string, s *PipelineState) error {
	s.UpdatedAt = time.Now()
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0644)
}

// SortedCasinos returns casinos sorted by priority.
func SortedCasinos(casinos []Casino) []Casino {
	sorted := make([]Casino, len(casinos))
	copy(sorted, casinos)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})
	return sorted
}

// FindCasino finds a casino by ID in the database.
func FindCasino(db *PipelineDB, id string) *Casino {
	for i := range db.Casinos {
		if db.Casinos[i].ID == id {
			return &db.Casinos[i]
		}
	}
	return nil
}

// NextPendingCasino returns the next casino to play (first pending by priority).
func NextPendingCasino(db *PipelineDB, state *PipelineState) *Casino {
	sorted := SortedCasinos(db.Casinos)
	for _, c := range sorted {
		cs := state.Casinos[c.ID]
		if cs.Status == "done" || cs.Status == "skipped" || cs.Status == "failed" {
			continue
		}
		found := FindCasino(db, c.ID)
		return found
	}
	return nil
}

// MarkDone marks a casino as completed and updates bankroll.
func MarkDone(db *PipelineDB, state *PipelineState, casinoID string, result float64) error {
	c := FindCasino(db, casinoID)
	if c == nil {
		return fmt.Errorf("casino '%s' not found", casinoID)
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
		CasinoName: c.Name,
		At:         time.Now(),
		Input:      state.CurrentBankroll,
		Output:     result,
		EV:         cs.EVActual,
	})

	state.TotalEVActual += cs.EVActual
	state.CurrentBankroll = result
	return nil
}

// InitPipeline initializes the pipeline with a starting bankroll.
func InitPipeline(state *PipelineState, bankroll float64) error {
	if state.InitialBankroll > 0 {
		return fmt.Errorf("pipeline already initialized (bankroll: %.2f)", state.InitialBankroll)
	}
	state.InitialBankroll = bankroll
	state.CurrentBankroll = bankroll
	state.StartedAt = time.Now()
	return nil
}

// ResetPipeline resets the pipeline state.
func ResetPipeline(state *PipelineState) {
	*state = PipelineState{
		Casinos:   make(map[string]CasinoState),
		StartedAt: time.Now(),
		UpdatedAt: time.Now(),
	}
}

// PipelineStats holds summary statistics.
type PipelineStats struct {
	TotalCasinos int
	DoneCount    int
	PendingCount int
	SkippedCount int
	FailedCount  int
	PnL          float64
}

// GetStats returns pipeline summary statistics.
func GetStats(db *PipelineDB, state *PipelineState) PipelineStats {
	stats := PipelineStats{TotalCasinos: len(db.Casinos)}
	for _, c := range db.Casinos {
		cs := state.Casinos[c.ID]
		switch cs.Status {
		case "done":
			stats.DoneCount++
		case "skipped":
			stats.SkippedCount++
		case "failed":
			stats.FailedCount++
		default:
			stats.PendingCount++
		}
	}
	stats.PnL = state.CurrentBankroll - state.InitialBankroll
	return stats
}
