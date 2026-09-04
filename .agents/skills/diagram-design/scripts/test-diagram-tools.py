#!/usr/bin/env python3
"""Focused regression tests for the shipped Diagram Design tools."""

from __future__ import annotations

import importlib.util
import sys
import tempfile
import unittest
from pathlib import Path
from types import ModuleType


SCRIPT_DIR = Path(__file__).resolve().parent
SKILL_DIR = SCRIPT_DIR.parent
REPO_ROOT = SCRIPT_DIR.parents[3]


def load_tool(name: str, filename: str) -> ModuleType:
    path = SCRIPT_DIR / filename
    spec = importlib.util.spec_from_file_location(name, path)
    if spec is None or spec.loader is None:
        raise AssertionError(f"could not load {path}")
    module = importlib.util.module_from_spec(spec)
    sys.modules[name] = module
    spec.loader.exec_module(module)
    return module


def write_candidate(directory: Path, suffix: str, source: str) -> Path:
    path = directory / f"candidate{suffix}"
    path.write_text(source, encoding="utf-8")
    return path


class DiagramToolTests(unittest.TestCase):
    @classmethod
    def setUpClass(cls) -> None:
        cls.drawio = load_tool("diagram_drawio_extract", "drawio_extract.py")
        cls.mermaid = load_tool("diagram_mermaid_extract", "mermaid_extract.py")
        cls.self_check = load_tool("diagram_self_check", "self_check.py")
        cls.geometry = load_tool("diagram_verify_geometry", "verify-geometry.py")
        cls.motion = load_tool("diagram_verify_motion", "verify-motion.py")
        cls.skin = load_tool("diagram_lint_skin", "lint-skin.py")

    def test_mermaid_extractor_builds_graph_and_rejects_unsupported_kind(self) -> None:
        source = "flowchart LR\n  A[Source] -->|uses| B{Gate}\n  B --> C[Done]\n"
        with tempfile.TemporaryDirectory() as temporary:
            path = write_candidate(Path(temporary), ".mmd", source)
            blocks = self.mermaid.load_blocks(path)
            self.assertEqual(len(blocks), 1)
            diagram = self.mermaid.parse_block(blocks[0])
            self.assertEqual([node.id for node in diagram.nodes], ["A", "B", "C"])
            self.assertEqual([edge.label for edge in diagram.edges], ["uses", ""])
            self.assertEqual(self.mermaid.analyze(diagram)["edges_total"], 2)

            unsupported = write_candidate(Path(temporary), ".mmd", "pie title Unsupported\n")
            with self.assertRaises(SystemExit):
                self.mermaid.parse_block(self.mermaid.load_blocks(unsupported)[0])

            fixture = SCRIPT_DIR / "fixtures" / "sample-flowchart.mmd"
            fixture_blocks = self.mermaid.load_blocks(fixture)
            self.assertEqual(len(fixture_blocks), 1)
            self.assertEqual(self.mermaid.analyze(self.mermaid.parse_block(fixture_blocks[0]))["nodes_total"], 9)
            readme_fixture = SCRIPT_DIR / "fixtures" / "sample-readme-with-mermaid.md"
            readme_blocks = self.mermaid.load_blocks(readme_fixture)
            self.assertEqual(len(readme_blocks), 2)
            self.assertEqual(self.mermaid.parse_block(readme_blocks[1]).kind, "sequenceDiagram")

    def test_drawio_extractor_reads_pages_and_rejects_external_entities(self) -> None:
        source = """<mxfile>
  <diagram id="page-one" name="Page One">
    <mxGraphModel><root>
      <mxCell id="0" />
      <mxCell id="1" value="Source" vertex="1">
        <mxGeometry x="20" y="30" width="80" height="40" as="geometry" />
      </mxCell>
      <mxCell id="2" value="Target" vertex="1">
        <mxGeometry x="200" y="30" width="80" height="40" as="geometry" />
      </mxCell>
      <mxCell id="edge" edge="1" source="1" target="2">
        <mxGeometry relative="1" as="geometry" />
      </mxCell>
    </root></mxGraphModel>
  </diagram>
</mxfile>
        """
        fixture_pages = self.drawio.parse_file(SCRIPT_DIR / "fixtures" / "sample-architecture.drawio")
        self.assertEqual(len(fixture_pages), 1)
        self.assertEqual(self.drawio.analyze(fixture_pages[0])["nodes_total"], 12)
        self.assertEqual(self.drawio.analyze(fixture_pages[0])["edges_total"], 8)

        with tempfile.TemporaryDirectory() as temporary:
            path = write_candidate(Path(temporary), ".drawio", source)
            pages = self.drawio.parse_file(path)
            self.assertEqual(len(pages), 1)
            self.assertEqual([node.label for node in pages[0].nodes], ["Source", "Target"])
            self.assertEqual(len(pages[0].edges), 1)
            self.assertEqual(self.drawio.analyze(pages[0])["edges_total"], 1)

            unsafe = write_candidate(
                Path(temporary),
                "-unsafe.drawio",
                '<!DOCTYPE mxfile [<!ENTITY xxe "file:///etc/passwd">]><mxfile />',
            )
            with self.assertRaises(SystemExit):
                self.drawio.parse_file(unsafe)

    def test_self_check_rejects_css_resource_loading_but_allows_fragment_urls(self) -> None:
        document = """<!doctype html><html><head>{head}</head><body>
<svg role="img" aria-labelledby="sample-title sample-desc">
  <title id="sample-title">Sample</title>
  <desc id="sample-desc">A sample diagram.</desc>
</svg></body></html>
"""
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            safe = write_candidate(
                directory,
                "-safe.html",
                document.format(head="<style>.mask {{ mask: url(#mask); }}</style>"),
            )
            self.assertEqual(self.self_check.verify(safe), [])

            unsafe = write_candidate(
                directory,
                "-unsafe.html",
                document.format(
                    head=(
                        '<style>@import url("https://example.test/theme.css"); '
                        '.node { background: url(assets/node.png); }</style>'
                    )
                ),
            )
            errors = self.self_check.verify(unsafe)
            self.assertIn("CSS @import is not allowed", errors)
            self.assertTrue(any("non-fragment CSS url()" in error for error in errors))
            self.assertTrue(
                any(
                    "non-fragment CSS url()" in error
                    for error in self.self_check.css_reference_errors(
                        [".node { background: url(", "assets/node.png); }"]
                    )
                )
            )

    def test_geometry_parser_accepts_attribute_order_and_quote_styles(self) -> None:
        source = """<svg>
  <rect width='48' height='12' y='80' x='240' rx='2' />
  <rect height=64 width=160 x=+100 y=' 60 ' rx='6' />
  <rect x='1' y='2' width='calc(100%)' height='4' />
  <rect x='1' y='2' height='4' />
</svg>
"""
        rects = self.geometry.parse_rects(source)
        self.assertEqual(
            [(rect.x, rect.y, rect.w, rect.h) for rect in rects],
            [(240, 80, 48, 12), (100, 60, 160, 64)],
        )

        with tempfile.TemporaryDirectory() as temporary:
            path = write_candidate(Path(temporary), ".html", source)
            findings = self.geometry.check(path)
            self.assertEqual(len(findings), 1)

    def test_motion_verifier_covers_canonical_and_mutated_controller(self) -> None:
        source = (SKILL_DIR / "assets" / "template-motion.html").read_text(encoding="utf-8")
        with tempfile.TemporaryDirectory() as temporary:
            directory = Path(temporary)
            canonical = write_candidate(directory, "-canonical.html", source)
            self.assertEqual(self.motion.verify(canonical), [])

            mutated = write_candidate(
                directory,
                "-mutated.html",
                source.replace("data-motion-mode=\"step\"", "data-motion-mode=\"none\"", 1),
            )
            errors = self.motion.verify(mutated)
            self.assertTrue(any("none mode must be script-free" in error for error in errors))

    def test_skin_linter_accepts_kandev_source_and_rejects_pure_black(self) -> None:
        source_path = REPO_ROOT / "docs" / "diagrams" / "architecture.html"
        source = source_path.read_text(encoding="utf-8")
        colors, rgb_triplets = self.skin.allowed_colors()
        self.assertEqual(
            self.skin.lint_text(source, colors, rgb_triplets, "architecture"),
            [],
        )

        findings = self.skin.lint_text(
            source.replace("#6468f0", "#000000", 1),
            colors,
            rgb_triplets,
            "architecture",
        )
        self.assertTrue(any("pure black" in finding[3] for finding in findings))


if __name__ == "__main__":
    unittest.main()
