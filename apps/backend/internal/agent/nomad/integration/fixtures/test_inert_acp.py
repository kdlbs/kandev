import json
import os
import signal
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path


FIXTURE = Path(__file__).with_name("inert_acp.py")


class InertACPTest(unittest.TestCase):
    def test_acp_turn_emits_result_and_records_same_group_descendants(self):
        with tempfile.TemporaryDirectory(prefix="nomad-inert-acp-") as temp:
            identities = Path(temp) / "identities"
            process = subprocess.Popen(
                [sys.executable, str(FIXTURE), "--identity-dir", str(identities)],
                stdin=subprocess.PIPE,
                stdout=subprocess.PIPE,
                stderr=subprocess.PIPE,
                text=True,
                start_new_session=True,
            )
            try:
                self.assertTrue(_wait_for(identities / "acp.json"))
                for request_id, method, params in (
                    (1, "initialize", {}),
                    (2, "session/new", {}),
                    (3, "session/prompt", {"sessionId": "nomad-inert-session", "prompt": []}),
                ):
                    process.stdin.write(json.dumps({"jsonrpc": "2.0", "id": request_id, "method": method, "params": params}) + "\n")
                    process.stdin.flush()

                responses = []
                deadline = time.monotonic() + 5
                while time.monotonic() < deadline and not any(item.get("id") == 3 for item in responses):
                    line = process.stdout.readline()
                    if line:
                        responses.append(json.loads(line))
                result = next(item for item in responses if item.get("id") == 3)
                self.assertEqual(result["result"]["stopReason"], "end_turn")
                self.assertTrue(any(item.get("method") == "session/update" for item in responses))
                self.assertEqual(
                    next(item["params"]["update"]["content"]["text"] for item in responses if item.get("method") == "session/update"),
                    "nomad-inert-result",
                )

                child_file = identities / "child.json"
                grandchild_file = identities / "child.json.grandchild"
                self.assertTrue(_wait_for(child_file))
                self.assertTrue(_wait_for(grandchild_file))
                pids = [json.loads((identities / name).read_text())["pid"] for name in ("acp.json", "child.json", "child.json.grandchild")]
                pgids = [json.loads((identities / name).read_text())["pgid"] for name in ("acp.json", "child.json", "child.json.grandchild")]
                self.assertEqual(len(set(pgids)), 1)
                self.assertEqual(pgids[0], process.pid)
                for pid in pids:
                    os.kill(pid, 0)
            finally:
                try:
                    os.killpg(process.pid, signal.SIGTERM)
                except ProcessLookupError:
                    pass
                try:
                    process.wait(timeout=5)
                except subprocess.TimeoutExpired:
                    os.killpg(process.pid, signal.SIGKILL)
                    process.wait(timeout=5)


def _wait_for(path: Path) -> bool:
    deadline = time.monotonic() + 5
    while time.monotonic() < deadline:
        if path.is_file():
            return True
        time.sleep(0.025)
    return path.is_file()


if __name__ == "__main__":
    unittest.main()
