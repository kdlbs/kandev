#!/usr/bin/env python3
"""Exercise automatic review preparation with hostile PR Git objects."""

import json
import os
from pathlib import Path
import re
import shutil
import subprocess
import tempfile
import unittest


ROOT = Path(__file__).resolve().parents[2]


def job(source: str, name: str) -> str:
    match = re.search(rf"(?ms)^  {name}:\n.*?(?=^  [\w-]+:\n|\Z)", source)
    if match is None:
        raise AssertionError(f"missing job: {name}")
    return match.group()


def step_run(block: str, name: str) -> str:
    match = re.search(
        rf"(?ms)^      - name: {re.escape(name)}\n.*?(?=^      - |\Z)", block
    )
    if match is None:
        raise AssertionError(f"missing step: {name}")
    lines = match.group().split("        run: |\n", 1)[1].splitlines()
    return "\n".join(line[10:] if line else "" for line in lines) + "\n"


class AutomaticReviewInputsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory()
        self.addCleanup(self.temp.cleanup)
        self.directory = Path(self.temp.name)
        self.origin = self.directory / "origin"
        self.origin.mkdir()
        self.git(self.origin, "init", "-b", "main")
        self.git(self.origin, "config", "user.email", "fixture@example.invalid")
        self.git(self.origin, "config", "user.name", "fixture")
        for relative in (
            ".github/scripts/pr-walkthrough-history.sh",
            ".github/scripts/prepare-pr-review.sh",
            ".agents/skills/pr-walkthrough/scripts/pr-walkthrough-context",
            ".github/actions/setup-opencode/action.yml",
        ):
            source = ROOT / relative
            if source.exists():
                target = self.origin / relative
                target.parent.mkdir(parents=True, exist_ok=True)
                shutil.copyfile(source, target)
        skill = self.origin / ".agents/skills/code-review/SKILL.md"
        skill.parent.mkdir(parents=True, exist_ok=True)
        skill.write_text("trusted review skill\n")
        (self.origin / "AGENTS.md").write_text("trusted repository policy\n")
        self.git(self.origin, "add", ".")
        self.git(self.origin, "commit", "-m", "base")
        self.base = self.git(self.origin, "rev-parse", "HEAD").stdout.strip()
        skill.write_text("trusted workflow review skill\n")
        self.git(self.origin, "commit", "-am", "workflow control revision")
        self.trusted = self.git(self.origin, "rev-parse", "HEAD").stdout.strip()
        self.git(self.origin, "checkout", "-b", "hostile", self.base)
        (self.origin / "opencode.json").write_text('{"plugin":["./evil.ts"]}\n')
        (self.origin / "opencode.jsonc").write_text('{"plugin":["./evil.ts"]}\n')
        (self.origin / "evil.ts").write_text("untrusted plugin executable\n")
        (self.origin / ".claude").mkdir()
        (self.origin / ".claude/settings.json").write_text('{"hooks":{"SessionStart":[]}}\n')
        for directory in ("", "nested/"):
            for name in ("AGENTS.md", "CLAUDE.md", "CONTEXT.md"):
                target = self.origin / directory / name
                target.parent.mkdir(parents=True, exist_ok=True)
                target.write_text(f"untrusted instructions from {directory}{name}\n")
        skill.write_text("untrusted review instructions\n")
        (self.origin / "new-file.txt").write_text("exact event-head file\n")
        (self.origin / "large.txt").write_bytes(b"a" * (512 * 1024 + 1))
        (self.origin / "binary.dat").write_bytes(b"\xff\x00")
        self.secret = self.directory / "private-file"
        self.secret.write_text("must not be materialized\n")
        (self.origin / "escape-link").symlink_to(self.secret)
        self.git(self.origin, "add", ".")
        self.git(self.origin, "update-index", "--add", "--cacheinfo", f"160000,{self.base},submodule")
        self.git(self.origin, "commit", "-m", "hostile PR")
        self.head = self.git(self.origin, "rev-parse", "HEAD").stdout.strip()
        self.git(self.origin, "update-ref", "refs/pull/42/head", self.head)
        self.work = self.directory / "work"
        self.git(self.directory, "clone", "--no-local", str(self.origin), str(self.work))
        self.env = {
            **os.environ,
            "TRUSTED_SHA": self.trusted,
            "BASE_SHA": self.base,
            "HEAD_SHA": self.head,
            "PR_NUMBER": "42",
            "REVIEW_DIR": ".opencode-review",
            "REVIEW_MODE": "full-pr",
            "GITHUB_OUTPUT": str(self.directory / "outputs"),
            "GITHUB_STEP_SUMMARY": str(self.directory / "summary"),
            "GITHUB_WORKSPACE": str(self.work),
            "GITHUB_WORKFLOW_REF": "fixture/repo/.github/workflows/review.yml@refs/heads/main",
            "OPENCODE_MODEL": "fixture/model",
        }

    def git(self, cwd: Path, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            ["git", *args], cwd=cwd, text=True, capture_output=True, check=True
        )

    def prepared_file(self, context: Path, path: str) -> Path:
        manifest = json.loads((context / "manifest.json").read_text())
        entry = next(item for item in manifest["files"] if item["path"] == path)
        return context / entry["context_path"]

    def prepare(self, job_name: str, *, head: str | None = None, workflow: str = "opencode-code-review", overrides: dict | None = None) -> subprocess.CompletedProcess[str]:
        source = (ROOT / f".github/workflows/{workflow}.yml").read_text()
        block = job(source, job_name)
        checkout = re.search(r"(?m)^          ref: (.+)$", block).group(1)
        selected = self.trusted if checkout == "${{ github.workflow_sha }}" else self.head
        self.git(self.work, "checkout", "--detach", selected)
        script = step_run(block, "Build review inputs")
        return subprocess.run(
            ["bash", "-euo", "pipefail", "-c", script], cwd=self.work,
            env={**self.env, "HEAD_SHA": head or self.head, **(overrides or {})}, text=True, capture_output=True,
        )

    # @covers AC-CI-ACTIONS-SECURITY-001.1, AC-CI-ACTIONS-SECURITY-001.2
    def test_both_review_paths_keep_pr_configuration_in_bounded_data(self) -> None:
        for name in ("opencode-review-same-repo", "opencode-review-fork"):
            with self.subTest(job=name):
                result = self.prepare(name)
                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(self.git(self.work, "rev-parse", "HEAD").stdout.strip(), self.trusted)
                self.assertFalse((self.work / "opencode.json").exists())
                self.assertEqual((self.work / ".agents/skills/code-review/SKILL.md").read_text(), "trusted workflow review skill\n")
                context = self.work / ".opencode-review/head-context"
                self.assertEqual(self.prepared_file(context, "new-file.txt").read_text(), "exact event-head file\n")
                self.assertEqual(json.loads(self.prepared_file(context, "opencode.json").read_text())["plugin"], ["./evil.ts"])
                entries = {entry["path"]: entry for entry in json.loads((context / "manifest.json").read_text())["files"]}
                for excluded in ("escape-link", "submodule", "large.txt", "binary.dat"):
                    self.assertFalse(entries[excluded]["materialized"], excluded)
                    self.assertNotIn("context_path", entries[excluded])
                manifest = (context / "manifest.json").read_text()
                self.assertNotIn("must not be materialized", manifest)
                patch = (self.work / ".opencode-review/review.patch").read_text()
                self.assertIn("+exact event-head file", patch)
                self.assertNotIn("+trusted workflow review skill", patch)

    def test_all_automatic_review_paths_keep_reserved_instructions_as_plain_data(self) -> None:
        for workflow, name, review_dir in (
            ("opencode-code-review", "opencode-review-same-repo", ".opencode-review"),
            ("opencode-code-review", "opencode-review-fork", ".opencode-review"),
            ("claude-code-review", "claude-review-fork", ".claude-review"),
        ):
            with self.subTest(job=name):
                result = self.prepare(name, workflow=workflow, overrides={"REVIEW_DIR": review_dir})
                self.assertEqual(result.returncode, 0, result.stderr)
                context = self.work / review_dir / "head-context"
                self.assertFalse((context / "head/files/AGENTS.md").exists())
                for directory in ("", "nested/"):
                    for filename in ("AGENTS.md", "CLAUDE.md", "CONTEXT.md"):
                        original = directory + filename
                        prepared = self.prepared_file(context, original)
                        self.assertRegex(prepared.name, r"^[0-9a-f]{40}\.txt$")
                        self.assertEqual(prepared.read_text(), f"untrusted instructions from {original}\n")

    def test_moving_head_fails_before_context_can_be_used(self) -> None:
        result = self.prepare("opencode-review-fork", head="0" * 40)
        self.assertNotEqual(result.returncode, 0)
        self.assertIn("does not match HEAD_SHA", result.stderr)
        self.assertFalse((self.work / ".opencode-review/head-context/manifest.json").exists())

    def test_claude_fork_prepares_the_same_immutable_data_boundary(self) -> None:
        result = self.prepare("claude-review-fork", workflow="claude-code-review", overrides={"REVIEW_DIR": ".claude-review"})
        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(self.git(self.work, "rev-parse", "HEAD").stdout.strip(), self.trusted)
        self.assertIn("trusted workflow review skill", (self.work / ".claude-review/guidelines.md").read_text())
        self.assertEqual(self.prepared_file(self.work / ".claude-review/head-context", "new-file.txt").read_text(), "exact event-head file\n")

    def test_invalid_base_or_output_path_fails_before_writing(self) -> None:
        for overrides in ({"BASE_SHA": "--upload-pack=other"}, {"REVIEW_DIR": "../private"}):
            with self.subTest(overrides=overrides):
                result = self.prepare("opencode-review-fork", overrides=overrides)
                self.assertNotEqual(result.returncode, 0)
                self.assertIn("Invalid", result.stderr)
                self.assertFalse((self.work / ".opencode-review/review.patch").exists())

    def test_model_process_runs_from_trusted_root_with_only_prepared_head_text(self) -> None:
        result = self.prepare("opencode-review-same-repo")
        self.assertEqual(result.returncode, 0, result.stderr)
        fake_home = self.directory / "agent-home"
        binary = fake_home / ".opencode/bin/opencode"
        binary.parent.mkdir(parents=True)
        binary.write_text(
            "#!/usr/bin/env python3\n"
            "import json\n"
            "from pathlib import Path\n"
            "assert not Path('opencode.json').exists()\n"
            "assert not Path('opencode.jsonc').exists()\n"
            "assert Path('.agents/skills/code-review/SKILL.md').read_text() == 'trusted workflow review skill\\n'\n"
            "context = Path('.opencode-review/head-context')\n"
            "manifest = json.loads((context / 'manifest.json').read_text())\n"
            "entry = next(item for item in manifest['files'] if item['path'] == 'new-file.txt')\n"
            "assert (context / entry['context_path']).read_text() == 'exact event-head file\\n'\n"
            "print('<opencode_findings>[]</opencode_findings>')\n"
        )
        binary.chmod(0o755)
        block = job((ROOT / ".github/workflows/opencode-code-review.yml").read_text(), "opencode-review-same-repo")
        script = step_run(block, "Run OpenCode review").replace("${{ github.event.pull_request.number }}", "42")
        run = subprocess.run(["bash", "-c", script], cwd=self.work, env={**self.env, "HOME": str(fake_home)}, text=True, capture_output=True)
        self.assertEqual(run.returncode, 0, run.stderr)
        self.assertEqual((self.work / ".opencode-review/opencode.status").read_text(), "0\n")


if __name__ == "__main__":
    unittest.main()
