"""Exercise measurement entry points with small reports and an isolated runner."""
import io
import json
import os
from pathlib import Path
import subprocess
import sys
import tempfile
import unittest
import zipfile

SCRIPTS = Path(__file__).resolve().parent
RUN = "https://github.com/kdlbs/kandev/actions/runs/123"


class MeasurementScriptsTest(unittest.TestCase):
    def setUp(self):
        self.temporary = tempfile.TemporaryDirectory()
        self.addCleanup(self.temporary.cleanup)
        self.root = Path(self.temporary.name)

    def run_script(self, name, *args, env=None):
        return subprocess.run([sys.executable, str(SCRIPTS / name), *map(str, args)],
                              cwd=self.root, env=env, capture_output=True, text=True)

    def test_relative_benchmark_directory_keeps_all_reports_together(self):
        (self.root / "apps/web").mkdir(parents=True)
        binary = self.root / "pnpm"
        binary.write_text("#!/usr/bin/env python3\nimport pathlib,sys\n"
                          "pathlib.Path(sys.argv[sys.argv.index('--outputFile')+1]).write_text('{}')\n")
        binary.chmod(0o755)
        selection = self.root / "selection.json"
        selection.write_text(json.dumps({"node": ["one.test.ts"], "english-browser": []}))
        env = dict(os.environ, PATH=f"{self.root}{os.pathsep}{os.environ['PATH']}")
        result = self.run_script("benchmark-setup.py", "results", selection, env=env)
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        records = json.loads((self.root / "results/benchmark-results.json").read_text())
        self.assertEqual(len(records), 6)
        for record in records:
            report = self.root / f"results/bench-{record['mode']}-{record['sample']}.json"
            self.assertEqual(json.loads(report.read_text()), {})
        self.assertFalse((self.root / "apps/web/.ci-investigation-temporary.config.ts").exists())

    def compare(self, status):
        report = {"success": True, "startTime": 0, "testResults": [{
            "name": "/repo/apps/web/one.test.ts", "endTime": 1000,
            "assertionResults": [{"fullName": "one assertion", "status": status}]}]}
        for name in ("baseline.json", "candidate.json"):
            (self.root / name).write_text(json.dumps(report))
        return self.run_script("compare-setup-reports.py", "baseline.json", "candidate.json")

    def test_skipped_only_reports_are_not_successful_measurements(self):
        result = self.compare("pending")
        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertFalse(json.loads(result.stdout)["all_passed_and_equivalent"])

    def test_passing_matching_reports_are_accepted(self):
        result = self.compare("passed")
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertTrue(json.loads(result.stdout)["all_passed_and_equivalent"])

    def artifact(self, number, url):
        event = {"method": "onProject", "params": {"project": {
            "name": "chromium", "metadata": {"ci": {"buildHref": url}}, "suites": []}}}
        inner = io.BytesIO()
        with zipfile.ZipFile(inner, "w") as blob:
            blob.writestr("report.jsonl", json.dumps(event) + "\n")
        with zipfile.ZipFile(self.root / f"blob-report-{number}.zip", "w") as outer:
            outer.writestr("report.zip", inner.getvalue())

    def profile(self):
        return self.run_script("profile-existing-e2e.py", self.root, "profile.json")

    def test_empty_artifacts_are_rejected_without_output(self):
        result = self.profile()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "profile.json").exists())

    def test_mixed_workflow_runs_are_rejected_without_output(self):
        self.artifact(1, RUN)
        self.artifact(2, RUN + "4")
        result = self.profile()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "profile.json").exists())

    def test_unknown_workflow_is_rejected_without_output(self):
        self.artifact(1, RUN)
        self.artifact(2, "unknown")
        result = self.profile()
        self.assertNotEqual(result.returncode, 0)
        self.assertFalse((self.root / "profile.json").exists())

    def test_same_workflow_marks_unverified_attempt_unknown(self):
        self.artifact(1, RUN)
        self.artifact(2, RUN)
        result = self.profile()
        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        report = json.loads((self.root / "profile.json").read_text())
        self.assertEqual(report["workflow_urls"], [RUN])
        self.assertEqual(report["artifacts"], 2)
        self.assertIsNone(report["run_attempt"])
        self.assertEqual(report["attempt_attribution"], "unknown")


if __name__ == "__main__":
    unittest.main()
