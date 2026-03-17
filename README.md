# 💰 Bonus Pipeline

Semi-automated casino bonus hunting pipeline. Output di ogni casino = input del successivo.

## Come funziona

```
€100 → BC.Game(0x) → BC Poker($10 free) → BitStarz(20FS) → LeoVegas(1x) → ... → 💸 Prelievo finale
```

Ogni casino è ordinato per **EV decrescente**. Reinvesti il 100% fino alla fine.

## Setup

```bash
go build -o pipeline .
```

## Uso

```bash
# Inizializza con €100
./pipeline init 100

# Cosa fare ora
./pipeline next

# Dopo aver finito un casino
./pipeline done bc-game --result 12.50

# Stato completo
./pipeline status

# Calcolo EV per casino specifico
./pipeline ev leovegas-it

# Lista tutti i casino
./pipeline list
```

## Dashboard

Apri `dashboard.html` nel browser. Funziona offline, salva tutto in localStorage.

## Regole fondamentali

- **Bet size**: 1% del bonus per spin. MAI di più.
- **Stop-loss**: se il saldo scende sotto il 30% del bonus iniziale → abbandona quella sessione
- **Reinvestimento**: 100% del capitale fino al casino finale
- **Prelievo finale**: cripto istantaneo > PayPal/Skrill > wire

## Casino priority queue

| # | Casino | Wager | EV | Prelievo |
|---|--------|-------|----|----------|
| 1 | BC.Game | 0x | +€3 puro | Crypto 5min |
| 2 | BC Poker | 0x | skill-based | Crypto 5min |
| 3 | BitStarz | 40x su FS | +€1.60 | BTC <10min |
| 4 | Winz.io | 0x | variabile | Crypto 5min |
| 5 | LeoVegas IT | 1x | +€8.50 | PayPal |
| 6 | Betsson IT | 35x | +€47 | PayPal |
| 7 | William Hill IT | 0x | +€50 | PayPal |
| 8 | BetPanda | 0x cashback | ongoing | Crypto 1min |

## Bannati ⛔

- **AdmiralBet IT**: dichiara 65x wagering, reale è 100x — FRODE
- **StarVegas IT**: stessa frode — BANNATO  
- **Betway IT**: chiuso da novembre 2025
- **Unibet IT**: chiuso da novembre 2025

## Matematica

```
EV = Bonus - (WageringMultiplier × Bonus × HouseEdge)
HouseEdge = 1 - RTP

Esempi:
LeoVegas: 100FS × €0.10 = €10 bonus, wagering 1x
EV = €10 - (1 × €10 × 0.015) = €10 - €0.15 = +€9.85

Betsson: €200 bonus, wagering 35x, BJ 99.5% RTP
EV = €200 - (35 × €200 × 0.005) = €200 - €35 = +€165
```

## Supporto live

Durante le sessioni di poker o blackjack, chiedi a Cicciobot:
- **Poker**: "ho KQo BTN, 6 player, pot 8BB, call 3BB, flop As 7h 2d"
- **Blackjack**: "ho 14 vs dealer 6"
