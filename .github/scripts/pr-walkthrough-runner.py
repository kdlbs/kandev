#!/usr/bin/env python3
"""Supervise PR walkthrough generation and verify completed output."""

from dataclasses import dataclass
from datetime import datetime, timezone
import json
import os
from pathlib import Path
import re
import shutil
import signal
import stat
import subprocess
import sys
import threading
import time


TOTAL_DEADLINE_SECONDS = 600
POLL_INTERVAL_SECONDS = 0.25
TERM_GRACE_SECONDS = 5
CLEANUP_TIMEOUT_SECONDS = 10
VERIFIER_TIMEOUT_SECONDS = 30
DISCOVERY_TIMEOUT_SECONDS = 60
MAX_INCOMPLETE_RETRIES = 1
PREFERRED_FREE_MODEL = "opencode/deepseek-v4.1-flash-free"
FREE_MODEL_PROVIDER = "opencode"
GO_FALLBACK_MODEL = "opencode-go/muse-spark-1.3-contributor"
PREFERRED_VARIANT = "high"
MODEL_PLACEHOLDER = "{model}"
VARIANT_PLACEHOLDER = "{variant}"
WORK_DIR = Path(".pr-walkthrough")
DISCOVERY_DIR = WORK_DIR / "discovery"
DISCOVERY_RECORD_PATH = DISCOVERY_DIR / "discovery.json"
RECEIPT_PATH = WORK_DIR / "render-complete.json"
DRAFT_PATH = WORK_DIR / "draft.json"
OUTPUT_DIR = Path("docs/pr-walkthrough")
VERIFIER_PATH = Path(".agents/skills/pr-walkthrough/scripts/pr-walkthrough-verify")
PROC_ROOT = Path("/proc")

MODEL_ID_RE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*/[A-Za-z0-9][A-Za-z0-9._-]*$")
QUOTA_ERROR_PATTERNS = (
    re.compile(r"\bfree[\s_-]*usage[\s_-]*(?:limit|quota|exceeded)\b", re.IGNORECASE),
    re.compile(r"\b(?:free|go)usage[\s_-]*limit[\s_-]*error\b", re.IGNORECASE),
    re.compile(r"\busage\s+limit\s+(?:reached|exceeded)\b", re.IGNORECASE),
    re.compile(r"\bgo\s+usage\s+limit\s+exceeded\b", re.IGNORECASE),
    re.compile(r"\binsufficient_quota\b", re.IGNORECASE),
    re.compile(r"\bquota[\s_-]+(?:exceeded|exhausted|reached)\b", re.IGNORECASE),
    re.compile(r"\bexceeded\s+(?:your\s+)?(?:current\s+)?quota\b", re.IGNORECASE),
    re.compile(r"\bout\s+of\s+quota\b", re.IGNORECASE),
    re.compile(r"\bresource_exhausted\b", re.IGNORECASE),
    re.compile(r"\bcredit\s+limit\s+reached\b", re.IGNORECASE),
    re.compile(r"\bout\s+of\s+credits?\b", re.IGNORECASE),
    re.compile(r"\binsufficient\s+credits?\b", re.IGNORECASE),
    re.compile(r"\binsufficient\s+balance\b", re.IGNORECASE),
)
RATE_LIMIT_ERROR_PATTERNS = (
    re.compile(r"\b429\s+too\s+many\s+requests\b", re.IGNORECASE),
    re.compile(r"\bAI_APICallError:\s*429\b", re.IGNORECASE),
    re.compile(r"\bHTTP\s+429\b", re.IGNORECASE),
    re.compile(r"\bstatus[\s_-]*(?:code)?\s*[=:]\s*429\b", re.IGNORECASE),
    re.compile(r"\btoo\s+many\s+requests\b", re.IGNORECASE),
    re.compile(r"\brate[\s_-]?limit(?:ed)?\b[\s:]*(?:exceeded|reached)\b", re.IGNORECASE),
    re.compile(r"\brate[\s_-]?limited\b", re.IGNORECASE),
    re.compile(r"\brate_limit_(?:exceeded|error)\b", re.IGNORECASE),
    re.compile(r"\brate\s+increased\s+too\s+quickly\b", re.IGNORECASE),
)
UNAVAILABLE_ERROR_PATTERNS = (
    re.compile(r"\bProviderModelNotFoundError\b", re.IGNORECASE),
    re.compile(r"\bmodel(?:\s+is)?\s+not\s+found\b", re.IGNORECASE),
    re.compile(r"\bmodel_not_found\b", re.IGNORECASE),
    re.compile(r"\bunknown\s+model\b", re.IGNORECASE),
    re.compile(r"\bmodel(?:\s+is)?\s+(?:not\s+available|unavailable)\b", re.IGNORECASE),
    re.compile(r"\bprovider\s+is\s+overloaded\b", re.IGNORECASE),
)
LOG_FIELD_RE = re.compile(r'(?:^|\s)([A-Za-z0-9_.-]+)=(?:"((?:[^"\\]|\\.)*)"|(\S+))')
PROVIDER_ERROR_MARKERS = ("AI_APICallError", "AI_RetryError", "ProviderModelNotFoundError")


@dataclass(frozen=True)
class RunConfig:
    deadline_seconds: float = TOTAL_DEADLINE_SECONDS
    poll_interval_seconds: float = POLL_INTERVAL_SECONDS
    term_grace_seconds: float = TERM_GRACE_SECONDS
    cleanup_timeout_seconds: float = CLEANUP_TIMEOUT_SECONDS
    verifier_timeout_seconds: float = VERIFIER_TIMEOUT_SECONDS
    discovery_timeout_seconds: float = DISCOVERY_TIMEOUT_SECONDS
    max_incomplete_retries: int = MAX_INCOMPLETE_RETRIES


@dataclass(frozen=True)
class ModelMetadata:
    model_id: str
    variants: frozenset[str]
    free: bool

    @property
    def provider_id(self) -> str:
        return self.model_id.split("/", 1)[0]

    @property
    def model_name(self) -> str:
        return self.model_id.split("/", 1)[1]


@dataclass(frozen=True)
class Candidate:
    model_id: str
    variants: frozenset[str]
    source: str

    @property
    def provider_id(self) -> str:
        return self.model_id.split("/", 1)[0]

    @property
    def model_name(self) -> str:
        return self.model_id.split("/", 1)[1]


@dataclass(frozen=True)
class ModelDiscovery:
    status: str
    models: tuple[ModelMetadata, ...] = ()
    error: str | None = None

    @property
    def candidates(self) -> tuple[Candidate, ...]:
        free = [
            model
            for model in self.models
            if model.free and model.provider_id == FREE_MODEL_PROVIDER
        ]
        unique: dict[str, ModelMetadata] = {}
        for model in free:
            unique.setdefault(model.model_id, model)
        ordered = sorted(
            unique.values(),
            key=lambda model: (
                0 if model.model_id == PREFERRED_FREE_MODEL else 1,
                model.model_id,
            ),
        )
        return tuple(
            Candidate(model.model_id, model.variants, "free") for model in ordered
        )


def utc_now() -> str:
    return datetime.now(timezone.utc).isoformat(timespec="milliseconds").replace("+00:00", "Z")


def positive_pr_number(env: dict[str, str]) -> int:
    value = env.get("PR_NUMBER", "")
    if not value.isdigit() or int(value) < 1:
        raise ValueError("PR_NUMBER must be a positive integer")
    return int(value)


def group_has_live_members(pgid: int) -> bool:
    """Check for executing members; Linux keeps stopped children as zombies."""
    try:
        process_dirs = list(PROC_ROOT.iterdir())
    except OSError:
        return True

    for process_dir in process_dirs:
        if not process_dir.name.isdecimal():
            continue
        try:
            stat_text = (process_dir / "stat").read_text(encoding="utf-8")
            fields = stat_text.rsplit(")", 1)[1].split()
            state = fields[0]
            member_pgid = int(fields[2])
        except FileNotFoundError:
            continue
        except (IndexError, OSError, ValueError):
            return True
        if member_pgid == pgid and state not in {"Z", "X"}:
            return True
    return False


def signal_group(pgid: int, signum: int) -> None:
    try:
        os.killpg(pgid, signum)
    except ProcessLookupError:
        pass


def stop_process_group(process: subprocess.Popen[bytes], config: RunConfig) -> bool:
    """Stop the group whose PGID equals process.pid and reap its direct child.

    The process must be launched with start_new_session=True for this PGID invariant.
    """
    cleanup_deadline = time.monotonic() + config.cleanup_timeout_seconds
    signal_group(process.pid, signal.SIGTERM)
    term_deadline = min(cleanup_deadline, time.monotonic() + config.term_grace_seconds)
    while group_has_live_members(process.pid) and time.monotonic() < term_deadline:
        process.poll()
        time.sleep(min(config.poll_interval_seconds, max(0, term_deadline - time.monotonic())))

    if group_has_live_members(process.pid):
        signal_group(process.pid, signal.SIGKILL)

    remaining = max(0, cleanup_deadline - time.monotonic())
    try:
        process.wait(timeout=remaining)
    except subprocess.TimeoutExpired:
        signal_group(process.pid, signal.SIGKILL)
        remaining = max(0, cleanup_deadline - time.monotonic())
        if remaining:
            try:
                process.wait(timeout=remaining)
            except subprocess.TimeoutExpired:
                return False
        else:
            return False

    while group_has_live_members(process.pid) and time.monotonic() < cleanup_deadline:
        signal_group(process.pid, signal.SIGKILL)
        time.sleep(min(config.poll_interval_seconds, max(0, cleanup_deadline - time.monotonic())))
    return process.poll() is not None and not group_has_live_members(process.pid)


def clear_attempt_outputs(cwd: Path, pr_number: int) -> None:
    for relative in (
        RECEIPT_PATH,
        DRAFT_PATH,
        OUTPUT_DIR / f"pr-{pr_number}.json",
        OUTPUT_DIR / f"pr-{pr_number}.html",
    ):
        (cwd / relative).unlink(missing_ok=True)


def copy_diagnostic(source: Path, target: Path) -> None:
    try:
        mode = source.lstat().st_mode
    except FileNotFoundError:
        return
    if not stat.S_ISREG(mode):
        return
    shutil.copyfile(source, target)


def retain_outputs(cwd: Path, attempt_dir: Path, pr_number: int) -> None:
    for relative, name in (
        (DRAFT_PATH, "draft.json"),
        (RECEIPT_PATH, "render-complete.json"),
        (OUTPUT_DIR / f"pr-{pr_number}.json", f"pr-{pr_number}.json"),
        (OUTPUT_DIR / f"pr-{pr_number}.html", f"pr-{pr_number}.html"),
    ):
        copy_diagnostic(cwd / relative, attempt_dir / name)


def write_json_atomic(path: Path, value: dict[str, object]) -> None:
    path.parent.mkdir(parents=True, exist_ok=True)
    temp_path = path.with_name(f".{path.name}.tmp")
    temp_path.write_text(json.dumps(value, indent=2) + "\n", encoding="utf-8")
    os.replace(temp_path, path)


def write_outcome(
    attempt_dir: Path,
    *,
    model: str,
    model_source: str,
    started_at: str,
    started_mono: float,
    raw_exit_status: int | None,
    stop_reason: str,
    verification_result: str,
    provider_error: str | None,
) -> None:
    outcome = {
        "model": model,
        "model_source": model_source,
        "started_at": started_at,
        "finished_at": utc_now(),
        "elapsed_seconds": round(max(0, time.monotonic() - started_mono), 3),
        "raw_exit_status": raw_exit_status,
        "stop_reason": stop_reason,
        "verification_result": verification_result,
        "provider_error": provider_error,
    }
    write_json_atomic(attempt_dir / "outcome.json", outcome)


def write_discovery_record(
    cwd: Path,
    discovery: ModelDiscovery,
    env: dict[str, str],
    elapsed_seconds: float = 0.0,
) -> None:
    record = {
        "status": discovery.status,
        "elapsed_seconds": round(max(0, elapsed_seconds), 3),
        "model_count": len(discovery.models),
        "eligible_free_models": [candidate.model_id for candidate in discovery.candidates],
        "go_fallback": {
            "model": GO_FALLBACK_MODEL,
            "credential_available": bool(env.get("OPENCODE_API_KEY", "").strip()),
        },
        "error": discovery.error,
    }
    try:
        write_json_atomic(cwd / DISCOVERY_RECORD_PATH, record)
    except OSError as exc:
        print(f"could not write walkthrough model discovery diagnostics: {exc}", file=sys.stderr)


def run_verifier(
    cwd: Path,
    env: dict[str, str],
    attempt_dir: Path,
    config: RunConfig,
) -> str:
    verifier = cwd / VERIFIER_PATH
    if not verifier.is_file() or verifier.is_symlink():
        result = "failed: trusted verifier is missing or not a regular file"
        (attempt_dir / "verification.stderr").write_text(result + "\n", encoding="utf-8")
        return result
    try:
        completed = subprocess.run(
            [sys.executable, str(verifier)],
            cwd=cwd,
            env=env,
            stdin=subprocess.DEVNULL,
            capture_output=True,
            timeout=config.verifier_timeout_seconds,
            check=False,
        )
    except subprocess.TimeoutExpired as exc:
        stdout = exc.stdout or b""
        stderr = exc.stderr or b""
        (attempt_dir / "verification.stdout").write_bytes(stdout)
        (attempt_dir / "verification.stderr").write_bytes(stderr)
        return f"failed: verifier exceeded {config.verifier_timeout_seconds:g} seconds"
    except OSError as exc:
        message = f"failed: verifier could not start: {exc}"
        (attempt_dir / "verification.stderr").write_text(message + "\n", encoding="utf-8")
        return message

    (attempt_dir / "verification.stdout").write_bytes(completed.stdout)
    (attempt_dir / "verification.stderr").write_bytes(completed.stderr)
    if completed.returncode == 0:
        return "passed"
    detail = completed.stderr.decode("utf-8", errors="replace").strip()
    if not detail:
        detail = f"verifier exited with status {completed.returncode}"
    return f"failed: {detail}"


def wait_or_cancel(cancellation: threading.Event | None, timeout: float) -> None:
    if cancellation is None:
        time.sleep(timeout)
    else:
        cancellation.wait(timeout)


def is_zero_price(value: object) -> bool:
    return isinstance(value, (int, float)) and not isinstance(value, bool) and value == 0


def is_free_model(data: dict[str, object]) -> bool:
    cost = data.get("cost")
    if not isinstance(cost, dict):
        return False
    if not is_zero_price(cost.get("input")) or not is_zero_price(cost.get("output")):
        return False
    cache = cost.get("cache")
    if cache is not None:
        if not isinstance(cache, dict):
            return False
        for key in ("read", "write"):
            if key in cache and not is_zero_price(cache[key]):
                return False
    capabilities = data.get("capabilities")
    if not isinstance(capabilities, dict):
        return False
    if capabilities.get("toolcall") is not True:
        return False
    for direction in ("input", "output"):
        channel = capabilities.get(direction)
        if not isinstance(channel, dict) or channel.get("text") is not True:
            return False
    return True


def parse_model_metadata(model_id: str, block: str) -> ModelMetadata | None:
    try:
        data = json.loads(block)
    except json.JSONDecodeError:
        return None
    if not isinstance(data, dict):
        return None
    provider_id, model_name = model_id.split("/", 1)
    if data.get("providerID") != provider_id or data.get("id") != model_name:
        return None
    variants = data.get("variants")
    variant_names = (
        frozenset(name for name in variants if isinstance(name, str))
        if isinstance(variants, dict)
        else frozenset()
    )
    return ModelMetadata(model_id, variant_names, is_free_model(data))


def parse_model_catalog(text: str) -> tuple[ModelMetadata, ...]:
    blocks: list[tuple[str, list[str]]] = []
    current_id: str | None = None
    current_block: list[str] = []
    for line in text.splitlines():
        if not line.strip() or line[0].isspace() or line.startswith(("{", "}", "[", "]")):
            if current_id is not None:
                current_block.append(line)
            continue
        if current_id is not None:
            blocks.append((current_id, current_block))
        stripped = line.strip()
        if MODEL_ID_RE.match(stripped):
            current_id = stripped
            current_block = []
        else:
            current_id = None
            current_block = []
    if current_id is not None:
        blocks.append((current_id, current_block))

    if not blocks:
        if text.strip():
            raise ValueError("model catalog output is malformed")
        return ()

    seen: dict[str, ModelMetadata] = {}
    conflicts: set[str] = set()
    for model_id, block in blocks:
        metadata = parse_model_metadata(model_id, "\n".join(block))
        if metadata is None:
            continue
        prior = seen.get(model_id)
        if prior is None:
            seen[model_id] = metadata
        elif prior != metadata:
            conflicts.add(model_id)
    models = [
        metadata for model_id, metadata in seen.items() if model_id not in conflicts
    ]
    if not models:
        raise ValueError("model catalog contains no valid model metadata")
    return tuple(models)


def discover_models(
    cli_command: list[str],
    *,
    cwd: Path,
    env: dict[str, str],
    deadline: float,
    config: RunConfig = RunConfig(),
    cancellation: threading.Event | None = None,
) -> ModelDiscovery:
    if not cli_command:
        return ModelDiscovery("failed", error="OpenCode CLI command is empty")
    discovery_dir = cwd / DISCOVERY_DIR
    try:
        discovery_dir.mkdir(parents=True, exist_ok=True)
        stdout_path = discovery_dir / "models.stdout"
        stderr_path = discovery_dir / "models.stderr"
    except OSError as exc:
        return ModelDiscovery("failed", error=f"model discovery directory is unavailable: {exc}")

    discovery_deadline = min(
        deadline,
        time.monotonic() + max(0, config.discovery_timeout_seconds),
    )
    command = [*cli_command, "models", "--verbose", "--refresh"]
    process: subprocess.Popen[bytes] | None = None
    try:
        with stdout_path.open("wb") as stdout_stream, stderr_path.open("wb") as stderr_stream:
            process = subprocess.Popen(
                command,
                cwd=cwd,
                env=env,
                stdin=subprocess.DEVNULL,
                stdout=stdout_stream,
                stderr=stderr_stream,
                start_new_session=True,
            )
            while True:
                if cancellation is not None and cancellation.is_set():
                    if not stop_process_group(process, config):
                        return ModelDiscovery(
                            "failed", error="model discovery process cleanup failed"
                        )
                    return ModelDiscovery("cancelled", error="cancelled during model discovery")
                if time.monotonic() >= discovery_deadline:
                    if not stop_process_group(process, config):
                        return ModelDiscovery(
                            "failed", error="model discovery process cleanup failed"
                        )
                    return ModelDiscovery("deadline", error="model discovery exceeded its time limit")
                if process.poll() is not None:
                    break
                wait_or_cancel(
                    cancellation,
                    min(
                        config.poll_interval_seconds,
                        max(0, discovery_deadline - time.monotonic()),
                    ),
                )
    except OSError as exc:
        return ModelDiscovery("failed", error=f"model discovery could not start: {exc}")

    if not stop_process_group(process, config):
        return ModelDiscovery("failed", error="model discovery process cleanup failed")
    if process.returncode != 0:
        return ModelDiscovery(
            "failed",
            error=f"model discovery exited with status {process.returncode}",
        )
    try:
        output = stdout_path.read_text(encoding="utf-8", errors="replace")
    except OSError as exc:
        return ModelDiscovery("failed", error=f"model catalog could not be read: {exc}")
    try:
        models = parse_model_catalog(output)
    except ValueError as exc:
        return ModelDiscovery("failed", error=str(exc))
    return ModelDiscovery("succeeded", models=models)


def build_candidates(discovery: ModelDiscovery, env: dict[str, str]) -> tuple[Candidate, ...]:
    candidates = list(discovery.candidates)
    if not env.get("OPENCODE_API_KEY", "").strip():
        return tuple(candidates)
    if any(candidate.model_id == GO_FALLBACK_MODEL for candidate in candidates):
        return tuple(candidates)
    metadata = next(
        (model for model in discovery.models if model.model_id == GO_FALLBACK_MODEL),
        None,
    )
    variants = metadata.variants if metadata is not None else frozenset()
    candidates.append(Candidate(GO_FALLBACK_MODEL, variants, "go"))
    return tuple(candidates)


def render_agent_command(command: list[str], candidate: Candidate) -> list[str]:
    supports_variant = PREFERRED_VARIANT in candidate.variants
    rendered: list[str] = []
    index = 0
    after_separator = False
    while index < len(command):
        argument = command[index]
        if argument == "--" and not after_separator:
            after_separator = True
            rendered.append(argument)
            index += 1
            continue
        if not after_separator and argument == "--variant":
            if index + 1 >= len(command):
                raise ValueError("--variant requires a value")
            value = command[index + 1]
            if supports_variant:
                rendered.extend(
                    ["--variant", PREFERRED_VARIANT if value == VARIANT_PLACEHOLDER else value]
                )
            index += 2
            continue
        if not after_separator and argument.startswith("--variant="):
            value = argument.split("=", 1)[1]
            if supports_variant:
                rendered.append(
                    "--variant=" + (PREFERRED_VARIANT if value == VARIANT_PLACEHOLDER else value)
                )
            index += 1
            continue
        if after_separator:
            rendered.append(argument)
        else:
            rendered.append(argument.replace(MODEL_PLACEHOLDER, candidate.model_id))
        index += 1
    return rendered


def log_field(line: str, name: str) -> str | None:
    for match in LOG_FIELD_RE.finditer(line):
        if match.group(1) != name:
            continue
        quoted, unquoted = match.group(2), match.group(3)
        return quoted if quoted is not None else unquoted
    return None


def classify_provider_error(line: str, candidate: Candidate) -> str | None:
    """Return a recoverable model failure category for an attributable log line."""
    is_log_error = "level=ERROR" in line
    stripped = line.lstrip()
    is_raw_error = stripped.startswith(
        ("AI_APICallError:", "ProviderModelNotFoundError:")
    )
    if not is_log_error and not is_raw_error:
        return None
    if is_log_error and not any(marker in line for marker in PROVIDER_ERROR_MARKERS):
        return None
    if is_log_error and log_field(line, "message") not in {"stream error", "failed"}:
        return None

    message = log_field(line, "message")
    if message is not None and "failed to generate title" in message.lower():
        return None
    if log_field(line, "small") == "true" or log_field(line, "agent") == "title":
        return None

    model = log_field(line, "modelID")
    provider = log_field(line, "providerID")
    if model is not None:
        if model not in {candidate.model_name, candidate.model_id}:
            return None
        if provider is not None and provider != candidate.provider_id:
            return None
    elif candidate.model_id not in line:
        return None

    if any(pattern.search(line) for pattern in QUOTA_ERROR_PATTERNS):
        return "model_quota"
    if any(pattern.search(line) for pattern in RATE_LIMIT_ERROR_PATTERNS):
        return "model_rate_limit"
    if any(pattern.search(line) for pattern in UNAVAILABLE_ERROR_PATTERNS):
        return "model_unavailable"
    return None


class ProviderErrorWatcher:
    def __init__(self, path: Path, candidate: Candidate) -> None:
        self.path = path
        self.candidate = candidate
        self.offset = 0
        self.buffer = ""

    def poll(self, *, final: bool = False) -> str | None:
        try:
            with self.path.open("rb") as stream:
                stream.seek(self.offset)
                chunk = stream.read()
        except OSError:
            return None
        if chunk:
            self.offset += len(chunk)
        text = self.buffer + chunk.decode("utf-8", errors="replace")
        if final:
            lines = text.split("\n")
            self.buffer = ""
        else:
            if not chunk:
                return None
            lines = text.split("\n")
            self.buffer = lines.pop()
        for line in lines:
            if not line:
                continue
            kind = classify_provider_error(line, self.candidate)
            if kind is not None:
                return kind
        return None


def supervise_process(
    process: subprocess.Popen[bytes],
    *,
    candidate: Candidate,
    cwd: Path,
    deadline: float,
    cancel_event: threading.Event,
    config: RunConfig,
    stderr_path: Path,
) -> tuple[str, int | None, str | None, bool]:
    watcher = ProviderErrorWatcher(stderr_path, candidate)
    stop_reason = "launch_failure"
    raw_exit_status: int | None = None
    provider_error: str | None = None
    while True:
        if cancel_event.is_set():
            stop_reason = "cancellation"
            break
        if time.monotonic() >= deadline:
            stop_reason = "deadline"
            break

        failure = watcher.poll()
        status = process.poll()
        if status is not None:
            failure = watcher.poll(final=True) or failure
        has_receipt = os.path.lexists(cwd / RECEIPT_PATH)
        if status is not None:
            raw_exit_status = status
            if status == 0 and has_receipt:
                stop_reason = "render_complete"
            elif failure is not None:
                stop_reason = failure
                provider_error = failure
            elif status != 0:
                stop_reason = "unexpected_exit"
            else:
                stop_reason = "incomplete_zero_exit"
            break
        if has_receipt:
            stop_reason = "render_complete"
            break
        if failure is not None:
            stop_reason = failure
            provider_error = failure
            break
        cancel_event.wait(
            min(config.poll_interval_seconds, max(0, deadline - time.monotonic()))
        )

    if cancel_event.is_set() and stop_reason != "render_complete":
        stop_reason = "cancellation"
        provider_error = None

    cleanup_ok = stop_process_group(process, config)
    if process.poll() is not None:
        raw_exit_status = process.returncode
    return stop_reason, raw_exit_status, provider_error, not cleanup_ok


def run_agent(
    command: list[str],
    *,
    cwd: Path,
    env: dict[str, str],
    config: RunConfig = RunConfig(),
    cancellation: threading.Event | None = None,
    models_command: list[str] | None = None,
    discovery: ModelDiscovery | None = None,
) -> int:
    if not command or any(not isinstance(argument, str) for argument in command):
        print("agent command is empty or invalid", file=sys.stderr)
        return 1
    try:
        pr_number = positive_pr_number(env)
    except ValueError as exc:
        print(f"walkthrough runner failed: {exc}", file=sys.stderr)
        return 1
    options = command[: command.index("--")] if "--" in command else command
    model_values: list[str] = []
    for index, argument in enumerate(options):
        if argument == "--model":
            if index + 1 >= len(options):
                model_values.append("")
            else:
                model_values.append(options[index + 1])
        elif argument.startswith("--model="):
            model_values.append(argument.split("=", 1)[1])
    if len(model_values) != 1 or MODEL_PLACEHOLDER not in model_values[0]:
        print("agent command must pass exactly one --model {model} placeholder", file=sys.stderr)
        return 1

    cancel_event = cancellation or threading.Event()
    start_mono = time.monotonic()
    deadline = start_mono + config.deadline_seconds

    if discovery is None:
        cli_command = models_command or [command[0]]
        discovery_started = time.monotonic()
        discovery = discover_models(
            cli_command,
            cwd=cwd,
            env=env,
            deadline=deadline,
            config=config,
            cancellation=cancel_event,
        )
        discovery_elapsed = time.monotonic() - discovery_started
    else:
        discovery_elapsed = 0.0
    write_discovery_record(cwd, discovery, env, discovery_elapsed)

    if discovery.status == "cancelled":
        print("walkthrough generation cancelled during model discovery", file=sys.stderr)
        return 1
    if discovery.status == "deadline":
        print("walkthrough model discovery exceeded the generation deadline", file=sys.stderr)
        return 1
    if discovery.status != "succeeded":
        print(
            f"walkthrough model discovery failed: {discovery.error or 'unknown error'}",
            file=sys.stderr,
        )
        return 1

    candidates = build_candidates(discovery, env)
    if not candidates:
        print(
            "walkthrough generation found no eligible free model and no Go credential",
            file=sys.stderr,
        )
        return 1

    attempt_number = 0
    incomplete_retries = 0
    for candidate in candidates:
        while True:
            if cancel_event.is_set():
                print("walkthrough generation cancelled before an attempt", file=sys.stderr)
                return 1
            if time.monotonic() >= deadline:
                print("walkthrough generation deadline reached before an attempt", file=sys.stderr)
                return 1

            attempt_number += 1
            attempt_dir = cwd / WORK_DIR / f"attempt-{attempt_number}"
            if attempt_dir.exists():
                shutil.rmtree(attempt_dir)
            attempt_dir.mkdir(parents=True, exist_ok=True)
            try:
                clear_attempt_outputs(cwd, pr_number)
                (cwd / DRAFT_PATH).write_text("{}\n", encoding="utf-8")
            except OSError as exc:
                print(f"walkthrough runner could not clear attempt outputs: {exc}", file=sys.stderr)
                return 1
            try:
                rendered_command = render_agent_command(command, candidate)
            except ValueError as exc:
                print(f"walkthrough runner has an invalid agent command: {exc}", file=sys.stderr)
                return 1

            started_at = utc_now()
            started_mono = time.monotonic()
            stop_reason = "launch_failure"
            verification_result = "not_run"
            raw_exit_status: int | None = None
            provider_error: str | None = None
            stdout_path = attempt_dir / "stdout"
            stderr_path = attempt_dir / "stderr"

            try:
                with stdout_path.open("wb") as stdout_stream, stderr_path.open("wb") as stderr_stream:
                    process = subprocess.Popen(
                        rendered_command,
                        cwd=cwd,
                        env=env,
                        stdin=subprocess.DEVNULL,
                        stdout=stdout_stream,
                        stderr=stderr_stream,
                        start_new_session=True,
                    )
                    try:
                        (
                            stop_reason,
                            raw_exit_status,
                            provider_error,
                            cleanup_failed,
                        ) = supervise_process(
                            process,
                            candidate=candidate,
                            cwd=cwd,
                            deadline=deadline,
                            cancel_event=cancel_event,
                            config=config,
                            stderr_path=stderr_path,
                        )
                    except BaseException:
                        stop_process_group(process, config)
                        raise
                    if cleanup_failed:
                        verification_result = "cleanup failed"
                        if stop_reason in {
                            "render_complete",
                            "incomplete_zero_exit",
                            "model_quota",
                            "model_rate_limit",
                            "model_unavailable",
                        }:
                            stop_reason = "cleanup_failure"
            except OSError as exc:
                (attempt_dir / "stderr").write_text(
                    f"unable to start agent: {exc}\n", encoding="utf-8"
                )
                stop_reason = "launch_failure"

            retain_outputs(cwd, attempt_dir, pr_number)
            if stop_reason == "render_complete" and verification_result != "cleanup failed":
                if cancel_event.is_set():
                    stop_reason = "cancellation"
                    verification_result = "cancelled before verification"
                else:
                    verification_result = run_verifier(cwd, env, attempt_dir, config)
                    if cancel_event.is_set():
                        stop_reason = "cancellation"
                        verification_result = "cancelled during verification"
            write_outcome(
                attempt_dir,
                model=candidate.model_id,
                model_source=candidate.source,
                started_at=started_at,
                started_mono=started_mono,
                raw_exit_status=raw_exit_status,
                stop_reason=stop_reason,
                verification_result=verification_result,
                provider_error=provider_error,
            )

            if stop_reason == "render_complete":
                if verification_result == "passed":
                    print(
                        f"walkthrough outputs verified after supervised render completion "
                        f"with {candidate.model_id}"
                    )
                    return 0
                print(
                    f"walkthrough output verification failed with {candidate.model_id}: "
                    f"{verification_result}",
                    file=sys.stderr,
                )
                return 1
            if stop_reason in {"model_quota", "model_rate_limit", "model_unavailable"}:
                print(
                    f"walkthrough model {candidate.model_id} failed with {stop_reason}; "
                    "trying the next model",
                    file=sys.stderr,
                )
                break
            if stop_reason != "incomplete_zero_exit":
                print(
                    f"walkthrough generation stopped with {candidate.model_id}: {stop_reason}",
                    file=sys.stderr,
                )
                return 1
            if (
                incomplete_retries >= config.max_incomplete_retries
                or time.monotonic() >= deadline
            ):
                print(
                    "agent exited successfully without verified output; retry limit reached",
                    file=sys.stderr,
                )
                return 1
            incomplete_retries += 1

    print("walkthrough generation exhausted every eligible model", file=sys.stderr)
    return 1


def attempt_sort_key(path: Path) -> int:
    suffix = path.parent.name.rsplit("-", 1)[-1]
    return int(suffix) if suffix.isdigit() else 0


def write_step_summary(cwd: Path, env: dict[str, str], result: int) -> None:
    summary_name = env.get("GITHUB_STEP_SUMMARY")
    if not summary_name:
        return
    outcomes = []
    outcome_paths = list((cwd / WORK_DIR).glob("attempt-*/outcome.json"))
    outcome_paths.sort(key=attempt_sort_key)
    for path in outcome_paths:
        try:
            outcome = json.loads(path.read_text(encoding="utf-8"))
        except (OSError, json.JSONDecodeError):
            continue
        if isinstance(outcome, dict):
            outcomes.append(outcome)

    elapsed = sum(
        value for item in outcomes
        if isinstance((value := item.get("elapsed_seconds")), (int, float))
    )
    lines = [
        "## PR walkthrough generation",
        "",
        f"Result: {'verified' if result == 0 else 'failed'}",
        f"Generation time: {elapsed:.2f}s",
    ]

    discovery_path = cwd / DISCOVERY_RECORD_PATH
    try:
        discovery = json.loads(discovery_path.read_text(encoding="utf-8"))
    except (OSError, json.JSONDecodeError):
        discovery = None
    if isinstance(discovery, dict):
        status = discovery.get("status")
        discovery_elapsed = discovery.get("elapsed_seconds")
        if isinstance(discovery_elapsed, (int, float)) and not isinstance(
            discovery_elapsed, bool
        ):
            lines.append(f"Discovery time: {discovery_elapsed:.2f}s")
        if status == "succeeded":
            eligible = discovery.get("eligible_free_models")
            count = len(eligible) if isinstance(eligible, list) else 0
            lines.append(f"Model discovery: {count} eligible free model(s)")
            go_fallback = discovery.get("go_fallback")
            if isinstance(go_fallback, dict):
                credential = go_fallback.get("credential_available")
                if credential is True:
                    lines.append("Go fallback: available")
                elif credential is False:
                    lines.append("Go fallback: skipped (OpenCode API key is not configured)")
        elif isinstance(status, str) and status:
            lines.append(f"Model discovery: {status}")

    for index, outcome in enumerate(outcomes, start=1):
        stop_reason = outcome.get("stop_reason")
        if stop_reason not in {
            "render_complete", "incomplete_zero_exit", "unexpected_exit",
            "deadline", "cancellation", "launch_failure", "cleanup_failure",
            "model_quota", "model_rate_limit", "model_unavailable",
        }:
            stop_reason = "unknown"
        model = outcome.get("model")
        model_text = model if isinstance(model, str) and model else "unknown model"
        raw_exit_status = outcome.get("raw_exit_status")
        exit_text = str(raw_exit_status) if isinstance(raw_exit_status, int) else "none"
        verification = outcome.get("verification_result")
        if verification == "passed":
            verification_text = "passed"
        elif verification == "cleanup failed":
            verification_text = "cleanup failed"
        elif isinstance(verification, str) and verification.startswith("failed:"):
            verification_text = "failed"
        elif isinstance(verification, str) and verification.startswith("cancelled"):
            verification_text = "cancelled"
        else:
            verification_text = "not run"
        attempt_elapsed = outcome.get("elapsed_seconds")
        duration = attempt_elapsed if isinstance(attempt_elapsed, (int, float)) else 0
        lines.append(
            f"Attempt {index}: {model_text}, {stop_reason}, {duration:.2f}s, exit {exit_text}, "
            f"verification {verification_text}"
        )
    if result == 0 and outcomes:
        selected = outcomes[-1].get("model")
        if isinstance(selected, str) and selected:
            lines.append(f"Selected model: {selected}")
    try:
        with Path(summary_name).open("a", encoding="utf-8") as summary:
            summary.write("\n".join(lines) + "\n")
    except OSError as exc:
        print(f"could not write walkthrough workflow summary: {exc}", file=sys.stderr)


def main(argv: list[str] | None = None) -> int:
    args = sys.argv[1:] if argv is None else argv
    if len(args) < 2 or args[0] != "--":
        print("usage: pr-walkthrough-runner.py -- <agent command>", file=sys.stderr)
        return 2
    cancellation = threading.Event()
    previous_handlers: dict[int, object] = {}

    def request_cancellation(_signum: int, _frame: object) -> None:
        cancellation.set()

    for signum in (signal.SIGTERM, signal.SIGINT):
        previous_handlers[signum] = signal.signal(signum, request_cancellation)
    try:
        env = dict(os.environ)
        cwd = Path.cwd()
        result = run_agent(args[1:], cwd=cwd, env=env, cancellation=cancellation)
        write_step_summary(cwd, env, result)
        return result
    finally:
        for signum, handler in previous_handlers.items():
            signal.signal(signum, handler)


if __name__ == "__main__":
    raise SystemExit(main())
