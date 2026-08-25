#!/usr/bin/env python3
import importlib.util
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
            "legacy_size_exceptions": {},
        }

    def write(self, relative: str, content: str) -> Path:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
        return path

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

    def valid_system_index(self) -> str:
        return """---
status: draft
system: task-system
specification_version: 1
migration: in_progress
---
# Task System
"""

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
        self.config["legacy_size_exceptions"] = {"docs/specs/legacy/spec.md": 12}

        linter = load_linter()
        self.assertEqual(linter.lint_specs(self.root, self.config), [])

        path.write_text("x" * 13, encoding="utf-8")
        violations = linter.lint_specs(self.root, self.config)
        self.assertIn("file-size", self.rules(violations))
        self.assertIn("frozen ceiling 12", violations[0].message)

    def test_reports_a_stale_legacy_size_exception(self) -> None:
        self.write("docs/specs/legacy/spec.md", "small")
        self.config["limits"]["legacy"] = 10
        self.config["legacy_size_exceptions"] = {"docs/specs/legacy/spec.md": 12}

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertEqual(self.rules(violations), ["stale-size-exception"])

    def test_requires_the_legacy_ceiling_to_follow_every_size_reduction(self) -> None:
        self.write("docs/specs/legacy/spec.md", "x" * 11)
        self.config["limits"]["legacy"] = 10
        self.config["legacy_size_exceptions"] = {"docs/specs/legacy/spec.md": 12}

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertEqual(self.rules(violations), ["stale-size-exception"])
        self.assertIn("Lower the frozen ceiling to 11", violations[0].message)

    def test_reports_a_redundant_legacy_size_exception(self) -> None:
        self.write("docs/specs/legacy/spec.md", "x" * 8)
        self.config["limits"]["legacy"] = 10
        self.config["legacy_size_exceptions"] = {"docs/specs/legacy/spec.md": 8}

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertEqual(self.rules(violations), ["stale-size-exception"])
        self.assertIn("at or below the default limit", violations[0].message)

    def test_rejects_a_raised_legacy_size_ceiling(self) -> None:
        path = self.write("docs/specs/legacy/spec.md", "x" * 13)
        previous_config = {
            **self.config,
            "legacy_size_exceptions": {"docs/specs/legacy/spec.md": 12},
        }
        current_config = {
            **self.config,
            "legacy_size_exceptions": {"docs/specs/legacy/spec.md": 13},
        }

        violations = load_linter().check_legacy_size_ratchet(
            self.root, current_config, previous_config
        )

        self.assertEqual(self.rules(violations), ["legacy-size-ratchet"])
        self.assertEqual(violations[0].path, path)

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
            "---\nstatus: active\nsystem: task-system\nspecification_version: 1\nmigration: complete\n---\n# Task System\n",
        )
        self.write("docs/specs/task-system/spec.md", "# Generic spec\n")

        violations = load_linter().lint_specs(self.root, self.config)

        self.assertIn("system-layout", self.rules(violations))

    def test_allows_legacy_files_while_a_system_migration_is_in_progress(self) -> None:
        self.write(
            "docs/specs/task-system/README.md",
            "---\nstatus: active\nsystem: task-system\nspecification_version: 1\nmigration: in_progress\n---\n# Task System\n",
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


if __name__ == "__main__":
    unittest.main()
