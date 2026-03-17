# Bonus Pipeline

Semi-automated casino bonus hunting pipeline con architettura FrameGoTUI.

```
€100 → BC.Game(0x) → BC Poker($10 free) → BitStarz(20FS) → LeoVegas(1x) → ... → Prelievo finale
```

## Architettura

```
                    ┌──────────────────┐
                    │  gRPC Daemon     │
                    │  cmd/daemon      │
                    │  :50051          │
                    └──┬─────────┬─────┘
                       │         │
              ┌────────┘         └────────┐
              │                           │
   ┌──────────▼──────────┐   ┌────────────▼────────────┐
   │  TUI (FrameGoTUI)   │   │  Agent (browser-use)    │
   │  cmd/tui             │   │  agent/main.py          │
   │                      │   │  + Qwen3-VL (Ollama)    │
   │  ┌────────┬────────┐ │   └─────────────────────────┘
   │  │Pipeline│EV Calc │ │
   │  ├────────┼────────┤ │   ┌─────────────────────────┐
   │  │Live    │Agent   │ │   │  WebUI (HTMX)           │
   │  │Advisor │Log     │ │   │  dashboard.html          │
   │  └────────┴────────┘ │   └─────────────────────────┘
   └──────────────────────┘
```

**Backend**: gRPC daemon Go — gestisce stato pipeline, EV calc, strategia, task dispatch
**Frontend 1**: TUI FrameGoTUI — 4 pannelli con GUIDED mode (BJ/Poker real-time)
**Frontend 2**: WebUI HTMX — dashboard HTML standalone
**Agent**: Python browser-use + Qwen3-VL per computer-use automation

## Quickstart

### Prerequisiti

- Go 1.21+
- Python 3.11+
- tmux
- Ollama (per l'agent, opzionale)

### 1. Build

```bash
git clone https://github.com/biodoia/bonus-pipeline.git
cd bonus-pipeline
make build
```

Crea tre binari in `bin/`: `daemon`, `tui`, `pipeline`.

### 2. Avvio rapido (tutto in tmux)

```bash
make start
```

Questo avvia in una sessione tmux `bonus-pipeline`:
- **Pane 0**: daemon gRPC su `:50051`
- **Pane 1**: TUI interattivo
- **Pane 2**: agent Python (se Ollama disponibile)

Per attach alla sessione: `tmux attach -t bonus-pipeline`

### 3. Avvio manuale (senza tmux)

```bash
# Terminal 1: daemon
make daemon

# Terminal 2: TUI
make tui

# Terminal 3: agent (opzionale)
make agent
```

### 4. Solo CLI legacy (senza daemon)

```bash
./bin/pipeline init 100
./bin/pipeline next
./bin/pipeline done bc-game --result 12.50
./bin/pipeline status
```

## Setup Ollama + Qwen3-VL (per agent)

```bash
# Installa Ollama
curl -fsSL https://ollama.com/install.sh | sh

# Scarica Qwen3-VL (30B, richiede ~20GB VRAM)
ollama pull qwen3-vl:30b

# Verifica
ollama list
curl http://localhost:11434/api/tags

# Se non hai abbastanza VRAM, usa la versione 8B
ollama pull qwen3-vl:8b
```

### Setup agent Python

```bash
cd agent
python -m venv .venv
source .venv/bin/activate
pip install -r requirements.txt
playwright install chromium

# Demo mode (primo casino da pipeline.json)
python main.py --demo --ollama http://localhost:11434

# Production mode (polling dal daemon)
python main.py --daemon localhost:50051

# Con modello piu leggero
python main.py --demo --model qwen3-vl:8b
```

## TUI — Comandi

### Navigazione pipeline
| Tasto | Azione |
|-------|--------|
| `n` | Mostra prossimo casino |
| `d` | Segna casino come completato |
| `s` | Salta casino |
| `i` | Inizializza pipeline con bankroll |
| `q` | Esci |

### GUIDED mode (sessione live)
| Tasto | Azione |
|-------|--------|
| `g` | Avvia/termina sessione guidata |
| `h` | Inserisci mano (BJ o poker) |
| `o` | Calcola pot odds (solo poker) |
| `w` | Registra vincita |
| `l` | Registra perdita |
| `d` | Chiudi sessione + mark done |

### Formato input mani

**Blackjack** — `totale,dealer_upcard`:
```
16,10    → Hard 16 vs dealer 10 → HIT
s17,6    → Soft 17 vs dealer 6  → DOUBLE
p8,5     → Pair 8 vs dealer 5   → SPLIT
11,7     → Hard 11 vs dealer 7  → DOUBLE
```

**Poker** — `mano,posizione`:
```
AKs,late    → [PREMIUM] RAISE/RE-RAISE
QQ,early    → [PREMIUM] RAISE/RE-RAISE
77,middle   → [PLAYABLE] RAISE da middle/late
T9s,late    → [SPECULATIVE] CALL se cheap
```

**Pot odds** — `pot,call,outs` o `pot,call,draw`:
```
100,20,9       → 9 outs, equity 36% > pot odds 17% → CALL
100,50,4       → 4 outs, equity 16% < pot odds 33% → FOLD
100,20,flush   → riconosce flush draw (9 outs)
100,20,oesd    → open-ended straight draw (8 outs)
100,20,gutshot → gutshot (4 outs)
100,20,combo   → flush+OESD (15 outs)
```

## Casino priority queue

| # | Casino | Tier | Wager | EV | Prelievo |
|---|--------|------|-------|----|----------|
| 1 | BC.Game | Crypto | 0x | +3.00 | Crypto 5min |
| 2 | BC Poker | Crypto | 0x | +10.00 | Crypto 5min |
| 3 | BitStarz | Crypto | 40x | +1.60 | BTC <10min |
| 4 | Winz.io | Crypto | 0x | variabile | Crypto 5min |
| 5 | LeoVegas IT | ADM | 1x | +9.80 | PayPal |
| 6 | Betsson IT | ADM | 35x | +165.00 | PayPal |
| 7 | William Hill IT | ADM | 0x | +50.00 | PayPal |
| 8 | BetPanda | Crypto | 0x cb | ongoing | Crypto 1min |
| 12 | Betflag IT | ADM | 3x | +2730.00 | PayPal 30s |

## Bannati

- **AdmiralBet IT**: dichiara 65x wagering, reale 100x — FRODE
- **StarVegas IT**: stessa frode — BANNATO
- **Betway IT**: chiuso (riforma ADM nov 2025)
- **Unibet IT**: chiuso (riforma ADM nov 2025)

## Matematica EV

```
EV = Bonus - (WageringMultiplier x Bonus x HouseEdge)
HouseEdge = 1 - RTP

LeoVegas: 100FS x €0.10 = €10, wager 1x, Blood Suckers 98% RTP
EV = €10 - (1 x €10 x 0.02) = +€9.80

Betsson: €200 bonus, wager 35x, BJ basic strategy 99.5% RTP
EV = €200 - (35 x €200 x 0.005) = +€165.00

Betflag: €3000 in tranche €200, wager 3x, 97% RTP
EV = €3000 - (3 x €3000 x 0.03) = +€2730.00
```

## Struttura progetto

```
bonus-pipeline/
├── cmd/daemon/main.go       # gRPC server (PipelineService + AgentService)
├── cmd/tui/main.go          # TUI FrameGoTUI con GUIDED mode
├── pkg/engine/
│   ├── types.go             # Tipi (compat pipeline.json/state.json)
│   ├── ev.go                # EV calc + BJ basic strategy + poker advisor
│   ├── state.go             # Load/Save state, pipeline operations
│   └── guided.go            # Sessioni guidate, pot odds, render panels
├── proto/
│   ├── pipeline.proto       # Definizione servizi gRPC
│   └── pipelinepb/          # Go stubs (hand-written)
├── agent/
│   ├── main.py              # Computer-use agent (browser-use + Qwen3-VL)
│   └── requirements.txt     # Dipendenze Python
├── main.go                  # CLI legacy
├── pipeline.json            # Database casino
├── state.json               # Stato pipeline
├── dashboard.html           # WebUI HTMX standalone
└── Makefile                 # Build, run, tmux orchestration
```
