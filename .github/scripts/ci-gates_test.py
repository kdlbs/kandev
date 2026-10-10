#!/usr/bin/env python3
"""Execute required-gate shell programs against dependency outcome fixtures."""
import os
from pathlib import Path
import re
import subprocess
import tempfile
import textwrap
import unittest

ROOT = Path(__file__).resolve().parents[2]
WORKFLOWS = ROOT / ".github/workflows"


def gate(workflow, job):
    source = (WORKFLOWS / workflow).read_text().split(f"  {job}:\n", 1)[1]
    source = re.split(r"\n  [\w-]+:\n", source)[0]
    step = source.rsplit("      - name: ", 1)[1]
    script = textwrap.dedent(step.split("        run: |\n", 1)[1])
    env = {key: "success" for key in re.findall(r"^          (\w+_RESULT):", step, re.M)}
    return source, script, env


class GateTests(unittest.TestCase):
    # @covers AC-PLATFORM-CI-PERFORMANCE-005.4
    def test_all_required_gates_reject_unknown_skips_failures_and_cancellations(self):
        for workflow, job in [("frontend-tests.yml", "frontend-gate"),
                              ("backend-tests.yml", "test"), ("e2e-tests.yml", "e2e-report")]:
            with self.subTest(workflow=workflow), tempfile.TemporaryDirectory() as directory:
                source, script, env = gate(workflow, job)
                env.update(RUN_REQUIRED="true", SIDEBAR_REQUIRED="false",
                           RUNNER_PLAN="{}", SKIP_REASON="irrelevant-paths",
                           GITHUB_STEP_SUMMARY=str(Path(directory)/"summary"))
                # The consolidated report must prove reports exist, not only green shard jobs.
                for filename in ("apps/web/e2e/retry-summary.json", "apps/web/e2e/timing-profile.json", "apps/web/e2e/merged-report.json"):
                    target = Path(directory) / filename
                    target.parent.mkdir(parents=True, exist_ok=True)
                    target.write_text("{}")
                if "SIDEBAR_RESOURCES_RESULT" in env:
                    env["SIDEBAR_RESOURCES_RESULT"] = "skipped"

                def run(overrides):
                    return subprocess.run(["bash", "-c", script], env={**os.environ, **env, **overrides},
                                          cwd=directory, capture_output=True, text=True).returncode

                self.assertEqual(run({}), 0)
                self.assertNotEqual(run({"RUNNER_PLAN": ""}), 0)
                for key in [k for k in env if k.endswith("_RESULT") and k != "SIDEBAR_RESOURCES_RESULT"]:
                    for outcome in ("failure", "cancelled", "skipped", ""):
                        self.assertNotEqual(run({key: outcome}), 0, (workflow, key, outcome))
                skipped = {k: "skipped" for k in env if k.endswith("_RESULT") and k not in ("CHANGES_RESULT", "REPORT_RESULT")}
                self.assertEqual(run({**skipped, "RUN_REQUIRED": "false"}), 0)
                self.assertNotEqual(run({**skipped, "RUN_REQUIRED": ""}), 0)
                if job == "test":
                    self.assertEqual(run({**skipped, "RUN_REQUIRED": "false", "SIDEBAR_REQUIRED": "true", "SIDEBAR_RESOURCES_RESULT": "success"}), 0)
                    self.assertNotEqual(run({"SIDEBAR_REQUIRED": "true", "SIDEBAR_RESOURCES_RESULT": "skipped"}), 0)
                    self.assertNotEqual(run({"SIDEBAR_REQUIRED": ""}), 0)

                if job == "e2e-report":
                    self.assertIn("name: E2E Tests Passed", source)
                    self.assertIn("if: always()", source)
                    Path(directory, "apps/web/e2e/merged-report.json").unlink()
                    self.assertNotEqual(run({}), 0)

    def test_unknown_pr_base_runs_e2e_instead_of_creating_an_exemption(self):
        workflow = (WORKFLOWS / "e2e-tests.yml").read_text()
        step = workflow.split("      - name: Detect relevant changes\n", 1)[1].split("      - name:", 1)[0]
        script = textwrap.dedent(step.split("        run: |\n", 1)[1])
        with tempfile.TemporaryDirectory() as directory:
            subprocess.run(["git", "init", "-q", directory], check=True)
            output = Path(directory) / "outputs"
            result = subprocess.run(["bash", "-c", script], cwd=directory, capture_output=True, text=True,
                                    env={**os.environ, "EVENT_NAME": "pull_request", "BASE_REF": "missing",
                                         "GITHUB_OUTPUT": str(output)})
            self.assertEqual(result.returncode, 0, result.stderr)
            self.assertIn("run=true", output.read_text())

    def test_each_shard_requires_its_blob_upload(self):
        workflow = (WORKFLOWS / "e2e-tests.yml").read_text()
        for name in ("Upload blob report", "Upload containers blob report"):
            step = workflow.split(f"      - name: {name}\n", 1)[1].split("      - name:", 1)[0]
            self.assertIn("if-no-files-found: error", step)

    def test_go_test_exemption_only_overrides_a_verified_relevant_pr_diff(self):
        workflow = (WORKFLOWS / "e2e-tests.yml").read_text()
        self.assertIn("run: ${{ steps.test-only.outputs.run || steps.detect.outputs.run }}", workflow)
        self.assertIn("if: github.event_name == 'pull_request' && steps.detect.outputs.run == 'true' && steps.detect.outputs.base != ''", workflow)
        self.assertIn("BASE_SHA: ${{ steps.detect.outputs.base }}", workflow)
        self.assertIn('ci-test-only-changes.py --base "$BASE_SHA" --event pull_request', workflow)
        self.assertIn("SKIP_REASON: ${{ needs.changes.outputs.reason }}", workflow)

    def test_bootstrap_does_not_wait_for_a_separate_allocation_job(self):
        for workflow in ("backend-tests.yml", "frontend-tests.yml", "e2e-tests.yml"):
            source = (WORKFLOWS / workflow).read_text()
            self.assertNotIn("  runner_plan:\n", source)
            changes = re.split(r"\n  [\w-]+:\n", source.split("  changes:\n", 1)[1])[0]
            self.assertIn("runs-on: ubuntu-latest", changes)
            self.assertNotIn("    needs:", changes)
            self.assertIn("plan: ${{ steps.plan.outputs.plan }}", changes)
        self.assertNotIn("  e2e-gate:\n", (WORKFLOWS / "e2e-tests.yml").read_text())


if __name__ == "__main__":
    unittest.main()
