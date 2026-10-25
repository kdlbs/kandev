#!/usr/bin/env python3
import json
import subprocess
import sys
import tempfile
import unittest
from pathlib import Path


ROOT = Path(__file__).resolve().parents[1]
SCRIPT = ROOT / "scripts" / "list-docs.py"


class ListDocsTest(unittest.TestCase):
    def setUp(self) -> None:
        self.tmp = tempfile.TemporaryDirectory()
        self.addCleanup(self.tmp.cleanup)
        self.root = Path(self.tmp.name)

    def write(self, relative: str, content: str) -> Path:
        path = self.root / relative
        path.parent.mkdir(parents=True, exist_ok=True)
        path.write_text(content, encoding="utf-8")
        return path

    def run_cli(self, *args: str) -> subprocess.CompletedProcess[str]:
        return subprocess.run(
            [sys.executable, str(SCRIPT), "--root", str(self.root), *args],
            cwd=ROOT,
            text=True,
            capture_output=True,
            check=False,
        )

    def add_decisions(self) -> None:
        self.write(
            "docs/decisions/0010-pipe.md",
            """# 0010: Pipe | choice

**Status:** accepted (amended)
**Date:** 2026-01-02 (amended)
**Area:** backend, frontend

## Context

The searchable needle is here.
""",
        )
        self.write(
            "docs/decisions/0002-bullet.md",
            """# 0002 — Bullet metadata

- Status: Proposed
- Date: 2025-12-01
- Area: workflow, infra

## Context

This decision uses bullet metadata.
""",
        )
        self.write(
            "docs/decisions/2026-01-03-frontmatter.md",
            """---
status: accepted
date: 2026-01-03
area: protocol
---

# Frontmatter decision

## Context

This decision uses YAML metadata.
""",
        )
        self.write(
            "docs/decisions/0019-status-section.md",
            """# 0019: Status section

## Status

accepted

## Context

Older decisions can omit date and area metadata.
""",
        )
        self.write(
            "docs/decisions/2026-01-04-table.md",
            """# Table metadata

| Metadata | Value |
| --- | --- |
| Date | 2026-01-04 |
| Status | superseded by another decision |
| Tags | backend, security |

## Context

This decision uses a metadata table.
""",
        )
        self.write(
            "docs/decisions/2026-01-05-wrapped-status.md",
            """# Wrapped status metadata

**Status:** accepted (amended by
2026-01-05)
**Date:** 2026-01-05
**Area:** workflow

## Context

This decision wraps one metadata value.
""",
        )
        self.write(
            "docs/decisions/0028-status-punctuation.md",
            """# 0028: Status punctuation

**Status:** accepted; browser-cache portion superseded
**Date:** 2026-01-06
**Area:** backend

## Context

This decision uses punctuation after the status class.
""",
        )

    def add_specs(self) -> None:
        self.write(
            "docs/specs/ui/README.md",
            """---
status: active
system: ui
specification_version: 1
migration: complete
---

# UI system
""",
        )
        self.write(
            "docs/specs/ui/requirements/first.md",
            """---
id: ui-first
title: Frontmatter requirement title
status: `active`
system: `ui`
created: 2026-01-01
---

# Heading requirement title

The specification body contains the Searchable Spec Term.
""",
        )
        self.write(
            "docs/specs/ui/system-design/first.md",
            """---
id: ui-design
status: current
system: ui
requirements: []
---

# UI system design
""",
        )
        self.write(
            "docs/specs/legacy/spec.md",
            """# Legacy specification

This file has no frontmatter and remains in the legacy layout.
""",
        )
        self.write(
            "docs/specs/product/overview.md",
            """# Product overview

This document describes the product boundary.
""",
        )
        self.write(
            "docs/specs/ui/glossary.md",
            """# UI glossary

This document defines UI terms.
""",
        )

    def test_decisions_support_variants_filters_and_stable_sorting(self) -> None:
        self.add_decisions()

        result = self.run_cli("decisions", "--status", "accepted", "--format", "paths")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertEqual(
            result.stdout.splitlines(),
            [
                "docs/decisions/0010-pipe.md",
                "docs/decisions/0019-status-section.md",
                "docs/decisions/0028-status-punctuation.md",
                "docs/decisions/2026-01-03-frontmatter.md",
                "docs/decisions/2026-01-05-wrapped-status.md",
            ],
        )

        area_result = self.run_cli("decisions", "--area", "FRONTEND", "--format", "paths")
        self.assertEqual(area_result.returncode, 0, area_result.stderr)
        self.assertEqual(area_result.stdout.splitlines(), ["docs/decisions/0010-pipe.md"])

        text_result = self.run_cli("decisions", "--text", "NEEDLE", "--format", "paths")
        self.assertEqual(text_result.returncode, 0, text_result.stderr)
        self.assertEqual(text_result.stdout.splitlines(), ["docs/decisions/0010-pipe.md"])

        wrapped = self.run_cli("decisions", "--format", "json")
        self.assertEqual(wrapped.returncode, 0, wrapped.stderr)
        wrapped_document = next(
            document
            for document in json.loads(wrapped.stdout)["documents"]
            if document["id"] == "2026-01-05-wrapped-status"
        )
        self.assertEqual(wrapped_document["status"], "accepted (amended by 2026-01-05)")

        punctuation = self.run_cli(
            "decisions", "--status", "accepted", "--format", "paths"
        )
        self.assertEqual(punctuation.returncode, 0, punctuation.stderr)
        self.assertIn(
            "docs/decisions/0028-status-punctuation.md", punctuation.stdout.splitlines()
        )

    def test_decision_outputs_escape_markdown_and_preserve_metadata_in_json(self) -> None:
        self.add_decisions()

        markdown = self.run_cli("decisions", "--format", "markdown")
        self.assertEqual(markdown.returncode, 0, markdown.stderr)
        self.assertIn("Pipe \\| choice", markdown.stdout)
        self.assertIn("| 0010-pipe |", markdown.stdout)

        first = self.run_cli("decisions", "--format", "json")
        second = self.run_cli("decisions", "--format", "json")
        self.assertEqual(first.returncode, 0, first.stderr)
        self.assertEqual(first.stdout, second.stdout)
        payload = json.loads(first.stdout)
        self.assertEqual(payload["schema_version"], 1)
        self.assertEqual(payload["documents"][0]["id"], "0002-bullet")
        self.assertEqual(payload["documents"][0]["status"], "Proposed")
        self.assertEqual(payload["documents"][0]["area"], "workflow, infra")

    def test_specs_support_kinds_filters_json_and_empty_results(self) -> None:
        self.add_specs()

        result = self.run_cli(
            "specs",
            "--system",
            "ui",
            "--kind",
            "requirement",
            "--status",
            "active",
            "--text",
            "searchable spec term",
            "--format",
            "json",
        )

        self.assertEqual(result.returncode, 0, result.stderr)
        payload = json.loads(result.stdout)
        self.assertEqual(len(payload["documents"]), 1)
        document = payload["documents"][0]
        self.assertEqual(document["kind"], "requirement")
        self.assertEqual(document["system"], "ui")
        self.assertEqual(document["title"], "Frontmatter requirement title")
        self.assertEqual(document["id"], "ui-first")

        paths = self.run_cli("specs", "--kind", "legacy", "--format", "paths")
        self.assertEqual(paths.returncode, 0, paths.stderr)
        self.assertEqual(paths.stdout.splitlines(), ["docs/specs/legacy/spec.md"])

        product_paths = self.run_cli("specs", "--kind", "product", "--format", "paths")
        self.assertEqual(product_paths.returncode, 0, product_paths.stderr)
        self.assertEqual(product_paths.stdout.splitlines(), ["docs/specs/product/overview.md"])

        glossary_paths = self.run_cli("specs", "--kind", "glossary", "--format", "paths")
        self.assertEqual(glossary_paths.returncode, 0, glossary_paths.stderr)
        self.assertEqual(glossary_paths.stdout.splitlines(), ["docs/specs/ui/glossary.md"])

        empty = self.run_cli("specs", "--system", "missing", "--format", "json")
        self.assertEqual(empty.returncode, 0, empty.stderr)
        self.assertEqual(json.loads(empty.stdout)["documents"], [])

    def test_spec_kind_filters_cover_each_linter_source_category(self) -> None:
        self.add_specs()
        expected = {
            "system": "docs/specs/ui/README.md",
            "glossary": "docs/specs/ui/glossary.md",
            "requirement": "docs/specs/ui/requirements/first.md",
            "system-design": "docs/specs/ui/system-design/first.md",
            "product": "docs/specs/product/overview.md",
            "legacy": "docs/specs/legacy/spec.md",
        }

        for kind, path in expected.items():
            with self.subTest(kind=kind):
                result = self.run_cli("specs", "--kind", kind, "--format", "paths")

                self.assertEqual(result.returncode, 0, result.stderr)
                self.assertEqual(result.stdout.splitlines(), [path])

    def test_empty_markdown_keeps_the_catalog_type(self) -> None:
        self.add_decisions()
        result = self.run_cli("decisions", "--status", "missing", "--format", "markdown")

        self.assertEqual(result.returncode, 0, result.stderr)
        self.assertIn("| ID | Title | Status | Area | Date | Path |", result.stdout)

    def test_specs_skip_entry_readmes_and_derive_titles_without_level_one_headings(self) -> None:
        self.write("docs/specs/product/README.md", "# Product Specifications\n")
        self.write(
            "docs/specs/ui/README.md",
            """---
status: active
system: ui
specification_version: 1
migration: complete
---

# UI system
""",
        )
        self.write(
            "docs/specs/ui/requirements/no-level-one.md",
            """---
status: active
system: ui
---

## Overview

This requirement has no level-one heading.
""",
        )

        result = self.run_cli("specs", "--system", "ui", "--format", "json")

        self.assertEqual(result.returncode, 0, result.stderr)
        documents = json.loads(result.stdout)["documents"]
        requirement = next(
            document
            for document in documents
            if document["path"].endswith("no-level-one.md")
        )
        self.assertEqual(requirement["title"], "No Level One")
        self.assertNotIn("docs/specs/product/README.md", result.stdout)

    def test_validate_rejects_malformed_metadata_and_duplicate_spec_ids(self) -> None:
        self.write(
            "docs/decisions/bad.md",
            """# Bad decision

**Date:** 2026-01-01
**Area:** backend
""",
        )
        self.write(
            "docs/specs/ui/README.md",
            """---
status: active
system: ui
specification_version: 1
migration: complete
---

# UI system
""",
        )
        requirement = """---
id: duplicate
status: active
system: ui
---

# Requirement
"""
        self.write("docs/specs/ui/requirements/one.md", requirement)
        self.write("docs/specs/ui/requirements/two.md", requirement)

        result = self.run_cli("validate")

        self.assertNotEqual(result.returncode, 0)
        combined = result.stdout + result.stderr
        self.assertIn("docs/decisions/bad.md", combined)
        self.assertIn("docs/specs/ui/requirements/two.md", combined)
        self.assertIn("duplicate", combined.lower())

    def test_validate_rejects_system_readmes_without_valid_frontmatter(self) -> None:
        for content in ("# UI system\n", "---\nstatus: active\n---\n# UI system\n"):
            with self.subTest(content=content):
                self.write("docs/specs/ui/README.md", content)

                result = self.run_cli("validate")

                self.assertNotEqual(result.returncode, 0)
                combined = result.stdout + result.stderr
                self.assertIn("docs/specs/ui/README.md", combined)

    def test_invalid_kind_is_an_actionable_cli_error(self) -> None:
        result = self.run_cli("specs", "--kind", "unknown")

        self.assertNotEqual(result.returncode, 0)
        self.assertIn("invalid choice", result.stderr)


if __name__ == "__main__":
    unittest.main()
