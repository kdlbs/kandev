#!/usr/bin/env python3
"""Credential-free safety checks for the opt-in provider acceptance runner."""
import importlib.util
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import time
import unittest
from unittest.mock import patch

spec = importlib.util.spec_from_file_location(
    "wrapper_real", Path(__file__).with_name("check-generic-wrapper-real.py")
)
runner = importlib.util.module_from_spec(spec)
sys.modules[spec.name] = runner
spec.loader.exec_module(runner)


# @covers AC-AGENTS-WRAPPERS-004.2, AC-AGENTS-WRAPPERS-004.5, AC-AGENTS-WRAPPERS-005.2
class AcceptanceRunnerTests(unittest.TestCase):
    def test_missing_inputs_refuse_before_starting_any_process(self):
        with patch.dict(os.environ, {}, clear=True), patch.object(subprocess, "Popen") as start:
            self.assertEqual(runner.main(["--run"]), 2)
            start.assert_not_called()

    def test_prerequisite_check_never_starts_an_instance(self):
        with patch.dict(os.environ, {}, clear=True), patch.object(subprocess, "Popen") as start:
            self.assertEqual(runner.main(["--check-prerequisites"]), 2)
            start.assert_not_called()

    def test_cleanup_stops_owned_process_but_preserves_unrelated_process_and_files(self):
        unrelated = subprocess.Popen([sys.executable, "-c", "import time; time.sleep(60)"])
        try:
            with tempfile.TemporaryDirectory() as parent:
                sentinel = Path(parent) / "manual-instance"
                sentinel.write_text("preserve")
                with self.assertRaisesRegex(RuntimeError, "test failure"):
                    with runner.OwnedRun(Path(parent)) as owned:
                        root = owned.root
                        (root / "auth.json").write_text("secret")
                        child = owned.start("child", [sys.executable, "-c", "import time; time.sleep(60)"], {})
                        # A disk record is evidence, never authority to stop a foreign PID.
                        (root / "ownership.json").write_text(json.dumps({"pid": unrelated.pid}))
                        raise RuntimeError("test failure")
                self.assertIsNotNone(child.poll())
                self.assertIsNone(unrelated.poll())
                self.assertEqual(sentinel.read_text(), "preserve")
                self.assertFalse(root.exists())
        finally:
            unrelated.terminate()
            unrelated.wait(timeout=5)

    def test_cleanup_stops_a_detached_owned_descendant_after_its_parent_exits(self):
        with tempfile.TemporaryDirectory() as parent:
            pidfile = Path(parent) / "child.pid"
            with runner.OwnedRun(Path(parent)) as owned:
                code = ("import subprocess,sys,pathlib; "
                        "p=subprocess.Popen([sys.executable,'-c','import time; time.sleep(60)'],start_new_session=True); "
                        f"pathlib.Path({str(pidfile)!r}).write_text(str(p.pid))")
                proc = owned.start("parent", [sys.executable, "-c", code], {})
                proc.wait(timeout=5)
                pid = int(pidfile.read_text())
            def live():
                path = Path(f"/proc/{pid}/stat")
                return path.exists() and path.read_text().split(") ", 1)[1].split()[0] != "Z"
            deadline = time.monotonic() + 5
            while live() and time.monotonic() < deadline:
                time.sleep(0.05)
            try:
                self.assertFalse(live())
            finally:
                if live():
                    os.kill(pid, 9)

    def test_cleanup_stops_a_child_forked_during_term(self):
        with tempfile.TemporaryDirectory() as parent:
            ready = Path(parent) / "ready"
            forked = Path(parent) / "forked"
            owned = runner.OwnedRun(Path(parent))
            code = """import os,signal,time,pathlib
ready, forked = __import__('sys').argv[1:]
def stop(*args):
    if os.fork() == 0:
        os.setsid()
        signal.signal(signal.SIGTERM, signal.SIG_IGN)
        pathlib.Path(forked).write_text(str(os.getpid()))
        while True: time.sleep(1)
    os._exit(0)
signal.signal(signal.SIGTERM, stop)
pathlib.Path(ready).touch()
while True: time.sleep(1)
"""
            launcher = ("import subprocess,sys; subprocess.Popen([sys.executable,'-c'," +
                        repr(code) + ",*sys.argv[1:]],start_new_session=True)")
            try:
                with owned:
                    proc = owned.start("launcher", [sys.executable, "-c", launcher,
                                       str(ready), str(forked)], {})
                    proc.wait(timeout=5)
                    runner.wait_until(ready.exists, timeout=5)
                self.assertTrue(forked.exists(), "TERM handler must create the regression child")
                self.assertEqual(owned._owned_descendants(), {})
            finally:
                # Clean up the failing implementation without trusting PID receipts.
                identities = owned._owned_descendants()
                for pid, birth in identities.items():
                    if owned._owned_descendants().get(pid) == birth:
                        try:
                            os.kill(pid, 9)
                        except ProcessLookupError:
                            pass

    def test_cleanup_does_not_signal_a_reused_pid(self):
        with runner.OwnedRun() as owned:
            with patch.object(owned, "_owned_descendants", side_effect=[
                    {123: "old-birth"}, {123: "new-birth"}, {}]), \
                    patch.object(os, "kill") as kill:
                owned._stop_descendants()
                kill.assert_not_called()

    def test_a_foreign_listener_cannot_be_used_as_the_test_instance(self):
        import socket
        with socket.socket() as listener:
            listener.bind(("127.0.0.1", 0))
            listener.listen()
            with runner.OwnedRun() as owned:
                with self.assertRaises(runner.CheckFailed):
                    runner.require_owned_listener(owned, listener.getsockname()[1])
        with runner.OwnedRun() as owned:
            port = runner.free_port()
            receipt = owned.root / "listening"
            code = ("import socket,time,pathlib; s=socket.socket(); "
                    f"s.bind(('127.0.0.1',{port})); s.listen(); "
                    f"pathlib.Path({str(receipt)!r}).touch(); time.sleep(60)")
            owned.start("server", [sys.executable, "-c", code], {})
            runner.wait_until(receipt.exists, timeout=5)
            self.assertTrue(runner.require_owned_listener(owned, port))

    def test_public_result_drops_raw_logs_paths_and_credentials(self):
        secret = "test-secret-in-raw-trace"
        result = runner.public_result({"scenario": "discovery", "status": "passed", "models": 44,
            "modes": 5, "receipts": 1, "trace": secret, "credential": secret, "path": "/private/auth.json"})
        self.assertEqual(result, {"scenario": "discovery", "status": "passed", "models": 44,
            "modes": 5, "receipts": 1})
        self.assertNotIn(secret, json.dumps(result))
        with self.assertRaises(ValueError):
            runner.public_result({"scenario": "discovery", "status": secret})

    def test_output_without_the_required_tool_sequence_is_rejected(self):
        with tempfile.TemporaryDirectory() as parent:
            case = Path(parent)
            (case / "result.txt").write_text("42\n")
            sessions = case / "agent/sessions"
            sessions.mkdir(parents=True)
            (sessions / "run.jsonl").write_text(json.dumps({"message": {"role": "assistant", "content":
                [{"type": "toolCall", "name": "read"}] * 4}}) + "\n")
            with self.assertRaises(runner.CheckFailed):
                runner.verify_case(case, "42\n", timeout=0.01)

    def test_tool_names_in_the_wrong_order_do_not_prove_the_round_trip(self):
        with tempfile.TemporaryDirectory() as parent:
            case = Path(parent)
            (case / "result.txt").write_text("42\n")
            with patch.object(runner, "tool_calls", return_value=["read", "read", "bash", "write"]):
                with self.assertRaises(runner.CheckFailed):
                    runner.verify_case(case, "42\n", timeout=0.01)
            with patch.object(runner, "tool_calls", return_value=["read", "bash", "write", "read"]):
                self.assertEqual(runner.verify_case(case, "42\n", timeout=0.01), 4)

    def test_node_version_prefix_is_accepted_without_copying_other_output(self):
        self.assertEqual(runner.parse_version("v24.19.0\n"), "24.19.0")
        self.assertEqual(runner.parse_version("Version: 0.3.7\nprivate trailing text"), "0.3.7")

    def test_retained_logs_exclude_the_disposable_credential_file(self):
        with tempfile.TemporaryDirectory() as parent:
            saved = Path(parent) / "saved"
            with runner.OwnedRun(Path(parent), private_logs=saved) as owned:
                (owned.root / "logs/check.log").write_text("private trace")
                (owned.root / "auth.json").write_text("credential")
            self.assertEqual(len(list(saved.rglob("check.log"))), 1)
            self.assertEqual(list(saved.rglob("auth.json")), [])
            self.assertEqual(saved.stat().st_mode & 0o777, 0o700)

    def test_disposable_home_suppresses_first_run_questions_without_enabling_profiles(self):
        with runner.OwnedRun() as owned:
            prefs = owned.root / "home/.config/greywall/preferences.json"
            self.assertEqual(json.loads(prefs.read_text()), {"suppressProfilePrompt": ["pi", "npx"]})
            self.assertFalse((prefs.parent / "profiles").exists())

    def test_wrapper_prefix_uses_runtime_references_and_no_default_proxy_control(self):
        with tempfile.TemporaryDirectory() as parent:
            with runner.OwnedRun(Path(parent)) as owned:
                prefix = runner.wrapper_argv(owned, Path(parent) / "tools", 12345, 12346, 12347)
                self.assertIn("{{kandev.work_dir}}", prefix)
                self.assertIn("{{kandev.managed_npm_dir}}", prefix)
                self.assertIn("--no-network-rules", prefix)
                self.assertIn("--no-credential-protection", prefix)
                self.assertNotIn("--watch", prefix)
                self.assertNotIn("--learning", prefix)
                self.assertNotIn("43080", " ".join(prefix))


if __name__ == "__main__":
    unittest.main()
