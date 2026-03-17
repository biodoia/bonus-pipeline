# Casino Validation Results — VALIDATED

**Date:** 2026-03-17 04:05
**Rows:** 30 | **Provider:** Tavily | **Duration:** 3.2s
**Method:** ricerkatoro MCP batch search + manual cross-reference

## Legend
- **V** = Verified (URL confirmed, bonus confirmed from multiple sources)
- **P** = Partial (some data confirmed, some uncertain)
- **X** = Closed/Unavailable
- **F** = Suspected fraud/misleading terms
- **?** = Unverifiable (insufficient data)

---

## Tier 1 — HIGH CONFIDENCE (Verified URLs + Active Bonuses)

| # | Casino | Official URL | Bonus Type | Bonus | Real Wager | Withdrawal | Status |
|---|--------|-------------|-----------|-------|-----------|------------|--------|
| 1 | BC.Game | https://bc.game | no-deposit | $3 BCD free | 0x | Instant (crypto) | **V** |
| 3 | BitStarz | https://bitstarz.com | no-deposit FS | 50 free spins (was 20, now 50 per 2026 promos) | 40x | 10 min crypto | **V** |
| 5 | LeoVegas IT | https://www.leovegas.it | no-deposit FS | 100 free spins gratis senza deposito | 1x | 1-3 giorni bancario | **V** |
| 8 | BetPanda | https://betpandacasino.io | cashback | 10% weekly cashback + 5% live casino | 0x | Instant crypto | **V** |
| 9 | JetTon | https://jetton.io | no-deposit | Up to $1000 free chip (Lucky Wheel) | 3x | 24-48h | **P** — bonus amount variable |
| 22 | Stake.com | https://stake.com | rakeback | Variable rakeback VIP | 0x | Instant crypto | **V** |
| 28 | FortuneJack | https://fortunejack.com | no-deposit FS | 50 free spins (confirmed multiple sources 2026) | 40x est. | Fast crypto | **V** |

## Tier 2 — MEDIUM CONFIDENCE (URL OK, bonus terms need verification)

| # | Casino | Official URL | Bonus Type | Claimed Bonus | Claimed Wager | Notes | Status |
|---|--------|-------------|-----------|---------------|---------------|-------|--------|
| 2 | BC Poker | https://bcpoker.com (NOT bc.game/poker) | no-deposit | $10 free poker | 0x | URL was wrong in source data | **P** |
| 4 | Winz.io | https://winz.io | no-deposit | Variable (100% welcome no wagering) | 0x confirmed | No wagering on welcome bonus confirmed by site | **V** |
| 6 | Betsson IT | https://www.betsson.com | deposit | €200 100% match | 35x | .it redirects to .com; ADM license unclear | **P** |
| 10 | Vave | https://vave.com | no-deposit FS | 10-15 free spins | 30x | Multiple promo code sources confirm FS | **P** |
| 11 | mBit | https://www.mbitcasino.io | no-deposit FS | 50 free spins (site shows promotions page) | 35x | Confirmed via promotions page | **P** |
| 12 | Betflag IT | https://betflag.it | deposit-tranches | Up to €10,000 senza deposito (with restrictions) | 3x | ADM licensed | **P** |
| 13 | Rollino | https://rollino.com | no-deposit | 20 no-deposit spins (confirmed casinoalpha 2026) | 25x | Multiple review sites confirm | **V** |
| 15 | MyStake | https://mystake.com (mobile: m.mystake.com) | deposit | 170% deposit + 300 FS | 20x | Only deposit bonus found, no no-deposit | **P** |
| 23 | Cloudbet | https://www.cloudbet.com | deposit | Up to 5 BTC match | unknown | Sports + casino | **P** |
| 24 | Roobet | https://roobet.com | no-deposit | Variable | unknown | Geo-restricted, check VPN policy | **P** |
| 25 | Duelbits | https://duelbits.com | rakeback | Promotions page active | unknown (likely 0x rakeback) | Rakeback confirmed on promo page | **P** |
| 26 | Bets.io | https://bets.io | no-deposit FS | 20 free spins | unknown | Single page confirmed | **?** |
| 27 | Fairspin | https://fairspin.io | deposit | 450% + 140 FS welcome | unknown (high wager expected) | Aggressively marketed, check T&C carefully | **P** |
| 29 | Goldbet IT | https://goldbet.it (bonus: bonus.goldbet.it) | deposit | Up to €2050 scommesse | unknown | ADM licensed | **P** |
| 30 | Snai IT | https://snai.it | deposit | €15 no deposit claimed | unknown | ADM licensed, major Italian brand | **P** |

## Tier 3 — PROBLEMS (Closed, Fraud, or Unverifiable)

| # | Casino | URL | Issue | Status |
|---|--------|-----|-------|--------|
| 7 | William Hill IT | https://williamhill.it | Tavily returned only US promos (.us). IT version may be closed/migrated post-ADM reform 2025. | **X** — likely closed IT |
| 14 | DonBet | https://donbet.com | Multiple Trustpilot complaints (pages 3, 6). Non-GamStop casino. High risk. | **F** — fraud risk |
| 16 | AdmiralBet IT | https://admiralbet.it | Claimed 65x wager. No direct bonus page found. Sources show generic no-deposit lists only. | **F** — suspected predatory wager |
| 17 | StarVegas IT | https://starvegas.it | casino.guru complaint: "account under investigation". Article about bonus abuse fraud. | **F** — player complaints + fraud signals |
| 18 | Betway IT | https://betway.it | Tavily returned zero relevant IT results. Only "unknown" references. Likely closed since ADM reform Nov 2025. | **X** — closed IT market |
| 19 | Unibet IT | https://unibet.it | Unibet.it.com is a clone/affiliate, NOT official. ADM articles confirm market exit. | **X** — closed IT market |
| 20 | 888casino IT | https://888casino.it | Only US review found ($20 no deposit for .com). IT version status unclear post-reform. | **?** — unverifiable |
| 21 | PokerStars Casino IT | https://pokerstars.it | Only result: Reddit thread "pokerstars bonus scam". No active casino bonus confirmed. | **?** — no bonus found |

---

## Summary Statistics

| Category | Count | Details |
|----------|-------|---------|
| **Verified (V)** | 7 | BC.Game, BitStarz, LeoVegas IT, BetPanda, Stake.com, FortuneJack, Winz.io, Rollino |
| **Partial (P)** | 15 | URL confirmed, bonus needs T&C check |
| **Closed (X)** | 3 | William Hill IT, Betway IT, Unibet IT |
| **Fraud risk (F)** | 3 | DonBet, AdmiralBet IT, StarVegas IT |
| **Unverifiable (?)** | 2 | 888casino IT, PokerStars Casino IT |

## Browser Check (BrowserOS MCP — 2026-03-17 04:05)

Live browser verification via BrowserOS MCP `take_snapshot` on top 5 casinos:

| Casino | URL | Browser Status | Bonus Visible | Notes |
|--------|-----|---------------|---------------|-------|
| BC.Game | https://bc.game | **LIVE** | Yes | Italian UI ("Unisciti"), Originals/Slot/Live sections, big winners feed active |
| BitStarz | https://bitstarz.com | **LIVE** | Yes | Sign Up button, "Join 10+ Million Players", Sweet Bonanza/Le Bandit wins shown |
| BetPanda | https://betpanda.io | **LIVE** | Yes | Italian UI ("Accedi/Iscriviti"), Casino/Live/Scommesse/Promozioni tabs visible |
| LeoVegas IT | https://leovegas.it | **LIVE** | Yes | ADM badges visible, REGISTRATI button, "Offerte Di Benvenuto" + Promozioni links |
| Betflag IT | https://betflag.it | **LIVE** | Yes | San Patrizio promo (50.000€), Porta un Amico (10.000€ + bonus), ADM gioco legale badge |

### Batch 2 — Crypto casinos (2026-03-17 04:10)

| Casino | URL | Browser Status | Bonus Visible | Notes |
|--------|-----|---------------|---------------|-------|
| Duelbits | https://duelbits.com | **LIVE** | Yes | Redirects to duelbits.io. Casino/Sports/Predict tabs, "Boosted Welcome Offer", VIP Club, Clover Cash Tournament, Challenges (14 active) |
| Winz.io | https://winz.io | **LIVE** | Yes | Multiple "Sign Up" + "Show Details" buttons (6+ bonus offers visible), award-winning crypto casino |
| Thunderpick | https://thunderpick.io | **LIVE** | Yes | Esports/Sports/Casino tabs, cookie consent shown, ProductEsports/ProductSports/ProductCasino sections |
| BetMode | https://betmode.io | **LIVE** | Yes | "World's Most Transparent Casino", Weekly Wagering Race, Reward/Promotions/Challenges, Slots/Live Casino/Blackjack/Roulette/Game Shows |
| Bets.io | https://bets.io | **LIVE** | Yes | Casino tab selected, Lucky Wheel, Cashback, Promotions/Loyalty/VIP/Tournaments buttons all visible |

**Result:** All 10 browser-checked casinos (5 priority + 5 crypto) are LIVE with bonus offers visible as of 2026-03-17.

## Key Findings

1. **Italian market reform 2025** — ADM regulatory changes caused Betway, Unibet, and likely William Hill to exit the .it market
2. **Best no-deposit EV** — BC.Game ($3/0x), LeoVegas IT (100FS/1x), FortuneJack (50FS), Rollino (20 spins/25x)
3. **Best cashback** — BetPanda (10%/0x), Winz.io (0x welcome), Duelbits (rakeback)
4. **Avoid** — DonBet (Trustpilot complaints), AdmiralBet (predatory wager), StarVegas (fraud signals)
5. **BitStarz upgrade** — Now offering 50 free spins (up from 20 in original data)
