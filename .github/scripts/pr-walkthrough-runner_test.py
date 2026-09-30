#!/usr/bin/env python3
"""Tests for bounded PR walkthrough process supervision."""

import importlib.util
import json
import os
import shutil
import signal
import subprocess
import sys
import tempfile
import threading
import time
import unittest
from unittest import mock
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
RUNNER = REPO_ROOT / ".github" / "scripts" / "pr-walkthrough-runner.py"
SKILL = REPO_ROOT / ".agents" / "skills" / "pr-walkthrough"
SPEC = importlib.util.spec_from_file_location("pr_walkthrough_runner", RUNNER)
assert SPEC is not None and SPEC.loader is not None
runner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner)


class WaitForAgentExit:
    def __init__(self, pid_path: Path) -> None:
        self.pid_path = pid_path

    def is_set(self) -> bool:
        return False

    def wait(self, timeout: float | None = None) -> bool:
        deadline = time.monotonic() + 2
        while time.monotonic() < deadline:
            if self.pid_path.exists():
                pid = int(self.pid_path.read_text(encoding="utf-8"))
                stat_path = Path(f"/proc/{pid}/stat")
                if not stat_path.exists():
                    return False
                state = stat_path.read_text(encoding="utf-8").split(") ", 1)[1][0]
                if state in {"Z", "X"}:
                    return False
            time.sleep(0.001)
        raise AssertionError("agent did not exit before the next runner poll")


class WaitForPath:
    def __init__(self, path: Path) -> None:
        self.path = path

    def is_set(self) -> bool:
        return False

    def wait(self, timeout: float | None = None) -> bool:
        deadline = time.monotonic() + 2
        while time.monotonic() < deadline:
            if self.path.exists():
                return False
            time.sleep(0.001)
        raise AssertionError(f"timed out waiting for {self.path}")


class PRWalkthroughRunnerTest(unittest.TestCase):
    def test_trusted_runner_entry_point_exists(self) -> None:
        self.assertTrue(RUNNER.is_file(), "trusted walkthrough runner is missing")

    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.worktree = Path(self.tmp.name)
        skill_copy = self.worktree / ".agents" / "skills" / "pr-walkthrough"
        shutil.copytree(SKILL, skill_copy)
        self.renderer = skill_copy / "scripts" / "pr-walkthrough-render"
        self.data = json.loads(
            (skill_copy / "references" / "example.json").read_text(encoding="utf-8")
        )
        manifest_paths = {
            item["file"]
            for category in self.data["impact"].values()
            for item in category.get("items", [])
        }
        manifest_paths.update(change["file"] for change in self.data["changes"])
        manifest_dir = self.worktree / ".pr-walkthrough" / "head-context"
        manifest_dir.mkdir(parents=True)
        (manifest_dir / "manifest.json").write_text(
            json.dumps({"files": [{"path": path} for path in sorted(manifest_paths)]}),
            encoding="utf-8",
        )
        self.env = {
            **os.environ,
            "PR_NUMBER": "42",
            "PR_TITLE": "Trusted pull request title",
            "PR_URL": "https://github.com/kdlbs/kandev/pull/42",
            "PR_REPO": "kdlbs/kandev",
            "PR_BASE": "main",
            "PR_HEAD": "feature/walkthrough",
            "HEAD_SHA": "a" * 40,
        }
        self.config = runner.RunConfig(
            deadline_seconds=1.2,
            poll_interval_seconds=0.01,
            term_grace_seconds=0.12,
            cleanup_timeout_seconds=0.4,
            verifier_timeout_seconds=1,
            max_attempts=2,
        )

    def write_agent(self, source: str) -> list[str]:
        path = self.worktree / "fake-agent.py"
        path.write_text(source, encoding="utf-8")
        return [sys.executable, str(path)]

    def render_agent_source(self, *, idle: bool = True, descendant: bool = False) -> str:
        serialized = repr(json.dumps(self.data))
        render_command = repr([sys.executable, str(self.renderer)])
        lines = [
            "import json, subprocess, sys, time",
            "from pathlib import Path",
            "print('agent starting', flush=True)",
            "Path('initial-draft.json').write_text(Path('.pr-walkthrough/draft.json').read_text(), encoding='utf-8')",
            f"Path('.pr-walkthrough/draft.json').write_text({serialized}, encoding='utf-8')",
        ]
        if descendant:
            lines.extend(
                [
                    "import signal",
                    "child_code = \"import signal,time; signal.signal(signal.SIGTERM, signal.SIG_IGN); "
                    "time.sleep(60)\"",
                    "child = subprocess.Popen([sys.executable, '-c', child_code])",
                    "Path('descendant.pid').write_text(str(child.pid), encoding='utf-8')",
                ]
            )
        lines.append(f"subprocess.run({render_command}, check=True)")
        lines.append("print('fake agent output', flush=True)")
        lines.append("Path('agent.ready').write_text('ready', encoding='utf-8')")
        if idle:
            lines.append("while True: time.sleep(0.01)")
        return "\n".join(lines) + "\n"

    def run_agent(self, command: list[str], *, cancellation: threading.Event | None = None) -> int:
        return runner.run_agent(
            command,
            cwd=self.worktree,
            env=self.env,
            config=self.config,
            cancellation=cancellation,
        )

    def outcome(self, attempt: int = 1) -> dict[str, object]:
        path = self.worktree / ".pr-walkthrough" / f"attempt-{attempt}" / "outcome.json"
        return json.loads(path.read_text(encoding="utf-8"))

    def attempt_dirs(self) -> list[Path]:
        return sorted((self.worktree / ".pr-walkthrough").glob("attempt-*"))

    def test_render_then_idle_completes_and_stops_owned_group(self) -> None:
        command = self.write_agent(self.render_agent_source(descendant=True))

        result = self.run_agent(command)

        self.assertEqual(result, 0)
        record = self.outcome()
        self.assertEqual(record["stop_reason"], "render_complete")
        self.assertEqual(record["verification_result"], "passed")
        self.assertEqual(record["raw_exit_status"], -signal.SIGTERM)
        self.assertTrue(record["started_at"])
        self.assertTrue(record["finished_at"])
        self.assertGreaterEqual(record["elapsed_seconds"], 0)
        self.assertIn("agent starting", (self.attempt_dirs()[0] / "stdout").read_text())
        self.assertEqual((self.worktree / "initial-draft.json").read_text(), "{}\n")
        pid = int((self.worktree / "descendant.pid").read_text())
        self.assert_process_stopped(pid)

    def test_receipt_is_accepted_before_renderer_process_exits(self) -> None:
        slow_renderer = self.worktree / "slow-renderer.py"
        slow_renderer.write_text(
            "import runpy, time\n"
            f"renderer = runpy.run_path({str(self.renderer)!r}, run_name='managed_renderer')\n"
            "if renderer['main']() != 0: raise SystemExit(1)\n"
            "from pathlib import Path\n"
            "Path('renderer.ready').touch()\n"
            "while True: time.sleep(0.01)\n",
            encoding="utf-8",
        )
        source = self.render_agent_source().replace(
            repr([sys.executable, str(self.renderer)]),
            repr([sys.executable, str(slow_renderer)]),
        )
        command = self.write_agent(source)

        result = self.run_agent(
            command,
            cancellation=WaitForPath(self.worktree / "renderer.ready"),
        )

        self.assertEqual(result, 0)
        self.assertTrue((self.worktree / "renderer.ready").is_file())
        self.assertEqual(self.outcome()["stop_reason"], "render_complete")
        self.assertEqual(self.outcome()["verification_result"], "passed")

    def test_zombie_only_process_group_is_not_live(self) -> None:
        proc_root = self.worktree / "proc"
        zombie_stat = proc_root / "1201" / "stat"
        zombie_stat.parent.mkdir(parents=True)
        zombie_stat.write_text("1201 (renderer ) child) Z 1 4242 4242 0\n", encoding="utf-8")

        with mock.patch.object(runner, "PROC_ROOT", proc_root, create=True):
            self.assertFalse(runner.group_has_live_members(4242))
            live_stat = proc_root / "1202" / "stat"
            live_stat.parent.mkdir()
            live_stat.write_text("1202 (agent child) S 1 4242 4242 0\n", encoding="utf-8")
            self.assertTrue(runner.group_has_live_members(4242))

    def test_unexpected_nonzero_exit_does_not_retry(self) -> None:
        command = self.write_agent("raise SystemExit(7)\n")

        result = self.run_agent(command)

        self.assertNotEqual(result, 0)
        self.assertEqual(len(self.attempt_dirs()), 1)
        record = self.outcome()
        self.assertEqual(record["stop_reason"], "unexpected_exit")
        self.assertEqual(record["raw_exit_status"], 7)
        self.assertEqual(record["verification_result"], "not_run")

    def test_completed_receipt_does_not_mask_observed_nonzero_exit(self) -> None:
        source = self.render_agent_source(idle=False)
        source += "Path('agent.pid').write_text(str(__import__('os').getpid()))\n"
        source += "raise SystemExit(7)\n"
        command = self.write_agent(source)
        result = self.run_agent(
            command,
            cancellation=WaitForAgentExit(self.worktree / "agent.pid"),
        )

        self.assertNotEqual(result, 0)
        self.assertEqual(len(self.attempt_dirs()), 1)
        record = self.outcome()
        self.assertEqual(record["stop_reason"], "unexpected_exit")
        self.assertEqual(record["raw_exit_status"], 7)
        self.assertEqual(record["verification_result"], "not_run")
        self.assertTrue((self.attempt_dirs()[0] / "render-complete.json").is_file())

    def test_natural_zero_exit_with_receipt_is_verified(self) -> None:
        source = self.render_agent_source(idle=False)
        source += "Path('agent.pid').write_text(str(__import__('os').getpid()))\n"
        source += "raise SystemExit(0)\n"
        command = self.write_agent(source)

        result = self.run_agent(
            command,
            cancellation=WaitForAgentExit(self.worktree / "agent.pid"),
        )

        self.assertEqual(result, 0)
        self.assertEqual(len(self.attempt_dirs()), 1)
        record = self.outcome()
        self.assertEqual(record["stop_reason"], "render_complete")
        self.assertEqual(record["raw_exit_status"], 0)
        self.assertEqual(record["verification_result"], "passed")

    def test_incomplete_zero_exit_retries_once_then_completes(self) -> None:
        serialized = repr(json.dumps(self.data))
        render_command = repr([sys.executable, str(self.renderer)])
        source = f"""import subprocess,sys,time
from pathlib import Path
counter = Path('invocations')
if not counter.exists():
    counter.write_text('1')
    raise SystemExit(0)
Path('.pr-walkthrough/draft.json').write_text({serialized}, encoding='utf-8')
subprocess.run({render_command}, check=True)
while True: time.sleep(0.01)
"""
        command = self.write_agent(source)

        result = self.run_agent(command)

        self.assertEqual(result, 0)
        self.assertEqual(len(self.attempt_dirs()), 2)
        self.assertEqual(self.outcome(1)["stop_reason"], "incomplete_zero_exit")
        self.assertEqual(self.outcome(2)["verification_result"], "passed")

    def test_cleanup_failure_after_zero_exit_is_terminal(self) -> None:
        source = """from pathlib import Path
counter = Path('invocations')
count = int(counter.read_text()) if counter.exists() else 0
counter.write_text(str(count + 1))
raise SystemExit(0)
"""
        command = self.write_agent(source)

        with mock.patch.object(runner, "stop_process_group", return_value=False):
            with mock.patch.object(runner, "run_verifier") as verifier:
                result = self.run_agent(command)

        self.assertNotEqual(result, 0)
        self.assertEqual((self.worktree / "invocations").read_text(encoding="utf-8"), "1")
        self.assertEqual(len(self.attempt_dirs()), 1)
        self.assertEqual(self.outcome()["stop_reason"], "cleanup_failure")
        self.assertEqual(self.outcome()["verification_result"], "cleanup failed")
        verifier.assert_not_called()

    def test_second_incomplete_zero_exit_fails_without_a_third_attempt(self) -> None:
        command = self.write_agent("raise SystemExit(0)\n")

        result = self.run_agent(command)

        self.assertNotEqual(result, 0)
        self.assertEqual(len(self.attempt_dirs()), 2)
        self.assertEqual(self.outcome(2)["stop_reason"], "incomplete_zero_exit")

    def test_both_attempts_share_one_deadline(self) -> None:
        marker = repr("invocations")
        source = f"""from pathlib import Path
import time
counter = Path({marker})
if not counter.exists():
    counter.write_text('1')
    time.sleep(0.28)
    raise SystemExit(0)
Path('agent.ready').write_text('ready')
while True: time.sleep(0.01)
"""
        command = self.write_agent(source)
        config = runner.RunConfig(
            deadline_seconds=0.5,
            poll_interval_seconds=0.01,
            term_grace_seconds=0.08,
            cleanup_timeout_seconds=0.25,
            verifier_timeout_seconds=0.5,
            max_attempts=2,
        )
        start = time.monotonic()

        result = runner.run_agent(command, cwd=self.worktree, env=self.env, config=config)

        elapsed = time.monotonic() - start
        self.assertNotEqual(result, 0)
        self.assertEqual(self.outcome(2)["stop_reason"], "deadline")
        self.assertLess(elapsed, 0.7)
        self.assertLess(self.outcome(2)["elapsed_seconds"], 0.35)

    def test_external_cancellation_fails_even_with_output_files_present(self) -> None:
        source = """from pathlib import Path
import time
Path('agent.ready').write_text('ready')
while True: time.sleep(0.01)
"""
        command = self.write_agent(source)
        cancellation = threading.Event()
        result_holder: list[int] = []
        thread = threading.Thread(
            target=lambda: result_holder.append(
                self.run_agent(command, cancellation=cancellation)
            )
        )
        thread.start()
        self.wait_for_path(self.worktree / "agent.ready")
        output_dir = self.worktree / "docs" / "pr-walkthrough"
        output_dir.mkdir(parents=True, exist_ok=True)
        (output_dir / "pr-42.json").write_text("{}", encoding="utf-8")
        (output_dir / "pr-42.html").write_text("html", encoding="utf-8")
        cancellation.set()
        thread.join(timeout=2)

        self.assertFalse(thread.is_alive(), "runner did not stop after cancellation")
        self.assertEqual(result_holder, [1])
        self.assertEqual(self.outcome()["stop_reason"], "cancellation")
        self.assertEqual(self.outcome()["verification_result"], "not_run")

    def test_cancellation_during_verification_cannot_succeed(self) -> None:
        verifier = self.worktree / ".agents" / "skills" / "pr-walkthrough" / "scripts" / "pr-walkthrough-verify"
        verifier.write_text(
            "from pathlib import Path\n"
            "import time\n"
            "Path('verify.ready').touch()\n"
            "while not Path('cancel.signal').exists(): time.sleep(0.005)\n",
            encoding="utf-8",
        )
        command = self.write_agent(self.render_agent_source())
        cancellation = threading.Event()
        result_holder: list[int] = []
        thread = threading.Thread(
            target=lambda: result_holder.append(
                self.run_agent(command, cancellation=cancellation)
            )
        )
        thread.start()
        self.wait_for_path(self.worktree / "verify.ready")
        (self.worktree / "cancel.signal").touch()
        cancellation.set()
        thread.join(timeout=2)

        self.assertFalse(thread.is_alive(), "runner did not stop after cancellation")
        self.assertEqual(result_holder, [1])
        self.assertEqual(self.outcome()["stop_reason"], "cancellation")
        self.assertNotEqual(self.outcome()["verification_result"], "passed")

    def test_stale_receipt_is_removed_before_an_attempt(self) -> None:
        receipt = self.worktree / ".pr-walkthrough" / "render-complete.json"
        receipt.write_text("{}", encoding="utf-8")
        command = self.write_agent("raise SystemExit(0)\n")
        config = runner.RunConfig(**(self.config.__dict__ | {"max_attempts": 1}))

        result = runner.run_agent(command, cwd=self.worktree, env=self.env, config=config)

        self.assertNotEqual(result, 0)
        self.assertFalse(receipt.exists())
        self.assertEqual(self.outcome()["stop_reason"], "incomplete_zero_exit")

    def test_invalid_receipt_fails_without_retry(self) -> None:
        command = self.write_agent(
            "from pathlib import Path\n"
            "Path('.pr-walkthrough/render-complete.json').write_text('{}')\n"
            "Path('agent.ready').write_text('ready')\n"
            "import time\nwhile True: time.sleep(0.01)\n"
        )

        result = self.run_agent(command)

        self.assertNotEqual(result, 0)
        self.assertEqual(len(self.attempt_dirs()), 1)
        self.assertEqual(self.outcome()["stop_reason"], "render_complete")
        self.assertIn("failed", self.outcome()["verification_result"])

    def test_workflow_summary_reports_attempt_durations_and_result(self) -> None:
        attempt_dir = self.worktree / ".pr-walkthrough" / "attempt-1"
        attempt_dir.mkdir(parents=True)
        (attempt_dir / "outcome.json").write_text(
            json.dumps(
                {
                    "elapsed_seconds": 4.25,
                    "raw_exit_status": -signal.SIGTERM,
                    "stop_reason": "render_complete",
                    "verification_result": "passed",
                }
            ),
            encoding="utf-8",
        )
        summary_path = self.worktree / "step-summary.md"
        previous_cwd = Path.cwd()
        try:
            os.chdir(self.worktree)
            with mock.patch.object(runner, "run_agent", return_value=0):
                with mock.patch.dict(os.environ, {"GITHUB_STEP_SUMMARY": str(summary_path)}):
                    result = runner.main(["--", "trusted-agent"])
        finally:
            os.chdir(previous_cwd)

        self.assertEqual(result, 0)
        self.assertTrue(summary_path.is_file(), "runner did not write the workflow summary")
        summary = summary_path.read_text(encoding="utf-8")
        self.assertIn("verified", summary)
        self.assertIn("4.25s", summary)
        self.assertIn("render_complete", summary)

    def test_workflow_summary_reports_cleanup_failure(self) -> None:
        attempt_dir = self.worktree / ".pr-walkthrough" / "attempt-1"
        attempt_dir.mkdir(parents=True)
        (attempt_dir / "outcome.json").write_text(
            json.dumps(
                {
                    "elapsed_seconds": 1.25,
                    "raw_exit_status": 0,
                    "stop_reason": "cleanup_failure",
                    "verification_result": "cleanup failed",
                }
            ),
            encoding="utf-8",
        )
        summary_path = self.worktree / "step-summary.md"

        runner.write_step_summary(
            self.worktree,
            {**self.env, "GITHUB_STEP_SUMMARY": str(summary_path)},
            1,
        )

        summary = summary_path.read_text(encoding="utf-8")
        self.assertIn("Attempt 1: cleanup_failure", summary)
        self.assertIn("verification cleanup failed", summary)
        self.assertNotIn("verification not run", summary)

    def wait_for_path(self, path: Path, timeout: float = 1) -> None:
        deadline = time.monotonic() + timeout
        while time.monotonic() < deadline:
            if path.exists():
                return
            time.sleep(0.005)
        self.fail(f"timed out waiting for {path}")

    def assert_process_stopped(self, pid: int) -> None:
        deadline = time.monotonic() + 0.4
        while time.monotonic() < deadline:
            proc_stat = Path(f"/proc/{pid}/stat")
            if not proc_stat.exists():
                return
            state = proc_stat.read_text(encoding="utf-8").split(") ", 1)[1][0]
            if state in {"Z", "X"}:
                return
            time.sleep(0.005)
        self.fail(f"owned descendant {pid} is still running")


if __name__ == "__main__":
    unittest.main()
