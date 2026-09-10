#!/usr/bin/env python3
import importlib.util
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "lint-spec-files.py"


def load_linter():
    spec = importlib.util.spec_from_file_location("lint_spec_files", SCRIPT)
    assert spec is not None
    assert spec.loader is not None
    module = importlib.util.module_from_spec(spec)
    spec.loader.exec_module(module)
    return module


class SpecLinterTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)
        self.config = {
            "version": 1,
            "limits": {
                "guide": 100,
                "legacy": 100,
                "product": 100,
                "requirement": 300,
                "system-design": 300,
                "system-index": 200,
                "template": 100,
            },
        }

    def write(self, relative: str, content: str) -> Path:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
        return path

    def git(self, *args: str) -> None:
        subprocess.run(
            ["git", *args],
            cwd=self.root,
            check=True,
            capture_output=True,
            text=True,
        )

    def valid_requirement(self) -> str:
        return """---
status: active
system: task-system
---

# Dependencies Requirements

### REQ-TASK-DEP-001: Wait for a predecessor

#### Acceptance criteria

- **AC-TASK-DEP-001.1:** When a predecessor is pending, the system shall wait.
"""

    def valid_design(self, requirement: str = "REQ-TASK-DEP-001") -> str:
        return f"""---
status: current
system: task-system
requirements:
  - {requirement}
---

# Dependencies System Design
"""

    def valid_system_index(self, migration: str = "in_progress") -> str:
        return """---
status: draft
system: task-system
specification_version: 1
migration: {migration}
---
# Task System
""".format(migration=migration)

    def rules(self, violations) -> list[str]:
        return [violation.rule for violation in violations]

    def test_accepts_a_valid_requirement_and_system_design(self) -> None:
        self.write("docs/specs/task-system/README.md", self.valid_system_index())
        self.write("docs/specs/task-system/requirements/dependencies.md", self.valid_requirement())
        self.write("docs/specs/task-system/system-design/dependencies.md", self.valid_design())

        self.assertEqual(load_linter().lint_specs(self.root, self.config), [])

    def test_rejects_a_file_that_exceeds_its_artifact_limit(self) -> None:
        self.config["limits"]["requirement"] = 20
        path = self.write("docs/specs/task-system/requirements/dependencies.md", self.valid_requirement())

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("file-size", self.rules(violations))
        size_violation = next(item for item in violations if item.rule == "file-size")
        self.assertEqual(size_violation.path, path)
        self.assertIn("requirement", size_violation.message)

    def test_freezes_an_oversized_legacy_file_at_its_recorded_size(self) -> None:
        path = self.write("docs/specs/legacy/spec.md", "x" * 12)
        self.config["limits"]["legacy"] = 10
        exceptions = {"docs/specs/legacy/spec.md": 12}

        linter = load_linter()
        self.assertEqual(linter.lint_specs(self.root, self.config, exceptions), [])

        path.write_text("x" * 13, encoding="utf-8")
        violations = linter.lint_specs(self.root, self.config, exceptions)
        self.assertIn("file-size", self.rules(violations))
        self.assertIn("frozen ceiling 12", violations[0].message)

    def test_reports_a_stale_legacy_size_exception(self) -> None:
        self.write("docs/specs/legacy/spec.md", "small")
        self.config["limits"]["legacy"] = 10
        exceptions = {"docs/specs/legacy/spec.md": 12}

        violations = load_linter().lint_specs(self.root, self.config, exceptions)

        self.assertEqual(self.rules(violations), ["stale-size-exception"])

    def test_requires_the_legacy_ceiling_to_follow_every_size_reduction(self) -> None:
        self.write("docs/specs/legacy/spec.md", "x" * 11)
        self.config["limits"]["legacy"] = 10
        exceptions = {"docs/specs/legacy/spec.md": 12}

        violations = load_linter().lint_specs(self.root, self.config, exceptions)

        self.assertEqual(self.rules(violations), ["stale-size-exception"])
        self.assertIn("Lower the frozen ceiling to 11", violations[0].message)

    def test_reports_a_redundant_legacy_size_exception(self) -> None:
        self.write("docs/specs/legacy/spec.md", "x" * 8)
        self.config["limits"]["legacy"] = 10
        exceptions = {"docs/specs/legacy/spec.md": 8}

        violations = load_linter().lint_specs(self.root, self.config, exceptions)

        self.assertEqual(self.rules(violations), ["stale-size-exception"])
        self.assertIn("at or below the default limit", violations[0].message)

    def test_rejects_a_raised_legacy_size_ceiling(self) -> None:
        previous_exceptions = {"docs/specs/legacy/spec.md": 12}
        current_exceptions = {"docs/specs/legacy/spec.md": 13}

        violations = load_linter().check_legacy_size_ratchet(
            self.root, current_exceptions, previous_exceptions
        )

        self.assertEqual(self.rules(violations), ["legacy-size-ratchet"])
        self.assertEqual(violations[0].path, self.root / "docs/specs/legacy/spec.md")

    def test_rejects_a_raised_absolute_ceiling_without_crashing(self) -> None:
        previous_exceptions = {"/etc/passwd": 100}
        current_exceptions = {"/etc/passwd": 200}

        violations = load_linter().check_legacy_size_ratchet(
            self.root, current_exceptions, previous_exceptions
        )

        self.assertEqual(self.rules(violations), ["legacy-size-ratchet"])
        # main() calls violation.path.relative_to(root) on every violation; a
        # noncanonical (e.g. absolute) historical path must not produce a
        # violation path that raises there.
        violations[0].path.relative_to(self.root)

    def test_loads_the_previous_ceiling_from_git_history_and_flags_a_raise(self) -> None:
        self.config["limits"]["legacy"] = 10
        self.git("init")
        self.git("config", "user.email", "test@example.com")
        self.git("config", "user.name", "Spec Lint Test")
        self.write("docs/specs/spec-lint-exceptions.tsv", "docs/specs/legacy/spec.md\t12\n")
        self.write("docs/specs/legacy/spec.md", "x" * 12)
        self.git("add", "-A")
        self.git("commit", "-m", "freeze the legacy ceiling at 12")
        self.write("README.md", "placeholder\n")
        self.git("add", "-A")
        self.git("commit", "-m", "unrelated follow-up commit")

        self.write("docs/specs/spec-lint-exceptions.tsv", "docs/specs/legacy/spec.md\t13\n")
        self.write("docs/specs/legacy/spec.md", "x" * 13)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("legacy-size-ratchet", self.rules(violations))

    def test_loads_a_valid_sidecar_from_disk_and_enforces_its_ceiling(self) -> None:
        self.write("docs/specs/spec-lint-exceptions.tsv", "docs/specs/legacy/spec.md\t12\n")
        path = self.write("docs/specs/legacy/spec.md", "x" * 12)
        self.config["limits"]["legacy"] = 10

        self.assertEqual(load_linter().lint_specs(self.root, self.config), [])

        path.write_text("x" * 13, encoding="utf-8")
        violations = load_linter().lint_specs(self.root, self.config)
        self.assertIn("file-size", self.rules(violations))
        self.assertIn("frozen ceiling 12", violations[0].message)

    def test_rejects_a_symlinked_size_exception_sidecar(self) -> None:
        outside = tempfile.TemporaryDirectory()
        self.addCleanup(outside.cleanup)
        target = Path(outside.name) / "not-a-sidecar.txt"
        target.write_text("not a sidecar\n", encoding="utf-8")
        sidecar = self.root / "docs/specs/spec-lint-exceptions.tsv"
        sidecar.parent.mkdir(parents=True, exist_ok=True)
        sidecar.symlink_to(target)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertEqual(self.rules(violations), ["invalid-size-exception-catalog"])

    def test_rejects_noncanonical_size_exception_targets(self) -> None:
        self.write(
            "docs/specs/spec-lint-exceptions.tsv",
            "other/specs/not-a-spec.md\t100\n"
            "docs/specs/legacy/not-a-spec.txt\t100\n",
        )
        self.write("other/specs/not-a-spec.md", "x")
        self.write("docs/specs/legacy/not-a-spec.txt", "x")

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertEqual(self.rules(violations), ["invalid-size-exception", "invalid-size-exception"])

    def test_rejects_an_absolute_size_exception_target_without_crashing(self) -> None:
        self.write("docs/specs/spec-lint-exceptions.tsv", "/etc/passwd\t100\n")

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertEqual(self.rules(violations), ["invalid-size-exception"])
        # main() calls violation.path.relative_to(root) on every violation; an
        # absolute (or otherwise out-of-tree) target must not produce a violation
        # path that raises there.
        for violation in violations:
            violation.path.relative_to(self.root)

    def test_flags_a_malformed_size_exception_line(self) -> None:
        self.write("docs/specs/spec-lint-exceptions.tsv", "docs/specs/legacy/spec.md\n")
        self.write("docs/specs/legacy/spec.md", "x" * 5)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("malformed-size-exception", self.rules(violations))

    def test_flags_a_non_ascii_digit_as_a_malformed_size_exception_line(self) -> None:
        self.write("docs/specs/spec-lint-exceptions.tsv", "docs/specs/legacy/spec.md\t²\n")
        self.write("docs/specs/legacy/spec.md", "x" * 5)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("malformed-size-exception", self.rules(violations))

    def test_flags_a_duplicate_size_exception_path(self) -> None:
        self.config["limits"]["legacy"] = 11
        self.write(
            "docs/specs/spec-lint-exceptions.tsv",
            "docs/specs/legacy/spec.md\t12\ndocs/specs/legacy/spec.md\t13\n",
        )
        self.write("docs/specs/legacy/spec.md", "x" * 12)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertEqual(self.rules(violations), ["duplicate-size-exception"])

    def test_rejects_a_residual_legacy_size_exceptions_key_in_config(self) -> None:
        self.config["legacy_size_exceptions"] = {}

        with self.assertRaisesRegex(ValueError, "legacy_size_exceptions"):
            load_linter().lint_specs(self.root, self.config)

    def test_rejects_a_system_artifact_without_a_system_index(self) -> None:
        self.write("docs/specs/task-system/requirements/dependencies.md", self.valid_requirement())

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("missing-system-index", self.rules(violations))

    def test_requires_system_design_requirements_to_be_a_list(self) -> None:
        design = self.valid_design().replace(
            "requirements:\n  - REQ-TASK-DEP-001", "requirements: REQ-TASK-DEP-001"
        )
        self.write("docs/specs/task-system/system-design/dependencies.md", design)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("system-design-requirements", self.rules(violations))

    def test_rejects_unknown_size_limit_keys(self) -> None:
        self.config["limits"]["legacyy"] = 100

        with self.assertRaisesRegex(ValueError, "unknown limit keys: legacyy"):
            load_linter().lint_specs(self.root, self.config)

    def test_strips_backticks_from_scalar_frontmatter_values(self) -> None:
        metadata, _, error = load_linter().parse_frontmatter("---\nstatus: `draft`\n---\n")

        self.assertIsNone(error)
        self.assertIsNotNone(metadata)
        self.assertEqual(metadata["status"], "draft")

    def test_rejects_duplicate_requirement_and_acceptance_ids(self) -> None:
        content = self.valid_requirement()
        self.write("docs/specs/task-system/requirements/one.md", content)
        self.write("docs/specs/task-system/requirements/two.md", content)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("duplicate-requirement-id", self.rules(violations))
        self.assertIn("duplicate-acceptance-id", self.rules(violations))

    def test_rejects_an_acceptance_criterion_without_its_requirement(self) -> None:
        content = self.valid_requirement().replace("AC-TASK-DEP-001.1", "AC-TASK-OTHER-002.1")
        self.write("docs/specs/task-system/requirements/dependencies.md", content)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("orphan-acceptance-id", self.rules(violations))

    def test_rejects_invalid_frontmatter_for_new_artifacts(self) -> None:
        content = self.valid_requirement().replace("status: active", "status: shipped")
        self.write("docs/specs/task-system/requirements/dependencies.md", content)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("requirement-status", self.rules(violations))

    def test_rejects_a_system_design_reference_to_an_unknown_requirement(self) -> None:
        self.write("docs/specs/task-system/requirements/dependencies.md", self.valid_requirement())
        self.write(
            "docs/specs/task-system/system-design/dependencies.md",
            self.valid_design("REQ-TASK-MISSING-999"),
        )

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("unknown-requirement-reference", self.rules(violations))

    def test_rejects_a_system_design_reference_owned_by_another_system(self) -> None:
        self.write("docs/specs/task-system/README.md", self.valid_system_index())
        self.write("docs/specs/task-system/requirements/dependencies.md", self.valid_requirement())
        self.write(
            "docs/specs/ui/README.md",
            self.valid_system_index().replace("task-system", "ui"),
        )
        self.write(
            "docs/specs/ui/system-design/dependencies.md",
            self.valid_design().replace("system: task-system", "system: ui"),
        )

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("cross-system-requirement-reference", self.rules(violations))

    def test_rejects_a_requirement_without_acceptance_criteria(self) -> None:
        content = self.valid_requirement().split("#### Acceptance criteria", 1)[0]
        self.write("docs/specs/task-system/requirements/dependencies.md", content)

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("missing-acceptance-criteria", self.rules(violations))

    def test_rejects_a_stray_markdown_file_in_a_migrated_system(self) -> None:
        self.write(
            "docs/specs/task-system/README.md",
            self.valid_system_index("complete"),
        )
        self.write("docs/specs/task-system/spec.md", "# Generic spec\n")

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("system-layout", self.rules(violations))

    def test_allows_legacy_files_while_a_system_migration_is_in_progress(self) -> None:
        self.write(
            "docs/specs/task-system/README.md",
            self.valid_system_index(),
        )
        self.write("docs/specs/task-system/spec.md", "# Legacy spec\n")

        self.assertEqual(load_linter().lint_specs(self.root, self.config), [])

    def test_rejects_invalid_system_index_migration_metadata(self) -> None:
        self.write(
            "docs/specs/task-system/README.md",
            "---\nstatus: active\nsystem: task-system\nspecification_version: 1\nmigration: later\n---\n# Task System\n",
        )

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("system-index-migration", self.rules(violations))

    def test_completed_migration_accepts_real_requirement_and_design_content(self) -> None:
        self.write("docs/specs/task-system/README.md", self.valid_system_index("complete"))
        self.write("docs/specs/task-system/requirements/dependencies.md", self.valid_requirement())
        self.write("docs/specs/task-system/system-design/dependencies.md", self.valid_design())

        self.assertNotIn(
            "migration-content",
            self.rules(load_linter().lint_specs(self.root, self.config)),
        )

    def test_completed_migration_allows_observable_quantifier_content(self) -> None:
        self.write("docs/specs/task-system/README.md", self.valid_system_index("complete"))
        requirement = self.valid_requirement().replace(
            "When a predecessor is pending, the system shall wait.",
            "The response includes all details returned by the provider.",
        )
        self.write("docs/specs/task-system/requirements/dependencies.md", requirement)
        self.write("docs/specs/task-system/system-design/dependencies.md", self.valid_design())

        self.assertNotIn(
            "migration-content",
            self.rules(load_linter().lint_specs(self.root, self.config)),
        )

    def test_completed_migration_rejects_generic_criteria_and_source_wrapper(self) -> None:
        self.write("docs/specs/task-system/README.md", self.valid_system_index("complete"))
        requirement = self.valid_requirement().replace(
            "When a predecessor is pending, the system shall wait.",
            "The system shall preserve all behavior described in the legacy source.",
        )
        self.write("docs/specs/task-system/requirements/dependencies.md", requirement)
        self.write(
            "docs/specs/task-system/system-design/dependencies.md",
            self.valid_design() + "\n## Migrated source detail\n\nCopied source.\n",
        )

        violations = load_linter().lint_specs(self.root, self.config)

        migration_violations = [
            violation for violation in violations if violation.rule == "migration-content"
        ]
        self.assertEqual(len(migration_violations), 2)
        self.assertEqual(migration_violations[0].line, 12)
        self.assertEqual(migration_violations[1].line, 10)
        self.assertTrue(
            all(
                "legacy" in violation.message or "wrapper" in violation.message
                for violation in migration_violations
            )
        )

    def test_migration_audit_reports_in_progress_debt_without_blocking_normal_lint(self) -> None:
        self.write("docs/specs/task-system/README.md", self.valid_system_index())
        requirement = self.valid_requirement().replace(
            "When a predecessor is pending, the system shall wait.",
            "The system shall preserve all behavior described in the legacy source.",
        )
        self.write("docs/specs/task-system/requirements/dependencies.md", requirement)
        self.write(
            "docs/specs/task-system/system-design/dependencies.md",
            self.valid_design() + "\n## Migrated source detail\n",
        )

        linter = load_linter()
        self.assertEqual(linter.lint_specs(self.root, self.config), [])
        audit_rules = self.rules(linter.lint_specs(self.root, self.config, migration_audit=True))
        self.assertEqual(audit_rules, ["migration-audit", "migration-audit"])

    def test_templates_and_historical_docs_are_outside_the_migration_content_gate(self) -> None:
        self.write("docs/specs/task-system/README.md", self.valid_system_index("complete"))
        self.write(
            "docs/specs/templates/old-template.md",
            "## Migrated source detail\n- **AC-TASK-DEP-001.1:** see the legacy source.\n",
        )
        self.write(
            "docs/specs/legacy/historical.md",
            "## Migrated source detail\n- **AC-TASK-DEP-001.1:** see the legacy source.\n",
        )

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertNotIn("migration-content", self.rules(violations))

    def test_migration_audit_cli_keeps_audit_findings_non_blocking(self) -> None:
        self.write("docs/specs/task-system/README.md", self.valid_system_index())
        self.write("docs/specs/task-system/requirements/dependencies.md", self.valid_requirement())
        self.write(
            "docs/specs/task-system/system-design/dependencies.md",
            self.valid_design() + "\n## Migrated source detail\n",
        )
        self.write("docs/specs/spec-lint.json", json.dumps(self.config))

        result = subprocess.run(
            [sys.executable, str(SCRIPT), "--all", "--migration-audit"],
            cwd=self.root,
            capture_output=True,
            text=True,
            check=False,
        )

        self.assertEqual(result.returncode, 0)
        self.assertIn("non-blocking migration audit finding", result.stdout)


if __name__ == "__main__":
    unittest.main()
