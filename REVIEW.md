# Code Review — bonus-pipeline

Full audit of all Go, Python, proto, and config files. Findings ordered by priority.

---

## CRITICAL

### C1. gRPC channels are plaintext (no TLS)
**Files:** `cmd/daemon/main.go`, `cmd/tui/main.go:34`, `agent/main.py:179`
All gRPC connections use `insecure` credentials. Task data, balances, casino
credentials, and withdrawal amounts travel in cleartext. Any network observer
can intercept everything.
**Fix:** Use `grpc.ssl_channel_credentials()` (Python) / `credentials.NewTLS()` (Go).
At minimum, enforce localhost-only binding.

### C2. Screenshots in memory contain credentials and PII
**File:** `agent/main.py:64,300-304,487,544`
Full-page screenshots are held as base64 strings during the entire vision loop.
These contain login forms (password fields visible while typing), personal data
(name, email, phone during registration), KYC documents, and account balances.
A process dump or crash log exposes everything.
**Fix:** Encrypt screenshots in memory, delete immediately after vision analysis,
never log base64 data (even truncated). Consider selective region capture.

### C3. No input validation on Qwen3-VL JSON responses
**File:** `agent/main.py:83-101,493-501`
The `describe_and_act()` method parses JSON from the LLM and passes `target`
directly to Playwright selectors without sanitization. A compromised or
hallucinating model could inject arbitrary selectors or values (e.g., filling
a credit card field, clicking "Delete Account").
**Fix:** Whitelist allowed actions, validate target length and characters,
reject any value containing shell metacharacters or SQL keywords.

### C4. state.json not in .gitignore — committed to public repo
**File:** `.gitignore`, `state.json`
`state.json` contains bankroll values, timestamps, and session state. It's
tracked by git and pushed to a public GitHub repo. In production this would
contain real financial data.
**Fix:** Add `state.json` to `.gitignore`. Remove from git history if sensitive
data was ever committed.

---

## HIGH

### H1. Balance regex can return wrong numbers
**File:** `agent/main.py:114`
```python
match = re.search(r"[\d]+\.?\d*", raw)
```
This matches the FIRST number in the LLM response, which could be an error
code ("404"), a date ("2024"), or an irrelevant number. If vision says
"Error 500: balance is 42.50", it returns 500.
**Fix:** Use context-aware pattern: `(?:balance|saldo|account)[\s:]*([0-9]+\.?[0-9]*)`.
Return all matches and pick the most plausible one. Validate range (0-1M).

### H2. Stop-loss checks only every 5 actions
**File:** `agent/main.py:47,530-542`
`BALANCE_CHECK_INTERVAL = 5` means the agent can lose 4 bet cycles before
noticing balance dropped below threshold. In a fast game, this is real money lost.
**Fix:** Check balance after EVERY action (`BALANCE_CHECK_INTERVAL = 1`).
Add hard wall-clock timeout per casino (e.g., 1 hour max).

### H3. Unchecked fmt.Sscanf in TUI — zero amount silently accepted
**File:** `cmd/tui/main.go:268,308`
```go
fmt.Sscanf(amount, "%f", &result) // return value ignored
```
If user types "abc", `result` stays 0.0. The casino gets marked done with
result=0, wiping the bankroll tracking.
**Fix:** Check the return: `if n, err := fmt.Sscanf(...); n == 0 || err != nil`.

### H4. TUI ignores LoadDB error — nil pointer dereference
**File:** `cmd/tui/main.go:41`
```go
db, _ = engine.LoadDB(*dataDir + "/pipeline.json")
```
If the file doesn't exist, `db` is nil. Later calls to `engine.FindCasino(db, ...)`
(line 121) will panic with nil pointer dereference.
**Fix:** Check error and exit: `if err != nil { log.Fatalf(...) }`.

### H5. No browser crash recovery in wagering loop
**File:** `agent/main.py:521-576`
If the browser crashes mid-wagering, `take_screenshot()` returns None repeatedly.
The error counter increments but the loop keeps running until `MAX_CONSECUTIVE_ERRORS`.
No browser restart or cleanup happens.
**Fix:** Detect browser death (check `browser.current_page` liveness). Implement
browser restart with page recovery. Add try-except around entire wagering loop.

### H6. Duplicate httpx.AsyncClient — resource leak
**File:** `agent/main.py:62,800`
`QwenVision` creates an `httpx.AsyncClient`, then `OllamaLLMAdapter` creates
a second one to the same endpoint. Two connection pools to the same URL.
If exception occurs during shutdown, `vision.close()` may not be reached.
**Fix:** Share the client. Use `async with httpx.AsyncClient()` context manager.

### H7. No Ollama authentication
**File:** `agent/main.py:59-62`
Ollama API is accessed over plain HTTP with no auth. If exposed beyond localhost
(common with Docker), anyone can send prompts and extract screenshot data.
**Fix:** Restrict to localhost. Add API key header if Ollama supports it.

### H8. AgentService registered in daemon but Go stubs lack serialization
**File:** `cmd/daemon/main.go:397`, `proto/pipelinepb/pipeline.go`
The daemon registers `AgentServiceServer`, but the hand-written Go stubs have
no `Marshal`/`Unmarshal` methods on any message type. gRPC encoding will fail
at runtime when any RPC is called — the agent Python client will get errors.
**Fix:** Either implement proto encoding interfaces or generate stubs with `protoc`.

---

## MEDIUM

### M1. Division by zero in bankroll bar chart
**File:** `cmd/tui/main.go:385`
```go
barLen := int(s.Output / resp.InitialBankroll * 12)
```
If `InitialBankroll` is 0 (pipeline not initialized), this panics.
**Fix:** Guard: `if resp.InitialBankroll > 0 { ... }`.

### M2. Division by zero in slots ETA
**File:** `pkg/engine/guided.go:370`
```go
etaMins := remaining / rate
```
If `rate` is 0 (no hands played yet or elapsed=0), this is NaN/Inf.
**Fix:** Guard: `if rate > 0 { ... }`.

### M3. Race condition on session access in TUI
**File:** `cmd/tui/main.go` (global `session` var)
The `session` pointer is read by render functions (called from ticker goroutine)
and written by keybinding handlers (main goroutine) without synchronization.
**Fix:** Add `var sessionMu sync.RWMutex` and wrap all session access.

### M4. Redundant FindCasino in NextPendingCasino
**File:** `pkg/engine/state.go:82`
Already iterating over `SortedCasinos(db.Casinos)`, then calls `FindCasino(db, c.ID)`
to look up the same casino again. Wasteful.
**Fix:** Return `&db.Casinos[i]` directly using the index.

### M5. Redundant deposit calculation
**File:** `main.go:170-171`, `pkg/engine/ev.go:62-63`
```go
return bankroll - depositAmt + depositAmt + ev
```
Simplifies to `bankroll + ev`. The subtraction and re-addition cancel out.
This is either a bug (deposit should reduce bankroll) or dead logic.
**Fix:** Clarify intent. If deposit is consumed: `return bankroll - depositAmt + ev`.
If deposit is returned: `return bankroll + ev` (remove the dance).

### M6. Task ID collisions
**File:** `agent/main.py:866`
```python
task_id=f"{casino['id']}-fullauto-{int(_time.time())}"
```
Same-second execution produces duplicate IDs.
**Fix:** Use `uuid.uuid4()` or add milliseconds.

### M7. Vision loop has no wall-clock timeout
**File:** `agent/main.py:483-519`
`_vision_loop()` has step limit but no time limit. If Qwen3-VL takes 60s per
response, a 30-step loop runs for 30 minutes.
**Fix:** Add `asyncio.timeout(600)` around the loop.

### M8. Withdrawal amount defaults to last balance if vision fails
**File:** `agent/main.py:457`
```python
withdrawn = self.balance
```
If vision can't read the withdrawal confirmation, it assumes the last known
balance was withdrawn. This could be wildly wrong.
**Fix:** Have vision confirm withdrawal success before accepting. If uncertain,
report "unknown" and require human confirmation.

### M9. Poker hand classification incomplete
**File:** `agent/main.py:776-789`, `pkg/engine/ev.go:359-375`
Missing many common hands (ATo, KJo, QTo). Suited connectors only partially
covered. Case-sensitive matching fails on "aks" vs "AKs".
**Fix:** Normalize to uppercase. Add more hand mappings. Use range-based
classification instead of exact match.

### M10. Casino data: 5 casinos have amount=0 with bonus in notes
**File:** `pipeline.json` (Winz.io, JetTon, Rollino, DonBet, MyStake)
EV calculation returns 0 for these casinos since `CalcBonusValue` reads
`Bonus.Amount` which is 0. The real bonus value is described in the `notes`
field as unstructured text.
**Fix:** Add a `estimated_value` field or parse notes. Currently ~33% of casinos
have zero EV in calculations.

### M11. No schema version in state.json
**File:** `pkg/engine/types.go`, `state.json`
If the PipelineState struct changes (new fields, renamed fields), old state.json
files will silently load with zero values for new fields.
**Fix:** Add `"schema_version": 1` field. Check on load, migrate if needed.

### M12. Makefile `build` doesn't depend on `proto`
**File:** `Makefile:13`
If proto stubs become stale, `make build` won't regenerate them.
**Fix:** Add `build: proto` or document that stubs are hand-written.

---

## LOW

### L1. No upper bound on bar chart length
**File:** `cmd/tui/main.go:384-388`
`barLen` could be huge if `InitialBankroll` is tiny. `strings.Repeat("#", 10000)`
allocates excessive memory.
**Fix:** Cap: `if barLen > 50 { barLen = 50 }`.

### L2. Hardcoded config values
**File:** `agent/main.py:41-50`
`OLLAMA_URL`, `DAEMON_ADDR`, etc. are hardcoded. CLI flags exist but defaults
should come from env vars.
**Fix:** `os.getenv("OLLAMA_URL", "http://localhost:11434")`.

### L3. Silent RTP default
**File:** `pkg/engine/ev.go:36-39`
When `BestGame.RTP == 0`, it silently defaults to 0.97. No warning logged.
**Fix:** Log a warning when using default RTP.

### L4. Game type detection too loose
**File:** `agent/main.py:714-721`
`"21" in name` matches "2020 Super Slots". `"bj" in name` matches unrelated strings.
**Fix:** Use word boundaries: `re.search(r"\bblackjack\b|\bbj\b", name)`.

### L5. Fixed screenshot delay (1.5s)
**File:** `agent/main.py:50`
Too slow for fast games, too fast for loading pages. Not adaptive.
**Fix:** Use `page.wait_for_load_state("networkidle")` where possible.

### L6. No rate limiting between casino runs
**File:** `agent/main.py:919`
Only 5s between casinos. Rapid account creation from same IP may trigger bans.
**Fix:** Add jitter: `asyncio.sleep(5 + random.uniform(0, 10))`.

### L7. Sensitive data in logs
**File:** `agent/main.py:195-196,373-379`
Casino URLs (may contain referral codes), bonus amounts, balance values are
logged in plain text. Logs are often aggregated and stored long-term.
**Fix:** Redact URLs and financial details in production logs.

### L8. truncStr panic on maxLen <= 3
**File:** `cmd/tui/main.go:632-636`
If `maxLen` is 1 or 2, `s[:maxLen-3]` produces negative index → panic.
Current code has guard `maxLen <= 3` returning `s` unchanged, which is correct.
But caller could pass negative maxLen.
**Fix:** Guard: `if maxLen <= 0 { return "" }`.

### L9. Italian-only strings in engine
**File:** `pkg/engine/ev.go:380`, `pkg/engine/guided.go` throughout
Strategy descriptions are hardcoded in Italian. Not internationalizable.
**Fix:** Extract to constants or use i18n if needed.

### L10. proto/pipelinepb will break if regenerated with protoc
**File:** `proto/pipelinepb/pipeline.go`
Field naming in hand-written stubs (`CasinoId`) may differ from protoc output
(`CasinoId` vs `CasinoID`). Regenerating stubs will break all imports.
**Fix:** Document this clearly. Add `//go:generate` comment with exact protoc command.

---

## Summary

| Severity | Count | Key Areas |
|----------|-------|-----------|
| CRITICAL | 4 | gRPC security, screenshot PII, JSON injection, state.json in git |
| HIGH | 8 | Balance parsing, stop-loss bypass, nil deref, resource leaks |
| MEDIUM | 12 | Div/zero, race conditions, task IDs, incomplete poker/casino data |
| LOW | 10 | Config, logging, i18n, minor edge cases |

**Top 3 actions before any production use:**
1. Add TLS to gRPC channels (C1)
2. Add `state.json` to `.gitignore` and encrypt screenshots (C2, C4)
3. Validate all vision JSON responses before executing actions (C3)
