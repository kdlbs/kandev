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
from typing import Any
from unittest import mock
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
RUNNER = REPO_ROOT / ".github" / "scripts" / "pr-walkthrough-runner.py"
SKILL = REPO_ROOT / ".agents" / "skills" / "pr-walkthrough"
SPEC = importlib.util.spec_from_file_location("pr_walkthrough_runner", RUNNER)
assert SPEC is not None and SPEC.loader is not None
runner = importlib.util.module_from_spec(SPEC)
SPEC.loader.exec_module(runner)

PREFERRED = runner.PREFERRED_FREE_MODEL
GO_FALLBACK = runner.GO_FALLBACK_MODEL
USE_DEFAULT_DISCOVERY = object()


class WaitForAgentExit:
    def __init__(self, pid_path: Path) -> None:
        self.pid_path = pid_path

    def is_set(self) -> bool:
        return False

    def wait(self, timeout: float | None = None) -> bool:
        deadline = time.monotonic() + 2
        while time.monotonic() < deadline:
            if self.pid_path.exists():
                text = self.pid_path.read_text(encoding="utf-8").strip()
                if text:
                    pid = int(text)
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


def catalog_text(models: list[dict[str, object]]) -> str:
    blocks: list[str] = []
    for model in models:
        model_id = str(model["model_id"])
        provider_id, name = model_id.split("/", 1)
        data: dict[str, object] = {
            "id": name,
            "providerID": provider_id,
            "name": model.get("name", name),
            "cost": model.get(
                "cost",
                {"input": 0, "output": 0, "cache": {"read": 0, "write": 0}},
            ),
            "capabilities": model.get(
                "capabilities",
                {
                    "toolcall": True,
                    "input": {"text": True},
                    "output": {"text": True},
                },
            ),
            "variants": {
                variant: {"reasoningEffort": variant}
                for variant in model.get("variants", [])
            },
        }
        blocks.append(model_id)
        blocks.append(json.dumps(data, indent=2))
    return "\n".join(blocks) + "\n"


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
        self.env.pop("OPENCODE_API_KEY", None)
        self.config = runner.RunConfig(
            deadline_seconds=3.0,
            poll_interval_seconds=0.01,
            term_grace_seconds=0.3,
            cleanup_timeout_seconds=1.0,
            verifier_timeout_seconds=2,
            discovery_timeout_seconds=0.5,
            max_incomplete_retries=1,
        )
        self.default_discovery = runner.ModelDiscovery(
            "succeeded",
            models=(
                runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),
            ),
        )

    def write_agent(self, source: str, *, model: bool = True) -> list[str]:
        path = self.worktree / "fake-agent.py"
        path.write_text(source, encoding="utf-8")
        command = [sys.executable, str(path)]
        if model:
            command.extend(["--model", runner.MODEL_PLACEHOLDER])
        return command

    def write_catalog_cli(self, text: str, *, exit_code: int = 0, delay: float = 0) -> list[str]:
        command_script = self.worktree / "fake-opencode.py"
        command_script.write_text(
            "import sys, time\n"
            f"time.sleep({delay!r})\n"
            f"sys.stdout.write({text!r})\n"
            f"raise SystemExit({exit_code})\n",
            encoding="utf-8",
        )
        return [sys.executable, str(command_script)]

    def model_source(self) -> str:
        return (
            "import sys\n"
            "args = sys.argv[1:]\n"
            "model = args[args.index('--model') + 1] if '--model' in args else 'unknown'\n"
            "from pathlib import Path\n"
            "with Path('models.txt').open('a', encoding='utf-8') as stream:\n"
            "    stream.write(model + '\\n')\n"
        )

    def render_source(self) -> str:
        serialized = repr(json.dumps(self.data))
        render_command = repr([sys.executable, str(self.renderer)])
        return (
            "from pathlib import Path\n"
            f"Path('.pr-walkthrough/draft.json').write_text({serialized}, encoding='utf-8')\n"
            f"import subprocess\nsubprocess.run({render_command}, check=True)\n"
            "while True: __import__('time').sleep(0.01)\n"
        )

    def run_agent(
        self,
        command: list[str],
        *,
        cancellation: threading.Event | None = None,
        discovery: Any = USE_DEFAULT_DISCOVERY,
        models_command: list[str] | None = None,
        config: runner.RunConfig | None = None,
        env: dict[str, str] | None = None,
    ) -> int:
        selected_discovery = (
            self.default_discovery
            if discovery is USE_DEFAULT_DISCOVERY
            else discovery
        )
        return runner.run_agent(
            command,
            cwd=self.worktree,
            env=env or self.env,
            config=config or self.config,
            cancellation=cancellation,
            models_command=models_command,
            discovery=selected_discovery,
        )

    def outcome(self, attempt: int = 1) -> dict[str, object]:
        path = self.worktree / ".pr-walkthrough" / f"attempt-{attempt}" / "outcome.json"
        return json.loads(path.read_text(encoding="utf-8"))

    def attempt_dirs(self) -> list[Path]:
        return sorted((self.worktree / ".pr-walkthrough").glob("attempt-*"))

    def discovery_record(self) -> dict[str, object]:
        path = self.worktree / ".pr-walkthrough" / "discovery" / "discovery.json"
        return json.loads(path.read_text(encoding="utf-8"))

    def models_used(self) -> list[str]:
        path = self.worktree / "models.txt"
        if not path.exists():
            return []
        return path.read_text(encoding="utf-8").splitlines()

    def test_render_then_idle_completes_and_stops_owned_group(self) -> None:
        command = self.write_agent(self.render_source())

        result = self.run_agent(command)

        self.assertEqual(result, 0)
        record = self.outcome()
        self.assertEqual(record["stop_reason"], "render_complete")
        self.assertEqual(record["verification_result"], "passed")
        self.assertEqual(record["raw_exit_status"], -signal.SIGTERM)
        self.assertEqual(record["model"], PREFERRED)
        self.assertTrue(record["started_at"])
        self.assertTrue(record["finished_at"])
        self.assertGreaterEqual(record["elapsed_seconds"], 0)

    def test_render_completion_stops_owned_descendant(self) -> None:
        serialized = repr(json.dumps(self.data))
        render_command = repr([sys.executable, str(self.renderer)])
        source = (
            "import json, subprocess, sys, time\n"
            "from pathlib import Path\n"
            "child_code = \"import signal,time; signal.signal(signal.SIGTERM, signal.SIG_IGN); time.sleep(60)\"\n"
            "child = subprocess.Popen([sys.executable, '-c', child_code])\n"
            "Path('descendant.pid').write_text(str(child.pid), encoding='utf-8')\n"
            f"Path('.pr-walkthrough/draft.json').write_text({serialized}, encoding='utf-8')\n"
            f"subprocess.run({render_command}, check=True)\n"
            "while True: time.sleep(0.01)\n"
        )
        command = self.write_agent(source)

        result = self.run_agent(command)

        self.assertEqual(result, 0)
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
        serialized = repr(json.dumps(self.data))
        source = (
            "from pathlib import Path\n"
            f"Path('.pr-walkthrough/draft.json').write_text({serialized}, encoding='utf-8')\n"
            f"import subprocess\nsubprocess.run({repr([sys.executable, str(slow_renderer)])}, check=True)\n"
            "while True: __import__('time').sleep(0.01)\n"
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
        self.assertIsNone(record["provider_error"])

    def test_completed_receipt_does_not_mask_observed_nonzero_exit(self) -> None:
        source = self.render_source().replace(
            "while True: __import__('time').sleep(0.01)\n",
            "Path('agent.pid').write_text(str(__import__('os').getpid()))\nraise SystemExit(7)\n",
        )
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
        source = self.render_source().replace(
            "while True: __import__('time').sleep(0.01)\n",
            "Path('agent.pid').write_text(str(__import__('os').getpid()))\nraise SystemExit(0)\n",
        )
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
        self.assertEqual(self.models_used(), [])

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
        elapsed = 0.0

        def advance_clock(timeout: float) -> bool:
            nonlocal elapsed
            elapsed = round(elapsed + timeout, 6)
            return False

        cancellation = mock.Mock()
        cancellation.is_set.return_value = False
        cancellation.wait.side_effect = advance_clock
        first_process = mock.Mock(returncode=0)
        first_process.poll.side_effect = lambda: 0 if elapsed >= 0.28 else None
        second_process = mock.Mock(returncode=None)
        second_process.poll.return_value = None
        config = runner.RunConfig(
            deadline_seconds=0.5,
            poll_interval_seconds=0.01,
            term_grace_seconds=0.08,
            cleanup_timeout_seconds=0.25,
            verifier_timeout_seconds=0.5,
            discovery_timeout_seconds=0.25,
            max_incomplete_retries=1,
        )

        with (
            mock.patch.object(runner.time, "monotonic", side_effect=lambda: elapsed),
            mock.patch.object(
                runner.subprocess, "Popen", side_effect=[first_process, second_process]
            ) as launch,
            mock.patch.object(runner, "stop_process_group", return_value=True) as cleanup,
        ):
            result = runner.run_agent(
                ["fake-agent", "--model", runner.MODEL_PLACEHOLDER],
                cwd=self.worktree,
                env=self.env,
                config=config,
                cancellation=cancellation,
                discovery=self.default_discovery,
            )

        self.assertNotEqual(result, 0)
        self.assertEqual(launch.call_count, 2)
        self.assertEqual(
            cleanup.call_args_list,
            [mock.call(first_process, config), mock.call(second_process, config)],
        )
        self.assertEqual(self.outcome(1)["stop_reason"], "incomplete_zero_exit")
        self.assertEqual(self.outcome(2)["stop_reason"], "deadline")
        self.assertAlmostEqual(elapsed, 0.5)
        self.assertAlmostEqual(self.outcome(1)["elapsed_seconds"], 0.28)
        self.assertAlmostEqual(self.outcome(2)["elapsed_seconds"], 0.22)

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
        command = self.write_agent(self.render_source())
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
        config = runner.RunConfig(**(
            self.config.__dict__ | {"max_incomplete_retries": 0}
        ))

        result = self.run_agent(command, config=config)

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

    def test_catalog_requires_explicit_free_prices_and_text_tool_capabilities(self) -> None:
        models = [
            {"model_id": PREFERRED, "variants": ["low", "high"]},
            {"model_id": "opencode/big-pickle"},
            {"model_id": "opencode/beam-free"},
            {"model_id": "opencode/conflict"},
            {"model_id": "opencode/paid", "cost": {"input": 0.1, "output": 0.2}},
            {
                "model_id": "opencode/cache-paid",
                "cost": {"input": 0, "output": 0, "cache": {"read": 0.1, "write": 0}},
            },
            {
                "model_id": "opencode/image-output",
                "capabilities": {
                    "toolcall": True,
                    "input": {"text": True},
                    "output": {"text": False, "image": True},
                },
            },
            {
                "model_id": "opencode/no-tools",
                "capabilities": {
                    "toolcall": False,
                    "input": {"text": True},
                    "output": {"text": True},
                },
            },
            {
                "model_id": "opencode/string-price",
                "cost": {"input": "0", "output": 0},
            },
            {"model_id": "opencode/missing-cost", "cost": None},
            {"model_id": GO_FALLBACK, "cost": {"input": 0.1, "output": 0.2}, "variants": ["high"]},
        ]
        text = catalog_text(models)
        text += (
            "opencode/bad-json\n"
            "{not json}\n"
            f"{PREFERRED}\n"
            + json.dumps(
                {
                    "id": PREFERRED.split("/", 1)[1],
                    "providerID": "opencode",
                    "cost": {"input": 0, "output": 0},
                    "capabilities": {
                        "toolcall": True,
                        "input": {"text": True},
                        "output": {"text": True},
                    },
                    "variants": {"low": {}, "high": {}},
                }
            )
            + "\n"
            + "opencode/conflict\n"
            + json.dumps(
                {
                    "id": "conflict",
                    "providerID": "opencode",
                    "cost": {"input": 0.1, "output": 0.2},
                    "capabilities": {
                        "toolcall": True,
                        "input": {"text": True},
                        "output": {"text": True},
                    },
                    "variants": {},
                }
            )
            + "\n"
        )

        parsed = runner.parse_model_catalog(text)
        discovery = runner.ModelDiscovery("succeeded", models=parsed)
        candidates = discovery.candidates

        self.assertEqual(
            [candidate.model_id for candidate in candidates],
            [PREFERRED, "opencode/beam-free", "opencode/big-pickle"],
        )
        self.assertEqual(candidates[0].variants, frozenset({"low", "high"}))
        self.assertEqual(candidates[2].variants, frozenset())
        self.assertFalse(
            any(model.model_id == "opencode/bad-json" for model in parsed)
        )
        self.assertFalse(
            any(model.model_id == "opencode/conflict" for model in parsed)
        )
        self.assertTrue(any(model.model_id == GO_FALLBACK for model in parsed))

        self.assertEqual(
            [candidate.model_id for candidate in runner.build_candidates(discovery, {})],
            [PREFERRED, "opencode/beam-free", "opencode/big-pickle"],
        )
        self.assertEqual(
            runner.build_candidates(discovery, {"OPENCODE_API_KEY": "key"})[-1].model_id,
            GO_FALLBACK,
        )
        self.assertEqual(
            runner.build_candidates(discovery, {"OPENCODE_API_KEY": "key"})[-1].variants,
            frozenset({"high"}),
        )

    def test_catalog_skips_unrecognized_id_lines_without_dropping_prior_models(self) -> None:
        invalid_block = json.dumps(
            {
                "id": "anthropic/claude-3:free",
                "providerID": "openrouter",
                "cost": {"input": 0, "output": 0},
                "capabilities": {
                    "toolcall": True,
                    "input": {"text": True},
                    "output": {"text": True},
                },
                "variants": {},
            },
            indent=2,
        )
        text = (
            catalog_text([{"model_id": "opencode/valid-free"}])
            + "openrouter/anthropic/claude-3:free\n"
            + invalid_block
            + "\n"
            + catalog_text([{"model_id": "opencode/later-free"}])
        )

        parsed = runner.parse_model_catalog(text)

        self.assertEqual(
            [model.model_id for model in parsed],
            ["opencode/valid-free", "opencode/later-free"],
        )

    def test_malformed_catalog_is_a_discovery_failure(self) -> None:
        with self.assertRaises(ValueError):
            runner.parse_model_catalog("not a model catalog\n")
        with self.assertRaises(ValueError):
            runner.parse_model_catalog(
                "opencode/broken\n{not json}\n"
            )
        self.assertEqual(runner.parse_model_catalog(""), ())

    def test_free_rotation_is_limited_to_the_opencode_provider(self) -> None:
        text = catalog_text(
            [
                {"model_id": "opencode/free-one"},
                {"model_id": "opencode-go/beam-free"},
                {"model_id": "openrouter/free-two"},
                {
                    "model_id": GO_FALLBACK,
                    "cost": {"input": 0.1, "output": 0.2},
                    "variants": ["high"],
                },
            ]
        )
        discovery = runner.ModelDiscovery(
            "succeeded", models=runner.parse_model_catalog(text)
        )

        self.assertEqual(
            [candidate.model_id for candidate in discovery.candidates],
            ["opencode/free-one"],
        )
        self.assertTrue(
            any(
                model.model_id == "opencode-go/beam-free" and model.free
                for model in discovery.models
            )
        )
        built = runner.build_candidates(discovery, {"OPENCODE_API_KEY": "key"})
        self.assertEqual(
            [candidate.model_id for candidate in built],
            ["opencode/free-one", GO_FALLBACK],
        )
        self.assertEqual(built[-1].variants, frozenset({"high"}))

    def test_variant_is_only_passed_when_the_model_supports_it(self) -> None:
        command = [
            "opencode",
            "run",
            "--model",
            runner.MODEL_PLACEHOLDER,
            "--variant",
            runner.VARIANT_PLACEHOLDER,
            "--",
            "prompt",
        ]
        supported = runner.Candidate(PREFERRED, frozenset({"high"}), "free")
        unsupported = runner.Candidate("opencode/big-pickle", frozenset(), "free")

        self.assertEqual(
            runner.render_agent_command(command, supported),
            ["opencode", "run", "--model", PREFERRED, "--variant", "high", "--", "prompt"],
        )
        self.assertEqual(
            runner.render_agent_command(command, unsupported),
            ["opencode", "run", "--model", "opencode/big-pickle", "--", "prompt"],
        )
        fixed = [
            "opencode",
            "run",
            "--model",
            runner.MODEL_PLACEHOLDER,
            "--variant",
            "high",
        ]
        self.assertEqual(
            runner.render_agent_command(fixed, unsupported),
            ["opencode", "run", "--model", "opencode/big-pickle"],
        )
        after_separator = ["opencode", "run", "--", "--variant", runner.VARIANT_PLACEHOLDER]
        self.assertEqual(
            runner.render_agent_command(after_separator, supported),
            after_separator,
        )
        equals = [
            "opencode",
            "run",
            f"--model={runner.MODEL_PLACEHOLDER}",
            f"--variant={runner.VARIANT_PLACEHOLDER}",
        ]
        self.assertEqual(
            runner.render_agent_command(equals, supported),
            ["opencode", "run", f"--model={PREFERRED}", "--variant=high"],
        )
        self.assertEqual(
            runner.render_agent_command(equals, unsupported),
            ["opencode", "run", "--model=opencode/big-pickle"],
        )

    def test_command_with_a_fixed_model_is_rejected(self) -> None:
        command = self.write_agent(self.render_source(), model=False)
        result = self.run_agent(command + ["--model", PREFERRED])

        self.assertNotEqual(result, 0)
        self.assertEqual(self.attempt_dirs(), [])

    def test_command_without_a_model_placeholder_is_rejected(self) -> None:
        command = self.write_agent(self.render_source(), model=False)

        result = self.run_agent(command)

        self.assertNotEqual(result, 0)
        self.assertEqual(self.attempt_dirs(), [])

    def test_quota_error_rotates_models_while_the_process_stays_live(self) -> None:
        second = "opencode/next-free"
        discovery = runner.ModelDiscovery(
            "succeeded",
            models=(
                runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),
                runner.ModelMetadata(second, frozenset(), True),
            ),
        )
        source = self.model_source() + f"""
from pathlib import Path
model = Path('models.txt').read_text(encoding='utf-8').splitlines()[-1]
if model == {PREFERRED!r}:
    import sys
    print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message="stream error" providerID=opencode modelID=deepseek-v4.1-flash-free session.id=ses_1 small=false agent=build mode=primary error.error="AI_APICallError: Free usage limit reached"', file=sys.stderr, flush=True)
    while True: __import__('time').sleep(0.01)
{self.render_source()}"""
        command = self.write_agent(source)

        result = self.run_agent(command, discovery=discovery)

        self.assertEqual(result, 0)
        self.assertEqual(self.models_used(), [PREFERRED, second])
        self.assertEqual(len(self.attempt_dirs()), 2)
        first = self.outcome(1)
        self.assertEqual(first["model"], PREFERRED)
        self.assertEqual(first["model_source"], "free")
        self.assertEqual(first["stop_reason"], "model_quota")
        self.assertEqual(first["provider_error"], "model_quota")
        self.assertEqual(first["verification_result"], "not_run")
        second_record = self.outcome(2)
        self.assertEqual(second_record["model"], second)
        self.assertEqual(second_record["verification_result"], "passed")

    def test_rate_limit_error_rotates_models_while_the_process_stays_live(self) -> None:
        second = "opencode/next-free"
        discovery = runner.ModelDiscovery(
            "succeeded",
            models=(
                runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),
                runner.ModelMetadata(second, frozenset(), True),
            ),
        )
        source = self.model_source() + f"""
from pathlib import Path
model = Path('models.txt').read_text(encoding='utf-8').splitlines()[-1]
if model == {PREFERRED!r}:
    import sys
    print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message="stream error" providerID=opencode modelID=deepseek-v4.1-flash-free session.id=ses_1 small=false agent=build mode=primary error.error="AI_APICallError: 429 Too Many Requests"', file=sys.stderr, flush=True)
    while True: __import__('time').sleep(0.01)
{self.render_source()}"""
        command = self.write_agent(source)

        result = self.run_agent(command, discovery=discovery)

        self.assertEqual(result, 0)
        self.assertEqual(self.models_used(), [PREFERRED, second])
        self.assertEqual(len(self.attempt_dirs()), 2)
        first = self.outcome(1)
        self.assertEqual(first["model"], PREFERRED)
        self.assertEqual(first["stop_reason"], "model_rate_limit")
        self.assertEqual(first["provider_error"], "model_rate_limit")
        self.assertEqual(first["verification_result"], "not_run")
        self.assertEqual(self.outcome(2)["verification_result"], "passed")

    def test_provider_error_classification_matches_cli_signatures(self) -> None:
        candidate = runner.Candidate(PREFERRED, frozenset({"high"}), "free")
        prefix = (
            'timestamp=2026-10-28T00:00:00.000Z level=ERROR run=x message="stream error" '
            "providerID=opencode modelID=deepseek-v4.1-flash-free small=false agent=build "
        )
        for detail, expected in (
            ("AI_APICallError: Free usage limit reached", "model_quota"),
            ("AI_APICallError: Free usage exceeded, subscribe to Go", "model_quota"),
            ("AI_APICallError: FreeUsageLimitError", "model_quota"),
            ("AI_APICallError: GoUsageLimitError", "model_quota"),
            ("AI_APICallError: 5-hour usage limit reached. Resets in 4hr.", "model_quota"),
            ("AI_APICallError: Go usage limit exceeded", "model_quota"),
            ("AI_APICallError: credit limit reached", "model_quota"),
            ("AI_APICallError: insufficient_quota", "model_quota"),
            ("AI_APICallError: Rate limit exceeded", "model_rate_limit"),
            ("AI_APICallError: 429 Too Many Requests", "model_rate_limit"),
            ("AI_APICallError: HTTP 429", "model_rate_limit"),
            ("AI_APICallError: statusCode=429", "model_rate_limit"),
            ("AI_APICallError: status_code: 429", "model_rate_limit"),
            ("AI_APICallError: rate_limit_exceeded", "model_rate_limit"),
            ("AI_APICallError: rate limited", "model_rate_limit"),
            ("AI_APICallError: too many requests", "model_rate_limit"),
            ("AI_APICallError: Upstream request failed: Invalid credential", None),
            ("AI_APICallError: payment required to continue", None),
        ):
            line = f'{prefix}error.error="{detail}"'
            self.assertEqual(
                runner.classify_provider_error(line, candidate),
                expected,
                detail,
            )
        self.assertEqual(
            runner.classify_provider_error(
                'timestamp=1 level=ERROR message=failed error="ProviderModelNotFoundError: '
                "Model not found: opencode/deepseek-v4.1-flash-free.\"",
                candidate,
            ),
            "model_unavailable",
        )
        self.assertIsNone(
            runner.classify_provider_error(
                f'{prefix.replace("providerID=opencode", "providerID=other")}'
                'error.error="AI_APICallError: Free usage limit reached"',
                candidate,
            )
        )

    def test_quota_error_without_a_trailing_newline_rotates(self) -> None:
        second = "opencode/next-free"
        discovery = runner.ModelDiscovery(
            "succeeded",
            models=(
                runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),
                runner.ModelMetadata(second, frozenset(), True),
            ),
        )
        source = self.model_source() + f"""
from pathlib import Path
model = Path('models.txt').read_text(encoding='utf-8').splitlines()[-1]
if model == {PREFERRED!r}:
    import sys
    sys.stderr.write('timestamp=2026-10-28T00:00:00.000Z level=ERROR message="stream error" providerID=opencode modelID=deepseek-v4.1-flash-free small=false agent=build error.error="AI_APICallError: Free usage limit reached"')
    sys.stderr.flush()
    raise SystemExit(7)
{self.render_source()}"""
        command = self.write_agent(source)

        result = self.run_agent(command, discovery=discovery)

        self.assertEqual(result, 0)
        self.assertEqual(self.models_used(), [PREFERRED, second])
        self.assertEqual(self.outcome(1)["stop_reason"], "model_quota")
        self.assertEqual(self.outcome(2)["verification_result"], "passed")

    def test_cleanup_failure_after_quota_error_is_terminal(self) -> None:
        second = "opencode/next-free"
        discovery = runner.ModelDiscovery(
            "succeeded",
            models=(
                runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),
                runner.ModelMetadata(second, frozenset(), True),
            ),
        )
        source = self.model_source() + f"""
from pathlib import Path
model = Path('models.txt').read_text(encoding='utf-8').splitlines()[-1]
if model == {PREFERRED!r}:
    import sys
    print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message="stream error" providerID=opencode modelID=deepseek-v4.1-flash-free small=false agent=build error.error="AI_APICallError: Free usage limit reached"', file=sys.stderr, flush=True)
    while True: __import__('time').sleep(0.01)
{self.render_source()}"""
        command = self.write_agent(source)
        real_stop = runner.stop_process_group

        def fail_cleanup(process, config):
            real_stop(process, config)
            return False

        with mock.patch.object(runner, "stop_process_group", side_effect=fail_cleanup):
            result = self.run_agent(command, discovery=discovery)

        self.assertNotEqual(result, 0)
        self.assertEqual(self.models_used(), [PREFERRED])
        self.assertEqual(len(self.attempt_dirs()), 1)
        self.assertEqual(self.outcome()["stop_reason"], "cleanup_failure")
        self.assertEqual(self.outcome()["verification_result"], "cleanup failed")

    def test_unrecognized_candidate_error_does_not_rotate(self) -> None:
        second = "opencode/next-free"
        discovery = runner.ModelDiscovery(
            "succeeded",
            models=(
                runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),
                runner.ModelMetadata(second, frozenset(), True),
            ),
        )
        source = self.model_source() + f"""
from pathlib import Path
model = Path('models.txt').read_text(encoding='utf-8').splitlines()[-1]
if model == {PREFERRED!r}:
    import sys
    print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message="stream error" providerID=opencode modelID=deepseek-v4.1-flash-free small=false agent=build error.error="AI_APICallError: Upstream request failed: Invalid credential"', file=sys.stderr, flush=True)
    raise SystemExit(7)
{self.render_source()}"""
        command = self.write_agent(source)

        result = self.run_agent(command, discovery=discovery)

        self.assertNotEqual(result, 0)
        self.assertEqual(self.models_used(), [PREFERRED])
        self.assertEqual(len(self.attempt_dirs()), 1)
        self.assertEqual(self.outcome()["stop_reason"], "unexpected_exit")
        self.assertIsNone(self.outcome()["provider_error"])

    def test_discovered_catalog_drives_the_rotation(self) -> None:
        second = "opencode/next-free"
        models_command = self.write_catalog_cli(
            catalog_text(
                [
                    {"model_id": PREFERRED, "variants": ["high"]},
                    {"model_id": second},
                ]
            )
        )
        source = self.model_source() + f"""
from pathlib import Path
model = Path('models.txt').read_text(encoding='utf-8').splitlines()[-1]
if model == {PREFERRED!r}:
    import sys
    print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message="stream error" providerID=opencode modelID=deepseek-v4.1-flash-free small=false agent=build error.error="AI_APICallError: Free usage limit reached"', file=sys.stderr, flush=True)
    while True: __import__('time').sleep(0.01)
{self.render_source()}"""
        command = self.write_agent(source)

        result = self.run_agent(
            command, models_command=models_command, discovery=None
        )

        self.assertEqual(result, 0)
        self.assertEqual(self.models_used(), [PREFERRED, second])
        record = self.discovery_record()
        self.assertEqual(record["status"], "succeeded")
        self.assertEqual(
            record["eligible_free_models"], [PREFERRED, second]
        )
        self.assertEqual(self.outcome(1)["stop_reason"], "model_quota")
        self.assertEqual(self.outcome(2)["verification_result"], "passed")

    def test_model_not_found_rotates_to_the_next_candidate(self) -> None:
        second = "opencode/next-free"
        discovery = runner.ModelDiscovery(
            "succeeded",
            models=(
                runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),
                runner.ModelMetadata(second, frozenset(), True),
            ),
        )
        source = self.model_source() + f"""
from pathlib import Path
model = Path('models.txt').read_text(encoding='utf-8').splitlines()[-1]
if model == {PREFERRED!r}:
    import sys
    print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message=failed ref=err_1 error="ProviderModelNotFoundError: Model not found: opencode/deepseek-v4.1-flash-free."', file=sys.stderr, flush=True)
    while True: __import__('time').sleep(0.01)
{self.render_source()}"""
        command = self.write_agent(source)

        result = self.run_agent(command, discovery=discovery)

        self.assertEqual(result, 0)
        self.assertEqual(self.models_used(), [PREFERRED, second])
        self.assertEqual(self.outcome(1)["stop_reason"], "model_unavailable")
        self.assertEqual(self.outcome(2)["verification_result"], "passed")

    def test_title_generation_error_does_not_rotate(self) -> None:
        second = "opencode/next-free"
        discovery = runner.ModelDiscovery(
            "succeeded",
            models=(
                runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),
                runner.ModelMetadata(second, frozenset(), True),
            ),
        )
        source = (
            "import sys\n"
            "print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message=\"stream error\" providerID=opencode modelID=deepseek-v4.1-flash-free small=true agent=title error.error=\"AI_APICallError: Free usage limit reached\"', file=sys.stderr, flush=True)\n"
            "print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message=\"stream error\" providerID=opencode modelID=deepseek-v4.1-flash-free small=true agent=title error.error=\"AI_RetryError: Failed after 3 attempts. Last error: Free usage limit reached\"', file=sys.stderr, flush=True)\n"
            "print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message=\"stream error\" providerID=opencode modelID=deepseek-v4.1-flash-free small=true agent=title error.error=\"AI_APICallError: 429 Too Many Requests\"', file=sys.stderr, flush=True)\n"
            "print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message=\"failed to generate title\" error=\"AI_APICallError: Free usage limit reached\"', file=sys.stderr, flush=True)\n"
            + self.render_source()
        )
        command = self.write_agent(source)

        result = self.run_agent(command, discovery=discovery)

        self.assertEqual(result, 0)
        self.assertEqual(len(self.attempt_dirs()), 1)
        self.assertEqual(self.outcome()["verification_result"], "passed")

    def test_unattributed_error_text_does_not_rotate(self) -> None:
        second = "opencode/next-free"
        discovery = runner.ModelDiscovery(
            "succeeded",
            models=(
                runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),
                runner.ModelMetadata(second, frozenset(), True),
            ),
        )
        source = (
            "import sys\n"
            "print('Free usage limit reached in the pull request patch')\n"
            "print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message=\"stream error\" providerID=opencode modelID=another-model small=false agent=build error.error=\"AI_APICallError: Free usage limit reached\"', file=sys.stderr, flush=True)\n"
            "print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message=\"stream error\" providerID=opencode modelID=deepseek-v4.1-flash-free small=false agent=build error.error=\"Tool error: patch text says Free usage limit reached\"', file=sys.stderr, flush=True)\n"
            "print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message=\"tool error\" providerID=opencode modelID=deepseek-v4.1-flash-free small=false agent=build error.error=\"AI_APICallError: Free usage limit reached\"', file=sys.stderr, flush=True)\n"
            "print('a patch line says usage limit reached', file=sys.stderr, flush=True)\n"
            + self.render_source()
        )
        command = self.write_agent(source)

        result = self.run_agent(command, discovery=discovery)

        self.assertEqual(result, 0)
        self.assertEqual(len(self.attempt_dirs()), 1)
        self.assertEqual(self.outcome()["verification_result"], "passed")

    def test_go_fallback_follows_exhausted_free_models(self) -> None:
        discovery = runner.ModelDiscovery(
            "succeeded",
            models=(runner.ModelMetadata(PREFERRED, frozenset({"high"}), True),),
        )
        env = {**self.env, "OPENCODE_API_KEY": "test-key"}
        source = self.model_source() + f"""
from pathlib import Path
model = Path('models.txt').read_text(encoding='utf-8').splitlines()[-1]
if model == {PREFERRED!r}:
    import sys
    print('timestamp=2026-10-28T00:00:00.000Z level=ERROR message="stream error" providerID=opencode modelID=deepseek-v4.1-flash-free small=false agent=build error.error="AI_APICallError: Free usage limit reached"', file=sys.stderr, flush=True)
    while True: __import__('time').sleep(0.01)
{self.render_source()}"""
        command = self.write_agent(source)

        result = self.run_agent(command, discovery=discovery, env=env)

        self.assertEqual(result, 0)
        self.assertEqual(self.models_used(), [PREFERRED, GO_FALLBACK])
        self.assertEqual(self.outcome(2)["model"], GO_FALLBACK)
        self.assertEqual(self.outcome(2)["model_source"], "go")
        self.assertEqual(self.outcome(2)["verification_result"], "passed")
        summary_path = self.worktree / "step-summary.md"
        runner.write_step_summary(
            self.worktree,
            {**env, "GITHUB_STEP_SUMMARY": str(summary_path)},
            0,
        )
        summary = summary_path.read_text(encoding="utf-8")
        self.assertIn(f"Selected model: {GO_FALLBACK}", summary)
        self.assertNotIn("test-key", summary)

    def test_empty_catalog_uses_the_go_fallback(self) -> None:
        discovery = runner.ModelDiscovery("succeeded", models=())
        env = {**self.env, "OPENCODE_API_KEY": "test-key"}
        command = self.write_agent(self.render_source())

        result = self.run_agent(command, discovery=discovery, env=env)

        self.assertEqual(result, 0)
        self.assertEqual(self.outcome()["model"], GO_FALLBACK)
        self.assertEqual(self.outcome()["verification_result"], "passed")

    def test_go_fallback_is_skipped_without_credentials(self) -> None:
        discovery = runner.ModelDiscovery("succeeded", models=())

        result = self.run_agent(
            self.write_agent(self.render_source()),
            discovery=discovery,
        )

        self.assertNotEqual(result, 0)
        self.assertEqual(self.attempt_dirs(), [])
        self.assertFalse(self.discovery_record()["go_fallback"]["credential_available"])
        summary_path = self.worktree / "step-summary.md"
        runner.write_step_summary(
            self.worktree,
            {**self.env, "GITHUB_STEP_SUMMARY": str(summary_path)},
            1,
        )
        self.assertIn("Go fallback: skipped", summary_path.read_text(encoding="utf-8"))

    def test_failed_discovery_does_not_use_the_go_fallback(self) -> None:
        env = {**self.env, "OPENCODE_API_KEY": "test-key"}
        models_command = self.write_catalog_cli("", exit_code=1)
        command = self.write_agent(self.render_source())

        result = self.run_agent(
            command, models_command=models_command, discovery=None, env=env
        )

        self.assertNotEqual(result, 0)
        self.assertEqual(self.attempt_dirs(), [])
        record = self.discovery_record()
        self.assertEqual(record["status"], "failed")
        self.assertEqual(record["eligible_free_models"], [])
        self.assertFalse((self.worktree / "models.txt").exists())
        summary_path = self.worktree / "step-summary.md"
        runner.write_step_summary(
            self.worktree,
            {**env, "GITHUB_STEP_SUMMARY": str(summary_path)},
            1,
        )
        summary = summary_path.read_text(encoding="utf-8")
        self.assertIn("Model discovery: failed", summary)
        self.assertNotIn("Go fallback: available", summary)

    def test_malformed_catalog_does_not_use_the_go_fallback(self) -> None:
        env = {**self.env, "OPENCODE_API_KEY": "test-key"}
        models_command = self.write_catalog_cli("not a model catalog\n")
        command = self.write_agent(self.render_source())

        result = self.run_agent(
            command, models_command=models_command, discovery=None, env=env
        )

        self.assertNotEqual(result, 0)
        self.assertEqual(self.attempt_dirs(), [])
        record = self.discovery_record()
        self.assertEqual(record["status"], "failed")
        self.assertEqual(record["error"], "model catalog output is malformed")

    def test_discovery_cancellation_stops_the_catalog_process(self) -> None:
        models_command = self.write_catalog_cli(catalog_text([]), delay=2)
        cancellation = threading.Event()
        cancellation.set()
        start = time.monotonic()

        discovery = runner.discover_models(
            models_command,
            cwd=self.worktree,
            env=self.env,
            deadline=start + 2,
            config=runner.RunConfig(
                deadline_seconds=2,
                poll_interval_seconds=0.01,
                term_grace_seconds=0.3,
                cleanup_timeout_seconds=1,
                discovery_timeout_seconds=1,
            ),
            cancellation=cancellation,
        )

        self.assertEqual(discovery.status, "cancelled")
        self.assertLess(time.monotonic() - start, 1.5)

    def test_discovery_cleanup_failure_is_reported(self) -> None:
        models_command = self.write_catalog_cli(catalog_text([]), delay=2)
        real_stop = runner.stop_process_group

        def fail_cleanup(process, config):
            real_stop(process, config)
            return False

        start = time.monotonic()
        with mock.patch.object(runner, "stop_process_group", side_effect=fail_cleanup):
            discovery = runner.discover_models(
                models_command,
                cwd=self.worktree,
                env=self.env,
                deadline=start + 2,
                config=runner.RunConfig(
                    deadline_seconds=2,
                    poll_interval_seconds=0.01,
                    term_grace_seconds=0.3,
                    cleanup_timeout_seconds=1,
                    discovery_timeout_seconds=0.2,
                ),
            )

        self.assertEqual(discovery.status, "failed")
        self.assertEqual(discovery.error, "model discovery process cleanup failed")

    def test_discovery_timeout_consumes_the_shared_deadline(self) -> None:
        models_command = self.write_catalog_cli(catalog_text([]), delay=2)
        start = time.monotonic()
        discovery = runner.discover_models(
            models_command,
            cwd=self.worktree,
            env=self.env,
            deadline=start + 1.2,
            config=runner.RunConfig(
                deadline_seconds=1.2,
                poll_interval_seconds=0.01,
                term_grace_seconds=0.12,
                cleanup_timeout_seconds=0.4,
                discovery_timeout_seconds=0.2,
            ),
        )

        self.assertEqual(discovery.status, "deadline")
        self.assertLess(time.monotonic() - start, 1.1)

    def test_live_catalog_refresh_adds_and_removes_free_models(self) -> None:
        first_text = catalog_text(
            [
                {"model_id": PREFERRED, "variants": ["high"]},
                {"model_id": "opencode/retired-free"},
            ]
        )
        second_text = catalog_text([{"model_id": "opencode/new-free"}])

        first = runner.discover_models(
            self.write_catalog_cli(first_text),
            cwd=self.worktree,
            env=self.env,
            deadline=time.monotonic() + 1,
            config=self.config,
        )
        second = runner.discover_models(
            self.write_catalog_cli(second_text),
            cwd=self.worktree,
            env=self.env,
            deadline=time.monotonic() + 1,
            config=self.config,
        )

        self.assertEqual(first.status, "succeeded")
        self.assertEqual(
            [candidate.model_id for candidate in first.candidates],
            [PREFERRED, "opencode/retired-free"],
        )
        self.assertEqual(
            [candidate.model_id for candidate in second.candidates],
            ["opencode/new-free"],
        )

    def test_workflow_summary_reports_attempt_durations_and_result(self) -> None:
        attempt_dir = self.worktree / ".pr-walkthrough" / "attempt-1"
        attempt_dir.mkdir(parents=True)
        (attempt_dir / "outcome.json").write_text(
            json.dumps(
                {
                    "model": PREFERRED,
                    "elapsed_seconds": 4.25,
                    "raw_exit_status": -signal.SIGTERM,
                    "stop_reason": "render_complete",
                    "verification_result": "passed",
                    "provider_error": None,
                }
            ),
            encoding="utf-8",
        )
        discovery_dir = self.worktree / ".pr-walkthrough" / "discovery"
        discovery_dir.mkdir(parents=True)
        (discovery_dir / "discovery.json").write_text(
            json.dumps(
                {
                    "status": "succeeded",
                    "elapsed_seconds": 0.5,
                    "eligible_free_models": [PREFERRED],
                    "go_fallback": {"credential_available": True},
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
        self.assertIn(PREFERRED, summary)
        self.assertIn("Model discovery: 1 eligible free model(s)", summary)
        self.assertIn("Discovery time: 0.50s", summary)
        self.assertIn("Selected model", summary)

    def test_workflow_summary_reports_cleanup_failure(self) -> None:
        attempt_dir = self.worktree / ".pr-walkthrough" / "attempt-1"
        attempt_dir.mkdir(parents=True)
        (attempt_dir / "outcome.json").write_text(
            json.dumps(
                {
                    "model": PREFERRED,
                    "elapsed_seconds": 1.25,
                    "raw_exit_status": 0,
                    "stop_reason": "cleanup_failure",
                    "verification_result": "cleanup failed",
                    "provider_error": None,
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
        self.assertIn(f"Attempt 1: {PREFERRED}, cleanup_failure", summary)
        self.assertIn("verification cleanup failed", summary)
        self.assertNotIn("verification not run", summary)

    def test_workflow_summary_orders_double_digit_attempts_numerically(self) -> None:
        for number, model, stop_reason in (
            (2, "opencode/second", "model_quota"),
            (10, "opencode/tenth", "model_rate_limit"),
        ):
            attempt_dir = self.worktree / ".pr-walkthrough" / f"attempt-{number}"
            attempt_dir.mkdir(parents=True)
            (attempt_dir / "outcome.json").write_text(
                json.dumps(
                    {
                        "model": model,
                        "elapsed_seconds": 1.0,
                        "raw_exit_status": 1,
                        "stop_reason": stop_reason,
                        "verification_result": "not_run",
                        "provider_error": stop_reason,
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
        self.assertLess(
            summary.index("Attempt 1: opencode/second"),
            summary.index("Attempt 2: opencode/tenth"),
        )
        self.assertIn("Attempt 2: opencode/tenth, model_rate_limit", summary)

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
