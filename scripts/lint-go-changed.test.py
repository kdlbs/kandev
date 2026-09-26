#!/usr/bin/env python3
"""Exercise the Go lint hook with real Git/pre-commit and a recording linter."""

import json
import os
import shutil
import subprocess
import tempfile
import textwrap
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]


class ChangedGoLintTest(unittest.TestCase):
    def setUp(self):
        temporary = tempfile.TemporaryDirectory(prefix="go lint hook ")
        self.addCleanup(temporary.cleanup)
        self.repo = Path(temporary.name)
        self.calls = self.repo / "lint-calls.jsonl"
        self.env = {key: value for key, value in os.environ.items() if not key.startswith("GIT_")}
        self.env.pop("SKIP", None)
        self.env["GO_LINT_TEST_CALLS"] = str(self.calls)
        self.env["PATH"] = f"{self.repo / 'bin'}{os.pathsep}{self.env['PATH']}"
        self.write(
            "bin/golangci-lint",
            textwrap.dedent("""\
                #!/usr/bin/env python3
                import json
                import os
                import sys

                with open(os.environ["GO_LINT_TEST_CALLS"], "a") as calls:
                    calls.write(json.dumps({"cwd": os.getcwd(), "args": sys.argv[1:]}) + "\\n")
                sys.exit(int(os.environ.get("GO_LINT_TEST_EXIT", "0")))
                """),
        ).chmod(0o755)
        shutil.copy2(ROOT / ".pre-commit-config.yaml", self.repo)
        (self.repo / "scripts").mkdir()
        for name in ("resolve-go-lint-base", "lint-go-changed"):
            source = ROOT / "scripts" / name
            if source.exists():
                shutil.copy2(source, self.repo / "scripts" / name)
        self.write("apps/backend/internal/first/one.go", "package first\n")
        self.write("apps/backend/internal/first/two_test.go", "package first\n")
        self.write("apps/backend/internal/second/other.go", "package second\n")
        self.git("init", "-q", "-b", "main")
        self.git("config", "core.hooksPath", str(self.repo / "no-hooks"))
        self.git("config", "user.name", "Hook test")
        self.git("config", "user.email", "hook@example.test")
        self.git("add", ".")
        self.git("-c", "commit.gpgsign=false", "commit", "-qm", "initial")
        self.git("update-ref", "refs/remotes/origin/main", "HEAD")
        self.git("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
        self.base = self.git("rev-parse", "HEAD").stdout.strip()

    def write(self, relative, content):
        target = self.repo / relative
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(content, encoding="utf-8")
        return target

    def git(self, *args):
        return subprocess.run(
            ["git", *args], cwd=self.repo, env=self.env, text=True, capture_output=True, check=True
        )

    def run_hook(self, *args):
        return subprocess.run(
            ["pre-commit", "run", "go-lint", *args],
            cwd=self.repo, env=self.env, text=True, capture_output=True,
        )

    def lint_calls(self):
        if not self.calls.exists():
            return []
        return [json.loads(line) for line in self.calls.read_text().splitlines()]

    def test_lints_each_staged_package_once(self):
        for filename in ("one.go", "two_test.go"):
            self.write(f"apps/backend/internal/first/{filename}", "package first\n// staged\n")
        self.git("add", "apps/backend/internal/first")
        unstaged = "package second\n// unstaged\n"
        other = self.write("apps/backend/internal/second/other.go", unstaged)

        result = self.run_hook()

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.lint_calls(), [{
            "cwd": str(self.repo / "apps/backend"),
            "args": ["run", "./internal/first", f"--new-from-rev={self.base}", "--timeout=5m"],
        }])
        self.assertEqual(other.read_text(), unstaged)

    def test_lints_multiple_packages_in_one_process_without_recursing(self):
        files = [f"apps/backend/internal/first/file{index}.go" for index in range(12)]
        files.extend([
            "apps/backend/internal/first/child/child.go",
            "apps/backend/internal/second/other.go",
            "apps/backend/main.go",
        ])
        for filename in files:
            self.write(filename, "package fixture\n")

        result = self.run_hook("--files", *files)

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        calls = self.lint_calls()
        self.assertEqual(len(calls), 1, calls)
        self.assertCountEqual(calls[0]["args"], [
            "run", ".", "./internal/first", "./internal/first/child", "./internal/second",
            f"--new-from-rev={self.base}", "--timeout=5m",
        ])

    def test_non_go_commit_skips_lint(self):
        self.write("README.md", "Fixture\n")
        self.git("add", "README.md")

        result = self.run_hook()

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.lint_calls(), [])

    def test_empty_package_selection_never_falls_back_to_full_lint(self):
        for args in ([], ["README.md", "apps/web/example.ts", "other/example.go"]):
            with self.subTest(args=args):
                result = subprocess.run(
                    ["bash", "scripts/lint-go-changed", *args],
                    cwd=self.repo, env=self.env, text=True, capture_output=True,
                )
                self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
                self.assertEqual(self.lint_calls(), [])

    def test_lint_failure_blocks_commit(self):
        self.env["GO_LINT_TEST_EXIT"] = "7"

        result = self.run_hook("--files", "apps/backend/internal/first/one.go")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("exit code: 7", result.stdout)
        self.assertEqual(len(self.lint_calls()), 1)

    def test_invalid_comparison_base_fails_before_lint(self):
        self.git("config", "kandev.lintBaseRef", "origin/missing")

        result = self.run_hook("--files", "apps/backend/internal/first/one.go")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("base ref does not resolve locally", result.stdout)
        self.assertEqual(self.lint_calls(), [])

    def test_ignores_inherited_repository_environment(self):
        env = {
            **self.env,
            "GIT_DIR": str(self.repo / "foreign.git"),
            "GIT_WORK_TREE": str(self.repo / "foreign-worktree"),
            "GIT_COMMON_DIR": str(self.repo / "foreign-common"),
            "GIT_INDEX_FILE": str(self.repo / "foreign-index"),
            "GIT_OBJECT_DIRECTORY": str(self.repo / "foreign-objects"),
            "GIT_ALTERNATE_OBJECT_DIRECTORIES": str(self.repo / "foreign-alternates"),
        }

        result = subprocess.run(
            ["bash", "scripts/lint-go-changed", "apps/backend/internal/first/one.go"],
            cwd=self.repo, env=env, text=True, capture_output=True,
        )

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
        self.assertEqual(self.lint_calls(), [{
            "cwd": str(self.repo / "apps/backend"),
            "args": ["run", "./internal/first", f"--new-from-rev={self.base}", "--timeout=5m"],
        }])


if __name__ == "__main__":
    unittest.main()
