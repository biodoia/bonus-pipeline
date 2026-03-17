"""
Bonus Pipeline Agent — browser-use + Qwen3-VL (Ollama) computer-use agent.

Modes:
  --demo       Run first casino from pipeline.json (semi-auto)
  --full-auto  FULL-AUTO: Qwen3-VL plays autonomously, no human intervention
  (default)    Production: poll daemon for tasks

Usage:
    python agent/main.py --full-auto --ollama http://localhost:11434
"""

import argparse
import asyncio
import base64
import json
import logging
import os
import time as _time
from dataclasses import dataclass, field
from enum import Enum
from typing import Optional

import grpc
import httpx
from PIL import Image

from browser_use import Agent as BrowserAgent, Browser, BrowserConfig

# ── Logging ──────────────────────────────────────────────────────────────────

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("agent")

# ── Config ───────────────────────────────────────────────────────────────────

OLLAMA_URL = os.getenv("OLLAMA_URL", "http://localhost:11434")
OLLAMA_MODEL = os.getenv("OLLAMA_MODEL", "qwen3-vl:30b")
DAEMON_ADDR = os.getenv("DAEMON_ADDR", "localhost:50051")
AGENT_ID = os.getenv("AGENT_ID", "agent-01")

# Safety constants
BALANCE_CHECK_INTERVAL = 1      # check balance every N actions (H2: was 5, too slow)
MAX_CONSECUTIVE_ERRORS = 10     # abort after this many vision errors in a row
MAX_STALE_ROUNDS = 30           # abort if no wagering progress for this many rounds
SCREENSHOT_DELAY = 1.5          # seconds to wait after action before screenshot


# ── Qwen3-VL Vision Backend ─────────────────────────────────────────────────


class QwenVision:
    """Wrapper around Ollama API for Qwen3-VL vision analysis."""

    def __init__(self, base_url: str = OLLAMA_URL, model: str = OLLAMA_MODEL):
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.client = httpx.AsyncClient(timeout=120.0)

    async def analyze(self, screenshot_b64: str, prompt: str) -> str:
        """Send a screenshot to Qwen3-VL and get analysis."""
        payload = {
            "model": self.model,
            "prompt": prompt,
            "images": [screenshot_b64],
            "stream": False,
            "options": {"temperature": 0.1, "num_predict": 1024},
        }
        try:
            resp = await self.client.post(
                f"{self.base_url}/api/generate", json=payload
            )
            resp.raise_for_status()
            return resp.json().get("response", "")
        except Exception as e:
            log.error("Qwen3-VL error: %s", e)
            return f"[vision error: {e}]"

    async def describe_and_act(self, screenshot_b64: str, goal: str) -> dict:
        """Core computer-use prompt: describe what's visible, decide what to click."""
        prompt = f"""Sei un computer use agent. Stai guardando lo screenshot di un browser.

OBIETTIVO: {goal}

ISTRUZIONI:
1. Descrivi brevemente gli elementi interattivi visibili (bottoni, input, link, menu)
2. Identifica l'elemento piu rilevante per raggiungere l'obiettivo
3. Decidi l'azione da eseguire

Rispondi SOLO in JSON:
{{"elements": "<breve descrizione elementi visibili>", "action": "click|type|scroll|wait|done|error", "target": "<testo del bottone o CSS selector>", "value": "<valore da digitare se action=type>", "reasoning": "<perche questa azione>"}}

Se l'obiettivo e gia raggiunto: {{"elements": "...", "action": "done", "target": "", "value": "", "reasoning": "obiettivo raggiunto"}}
Se c'e un errore/captcha/blocco: {{"elements": "...", "action": "error", "target": "", "value": "", "reasoning": "<descrivi il problema>"}}"""

        raw = await self.analyze(screenshot_b64, prompt)
        return self._parse_json(raw)

    async def read_balance(self, screenshot_b64: str) -> Optional[float]:
        """Read the current balance from a screenshot."""
        raw = await self.analyze(
            screenshot_b64,
            "Leggi il saldo/balance corrente visibile in questo screenshot. "
            "Rispondi con SOLO il numero del saldo, niente altro (es: 42.50). "
            "Se non vedi un saldo scrivi esattamente: NONE",
        )
        cleaned = raw.strip()
        if not cleaned or "NONE" in cleaned.upper():
            return None
        # Remove currency symbols and normalize decimal separator
        cleaned = cleaned.replace("€", "").replace("$", "").replace("£", "")
        cleaned = cleaned.replace(" ", "").replace("\n", "")
        # H1 fix: use context-aware regex — match number near balance keywords,
        # or if response is just a number, take it directly
        import re
        # Try 1: look for number near balance/saldo keywords
        ctx_match = re.search(
            r"(?:balance|saldo|account|conto|credito|funds)[\s:=]*([0-9]+[.,]?[0-9]*)",
            cleaned, re.IGNORECASE,
        )
        if ctx_match:
            val = ctx_match.group(1).replace(",", ".")
            try:
                result = float(val)
                if 0 < result < 1_000_000:
                    return result
            except ValueError:
                pass
        # Try 2: if response is mostly just a number (vision was asked for number only)
        num_only = re.sub(r"[^0-9.,]", "", cleaned).replace(",", ".")
        # Remove trailing dots
        num_only = num_only.rstrip(".")
        if num_only:
            try:
                result = float(num_only)
                if 0 < result < 1_000_000:
                    return result
            except ValueError:
                pass
        log.warning("Could not parse balance from: %s", raw[:80])
        return None

    async def read_game_state(self, screenshot_b64: str, game_type: str) -> dict:
        """Read game-specific state (cards, bet amount, result)."""
        if game_type == "blackjack":
            prompt = """Guarda questo screenshot di un gioco di blackjack.
Rispondi in JSON:
{"player_total": <numero>, "dealer_upcard": <numero>, "is_soft": <true/false>, "is_pair": <true/false>, "pair_card": <numero o null>, "bet_amount": <numero>, "phase": "betting|playing|result", "result": "win|loss|push|none"}
Se non riesci a leggere un valore, usa null."""
        elif game_type == "poker":
            prompt = """Guarda questo screenshot di un gioco di poker.
Rispondi in JSON:
{"hand": "<carte es: AKs>", "community": "<carte board>", "pot_size": <numero>, "bet_to_call": <numero>, "phase": "preflop|flop|turn|river|showdown", "position": "early|middle|late|blinds"}
Se non riesci a leggere un valore, usa null."""
        else:
            prompt = """Guarda questo screenshot di una slot machine.
Rispondi in JSON:
{"bet_amount": <numero>, "balance": <numero>, "phase": "idle|spinning|result", "last_win": <numero o 0>}"""

        raw = await self.analyze(screenshot_b64, prompt)
        return self._parse_json(raw)

    async def check_health(self) -> bool:
        """Check if Ollama + Qwen3-VL are available."""
        try:
            resp = await self.client.get(f"{self.base_url}/api/tags")
            resp.raise_for_status()
            models = [m["name"] for m in resp.json().get("models", [])]
            available = any(self.model.split(":")[0] in m for m in models)
            if not available:
                log.warning("Model %s not found. Available: %s", self.model, models)
            return available
        except Exception as e:
            log.error("Ollama health check failed: %s", e)
            return False

    def _parse_json(self, raw: str) -> dict:
        """Parse JSON from LLM response, handling markdown code blocks."""
        try:
            if "```json" in raw:
                raw = raw.split("```json")[1].split("```")[0]
            elif "```" in raw:
                raw = raw.split("```")[1].split("```")[0]
            return json.loads(raw.strip())
        except (json.JSONDecodeError, IndexError):
            log.warning("Failed to parse JSON: %s", raw[:200])
            return {"action": "error", "reasoning": f"parse failed: {raw[:100]}"}

    async def close(self):
        await self.client.aclose()


# ── gRPC Daemon Client ───────────────────────────────────────────────────────


class GRPCDaemonClient:
    """Minimal gRPC client for PipelineService + AgentService."""

    def __init__(self, addr: str):
        self.channel = grpc.insecure_channel(addr)
        self.addr = addr

    def get_task(self, agent_id: str) -> Optional[dict]:
        """Poll daemon for next task."""
        try:
            method = "/pipeline.PipelineService/GetNext"
            response = self.channel.unary_unary(method)(b"", timeout=5)
            return self._parse_task_from_next(response)
        except grpc.RpcError as e:
            log.error("gRPC GetTask error: %s", e)
            return None

    def report_status(self, task_id: str, status: str, message: str,
                      balance: float = 0, wagered: float = 0) -> str:
        """Report task status to daemon. Returns instruction."""
        log.info("REPORT [%s] %s: %s (balance=%.2f wagered=%.2f)",
                 task_id, status, message, balance, wagered)
        return "continue"

    def mark_done(self, casino_id: str, result: float) -> bool:
        """Mark casino as completed."""
        log.info("MARK_DONE: %s result=%.2f", casino_id, result)
        return True

    def _parse_task_from_next(self, raw_response) -> Optional[dict]:
        return None

    def close(self):
        self.channel.close()


# ── Task Types ───────────────────────────────────────────────────────────────


class TaskType(str, Enum):
    REGISTER = "register"
    CLAIM_BONUS = "claim_bonus"
    DO_WAGERING = "do_wagering"
    WITHDRAW = "withdraw"
    FULL_AUTO = "full_auto"  # complete pipeline for one casino


@dataclass
class AgentTask:
    task_id: str
    task_type: TaskType
    casino_id: str
    casino_name: str
    casino_url: str
    bonus_type: str
    wager: float
    bet_size: float
    stop_loss: float
    wager_target: float
    best_game: str
    game_rtp: float
    strategy_ref: str = ""
    tips: list[str] = field(default_factory=list)
    withdrawal_methods: list[str] = field(default_factory=list)
    kyc_required: bool = False
    spid_required: bool = False
    params: dict = field(default_factory=dict)


# ── Safety Watchdog ──────────────────────────────────────────────────────────


class SafetyWatchdog:
    """Monitors balance and errors, triggers abort if thresholds exceeded."""

    def __init__(self, stop_loss: float):
        self.stop_loss = stop_loss
        self.consecutive_errors = 0
        self.last_balance: Optional[float] = None
        self.last_wagered: float = 0
        self.stale_rounds = 0
        self.aborted = False
        self.abort_reason = ""

    def record_success(self):
        self.consecutive_errors = 0

    def record_error(self):
        self.consecutive_errors += 1
        if self.consecutive_errors >= MAX_CONSECUTIVE_ERRORS:
            self.abort(f"Too many consecutive errors ({self.consecutive_errors})")

    def check_balance(self, balance: float) -> bool:
        """Returns False if stop-loss triggered."""
        self.last_balance = balance
        if balance > 0 and balance < self.stop_loss:
            self.abort(f"STOP-LOSS: balance {balance:.2f} < threshold {self.stop_loss:.2f}")
            return False
        return True

    def check_progress(self, wagered: float):
        """Track wagering progress, abort if stale."""
        if wagered <= self.last_wagered:
            self.stale_rounds += 1
            if self.stale_rounds >= MAX_STALE_ROUNDS:
                self.abort(f"No wagering progress for {self.stale_rounds} rounds")
        else:
            self.stale_rounds = 0
            self.last_wagered = wagered

    def abort(self, reason: str):
        if not self.aborted:
            log.error("SAFETY ABORT: %s", reason)
            self.aborted = True
            self.abort_reason = reason

    @property
    def is_ok(self) -> bool:
        return not self.aborted


# ── Browser helpers ──────────────────────────────────────────────────────────


async def take_screenshot(browser: Browser) -> Optional[str]:
    """Capture browser screenshot as base64."""
    try:
        if browser and browser.current_page:
            png_bytes = await browser.current_page.screenshot()
            return base64.b64encode(png_bytes).decode("utf-8")
    except Exception as e:
        log.error("Screenshot failed: %s", e)
    return None


async def execute_action(browser: Browser, action: str, target: str, value: str = ""):
    """Execute a browser action from vision decision."""
    try:
        page = browser.current_page if browser else None
        if not page:
            log.error("No browser page")
            return False

        if action == "click":
            if target:
                try:
                    await page.click(target, timeout=5000)
                except Exception:
                    try:
                        await page.get_by_text(target).first.click(timeout=5000)
                    except Exception:
                        # Last resort: try role-based
                        await page.get_by_role("button", name=target).first.click(timeout=5000)
        elif action == "type":
            if target and value:
                try:
                    await page.fill(target, value, timeout=5000)
                except Exception:
                    await page.get_by_placeholder(target).first.fill(value, timeout=5000)
        elif action == "scroll":
            await page.evaluate("window.scrollBy(0, 300)")
        elif action == "wait":
            await asyncio.sleep(2)
            return True
        else:
            return True

        await asyncio.sleep(SCREENSHOT_DELAY)
        return True
    except Exception as e:
        log.error("Action failed (%s %s): %s", action, target[:50] if target else "", e)
        return False


# ── Full-Auto Controller ─────────────────────────────────────────────────────


class FullAutoController:
    """
    Orchestrates the full autonomous pipeline for a single casino:
    Navigate → Register → Claim Bonus → Play Game → Wager → Withdraw

    Uses Qwen3-VL for ALL decisions. No human intervention.
    """

    def __init__(self, vision: QwenVision, browser: Browser, daemon: GRPCDaemonClient):
        self.vision = vision
        self.browser = browser
        self.daemon = daemon
        self.wagered: float = 0.0
        self.balance: float = 0.0
        self.actions_since_balance_check: int = 0

    async def run_casino(self, task: AgentTask) -> float:
        """Run the full pipeline for one casino. Returns final withdrawn amount."""
        watchdog = SafetyWatchdog(task.stop_loss)

        log.info("=" * 60)
        log.info("FULL-AUTO: %s (%s)", task.casino_name, task.casino_id)
        log.info("  URL: %s", task.casino_url)
        log.info("  Bonus: %s | Wager: %.0fx | Target: %.2f",
                 task.bonus_type, task.wager, task.wager_target)
        log.info("  Game: %s (RTP %.1f%%)", task.best_game, task.game_rtp * 100)
        log.info("  Bet: %.2f | Stop-loss: %.2f", task.bet_size, task.stop_loss)
        log.info("=" * 60)

        self.wagered = 0.0
        self.balance = 0.0
        self.actions_since_balance_check = 0
        withdrawn = 0.0

        try:
            # Step 1: Navigate to casino
            log.info("[1/5] Navigating to %s", task.casino_url)
            await self._vision_loop(
                goal=f"Vai su {task.casino_url} e attendi che la pagina carichi completamente",
                max_steps=10,
                watchdog=watchdog,
            )
            if not watchdog.is_ok:
                return 0.0

            # Step 2: Register
            log.info("[2/5] Registering account")
            spid_note = " Questo casino richiede SPID." if task.spid_required else ""
            kyc_note = " KYC sara richiesto." if task.kyc_required else ""
            await self._vision_loop(
                goal=f"Registra un nuovo account su questo casino. Cerca un bottone 'Sign Up', 'Register', 'Registrati' o simile. Compila i campi richiesti.{spid_note}{kyc_note}",
                max_steps=30,
                watchdog=watchdog,
            )
            if not watchdog.is_ok:
                return 0.0

            # Step 3: Claim bonus
            log.info("[3/5] Claiming %s bonus", task.bonus_type)
            if task.bonus_type in ("no-deposit", "no-deposit-freespins"):
                bonus_goal = "Riscatta il bonus senza deposito. Cerca nella sezione Bonus, Promozioni, o Free Spins. Attiva il bonus NO-DEPOSIT."
            else:
                bonus_goal = f"Vai alla cassa/deposito. Effettua un deposito per attivare il bonus di tipo {task.bonus_type}."
            await self._vision_loop(
                goal=bonus_goal,
                max_steps=20,
                watchdog=watchdog,
            )
            if not watchdog.is_ok:
                return 0.0

            # Step 4: Navigate to game and do wagering
            log.info("[4/5] Playing %s — wagering target: %.2f", task.best_game, task.wager_target)
            await self._vision_loop(
                goal=f"Naviga al gioco '{task.best_game}'. Usa la ricerca se disponibile, oppure sfoglia le categorie.",
                max_steps=15,
                watchdog=watchdog,
            )
            if not watchdog.is_ok:
                return 0.0

            # Wagering loop — the core of full-auto
            if task.wager_target > 0:
                await self._wagering_loop(task, watchdog)
            else:
                log.info("No wagering required (0x), skipping to withdrawal")

            # Step 5: Withdraw
            if watchdog.is_ok:
                log.info("[5/5] Withdrawing funds")
                preferred = task.withdrawal_methods[0] if task.withdrawal_methods else "crypto"
                await self._vision_loop(
                    goal=f"Vai alla sezione prelievo/withdrawal/cashier. Preleva tutto il saldo disponibile. Metodo preferito: {preferred}. Metodi disponibili: {', '.join(task.withdrawal_methods)}. Conferma il prelievo.",
                    max_steps=25,
                    watchdog=watchdog,
                )

                # Try to read withdrawn amount
                screenshot = await take_screenshot(self.browser)
                if screenshot:
                    amount = await self.vision.read_balance(screenshot)
                    if amount and amount > 0:
                        withdrawn = amount
                        log.info("Withdrawal amount detected: %.2f", withdrawn)
                    else:
                        withdrawn = self.balance
                        log.info("Using last known balance as withdrawal: %.2f", withdrawn)

        except Exception as e:
            log.error("Full-auto error: %s", e)
            watchdog.abort(f"Exception: {e}")

        # Report final status
        status = "completed" if watchdog.is_ok else "failed"
        self.daemon.report_status(
            task.task_id, status,
            watchdog.abort_reason if not watchdog.is_ok else f"Withdrawn: {withdrawn:.2f}",
            self.balance, self.wagered,
        )
        if withdrawn > 0:
            self.daemon.mark_done(task.casino_id, withdrawn)

        log.info("FULL-AUTO COMPLETE: %s | withdrawn=%.2f | wagered=%.2f | status=%s",
                 task.casino_name, withdrawn, self.wagered, status)
        if not watchdog.is_ok:
            log.error("Abort reason: %s", watchdog.abort_reason)

        return withdrawn

    async def _vision_loop(self, goal: str, max_steps: int, watchdog: SafetyWatchdog):
        """Generic vision-action loop: screenshot → Qwen3-VL decides → execute → repeat."""
        for step in range(max_steps):
            if not watchdog.is_ok:
                return

            screenshot = await take_screenshot(self.browser)
            if not screenshot:
                watchdog.record_error()
                await asyncio.sleep(2)
                continue

            decision = await self.vision.describe_and_act(screenshot, goal)
            action = decision.get("action", "wait")
            target = decision.get("target", "")
            value = decision.get("value", "")
            reasoning = decision.get("reasoning", "")
            elements = decision.get("elements", "")

            log.info("  [step %d/%d] %s → %s | %s",
                     step + 1, max_steps, action, target[:40] if target else "-", reasoning[:60])

            if action == "done":
                watchdog.record_success()
                return

            if action == "error":
                log.warning("  Vision reports error: %s", reasoning)
                watchdog.record_error()
                await asyncio.sleep(3)
                continue

            success = await execute_action(self.browser, action, target, value)
            if success:
                watchdog.record_success()
            else:
                watchdog.record_error()

        log.warning("  Vision loop exhausted max_steps=%d for goal: %s", max_steps, goal[:60])

    async def _wagering_loop(self, task: AgentTask, watchdog: SafetyWatchdog):
        """The core wagering loop: play the game repeatedly until target met."""
        game_type = self._detect_game_type(task.best_game)
        max_rounds = int(task.wager_target / task.bet_size) + 200
        round_num = 0

        log.info("Wagering loop: type=%s, target=%.2f, bet=%.2f, max_rounds=%d",
                 game_type, task.wager_target, task.bet_size, max_rounds)

        while self.wagered < task.wager_target and round_num < max_rounds:
            if not watchdog.is_ok:
                return

            round_num += 1
            self.actions_since_balance_check += 1

            # H2 fix: check balance every action (BALANCE_CHECK_INTERVAL=1)
            if self.actions_since_balance_check >= BALANCE_CHECK_INTERVAL:
                await self._check_balance_safe(watchdog)
                if not watchdog.is_ok:
                    return
                self.actions_since_balance_check = 0

            # H5 fix: browser crash recovery — detect dead browser and abort
            try:
                if not self.browser or not self.browser.current_page:
                    watchdog.abort("Browser died — no current page")
                    return

                screenshot = await take_screenshot(self.browser)
                if not screenshot:
                    watchdog.record_error()
                    await asyncio.sleep(2)
                    continue

                # Game-specific strategy via vision
                if game_type == "blackjack":
                    acted = await self._play_blackjack_hand(screenshot, task, watchdog)
                elif game_type == "poker":
                    acted = await self._play_poker_hand(screenshot, task, watchdog)
                else:
                    acted = await self._play_slot_spin(screenshot, task, watchdog)

                if acted:
                    self.wagered += task.bet_size
                    watchdog.record_success()
                    watchdog.check_progress(self.wagered)
                else:
                    watchdog.record_error()

            except Exception as e:
                log.error("Wagering round %d exception: %s", round_num, e)
                watchdog.record_error()
                # Check if browser is still alive after exception
                try:
                    if self.browser and self.browser.current_page:
                        await asyncio.sleep(2)
                    else:
                        watchdog.abort(f"Browser crashed during wagering: {e}")
                        return
                except Exception:
                    watchdog.abort(f"Browser unrecoverable after: {e}")
                    return

            # Progress log
            if round_num % 10 == 0:
                pct = (self.wagered / task.wager_target * 100) if task.wager_target > 0 else 100
                log.info("  Progress: round=%d wagered=%.2f/%.2f (%.0f%%) balance=%.2f",
                         round_num, self.wagered, task.wager_target, pct, self.balance)
                self.daemon.report_status(
                    task.task_id, "in_progress", f"Wagering {pct:.0f}%",
                    self.balance, self.wagered,
                )

        log.info("Wagering loop ended: wagered=%.2f/%.2f rounds=%d",
                 self.wagered, task.wager_target, round_num)

    async def _play_blackjack_hand(self, screenshot: str, task: AgentTask,
                                    watchdog: SafetyWatchdog) -> bool:
        """Play one BJ hand using vision to read cards and basic strategy to decide."""
        # Read game state
        state = await self.vision.read_game_state(screenshot, "blackjack")
        phase = state.get("phase", "betting")

        if phase == "betting":
            # Set bet and deal
            decision = await self.vision.describe_and_act(
                screenshot,
                f"Sei in un gioco di blackjack. Imposta la puntata a {task.bet_size:.2f} e premi 'Deal', 'Dai carte', o il bottone per iniziare la mano."
            )
            return await execute_action(
                self.browser, decision.get("action", "click"),
                decision.get("target", ""), decision.get("value", "")
            )

        elif phase == "playing":
            player = state.get("player_total")
            dealer = state.get("dealer_upcard")
            is_soft = state.get("is_soft", False)
            is_pair = state.get("is_pair", False)

            if player and dealer:
                # Use basic strategy to determine action
                action = self._bj_basic_strategy(player, dealer, is_soft, is_pair,
                                                  state.get("pair_card"))
                log.info("  BJ: player=%s%d vs dealer=%d → %s",
                         "S" if is_soft else ("P" if is_pair else ""), player, dealer, action)

                decision = await self.vision.describe_and_act(
                    screenshot,
                    f"Blackjack: devi fare {action}. Clicca il bottone '{action}' (o 'Hit'/'Stand'/'Double'/'Split' in base alla mossa)."
                )
                return await execute_action(
                    self.browser, decision.get("action", "click"),
                    decision.get("target", ""), decision.get("value", "")
                )
            else:
                # Vision couldn't read cards, try generic approach
                decision = await self.vision.describe_and_act(
                    screenshot,
                    "Sei in un gioco di blackjack in corso. Guarda le carte e decidi: se il totale e 17+ stai, se e 11 raddoppia, altrimenti chiedi carta."
                )
                return await execute_action(
                    self.browser, decision.get("action", "click"),
                    decision.get("target", ""), decision.get("value", "")
                )

        elif phase == "result":
            # Hand is over, click to continue/re-bet
            result = state.get("result", "none")
            if result == "win":
                log.info("  BJ: WIN")
            elif result == "loss":
                log.info("  BJ: LOSS")
            decision = await self.vision.describe_and_act(
                screenshot,
                "La mano di blackjack e finita. Clicca 'New Bet', 'Rebet', 'Continue', o il bottone per iniziare una nuova mano."
            )
            return await execute_action(
                self.browser, decision.get("action", "click"),
                decision.get("target", ""), decision.get("value", "")
            )

        return False

    async def _play_poker_hand(self, screenshot: str, task: AgentTask,
                                watchdog: SafetyWatchdog) -> bool:
        """Play one poker action using vision + pot odds."""
        state = await self.vision.read_game_state(screenshot, "poker")
        phase = state.get("phase", "preflop")
        hand = state.get("hand", "")
        pot = state.get("pot_size", 0) or 0
        call_amt = state.get("bet_to_call", 0) or 0

        if hand:
            # Classify hand and decide
            tier = self._poker_hand_tier(hand)
            if call_amt > 0 and pot > 0:
                pot_odds = call_amt / (pot + call_amt) * 100
                action_word = "CALL" if tier <= 2 or pot_odds < 20 else "FOLD"
            elif tier <= 1:
                action_word = "RAISE"
            elif tier <= 2:
                action_word = "CALL"
            else:
                action_word = "FOLD"

            log.info("  POKER: %s [tier=%d] pot=%.0f call=%.0f → %s",
                     hand, tier, pot, call_amt, action_word)

            decision = await self.vision.describe_and_act(
                screenshot,
                f"Poker: devi fare {action_word}. Clicca il bottone '{action_word}' (o Fold/Call/Raise/Check)."
            )
        else:
            decision = await self.vision.describe_and_act(
                screenshot,
                "Sei in un gioco di poker. Se puoi fare check, fai check. Se devi puntare, fai la puntata minima. Se non hai carte buone e devi pagare molto, fai fold."
            )

        return await execute_action(
            self.browser, decision.get("action", "click"),
            decision.get("target", ""), decision.get("value", "")
        )

    async def _play_slot_spin(self, screenshot: str, task: AgentTask,
                               watchdog: SafetyWatchdog) -> bool:
        """Play one slot spin using vision."""
        decision = await self.vision.describe_and_act(
            screenshot,
            f"Sei in una slot machine. Assicurati che la puntata sia impostata a {task.bet_size:.2f} (o il minimo possibile). Poi clicca il bottone 'Spin', 'Gira', o il bottone rotondo grande per avviare la giocata. Se il rullo sta girando, aspetta."
        )
        action = decision.get("action", "wait")
        if action == "wait":
            await asyncio.sleep(3)
            return True
        return await execute_action(
            self.browser, action,
            decision.get("target", ""), decision.get("value", "")
        )

    async def _check_balance_safe(self, watchdog: SafetyWatchdog):
        """Read balance and check stop-loss."""
        screenshot = await take_screenshot(self.browser)
        if not screenshot:
            return

        balance = await self.vision.read_balance(screenshot)
        if balance is not None:
            self.balance = balance
            if not watchdog.check_balance(balance):
                return  # stop-loss triggered, watchdog.aborted = True

    def _detect_game_type(self, game_name: str) -> str:
        """Detect game type from name."""
        name = game_name.lower()
        if "blackjack" in name or "bj" in name or "21" in name:
            return "blackjack"
        if "poker" in name:
            return "poker"
        return "slots"

    def _bj_basic_strategy(self, total: int, dealer: int, is_soft: bool,
                            is_pair: bool, pair_card: Optional[int]) -> str:
        """Basic strategy decision. Returns action string."""
        if is_pair and pair_card:
            if pair_card in (11, 8):
                return "SPLIT"
            if pair_card == 10:
                return "STAND"
            if pair_card == 9 and dealer not in (7, 10, 11):
                return "SPLIT"
            if pair_card in (2, 3, 7) and dealer <= 7:
                return "SPLIT"
            if pair_card == 6 and dealer <= 6:
                return "SPLIT"
            if pair_card == 5 and dealer <= 9:
                return "DOUBLE"
            if pair_card == 4 and dealer in (5, 6):
                return "SPLIT"

        if is_soft:
            if total >= 19:
                return "STAND"
            if total == 18:
                if 3 <= dealer <= 6:
                    return "DOUBLE"
                if dealer >= 9:
                    return "HIT"
                return "STAND"
            if total == 17 and 3 <= dealer <= 6:
                return "DOUBLE"
            return "HIT"

        # Hard totals
        if total >= 17:
            return "STAND"
        if total == 16 and dealer >= 9:
            return "SURRENDER"
        if total == 15 and dealer == 10:
            return "SURRENDER"
        if 13 <= total <= 16 and dealer <= 6:
            return "STAND"
        if total == 12 and 4 <= dealer <= 6:
            return "STAND"
        if total == 11:
            return "DOUBLE"
        if total == 10 and dealer <= 9:
            return "DOUBLE"
        if total == 9 and 3 <= dealer <= 6:
            return "DOUBLE"
        if total >= 12:
            return "HIT"
        return "HIT"

    def _poker_hand_tier(self, hand: str) -> int:
        """0=premium, 1=strong, 2=playable, 3=speculative, 4=fold."""
        h = hand.upper().replace(" ", "")
        if h in ("AA", "KK", "QQ", "AKS"):
            return 0
        if h in ("JJ", "TT", "AQS", "AKO"):
            return 1
        if h in ("99", "88", "77", "AJS", "ATS", "KQS", "AQO"):
            return 2
        if len(h) >= 2 and h[0] == h[1] and h[0] in "23456":
            return 3
        if h.endswith("S") and len(h) == 3:
            return 3
        return 4


# ── Ollama LLM Adapter for browser-use ───────────────────────────────────────


class OllamaLLMAdapter:
    """Adapter that makes Qwen3-VL compatible with browser-use LLM interface.
    H6 fix: reuses the QwenVision httpx client instead of creating a duplicate."""

    def __init__(self, vision: QwenVision):
        self.vision = vision
        self.client = vision.client  # H6: share client, don't create a new one
        self.base_url = vision.base_url
        self.model = vision.model

    async def generate(self, prompt: str, images: list[str] | None = None) -> str:
        payload = {
            "model": self.model,
            "prompt": prompt,
            "stream": False,
            "options": {"temperature": 0.1, "num_predict": 2048},
        }
        if images:
            payload["images"] = images
        try:
            resp = await self.client.post(f"{self.base_url}/api/generate", json=payload)
            resp.raise_for_status()
            return resp.json().get("response", "")
        except Exception as e:
            log.error("LLM generate error: %s", e)
            return f"Error: {e}"

    async def chat(self, messages: list[dict]) -> str:
        payload = {
            "model": self.model,
            "messages": messages,
            "stream": False,
            "options": {"temperature": 0.1, "num_predict": 2048},
        }
        try:
            resp = await self.client.post(f"{self.base_url}/api/chat", json=payload)
            resp.raise_for_status()
            return resp.json().get("message", {}).get("content", "")
        except Exception as e:
            log.error("LLM chat error: %s", e)
            return f"Error: {e}"


# ── Pipeline Runner ──────────────────────────────────────────────────────────


def load_pipeline_db() -> dict:
    """Load pipeline.json from project root."""
    for path in ["pipeline.json", "../pipeline.json",
                  os.path.join(os.path.dirname(os.path.dirname(__file__)), "pipeline.json")]:
        if os.path.exists(path):
            with open(path) as f:
                return json.load(f)
    raise FileNotFoundError("pipeline.json not found")


def build_tasks_for_casino(casino: dict, rules: dict) -> list[AgentTask]:
    """Build the task sequence for a single casino."""
    bonus = casino["bonus"]
    best_game = casino["best_game"]

    bonus_value = bonus.get("amount", 0)
    if bonus["type"] == "no-deposit-freespins":
        bonus_value = bonus["amount"] * bonus.get("spin_value", 0.10)
    elif bonus["type"] == "cashback":
        return []  # skip cashback

    bet_size = max(bonus_value * rules.get("bet_size_percent", 0.01), 0.10)
    wager_target = bonus.get("wager", 0) * bonus_value
    stop_loss = bonus_value * rules.get("stop_loss_percent", 0.30)

    return [AgentTask(
        task_id=f"{casino['id']}-fullauto-{int(_time.time())}",
        task_type=TaskType.FULL_AUTO,
        casino_id=casino["id"],
        casino_name=casino["name"],
        casino_url=casino["url"],
        bonus_type=bonus["type"],
        wager=bonus.get("wager", 0),
        bet_size=bet_size,
        stop_loss=stop_loss,
        wager_target=wager_target,
        best_game=best_game["name"],
        game_rtp=best_game["rtp"],
        tips=[
            f"Bet size: {bet_size:.2f}",
            f"Wager target: {wager_target:.2f}",
            f"Stop-loss: {stop_loss:.2f}",
        ],
        withdrawal_methods=casino["payment"].get("withdrawal", []),
        kyc_required=casino.get("kyc_required", False),
        spid_required=casino.get("spid_required", False),
    )]


async def run_full_auto(vision: QwenVision, browser: Browser, daemon: GRPCDaemonClient,
                         casino_filter: Optional[str] = None):
    """FULL-AUTO mode: run through all casinos autonomously."""
    db = load_pipeline_db()
    rules = db.get("rules", {})
    casinos = sorted(db["casinos"], key=lambda c: c["priority"])

    controller = FullAutoController(vision, browser, daemon)
    total_withdrawn = 0.0
    results = []

    for casino in casinos:
        if casino_filter and casino["id"] != casino_filter:
            continue
        if casino["bonus"]["type"] == "cashback":
            log.info("Skipping cashback casino: %s", casino["name"])
            continue

        tasks = build_tasks_for_casino(casino, rules)
        if not tasks:
            continue

        task = tasks[0]  # FULL_AUTO is a single task per casino
        withdrawn = await controller.run_casino(task)
        total_withdrawn += withdrawn
        results.append((casino["name"], withdrawn))

        if casino_filter:
            break  # only run the specified casino

        await asyncio.sleep(5)  # pause between casinos

    log.info("=" * 60)
    log.info("FULL-AUTO PIPELINE COMPLETE")
    log.info("=" * 60)
    for name, amount in results:
        log.info("  %-25s → %.2f", name, amount)
    log.info("  TOTAL WITHDRAWN: %.2f", total_withdrawn)

    return total_withdrawn


async def run_demo(vision: QwenVision, browser: Browser, daemon: GRPCDaemonClient):
    """Demo mode: run first casino only."""
    db = load_pipeline_db()
    rules = db.get("rules", {})

    for casino in sorted(db["casinos"], key=lambda c: c["priority"]):
        if casino["bonus"]["type"] == "cashback":
            continue
        tasks = build_tasks_for_casino(casino, rules)
        if tasks:
            controller = FullAutoController(vision, browser, daemon)
            await controller.run_casino(tasks[0])
            break


# ── Main ─────────────────────────────────────────────────────────────────────


async def main():
    parser = argparse.ArgumentParser(description="Bonus Pipeline Agent")
    parser.add_argument("--daemon", default=DAEMON_ADDR, help="gRPC daemon address")
    parser.add_argument("--ollama", default=OLLAMA_URL, help="Ollama API URL")
    parser.add_argument("--model", default=OLLAMA_MODEL, help="Ollama vision model")
    parser.add_argument("--headless", action="store_true", help="Run browser headless")
    parser.add_argument("--demo", action="store_true", help="Demo: run first casino only")
    parser.add_argument("--full-auto", action="store_true", help="FULL-AUTO: run all casinos autonomously")
    parser.add_argument("--casino", default=None, help="Run only this casino ID (with --full-auto)")
    args = parser.parse_args()

    vision = QwenVision(args.ollama, args.model)
    daemon = GRPCDaemonClient(args.daemon)

    log.info("Bonus Pipeline Agent starting...")
    log.info("  Ollama: %s (%s)", args.ollama, args.model)
    log.info("  Daemon: %s", args.daemon)

    if await vision.check_health():
        log.info("  Qwen3-VL: READY")
    else:
        log.error("  Qwen3-VL: NOT AVAILABLE — cannot run full-auto without vision")
        if args.full_auto:
            return

    config = BrowserConfig(headless=args.headless)
    browser = Browser(config)
    log.info("  Browser: initialized (headless=%s)", args.headless)

    try:
        if args.full_auto:
            log.info("MODE: FULL-AUTO")
            await run_full_auto(vision, browser, daemon, casino_filter=args.casino)
        elif args.demo:
            log.info("MODE: DEMO (first casino)")
            await run_demo(vision, browser, daemon)
        else:
            log.info("MODE: PRODUCTION (polling daemon)")
            while True:
                task_data = daemon.get_task(AGENT_ID)
                if task_data:
                    log.info("Got task: %s", task_data)
                else:
                    await asyncio.sleep(5)
    except KeyboardInterrupt:
        log.info("Agent interrupted")
    finally:
        await browser.close()
        await vision.close()
        daemon.close()
        log.info("Agent shutdown complete")


if __name__ == "__main__":
    asyncio.run(main())
