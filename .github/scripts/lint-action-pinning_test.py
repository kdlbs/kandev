#!/usr/bin/env python3
"""Regression tests for the workflow action pinning linter."""

import shutil
import subprocess
import sys
import tempfile
import textwrap
import unittest
from pathlib import Path


REPO_ROOT = Path(__file__).resolve().parents[2]
LINTER = REPO_ROOT / ".github" / "scripts" / "lint-action-pinning.py"


class LintActionPinningTest(unittest.TestCase):
    def run_lint(self, workflow: str) -> subprocess.CompletedProcess[str]:
        with tempfile.TemporaryDirectory() as tmp:
            tmp_path = Path(tmp)
            scripts_dir = tmp_path / ".github" / "scripts"
            workflows_dir = tmp_path / ".github" / "workflows"
            scripts_dir.mkdir(parents=True)
            workflows_dir.mkdir(parents=True)

            shutil.copy2(LINTER, scripts_dir / "lint-action-pinning.py")
            (workflows_dir / "test.yml").write_text(textwrap.dedent(workflow).strip() + "\n")

            return subprocess.run(
                [sys.executable, str(scripts_dir / "lint-action-pinning.py")],
                capture_output=True,
                text=True,
                check=False,
            )

    def test_rejects_docker_action_pinned_to_tag(self) -> None:
        result = self.run_lint(
            """
            name: test
            on: push
            jobs:
              test:
                runs-on: ubuntu-latest
                steps:
                  - uses: docker://alpine:latest
            """
        )

        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0, output)
        self.assertIn("docker://alpine:latest", output)

    def test_rejects_dynamic_uses_value(self) -> None:
        result = self.run_lint(
            """
            name: test
            on: push
            jobs:
              test:
                runs-on: ubuntu-latest
                steps:
                  - uses: ${{ inputs.action }}
            """
        )

        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0, output)
        self.assertIn("${{ inputs.action }}", output)

    def test_rejects_docker_action_with_short_digest(self) -> None:
        result = self.run_lint(
            """
            name: test
            on: push
            jobs:
              test:
                runs-on: ubuntu-latest
                steps:
                  - uses: docker://alpine@sha256:abc123
            """
        )

        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0, output)
        self.assertIn("docker://alpine@sha256:abc123", output)

    def test_rejects_docker_action_pinned_to_non_digest_ref(self) -> None:
        result = self.run_lint(
            """
            name: test
            on: push
            jobs:
              test:
                runs-on: ubuntu-latest
                steps:
                  - uses: docker://alpine@v1.0
            """
        )

        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0, output)
        self.assertIn("docker://alpine@v1.0", output)

    def test_rejects_unpinned_action_without_at_sign(self) -> None:
        result = self.run_lint(
            """
            name: test
            on: push
            jobs:
              test:
                runs-on: ubuntu-latest
                steps:
                  - uses: actions/checkout
            """
        )

        output = result.stdout + result.stderr
        self.assertNotEqual(result.returncode, 0, output)
        self.assertIn("uses: actions/checkout", output)

    def test_allows_pinned_action_local_action_and_docker_digest(self) -> None:
        digest = "a" * 64
        result = self.run_lint(
            f"""
            name: test
            on: push
            jobs:
              test:
                runs-on: ubuntu-latest
                steps:
                  - uses: actions/checkout@df4cb1c069e1874edd31b4311f1884172cec0e10 # v6
                  - uses: ./.github/actions/setup-fixture
                  - uses: ./.github/workflows/reusable.yml@main
                  - uses: docker://alpine@sha256:{digest}
            """
        )

        output = result.stdout + result.stderr
        self.assertEqual(result.returncode, 0, output)


if __name__ == "__main__":
    unittest.main()
