#!/usr/bin/env python3
import importlib.util
import os
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / ".github" / "scripts" / "lint-harness-files.py"


def load_linter():
    assert SCRIPT.exists(), f"{SCRIPT} does not exist"
    spec = importlib.util.spec_from_file_location("lint_harness_files", SCRIPT)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class HarnessLinterTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.workdir = Path(self.tmp.name)

    def write(self, relative: str, content: str) -> Path:
        path = self.workdir / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
        return path

    def init_git(self) -> None:
        subprocess.run(["git", "init"], cwd=self.workdir, check=True, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
        subprocess.run(["git", "add", "."], cwd=self.workdir, check=True, stdout=subprocess.DEVNULL)

    def run_cli(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT), *args],
            cwd=self.workdir,
            text=True,
            capture_output=True,
            check=False,
        )

    def test_classifies_kandev_harness_file_kinds(self) -> None:
        linter = load_linter()

        cases = {
            "AGENTS.md": "agent",
            "nested/CLAUDE.md": "agent",
            "nested/.cursorrules": "agent",
            ".agents/skills/fix/SKILL.md": "skill",
            ".augment/skills/fix/SKILL.md": "skill",
            ".claude/skills/fix/SKILL.md": "skill",
            "apps/backend/internal/office/configloader/skills/memory/SKILL.md": "skill",
            ".agents/skills/fix/references/troubleshooting.md": "reference",
            ".claude/skills/fix/references/troubleshooting.md": "reference",
            "apps/backend/internal/office/configloader/skills/memory/REFERENCE.md": "reference",
            ".agents/agents/qa.md": "role-agent",
            ".opencode/agents/qa.md": "role-agent",
            ".claude/agents/qa.md": "role-agent",
            ".codex/agents/qa.toml": "role-agent",
            ".codex/config.toml": "config",
            ".claude/commands/pr-fixup.md": "command",
            ".cursor/rules/kandev-harness.mdc": "cursor-rule",
            "README.md": "unknown",
        }

        for relative, expected in cases.items():
            with self.subTest(relative=relative):
                self.assertEqual(linter.classify_path(Path(relative)), expected)

    def test_line_limits_are_strict_and_disable_on_zero(self) -> None:
        linter = load_linter()
        path = self.write("AGENTS.md", "one\n" * 3)

        self.assertEqual(linter.lint_path(path, "agent", {"agent-line-limit": {"limit": 3}}), [])

        violations = linter.lint_path(path, "agent", {"agent-line-limit": {"limit": 2}})
        self.assertEqual([violation.rule for violation in violations], ["agent-line-limit"])
        self.assertIn("file is 3 lines (limit 2, over by 1)", violations[0].msg)

        self.assertEqual(linter.lint_path(path, "agent", {"agent-line-limit": {"limit": 0}}), [])

    def test_description_word_limit_supports_yaml_frontmatter(self) -> None:
        linter = load_linter()
        path = self.write(
            ".agents/skills/demo/SKILL.md",
            "---\n"
            "name: demo\n"
            "description: one two\n"
            "  three four\n"
            "---\n"
            "\n"
            "# Demo\n",
        )

        violations = linter.lint_path(path, "skill", {"description-word-limit": {"limit": 3}})

        self.assertEqual(len(violations), 1)
        self.assertEqual(violations[0].rule, "description-word-limit")
        self.assertEqual(violations[0].line, 1)
        self.assertIn("description is 4 words", violations[0].msg)

    def test_description_word_limit_ignores_yaml_block_scalar_markers(self) -> None:
        linter = load_linter()
        path = self.write(
            ".agents/skills/demo/SKILL.md",
            "---\n"
            "name: demo\n"
            "description: >\n"
            "  one two three four\n"
            "---\n",
        )

        violations = linter.lint_path(path, "skill", {"description-word-limit": {"limit": 3}})

        self.assertEqual(len(violations), 1)
        self.assertIn("description is 4 words", violations[0].msg)

    def test_description_word_limit_is_silent_when_metadata_missing_or_disabled(self) -> None:
        linter = load_linter()
        missing = self.write(".agents/skills/missing/SKILL.md", "# Missing\n")
        unclosed = self.write(".agents/skills/unclosed/SKILL.md", "---\ndescription: one two three\n")

        for path in (missing, unclosed):
            with self.subTest(path=path):
                self.assertEqual(linter.lint_path(path, "skill", {"description-word-limit": {"limit": 1}}), [])
                self.assertEqual(linter.lint_path(path, "skill", {"description-word-limit": {"limit": 0}}), [])

    def test_description_word_limit_supports_codex_toml_descriptions(self) -> None:
        linter = load_linter()
        path = self.write(
            ".codex/agents/qa.toml",
            'name = "qa"\n'
            'description = "Use when the user says \\"test it\\" before PRs."\n',
        )

        violations = linter.lint_path(path, "role-agent", {"description-word-limit": {"limit": 5}})

        self.assertEqual(len(violations), 1)
        self.assertEqual(violations[0].rule, "description-word-limit")
        self.assertIn("description is 9 words", violations[0].msg)

    def test_opt_in_line_scanners_report_expected_columns(self) -> None:
        linter = load_linter()
        path = self.write(
            "AGENTS.md",
            "ok\n"
            "bad \U0001f600 marker\n"
            "plain \u2014 dash\n"
            "\u8def see @./SKILL.md\n",
        )
        config = {
            "no-emoji": {},
            "no-em-dash": {},
            "no-at-dot-slash": {},
        }

        violations = linter.lint_path(path, "agent", config)

        self.assertEqual([violation.rule for violation in violations], ["no-emoji", "no-em-dash", "no-at-dot-slash"])
        self.assertIn('emoji "', violations[0].msg)
        self.assertIn("column 5", violations[0].msg)
        self.assertIn("column 7", violations[1].msg)
        self.assertIn("column 7", violations[2].msg)

    def test_description_when_supports_yaml_frontmatter_failures(self) -> None:
        linter = load_linter()
        missing_frontmatter = self.write(".agents/skills/a/SKILL.md", "# Missing\n")
        unclosed = self.write(".agents/skills/b/SKILL.md", "---\ndescription: Use when testing.\n")
        missing_description = self.write(".agents/skills/c/SKILL.md", "---\nname: c\n---\n")
        weak_description = self.write(".agents/skills/d/SKILL.md", "---\ndescription: General helper.\n---\n")

        cases = [
            (missing_frontmatter, "missing YAML frontmatter"),
            (unclosed, "frontmatter opens"),
            (missing_description, "has no `description:` field"),
            (weak_description, "lacks a routing phrase"),
        ]

        for path, expected in cases:
            with self.subTest(path=path):
                violations = linter.lint_path(path, "skill", {"description-when": {}})
                self.assertEqual(len(violations), 1)
                self.assertIn(expected, violations[0].msg)

    def test_description_when_supports_yaml_block_scalar_descriptions(self) -> None:
        linter = load_linter()
        valid = self.write(
            ".agents/skills/block/SKILL.md",
            "---\n"
            "name: block\n"
            "description: >\n"
            "  Use when testing. Do this.\n"
            "---\n",
        )
        empty = self.write(
            ".agents/skills/empty/SKILL.md",
            "---\n"
            "name: empty\n"
            "description: >\n"
            "---\n",
        )

        self.assertEqual(linter.lint_path(valid, "skill", {"description-when": {}}), [])

        violations = linter.lint_path(empty, "skill", {"description-when": {}})
        self.assertEqual(len(violations), 1)
        self.assertIn("empty `description:` field", violations[0].msg)

    def test_description_when_supports_toml_failures(self) -> None:
        linter = load_linter()
        missing = self.write(".codex/agents/missing.toml", 'name = "missing"\n')
        weak = self.write(".codex/agents/weak.toml", 'description = "General helper."\n')
        quoted = self.write(".codex/agents/quoted.toml", 'description = "Use when the user says \\"fix it\\"."\n')

        missing_violations = linter.lint_path(missing, "role-agent", {"description-when": {}})
        weak_violations = linter.lint_path(weak, "role-agent", {"description-when": {}})
        quoted_violations = linter.lint_path(quoted, "role-agent", {"description-when": {}})

        self.assertEqual(len(missing_violations), 1)
        self.assertIn('has no `description = "..."` field', missing_violations[0].msg)
        self.assertEqual(len(weak_violations), 1)
        self.assertIn("lacks a routing phrase", weak_violations[0].msg)
        self.assertEqual(quoted_violations, [])

    def test_description_when_uses_explicit_routing_phrases(self) -> None:
        linter = load_linter()
        weak = self.write(
            ".agents/skills/trigger/SKILL.md",
            "---\n"
            "description: This skill dispatches a trigger event to the runtime.\n"
            "---\n",
        )
        strong = self.write(
            ".agents/skills/trigger-on/SKILL.md",
            "---\n"
            "description: Trigger on task review requests. Do review setup.\n"
            "---\n",
        )

        self.assertEqual(len(linter.lint_path(weak, "skill", {"description-when": {}})), 1)
        self.assertEqual(linter.lint_path(strong, "skill", {"description-when": {}}), [])

    def test_codex_config_skips_description_when(self) -> None:
        linter = load_linter()
        config = self.write(".codex/config.toml", 'model = "gpt-5"\n')

        self.assertEqual(linter.lint_path(config, "config", {"description-when": {}}), [])

    def test_cli_all_uses_tracked_files_and_skips_untracked(self) -> None:
        self.write("AGENTS.md", "ok\n")
        self.write(".agents/skills/demo/SKILL.md", "line\n" * 501)
        self.init_git()
        self.write(".agents/skills/untracked/SKILL.md", "line\n" * 501)

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn(".agents/skills/demo/SKILL.md", result.stdout)
        self.assertNotIn("untracked", result.stdout)

    def test_cli_explicit_targets_disable_rules_and_recurse_without_symlink_dirs(self) -> None:
        self.write(".agents/skills/demo/SKILL.md", "line\n" * 501)
        linked_target = self.write("target/AGENTS.md", "line\n" * 301)
        link_dir = self.workdir / "mirror"
        try:
            os.symlink(linked_target.parent, link_dir)
        except OSError:
            link_dir = None

        disabled = self.run_cli("--disable", "skill-line-limit", ".agents/skills/demo/SKILL.md")
        self.assertEqual(disabled.returncode, 0, disabled.stdout + disabled.stderr)

        recursive = self.run_cli(".")
        self.assertEqual(recursive.returncode, 1)
        if link_dir is not None:
            self.assertNotIn("mirror/AGENTS.md", recursive.stdout)

    def test_cli_explicit_targets_can_enable_opt_in_rules(self) -> None:
        self.write("AGENTS.md", "plain \u2014 dash\n")

        default = self.run_cli("AGENTS.md")
        enabled = self.run_cli("--enable", "no-em-dash", "AGENTS.md")
        unknown = self.run_cli("--enable", "not-a-rule", "AGENTS.md")

        self.assertEqual(default.returncode, 0, default.stdout + default.stderr)
        self.assertEqual(enabled.returncode, 1)
        self.assertIn("no-em-dash", enabled.stdout)
        self.assertEqual(unknown.returncode, 2)
        self.assertIn("unknown opt-in rule", unknown.stderr)

    def test_cli_reports_github_annotations(self) -> None:
        self.write("AGENTS.md", "line\n" * 301)
        env = {**os.environ, "GITHUB_ACTIONS": "true"}
        result = subprocess.run(
            [sys.executable, str(SCRIPT), "AGENTS.md"],
            cwd=self.workdir,
            env=env,
            text=True,
            capture_output=True,
            check=False,
        )

        self.assertEqual(result.returncode, 1)
        self.assertIn("::error file=AGENTS.md::", result.stdout)


if __name__ == "__main__":
    unittest.main()
