#!/usr/bin/env python3
import json
import subprocess
import sys
import tempfile
import unittest
import zipfile
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "playwright-blob-audit"


def event(method: str, result: dict) -> str:
    return json.dumps({"method": method, "params": {"result": result}})


class PlaywrightBlobAuditTest(unittest.TestCase):
    def run_audit(self, path: Path) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT), str(path)],
            cwd=ROOT,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_passes_for_recursive_zip_report(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            root = Path(directory)
            archive_path = root / "nested" / "report-001.zip"
            archive_path.parent.mkdir()
            with zipfile.ZipFile(archive_path, "w") as archive:
                archive.writestr(
                    "report.jsonl",
                    "\n".join(
                        [
                            event("onTestBegin", {"retry": 0}),
                            event("onTestEnd", {"status": "passed", "errors": []}),
                            event("onTestBegin", {"retry": 0}),
                            event("onTestEnd", {"status": "skipped", "errors": []}),
                        ]
                    )
                    + "\n",
                )

            result = self.run_audit(root)

        self.assertEqual(result.returncode, 0, result.stderr)
        summary = json.loads(result.stdout)
        self.assertEqual(summary["report_count"], 1)
        self.assertEqual(summary["attempt_count"], 2)
        self.assertEqual(summary["retry_attempt_count"], 0)
        self.assertEqual(summary["status_counts"], {"passed": 1, "skipped": 1})

    def test_fails_for_retry_error_and_unexpected_status(self) -> None:
        with tempfile.TemporaryDirectory() as directory:
            report_path = Path(directory) / "report.jsonl"
            report_path.write_text(
                "\n".join(
                    [
                        event("onTestBegin", {"retry": 1}),
                        event("onTestEnd", {"status": "failed", "errors": [{"message": "boom"}]}),
                    ]
                )
                + "\n",
                encoding="utf-8",
            )

            result = self.run_audit(report_path)

        self.assertEqual(result.returncode, 1, result.stderr)
        summary = json.loads(result.stdout)
        self.assertEqual(summary["retry_attempt_count"], 1)
        self.assertEqual(summary["error_result_count"], 1)
        self.assertEqual(summary["unexpected_statuses"], {"failed": 1})
        self.assertEqual(
            summary["violations"],
            ["retry_attempts=1", "error_results=1", "unexpected_statuses={'failed': 1}"],
        )


if __name__ == "__main__":
    unittest.main()
