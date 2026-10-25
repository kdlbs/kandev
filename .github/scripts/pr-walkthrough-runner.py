#!/usr/bin/env python3
"""Supervise PR walkthrough generation and verify completed output."""

from dataclasses import dataclass
from datetime import datetime, timezone
import json
import os
from pathlib import Path
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
MAX_ATTEMPTS = 2
WORK_DIR = Path(".pr-walkthrough")
RECEIPT_PATH = WORK_DIR / "render-complete.json"
DRAFT_PATH = WORK_DIR / "draft.json"
OUTPUT_DIR = Path("docs/pr-walkthrough")
VERIFIER_PATH = Path(".agents/skills/pr-walkthrough/scripts/pr-walkthrough-verify")
PROC_ROOT = Path("/proc")


@dataclass(frozen=True)
class RunConfig:
    deadline_seconds: float = TOTAL_DEADLINE_SECONDS
    poll_interval_seconds: float = POLL_INTERVAL_SECONDS
    term_grace_seconds: float = TERM_GRACE_SECONDS
    cleanup_timeout_seconds: float = CLEANUP_TIMEOUT_SECONDS
    verifier_timeout_seconds: float = VERIFIER_TIMEOUT_SECONDS
    max_attempts: int = MAX_ATTEMPTS


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


def write_outcome(
    attempt_dir: Path,
    *,
    started_at: str,
    started_mono: float,
    raw_exit_status: int | None,
    stop_reason: str,
    verification_result: str,
) -> None:
    outcome = {
        "started_at": started_at,
        "finished_at": utc_now(),
        "elapsed_seconds": round(max(0, time.monotonic() - started_mono), 3),
        "raw_exit_status": raw_exit_status,
        "stop_reason": stop_reason,
        "verification_result": verification_result,
    }
    temp_path = attempt_dir / ".outcome.json.tmp"
    temp_path.write_text(json.dumps(outcome, indent=2) + "\n", encoding="utf-8")
    os.replace(temp_path, attempt_dir / "outcome.json")


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


def run_agent(
    command: list[str],
    *,
    cwd: Path,
    env: dict[str, str],
    config: RunConfig = RunConfig(),
    cancellation: threading.Event | None = None,
) -> int:
    if not command or any(not isinstance(argument, str) for argument in command):
        print("agent command is empty or invalid", file=sys.stderr)
        return 1
    try:
        pr_number = positive_pr_number(env)
    except ValueError as exc:
        print(f"walkthrough runner failed: {exc}", file=sys.stderr)
        return 1

    cancel_event = cancellation or threading.Event()
    start_mono = time.monotonic()
    deadline = start_mono + config.deadline_seconds
    for attempt in range(1, config.max_attempts + 1):
        if cancel_event.is_set():
            print("walkthrough generation cancelled before an attempt", file=sys.stderr)
            return 1

        attempt_dir = cwd / WORK_DIR / f"attempt-{attempt}"
        if attempt_dir.exists():
            shutil.rmtree(attempt_dir)
        attempt_dir.mkdir(parents=True, exist_ok=True)
        try:
            clear_attempt_outputs(cwd, pr_number)
            (cwd / DRAFT_PATH).write_text("{}\n", encoding="utf-8")
        except OSError as exc:
            print(f"walkthrough runner could not clear attempt outputs: {exc}", file=sys.stderr)
            return 1

        started_at = utc_now()
        started_mono = time.monotonic()
        stop_reason = "launch_failure"
        verification_result = "not_run"
        raw_exit_status: int | None = None
        stdout_path = attempt_dir / "stdout"
        stderr_path = attempt_dir / "stderr"

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
                    now = time.monotonic()
                    if cancel_event.is_set():
                        stop_reason = "cancellation"
                        if not stop_process_group(process, config):
                            verification_result = "cleanup failed"
                        break
                    if now >= deadline:
                        stop_reason = "deadline"
                        if not stop_process_group(process, config):
                            verification_result = "cleanup failed"
                        break
                    status = process.poll()
                    if status is not None:
                        raw_exit_status = status
                        has_receipt = os.path.lexists(cwd / RECEIPT_PATH)
                        if status == 0 and has_receipt:
                            stop_reason = "render_complete"
                        else:
                            stop_reason = "unexpected_exit" if status != 0 else "incomplete_zero_exit"
                        if not stop_process_group(process, config):
                            verification_result = "cleanup failed"
                            if stop_reason == "incomplete_zero_exit":
                                stop_reason = "cleanup_failure"
                        break
                    if os.path.lexists(cwd / RECEIPT_PATH):
                        stop_reason = "render_complete"
                        if not stop_process_group(process, config):
                            verification_result = "cleanup failed"
                            break
                        break
                    cancel_event.wait(
                        min(config.poll_interval_seconds, max(0, deadline - time.monotonic()))
                    )
                if process.poll() is not None:
                    raw_exit_status = process.returncode
        except OSError as exc:
            (attempt_dir / "stderr").write_text(f"unable to start agent: {exc}\n", encoding="utf-8")
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
            started_at=started_at,
            started_mono=started_mono,
            raw_exit_status=raw_exit_status,
            stop_reason=stop_reason,
            verification_result=verification_result,
        )

        if stop_reason == "render_complete":
            if verification_result == "passed":
                print("walkthrough outputs verified after supervised render completion")
                return 0
            print(f"walkthrough output verification failed: {verification_result}", file=sys.stderr)
            return 1
        if stop_reason != "incomplete_zero_exit":
            print(f"walkthrough generation stopped: {stop_reason}", file=sys.stderr)
            return 1
        if attempt >= config.max_attempts or time.monotonic() >= deadline:
            print("agent exited successfully without verified output; retry limit reached", file=sys.stderr)
            return 1

    return 1


def write_step_summary(cwd: Path, env: dict[str, str], result: int) -> None:
    summary_name = env.get("GITHUB_STEP_SUMMARY")
    if not summary_name:
        return
    outcomes = []
    for path in sorted((cwd / WORK_DIR).glob("attempt-*/outcome.json")):
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
    for index, outcome in enumerate(outcomes, start=1):
        stop_reason = outcome.get("stop_reason")
        if stop_reason not in {
            "render_complete", "incomplete_zero_exit", "unexpected_exit",
            "deadline", "cancellation", "launch_failure", "cleanup_failure",
        }:
            stop_reason = "unknown"
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
            f"Attempt {index}: {stop_reason}, {duration:.2f}s, exit {exit_text}, "
            f"verification {verification_text}"
        )
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
