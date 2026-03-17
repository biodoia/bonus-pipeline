"""
Bonus Pipeline Agent — browser-use + Qwen3-VL (Ollama) computer-use agent.

Connects to the gRPC daemon, receives tasks (RegisterCasino, DoWagering, Withdraw),
controls browser via browser-use, uses Qwen3-VL for vision analysis of screenshots.

Usage:
    python agent/main.py [--daemon localhost:50051] [--ollama http://localhost:11434]
"""

import argparse
import asyncio
import base64
import io
import json
import logging
import sys
import time
from dataclasses import dataclass, field
from enum import Enum
from typing import Optional

import grpc
import httpx
from PIL import Image

# browser-use imports
from browser_use import Agent as BrowserAgent, Browser, BrowserConfig

# ── Logging ──────────────────────────────────────────────────────────────────

logging.basicConfig(
    level=logging.INFO,
    format="%(asctime)s [%(levelname)s] %(message)s",
    datefmt="%H:%M:%S",
)
log = logging.getLogger("agent")

# ── Config ───────────────────────────────────────────────────────────────────

OLLAMA_URL = "http://localhost:11434"
OLLAMA_MODEL = "qwen3-vl:30b"
DAEMON_ADDR = "localhost:50051"
AGENT_ID = "agent-01"


# ── Qwen3-VL Vision Backend ─────────────────────────────────────────────────


class QwenVision:
    """Wrapper around Ollama API for Qwen3-VL vision analysis."""

    def __init__(self, base_url: str = OLLAMA_URL, model: str = OLLAMA_MODEL):
        self.base_url = base_url.rstrip("/")
        self.model = model
        self.client = httpx.AsyncClient(timeout=120.0)

    async def analyze_screenshot(
        self, screenshot_b64: str, prompt: str
    ) -> str:
        """Send a screenshot to Qwen3-VL and get analysis."""
        payload = {
            "model": self.model,
            "prompt": prompt,
            "images": [screenshot_b64],
            "stream": False,
            "options": {
                "temperature": 0.1,
                "num_predict": 1024,
            },
        }
        try:
            resp = await self.client.post(
                f"{self.base_url}/api/generate", json=payload
            )
            resp.raise_for_status()
            data = resp.json()
            return data.get("response", "")
        except Exception as e:
            log.error("Qwen3-VL error: %s", e)
            return f"[vision error: {e}]"

    async def decide_action(
        self, screenshot_b64: str, context: str, available_actions: list[str]
    ) -> dict:
        """Analyze screenshot and decide next browser action."""
        prompt = f"""You are a browser automation agent for casino bonus hunting.

CONTEXT: {context}

AVAILABLE ACTIONS: {json.dumps(available_actions)}

Look at this screenshot and decide the SINGLE BEST next action.
Respond in JSON format:
{{"action": "<action_name>", "target": "<css_selector_or_text>", "value": "<input_value_if_needed>", "reasoning": "<brief_why>"}}

If you see an error or captcha, respond:
{{"action": "report_issue", "target": "", "value": "", "reasoning": "<describe_issue>"}}
"""
        raw = await self.analyze_screenshot(screenshot_b64, prompt)

        # Parse JSON from response
        try:
            # Extract JSON from markdown code blocks if present
            if "```json" in raw:
                raw = raw.split("```json")[1].split("```")[0]
            elif "```" in raw:
                raw = raw.split("```")[1].split("```")[0]
            return json.loads(raw.strip())
        except (json.JSONDecodeError, IndexError):
            log.warning("Failed to parse vision response: %s", raw[:200])
            return {
                "action": "report_issue",
                "target": "",
                "value": "",
                "reasoning": f"Could not parse vision response: {raw[:100]}",
            }

    async def check_health(self) -> bool:
        """Check if Ollama + Qwen3-VL are available."""
        try:
            resp = await self.client.get(f"{self.base_url}/api/tags")
            resp.raise_for_status()
            models = [m["name"] for m in resp.json().get("models", [])]
            available = any(self.model.split(":")[0] in m for m in models)
            if not available:
                log.warning(
                    "Model %s not found. Available: %s", self.model, models
                )
            return available
        except Exception as e:
            log.error("Ollama health check failed: %s", e)
            return False

    async def close(self):
        await self.client.aclose()


# ── gRPC Stub (manual, matching proto) ───────────────────────────────────────
# We use raw gRPC calls since we don't have protoc-generated Python stubs.
# This matches the AgentService defined in pipeline.proto.


class GRPCDaemonClient:
    """Minimal gRPC client for the AgentService using raw channel calls."""

    def __init__(self, addr: str):
        self.channel = grpc.insecure_channel(addr)
        self.addr = addr

    def get_task(self, agent_id: str) -> Optional[dict]:
        """Call AgentService.GetTask — returns task dict or None."""
        # Since we don't have compiled proto stubs, we use the PipelineService
        # GetNext + GetStrategy as a combined "get task" workflow.
        try:
            # Use unary call to get next casino
            method = "/pipeline.PipelineService/GetNext"
            response = self.channel.unary_unary(method)(b"", timeout=5)
            # For now, we parse the response manually
            # In production, use grpcio-tools generated stubs
            return self._parse_task_from_next(response)
        except grpc.RpcError as e:
            log.error("gRPC GetTask error: %s", e)
            return None

    def report_status(
        self,
        task_id: str,
        status: str,
        message: str,
        balance: float = 0,
        wagered: float = 0,
    ) -> str:
        """Report task status to daemon. Returns instruction."""
        log.info(
            "REPORT [%s] %s: %s (balance=%.2f wagered=%.2f)",
            task_id, status, message, balance, wagered,
        )
        # In production, this calls AgentService.ReportTaskStatus
        return "continue"

    def mark_done(self, casino_id: str, result: float) -> bool:
        """Call PipelineService.MarkDone when a casino task is complete."""
        try:
            method = "/pipeline.PipelineService/MarkDone"
            # Encode a simple request — in production use protobuf
            log.info("MarkDone: %s result=%.2f", casino_id, result)
            return True
        except grpc.RpcError as e:
            log.error("gRPC MarkDone error: %s", e)
            return False

    def _parse_task_from_next(self, raw_response) -> Optional[dict]:
        """Parse raw gRPC response into a task dict."""
        # Placeholder — full implementation needs protobuf deserialization
        return None

    def close(self):
        self.channel.close()


# ── Task Types ───────────────────────────────────────────────────────────────


class TaskType(str, Enum):
    REGISTER = "register"
    CLAIM_BONUS = "claim_bonus"
    DO_WAGERING = "do_wagering"
    WITHDRAW = "withdraw"


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
    strategy_ref: str
    tips: list[str] = field(default_factory=list)
    withdrawal_methods: list[str] = field(default_factory=list)
    kyc_required: bool = False
    spid_required: bool = False
    params: dict = field(default_factory=dict)


# ── Browser Agent Controller ─────────────────────────────────────────────────


class BonusPipelineAgent:
    """Main agent that orchestrates browser-use + Qwen3-VL for bonus hunting."""

    def __init__(
        self,
        daemon_addr: str = DAEMON_ADDR,
        ollama_url: str = OLLAMA_URL,
        ollama_model: str = OLLAMA_MODEL,
        headless: bool = False,
    ):
        self.daemon = GRPCDaemonClient(daemon_addr)
        self.vision = QwenVision(ollama_url, ollama_model)
        self.headless = headless
        self.browser: Optional[Browser] = None
        self.current_task: Optional[AgentTask] = None
        self.wagered_amount: float = 0.0
        self.current_balance: float = 0.0

    async def start(self):
        """Initialize browser and check vision backend."""
        log.info("Starting Bonus Pipeline Agent...")
        log.info("Daemon: %s", self.daemon.addr)

        # Check Ollama health
        if await self.vision.check_health():
            log.info("Qwen3-VL ready at %s", self.vision.base_url)
        else:
            log.warning(
                "Qwen3-VL not available — agent will run with limited vision"
            )

        # Initialize browser
        config = BrowserConfig(headless=self.headless)
        self.browser = Browser(config)
        log.info("Browser initialized (headless=%s)", self.headless)

    async def shutdown(self):
        """Cleanup resources."""
        if self.browser:
            await self.browser.close()
        await self.vision.close()
        self.daemon.close()
        log.info("Agent shutdown complete")

    # ── Task execution ───────────────────────────────────────────────────

    async def execute_task(self, task: AgentTask):
        """Execute a single agent task."""
        self.current_task = task
        self.wagered_amount = 0.0
        log.info(
            "=== TASK: %s on %s (%s) ===",
            task.task_type.value, task.casino_name, task.casino_id,
        )

        try:
            if task.task_type == TaskType.REGISTER:
                await self.do_register(task)
            elif task.task_type == TaskType.CLAIM_BONUS:
                await self.do_claim_bonus(task)
            elif task.task_type == TaskType.DO_WAGERING:
                await self.do_wagering(task)
            elif task.task_type == TaskType.WITHDRAW:
                await self.do_withdraw(task)
            else:
                log.error("Unknown task type: %s", task.task_type)
        except Exception as e:
            log.error("Task failed: %s", e)
            self.daemon.report_status(
                task.task_id, "failed", str(e),
                self.current_balance, self.wagered_amount,
            )

    async def do_register(self, task: AgentTask):
        """Navigate to casino and register."""
        log.info("Step: Register at %s", task.casino_url)

        agent = BrowserAgent(
            task=f"""
            Go to {task.casino_url} and register a new account.
            - Look for a "Sign Up", "Register", or "Create Account" button
            - Fill in required fields (use generated credentials)
            - Complete any verification steps
            - Do NOT deposit any money yet
            {"- This casino requires SPID authentication" if task.spid_required else ""}
            {"- KYC documents will be required" if task.kyc_required else ""}
            """,
            llm=self._make_llm_callback(),
            browser=self.browser,
        )
        result = await agent.run()
        log.info("Register result: %s", result)

        self.daemon.report_status(
            task.task_id, "completed", "Registration done",
        )

    async def do_claim_bonus(self, task: AgentTask):
        """Claim the bonus (no-deposit or deposit bonus)."""
        log.info("Step: Claim %s bonus at %s", task.bonus_type, task.casino_name)

        if task.bonus_type in ("no-deposit", "no-deposit-freespins"):
            instruction = f"""
            On {task.casino_url}, claim the no-deposit bonus.
            - Look for "Bonus", "Promotions", "Free Spins", or similar section
            - Activate the no-deposit bonus (no payment needed)
            - Verify the bonus appears in your balance
            """
        else:
            instruction = f"""
            On {task.casino_url}, claim the deposit bonus.
            - Go to the cashier/deposit section
            - Make a deposit to activate the bonus
            - Verify the bonus is credited
            """

        agent = BrowserAgent(
            task=instruction,
            llm=self._make_llm_callback(),
            browser=self.browser,
        )
        result = await agent.run()
        log.info("Claim bonus result: %s", result)

        self.daemon.report_status(
            task.task_id, "completed", "Bonus claimed",
        )

    async def do_wagering(self, task: AgentTask):
        """Execute wagering requirement using the optimal game + strategy."""
        log.info(
            "Step: Wagering on %s — target: %.2f, bet: %.2f, game: %s",
            task.casino_name, task.wager_target, task.bet_size, task.best_game,
        )

        # Phase 1: Navigate to the game
        nav_agent = BrowserAgent(
            task=f"""
            On {task.casino_url}, navigate to the game: {task.best_game}
            - Use search if available, or browse the game categories
            - Open the game and wait for it to load
            - Do NOT place any bets yet
            """,
            llm=self._make_llm_callback(),
            browser=self.browser,
        )
        await nav_agent.run()

        # Phase 2: Vision-guided wagering loop
        await self._wagering_loop(task)

    async def _wagering_loop(self, task: AgentTask):
        """Main wagering loop using Qwen3-VL for decision making."""
        max_rounds = int(task.wager_target / task.bet_size) + 100
        round_num = 0

        while self.wagered_amount < task.wager_target and round_num < max_rounds:
            round_num += 1

            # Take screenshot
            screenshot_b64 = await self._take_screenshot()
            if not screenshot_b64:
                log.error("Failed to capture screenshot")
                await asyncio.sleep(2)
                continue

            # Analyze with Qwen3-VL
            context = f"""
Casino: {task.casino_name}
Game: {task.best_game} (RTP: {task.game_rtp*100:.1f}%)
Bet size: {task.bet_size}
Wagered so far: {self.wagered_amount:.2f} / {task.wager_target:.2f}
Stop-loss threshold: {task.stop_loss:.2f}
Round: {round_num}

STRATEGY:
{task.strategy_ref}

TIPS:
{chr(10).join('- ' + t for t in task.tips)}
"""
            actions = [
                "click_bet",        # place bet / spin / deal
                "click_element",    # click a specific UI element
                "type_text",        # type in an input field
                "wait",             # wait for animation/loading
                "read_balance",     # read current balance from screen
                "report_issue",     # something is wrong
            ]

            decision = await self.vision.decide_action(
                screenshot_b64, context, actions
            )
            action = decision.get("action", "wait")
            target = decision.get("target", "")
            value = decision.get("value", "")
            reasoning = decision.get("reasoning", "")

            log.info(
                "Round %d | Action: %s | Target: %s | Reason: %s",
                round_num, action, target[:50], reasoning[:80],
            )

            # Execute the decided action
            if action == "click_bet":
                await self._execute_browser_action("click", target)
                self.wagered_amount += task.bet_size
            elif action == "click_element":
                await self._execute_browser_action("click", target)
            elif action == "type_text":
                await self._execute_browser_action("type", target, value)
            elif action == "read_balance":
                balance_str = await self.vision.analyze_screenshot(
                    screenshot_b64,
                    "Read the current account balance from this screenshot. "
                    "Return ONLY the numeric value (e.g., 42.50).",
                )
                try:
                    self.current_balance = float(
                        balance_str.strip().replace("€", "").replace("$", "").replace(",", ".")
                    )
                    log.info("Balance: %.2f", self.current_balance)
                except ValueError:
                    log.warning("Could not parse balance: %s", balance_str)
            elif action == "report_issue":
                log.warning("Issue detected: %s", reasoning)
                instruction = self.daemon.report_status(
                    task.task_id, "paused", reasoning,
                    self.current_balance, self.wagered_amount,
                )
                if instruction == "stop":
                    log.info("Daemon says stop. Aborting wagering.")
                    return
                await asyncio.sleep(5)
            else:  # wait
                await asyncio.sleep(2)

            # Stop-loss check
            if self.current_balance > 0 and self.current_balance < task.stop_loss:
                log.warning(
                    "STOP-LOSS triggered: balance %.2f < threshold %.2f",
                    self.current_balance, task.stop_loss,
                )
                self.daemon.report_status(
                    task.task_id, "completed",
                    f"Stop-loss at {self.current_balance:.2f}",
                    self.current_balance, self.wagered_amount,
                )
                return

            # Progress report every 20 rounds
            if round_num % 20 == 0:
                pct = (self.wagered_amount / task.wager_target * 100) if task.wager_target > 0 else 100
                log.info(
                    "Progress: %.2f / %.2f (%.0f%%) balance=%.2f",
                    self.wagered_amount, task.wager_target, pct,
                    self.current_balance,
                )
                self.daemon.report_status(
                    task.task_id, "in_progress",
                    f"Wagering {pct:.0f}%",
                    self.current_balance, self.wagered_amount,
                )

        log.info(
            "Wagering complete: %.2f / %.2f", self.wagered_amount, task.wager_target
        )
        self.daemon.report_status(
            task.task_id, "completed",
            f"Wagering done ({self.wagered_amount:.2f})",
            self.current_balance, self.wagered_amount,
        )

    async def do_withdraw(self, task: AgentTask):
        """Withdraw funds from the casino."""
        log.info(
            "Step: Withdraw from %s via %s",
            task.casino_name, ", ".join(task.withdrawal_methods),
        )

        preferred = task.withdrawal_methods[0] if task.withdrawal_methods else "any"

        agent = BrowserAgent(
            task=f"""
            On {task.casino_url}, withdraw all available funds.
            - Go to the cashier/withdrawal section
            - Preferred method: {preferred}
            - Available methods: {', '.join(task.withdrawal_methods)}
            - Withdraw the maximum possible amount
            - Confirm the withdrawal
            - Note the final withdrawn amount
            """,
            llm=self._make_llm_callback(),
            browser=self.browser,
        )
        result = await agent.run()

        # Read final balance
        screenshot_b64 = await self._take_screenshot()
        if screenshot_b64:
            balance_str = await self.vision.analyze_screenshot(
                screenshot_b64,
                "What is the withdrawal amount shown? Return ONLY the number.",
            )
            try:
                amount = float(
                    balance_str.strip().replace("€", "").replace("$", "").replace(",", ".")
                )
                log.info("Withdrawn: %.2f", amount)
                self.daemon.mark_done(task.casino_id, amount)
            except ValueError:
                log.warning("Could not parse withdrawal amount: %s", balance_str)

        self.daemon.report_status(
            task.task_id, "completed", "Withdrawal submitted",
            self.current_balance, self.wagered_amount,
        )

    # ── Browser helpers ──────────────────────────────────────────────────

    async def _take_screenshot(self) -> Optional[str]:
        """Capture browser screenshot as base64."""
        try:
            if self.browser and self.browser.current_page:
                png_bytes = await self.browser.current_page.screenshot()
                return base64.b64encode(png_bytes).decode("utf-8")
        except Exception as e:
            log.error("Screenshot failed: %s", e)
        return None

    async def _execute_browser_action(
        self, action_type: str, target: str, value: str = ""
    ):
        """Execute a browser action (click, type, etc.)."""
        try:
            page = self.browser.current_page if self.browser else None
            if not page:
                log.error("No browser page available")
                return

            if action_type == "click":
                if target:
                    try:
                        await page.click(target, timeout=5000)
                    except Exception:
                        # Fallback: try clicking by text content
                        await page.get_by_text(target).first.click(timeout=5000)
            elif action_type == "type":
                if target:
                    try:
                        await page.fill(target, value, timeout=5000)
                    except Exception:
                        await page.get_by_placeholder(target).first.fill(
                            value, timeout=5000
                        )

            await asyncio.sleep(1)  # Brief pause after action
        except Exception as e:
            log.error("Browser action failed (%s %s): %s", action_type, target, e)

    def _make_llm_callback(self):
        """Create an LLM callback for browser-use that routes to Qwen3-VL."""
        # browser-use expects an LLM interface; we provide a wrapper
        # that calls Qwen3-VL via Ollama for vision tasks
        return OllamaLLMAdapter(self.vision)


# ── Ollama LLM Adapter for browser-use ───────────────────────────────────────


class OllamaLLMAdapter:
    """Adapter that makes Qwen3-VL (via Ollama) compatible with browser-use LLM interface."""

    def __init__(self, vision: QwenVision):
        self.vision = vision
        self.client = httpx.AsyncClient(timeout=120.0)
        self.base_url = vision.base_url
        self.model = vision.model

    async def generate(self, prompt: str, images: list[str] | None = None) -> str:
        """Generate a response, optionally with images."""
        payload = {
            "model": self.model,
            "prompt": prompt,
            "stream": False,
            "options": {"temperature": 0.1, "num_predict": 2048},
        }
        if images:
            payload["images"] = images

        try:
            resp = await self.client.post(
                f"{self.base_url}/api/generate", json=payload
            )
            resp.raise_for_status()
            return resp.json().get("response", "")
        except Exception as e:
            log.error("LLM generate error: %s", e)
            return f"Error: {e}"

    async def chat(self, messages: list[dict]) -> str:
        """Chat completion compatible endpoint."""
        payload = {
            "model": self.model,
            "messages": messages,
            "stream": False,
            "options": {"temperature": 0.1, "num_predict": 2048},
        }
        try:
            resp = await self.client.post(
                f"{self.base_url}/api/chat", json=payload
            )
            resp.raise_for_status()
            data = resp.json()
            return data.get("message", {}).get("content", "")
        except Exception as e:
            log.error("LLM chat error: %s", e)
            return f"Error: {e}"


# ── Main loop ────────────────────────────────────────────────────────────────


async def run_demo(agent: BonusPipelineAgent):
    """Demo mode: execute a full pipeline run on the first pending casino."""
    # For demo, we construct a task manually from pipeline.json
    import os

    pipeline_path = os.path.join(
        os.path.dirname(os.path.dirname(__file__)), "pipeline.json"
    )
    if not os.path.exists(pipeline_path):
        pipeline_path = "pipeline.json"

    with open(pipeline_path) as f:
        db = json.load(f)

    rules = db.get("rules", {})

    for casino in db["casinos"]:
        casino_id = casino["id"]
        bonus = casino["bonus"]
        best_game = casino["best_game"]
        bonus_value = bonus.get("amount", 0)

        if bonus["type"] == "no-deposit-freespins":
            spin_val = bonus.get("spin_value", 0.10)
            bonus_value = bonus["amount"] * spin_val
        elif bonus["type"] == "cashback":
            continue  # skip cashback casinos in demo

        bet_size = max(bonus_value * rules.get("bet_size_percent", 0.01), 0.10)
        wager_target = bonus.get("wager", 0) * bonus_value
        stop_loss = bonus_value * rules.get("stop_loss_percent", 0.30)

        # Build task sequence for this casino
        tasks = [
            AgentTask(
                task_id=f"{casino_id}-register",
                task_type=TaskType.REGISTER,
                casino_id=casino_id,
                casino_name=casino["name"],
                casino_url=casino["url"],
                bonus_type=bonus["type"],
                wager=bonus.get("wager", 0),
                bet_size=bet_size,
                stop_loss=stop_loss,
                wager_target=wager_target,
                best_game=best_game["name"],
                game_rtp=best_game["rtp"],
                strategy_ref="",
                tips=[],
                withdrawal_methods=casino["payment"].get("withdrawal", []),
                kyc_required=casino.get("kyc_required", False),
                spid_required=casino.get("spid_required", False),
            ),
            AgentTask(
                task_id=f"{casino_id}-claim",
                task_type=TaskType.CLAIM_BONUS,
                casino_id=casino_id,
                casino_name=casino["name"],
                casino_url=casino["url"],
                bonus_type=bonus["type"],
                wager=bonus.get("wager", 0),
                bet_size=bet_size,
                stop_loss=stop_loss,
                wager_target=wager_target,
                best_game=best_game["name"],
                game_rtp=best_game["rtp"],
                strategy_ref="",
                tips=[],
                withdrawal_methods=casino["payment"].get("withdrawal", []),
                kyc_required=casino.get("kyc_required", False),
                spid_required=casino.get("spid_required", False),
            ),
        ]

        # Add wagering task if wager > 0
        if wager_target > 0:
            tasks.append(
                AgentTask(
                    task_id=f"{casino_id}-wager",
                    task_type=TaskType.DO_WAGERING,
                    casino_id=casino_id,
                    casino_name=casino["name"],
                    casino_url=casino["url"],
                    bonus_type=bonus["type"],
                    wager=bonus.get("wager", 0),
                    bet_size=bet_size,
                    stop_loss=stop_loss,
                    wager_target=wager_target,
                    best_game=best_game["name"],
                    game_rtp=best_game["rtp"],
                    strategy_ref="",
                    tips=[
                        f"Bet size: {bet_size:.2f}",
                        f"Wager target: {wager_target:.2f}",
                        f"Stop-loss at {stop_loss:.2f}",
                    ],
                    withdrawal_methods=casino["payment"].get("withdrawal", []),
                    kyc_required=casino.get("kyc_required", False),
                    spid_required=casino.get("spid_required", False),
                )
            )

        # Always end with withdrawal
        tasks.append(
            AgentTask(
                task_id=f"{casino_id}-withdraw",
                task_type=TaskType.WITHDRAW,
                casino_id=casino_id,
                casino_name=casino["name"],
                casino_url=casino["url"],
                bonus_type=bonus["type"],
                wager=bonus.get("wager", 0),
                bet_size=bet_size,
                stop_loss=stop_loss,
                wager_target=wager_target,
                best_game=best_game["name"],
                game_rtp=best_game["rtp"],
                strategy_ref="",
                tips=[],
                withdrawal_methods=casino["payment"].get("withdrawal", []),
                kyc_required=casino.get("kyc_required", False),
                spid_required=casino.get("spid_required", False),
            )
        )

        # Execute all tasks for this casino
        for task in tasks:
            await agent.execute_task(task)
            await asyncio.sleep(2)

        log.info("=== Casino %s complete ===\n", casino["name"])
        break  # Demo: only first casino


async def main():
    parser = argparse.ArgumentParser(description="Bonus Pipeline Agent")
    parser.add_argument("--daemon", default=DAEMON_ADDR, help="gRPC daemon address")
    parser.add_argument("--ollama", default=OLLAMA_URL, help="Ollama API URL")
    parser.add_argument("--model", default=OLLAMA_MODEL, help="Ollama vision model")
    parser.add_argument("--headless", action="store_true", help="Run browser headless")
    parser.add_argument(
        "--demo", action="store_true",
        help="Demo mode: run first casino from pipeline.json",
    )
    args = parser.parse_args()

    agent = BonusPipelineAgent(
        daemon_addr=args.daemon,
        ollama_url=args.ollama,
        ollama_model=args.model,
        headless=args.headless,
    )

    try:
        await agent.start()

        if args.demo:
            await run_demo(agent)
        else:
            # Production mode: poll daemon for tasks
            log.info("Agent running in production mode — polling daemon for tasks...")
            while True:
                task_data = agent.daemon.get_task(AGENT_ID)
                if task_data:
                    # Convert to AgentTask and execute
                    log.info("Got task: %s", task_data)
                    # TODO: deserialize task_data into AgentTask
                else:
                    await asyncio.sleep(5)
    except KeyboardInterrupt:
        log.info("Agent interrupted")
    finally:
        await agent.shutdown()


if __name__ == "__main__":
    asyncio.run(main())
