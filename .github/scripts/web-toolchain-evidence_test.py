#!/usr/bin/env python3
"""Tests for the bounded native-web evidence harness.

The installed legacy compiler exercises the real diagnostic parser. The native
candidate and reference binaries are deterministic fakes so each failure mode
can be injected without depending on released compiler behavior.
"""

from __future__ import annotations

import argparse
import importlib.util
import json
import os
import subprocess
import sys
import tempfile
import time
import unittest
from pathlib import Path

SCRIPT = Path(__file__).resolve().parent / "web-toolchain-evidence.py"
SPEC = importlib.util.spec_from_file_location("web_toolchain_evidence", SCRIPT)
assert SPEC and SPEC.loader
MODULE = importlib.util.module_from_spec(SPEC)
sys.modules[SPEC.name] = MODULE
SPEC.loader.exec_module(MODULE)

FAKE_COMPILER = r"""#!/usr/bin/env python3
import json, os, sys
from pathlib import Path

name = Path(sys.argv[0]).name
base = Path(__file__).resolve().parent
project = Path.cwd()
args = sys.argv[1:]
if "--version" in args:
    if "reference" in name:
        print("Version 7.1.0-beta (fake reference)")
    elif "tsrs" in name:
        print("Version 7.1.0-beta (tsrs 0.20.1-ts7.1.0-beta, microsoft/TypeScript@3e9236361fa9)")
    else:
        print("Version 5.9.3")
    sys.exit(0)
if "--listFilesOnly" in args:
    if os.environ.get("FAKE_EMPTY") == "1":
        sys.exit(0)
    files = sorted(project.glob("*.ts"))
    if "candidate" in name and os.environ.get("FAKE_INVENTORY_GAP") == "1":
        files = files[:-1]
    if os.environ.get("FAKE_INVENTORY_NOT_A_PATH") == "1":
        print("not a path")
        sys.exit(0)
    for path in files:
        print(path.resolve())
    sys.exit(0)
if "kandev-incremental-probe" in str(project):
    sources = [path.read_text() for path in project.glob("*.ts")]
    if any("not a number" in source for source in sources):
        if os.environ.get("FAKE_STALE_CACHE") == "1":
            sys.exit(0)
        print('index.ts(1,14): error TS2322: Type \'string\' is not assignable to type \'number\'.')
        sys.exit(2)
    sys.exit(0)
if os.environ.get("FAKE_LONG_OUTPUT") == "1":
    print("x" * 500)
    sys.exit(2)
if os.environ.get("FAKE_STDERR_FAIL") == "1":
    print("compiler crashed before producing diagnostics", file=sys.stderr)
    sys.exit(2)
if os.environ.get("FAKE_SUCCESS_DIAGNOSTICS") == "1":
    print("index.ts(1,1): error TS2322: diagnostic on a success exit")
    sys.exit(0)
diagnostics_file = base / f"{name}.diagnostics.json"
if diagnostics_file.exists():
    diagnostics = json.loads(diagnostics_file.read_text())
    if diagnostics:
        for item in diagnostics:
            print(f"{item['file']}({item['line']},{item['column']}): error {item['code']}: {item['message']}")
        sys.exit(2)
if os.environ.get("FAKE_GARBAGE") == "1":
    print("this is not compiler output")
    sys.exit(2)
sys.exit(0)
"""


class EvidenceHarnessTest(unittest.TestCase):
    def setUp(self) -> None:
        self.temp = tempfile.TemporaryDirectory(prefix="kandev-evidence-test-")
        self.root = Path(self.temp.name)
        self.web_root = self.root / "web"
        self.web_root.mkdir()
        (self.web_root / "tsconfig.json").write_text(
            json.dumps(
                {
                    "compilerOptions": {"strict": True, "incremental": True},
                    "include": ["*.ts"],
                }
            ),
            encoding="utf-8",
        )
        (self.web_root / "index.ts").write_text("export const value = 1;\n", encoding="utf-8")
        self.reference = self._fake("reference-tsc")
        self.tsrs = self._fake("candidate-tsrs")
        self.tsc = self._fake("legacy-tsc")

    def tearDown(self) -> None:
        self.temp.cleanup()

    def _fake(self, name: str) -> Path:
        path = self.root / name
        path.write_text(FAKE_COMPILER, encoding="utf-8")
        path.chmod(0o755)
        return path

    def _diagnostics(self, binary: Path, items: list[dict]) -> None:
        path = self.root / f"{binary.name}.diagnostics.json"
        path.write_text(json.dumps(items), encoding="utf-8")

    def _compare(self, extra: list[str] | None = None) -> tuple[int, dict]:
        output = self.root / "report.json"
        argv = [
            "compare-types",
            "--web-root",
            str(self.web_root),
            "--reference-bin",
            str(self.reference),
            "--tsrs",
            str(self.tsrs),
            "--tsc",
            str(self.tsc),
            "--project",
            "tsconfig.json",
            "--no-corpus",
            "--output",
            str(output),
            *(extra or []),
        ]
        code = MODULE.main(argv)
        if not output.exists():
            return code, {}
        return code, json.loads(output.read_text(encoding="utf-8"))

    def test_identical_diagnostics_pass_and_bind_evidence(self) -> None:
        diagnostic = {
            "file": "index.ts",
            "line": 1,
            "column": 1,
            "code": "TS2322",
            "message": "Type 'number' is not assignable to type 'string'.",
        }
        for binary in (self.reference, self.tsrs, self.tsc):
            self._diagnostics(binary, [diagnostic])
        code, report = self._compare()
        self.assertEqual(code, 0, report["failures"])
        self.assertTrue(report["ok"])
        self.assertEqual(report["inventory"]["baseline"]["count"], 1)
        self.assertTrue(report["inventory"]["equal"])
        self.assertTrue(report["source"]["sha256"].startswith("sha256:"))
        self.assertEqual(report["pins"]["tsrs_expected"], MODULE.DEFAULT_TSRS)
        self.assertEqual(report["settings"]["cache_scenarios"], ["cold", "warm"])
        self.assertFalse(report["adoption_eligible"])
        self.assertEqual(report["corpus"]["mode"], "diagnostic-only")
        self.assertIn("max_rss_kb", report["attempts"][0])

    def test_missing_candidate_diagnostic_fails(self) -> None:
        diagnostics = [
            {
                "file": "index.ts",
                "line": 1,
                "column": 1,
                "code": "TS2322",
                "message": "first",
            },
            {
                "file": "index.ts",
                "line": 2,
                "column": 1,
                "code": "TS2339",
                "message": "second",
            },
        ]
        self._diagnostics(self.reference, diagnostics)
        self._diagnostics(self.tsc, diagnostics)
        self._diagnostics(self.tsrs, diagnostics[:1])
        code, report = self._compare()
        self.assertEqual(code, 2)
        self.assertFalse(report["ok"])
        fast = next(item for item in report["comparisons"] if item["candidate_attempt"].startswith("tsrs-fast"))
        self.assertEqual(len(fast["missing"]), 1)

    def test_extra_candidate_diagnostic_fails(self) -> None:
        base = {
            "file": "index.ts",
            "line": 1,
            "column": 1,
            "code": "TS2322",
            "message": "shared",
        }
        self._diagnostics(self.reference, [base])
        self._diagnostics(self.tsc, [base])
        self._diagnostics(
            self.tsrs,
            [base, {**base, "code": "TS9999", "message": "invented"}],
        )
        code, report = self._compare()
        self.assertEqual(code, 2)
        fast = next(item for item in report["comparisons"] if item["candidate_attempt"].startswith("tsrs-fast"))
        self.assertEqual(len(fast["extra"]), 1)

    def test_candidate_missing_cached_diagnostic_is_rejected(self) -> None:
        diagnostic = {
            "file": "index.ts",
            "line": 1,
            "column": 1,
            "code": "TS2322",
            "message": "cached failure",
        }
        self._diagnostics(self.reference, [diagnostic])
        self._diagnostics(self.tsc, [diagnostic])
        (self.root / f"{self.tsrs.name}.diagnostics.json").unlink(missing_ok=True)
        code, report = self._compare()
        self.assertEqual(code, 2)
        fast = next(item for item in report["comparisons"] if item["candidate_attempt"].startswith("tsrs-fast"))
        self.assertEqual(len(fast["missing"]), 1)

    def test_unparseable_output_fails(self) -> None:
        self._diagnostics(self.reference, [])
        self._diagnostics(self.tsc, [])
        with self.assertRaises(MODULE.EvidenceError):
            MODULE.parse_diagnostics("this is not compiler output\n", self.web_root)
        os.environ["FAKE_GARBAGE"] = "1"
        try:
            code, report = self._compare()
        finally:
            os.environ.pop("FAKE_GARBAGE", None)
        self.assertEqual(code, 2)
        self.assertFalse(report.get("ok", False))

    def test_missing_binary_cannot_succeed(self) -> None:
        output = self.root / "report.json"
        code = MODULE.main(
            [
                "compare-types",
                "--web-root",
                str(self.web_root),
                "--reference-bin",
                str(self.reference),
                "--tsrs",
                str(self.root / "missing-tsrs"),
                "--tsc",
                str(self.tsc),
                "--no-corpus",
                "--output",
                str(output),
            ]
        )
        self.assertEqual(code, 2)
        report = json.loads(output.read_text(encoding="utf-8"))
        self.assertFalse(report["ok"])
        self.assertFalse(report["complete"])
        self.assertTrue(any("tsrs" in failure for failure in report["failures"]))
        self.assertIn("version-reference", [item["label"] for item in report["attempts"]])

    def test_empty_type_inventory_fails(self) -> None:
        os.environ["FAKE_EMPTY"] = "1"
        try:
            with self.assertRaises(MODULE.EvidenceError):
                MODULE.collect_type_inventory(
                    self.web_root, str(self.tsc), project="tsconfig.json"
                )
        finally:
            os.environ.pop("FAKE_EMPTY", None)

    def test_cross_tool_inventory_mismatch_fails(self) -> None:
        (self.web_root / "second.ts").write_text("export const other = 2;\n", encoding="utf-8")
        os.environ["FAKE_INVENTORY_GAP"] = "1"
        try:
            code, report = self._compare()
        finally:
            os.environ.pop("FAKE_INVENTORY_GAP", None)
        self.assertEqual(code, 2)
        self.assertFalse(report["inventory"]["equal"])
        self.assertTrue(report["inventory"]["differences"])

    def test_inventory_mismatch_is_visible(self) -> None:
        (self.web_root / "second.ts").write_text("export const other = 2;\n", encoding="utf-8")
        code, report = self._compare()
        self.assertEqual(code, 0, report["failures"])
        self.assertEqual(report["inventory"]["baseline"]["count"], 2)

    def test_parser_rejects_malformed_line(self) -> None:
        with self.assertRaises(MODULE.EvidenceError):
            MODULE.parse_diagnostics("error: something exploded\n", self.web_root)

    def test_parse_diagnostics_handles_multiline_messages(self) -> None:
        diagnostics = MODULE.parse_diagnostics(
            "index.ts(1,2): error TS2322: first line\n  second line\n", self.web_root
        )
        self.assertEqual(len(diagnostics), 1)
        self.assertEqual(diagnostics[0].message, "first line second line")
        self.assertEqual(diagnostics[0].code, "TS2322")

    def test_timeout_kills_and_joins_child(self) -> None:
        started = time.monotonic()
        attempt = MODULE.run_process(
            "sleep", ["/bin/sh", "-c", "sleep 5"], self.web_root, timeout=0.2
        )
        elapsed = time.monotonic() - started
        self.assertTrue(attempt.timed_out)
        self.assertEqual(attempt.exit_code, -9)
        self.assertLess(elapsed, 3.0)

    def test_missing_executable_is_an_error(self) -> None:
        with self.assertRaises(MODULE.EvidenceError):
            MODULE.run_process("missing", ["definitely-not-a-real-binary"], self.web_root)

    def test_lint_finding_comparison_detects_duplicates(self) -> None:
        baseline = [{"file": "a.ts", "line": 1, "rule": "no-debugger", "severity": "error", "message": ""}]
        duplicate = baseline * 2
        comparison = MODULE.compare_lint_findings(baseline, duplicate)
        self.assertFalse(comparison.ok)
        self.assertEqual(len(comparison.extra), 1)

    def test_lint_comparison_uses_complete_finding_tuples(self) -> None:
        finding = {
            "file": "a.ts",
            "line": 3,
            "column": 7,
            "rule": "no-debugger",
            "severity": "error",
            "message": "debugger",
        }
        for field, value in (("column", 8), ("severity", "warning"), ("message", "other")):
            changed = dict(finding)
            changed[field] = value
            comparison = MODULE.compare_lint_findings([finding], [changed])
            self.assertFalse(comparison.ok, field)
            self.assertEqual(len(comparison.missing), 1)
            self.assertEqual(len(comparison.extra), 1)
        reordered = [finding, {**finding, "line": 9}]
        comparison = MODULE.compare_lint_findings(reordered, list(reversed(reordered)))
        self.assertTrue(comparison.ok)
        self.assertTrue(comparison.order_only)

    def test_malformed_lint_reports_fail(self) -> None:
        with self.assertRaises(MODULE.EvidenceError):
            MODULE.parse_eslint_json("{}")
        with self.assertRaises(MODULE.EvidenceError):
            MODULE.parse_oxlint_json('{"diagnostics": [{"message": "x"}]}')

    def test_config_identity_changes_with_configuration(self) -> None:
        first = MODULE.config_identity(self.web_root)
        (self.web_root / "tsconfig.json").write_text(
            json.dumps({"compilerOptions": {"strict": False}}), encoding="utf-8"
        )
        second = MODULE.config_identity(self.web_root)
        self.assertNotEqual(first["sha256"], second["sha256"])

    def test_source_identity_changes_with_source(self) -> None:
        first = MODULE.source_identity(self.web_root, (".ts",))
        (self.web_root / "index.ts").write_text("export const value = 2;\n", encoding="utf-8")
        second = MODULE.source_identity(self.web_root, (".ts",))
        self.assertNotEqual(first["sha256"], second["sha256"])

    def test_incremental_cache_isolation_moves_build_info(self) -> None:
        output = self.root / "benchmark-output"
        output.mkdir()
        build_info = self.web_root / "tsconfig.tsbuildinfo"
        build_info.write_text("{}", encoding="utf-8")
        cleared = MODULE._clear_incremental_caches(self.web_root, output)
        self.assertEqual(cleared, ["tsconfig.tsbuildinfo"])
        self.assertFalse(build_info.exists())
        self.assertTrue(any((output / "cache").iterdir()))

    def test_diagnostic_location_severity_and_order(self) -> None:
        reference = [
            MODULE.NormalizedDiagnostic("index.ts", 1, 2, "TS2322", "error", "same"),
            MODULE.NormalizedDiagnostic("index.ts", 5, 7, "TS2339", "error", "other"),
        ]
        for field, value in (("line", 2), ("column", 3), ("severity", "warning")):
            candidate = [
                MODULE.NormalizedDiagnostic(
                    d.file,
                    value if field == "line" and d.line == 1 else d.line,
                    value if field == "column" and d.line == 1 else d.column,
                    d.code,
                    value if field == "severity" and d.line == 1 else d.severity,
                    d.message,
                )
                for d in reference
            ]
            comparison = MODULE.compare_type_diagnostics(
                reference,
                candidate,
                reference_attempt="reference",
                candidate_attempt="candidate",
                label=field,
            )
            self.assertFalse(comparison.ok, f"changed {field} must fail")
            self.assertEqual(len(comparison.missing), 1)
            self.assertEqual(len(comparison.extra), 1)
        reordered = MODULE.compare_type_diagnostics(
            reference,
            [reference[1], reference[0]],
            reference_attempt="reference",
            candidate_attempt="candidate",
            label="order",
        )
        self.assertTrue(reordered.ok)
        self.assertTrue(reordered.order_only)

    def test_checker_arguments_reach_reference_and_tsrs(self) -> None:
        code, report = self._compare(["--checkers", "4"])
        self.assertEqual(code, 0, report["failures"])
        self.assertEqual(report["settings"]["checkers"], 4)
        by_label = {attempt["label"]: attempt["argv"] for attempt in report["attempts"]}
        for label in ("reference-cold", "reference-warm", "tsrs-default-cold", "tsrs-default-warm"):
            self.assertIn("--checkers", by_label[label], label)
            index = by_label[label].index("--checkers")
            self.assertEqual(by_label[label][index + 1], "4", label)
        for label in ("tsc-cold", "tsc-warm"):
            self.assertNotIn("--checkers", by_label[label], label)
        for label in ("tsrs-default-cold", "tsrs-default-warm"):
            index = by_label[label].index("--fast")
            self.assertEqual(by_label[label][index + 1], "false")
        for label in ("tsrs-fast-cold", "tsrs-fast-warm"):
            self.assertIn("--fast", by_label[label])

    def test_default_mode_ignores_inherited_fast_env(self) -> None:
        os.environ["TSRS_FAST"] = "1"
        try:
            code, report = self._compare()
        finally:
            os.environ.pop("TSRS_FAST", None)
        self.assertEqual(code, 0, report["failures"])
        by_label = {attempt["label"]: attempt["argv"] for attempt in report["attempts"]}
        for label in ("tsrs-default-cold", "tsrs-default-warm"):
            index = by_label[label].index("--fast")
            self.assertEqual(by_label[label][index + 1], "false")

    def test_unsupported_checker_count_is_reported(self) -> None:
        code, report = self._compare(["--checkers", "2"])
        self.assertEqual(code, 2)
        self.assertTrue(any("unsupported checker count" in item for item in report["failures"]))

    def test_stale_incremental_probe_is_rejected(self) -> None:
        os.environ["FAKE_STALE_CACHE"] = "1"
        try:
            code, report = self._compare()
        finally:
            os.environ.pop("FAKE_STALE_CACHE", None)
        self.assertEqual(code, 2)
        probe = report["cache_scenarios"]["incremental_probe"]
        self.assertTrue(probe)
        for result in probe.values():
            self.assertFalse(result["ok"])
            self.assertFalse(result["phases"]["changed"]["diagnostic_count"])
        self.assertTrue(any("incremental probe" in item for item in report["failures"]))

    def test_corpus_runs_reference_default_and_fast_with_both_caches(self) -> None:
        corpus = self.root / "corpus"
        project = corpus / "valid"
        project.mkdir(parents=True)
        (corpus / "corpus.json").write_text(
            json.dumps({"projects": {"valid": {"expected": "clean"}}}), encoding="utf-8"
        )
        (project / "tsconfig.json").write_text(
            json.dumps({"compilerOptions": {"incremental": True}}), encoding="utf-8"
        )
        (project / "index.ts").write_text("export const value = 1;\n", encoding="utf-8")
        attempts: list = []
        result = MODULE._run_corpus(
            corpus,
            {"reference": str(self.reference), "tsrs-default": str(self.tsrs), "tsrs-fast": str(self.tsrs)},
            None,
            {},
            attempts,
            self.root,
            ("cold", "warm"),
        )
        self.assertTrue(result["ok"])
        self.assertEqual(
            sorted(result["projects"][0]["modes"]),
            ["reference", "tsrs-default", "tsrs-fast"],
        )
        labels = {attempt.label for attempt in attempts}
        for tool in ("reference", "tsrs-default", "tsrs-fast"):
            for scenario in ("cold", "warm"):
                self.assertIn(f"corpus-valid-{tool}-{scenario}", labels)
        for attempt in attempts:
            if "tsrs-default" in attempt.label:
                self.assertIn("--fast", attempt.argv)

    def test_supported_checker_counts_match_on_corpus(self) -> None:
        corpus = self.root / "corpus"
        project = corpus / "valid"
        project.mkdir(parents=True)
        (corpus / "corpus.json").write_text(
            json.dumps({"projects": {"valid": {"expected": "clean"}}}), encoding="utf-8"
        )
        (project / "tsconfig.json").write_text(
            json.dumps({"compilerOptions": {"incremental": True}}), encoding="utf-8"
        )
        (project / "index.ts").write_text("export const value = 1;\n", encoding="utf-8")
        for checkers in MODULE.SUPPORTED_CHECKER_COUNTS:
            attempts: list = []
            result = MODULE._run_corpus(
                corpus,
                {
                    "reference": str(self.reference),
                    "tsrs-default": str(self.tsrs),
                    "tsrs-fast": str(self.tsrs),
                },
                checkers,
                {},
                attempts,
                self.root,
                ("cold",),
            )
            self.assertTrue(result["ok"], checkers)
            for attempt in attempts:
                if "reference" in attempt.label or "tsrs" in attempt.label:
                    index = attempt.argv.index("--checkers")
                    self.assertEqual(attempt.argv[index + 1], str(checkers))

    def test_missing_negative_corpus_expectation_fails(self) -> None:
        corpus = self.root / "corpus"
        project = corpus / "invalid"
        project.mkdir(parents=True)
        (corpus / "corpus.json").write_text(
            json.dumps(
                {
                    "projects": {
                        "invalid": {
                            "expected": "errors",
                            "minimum_count": 1,
                            "required_codes": ["TS2322"],
                        }
                    }
                }
            ),
            encoding="utf-8",
        )
        (project / "tsconfig.json").write_text(
            json.dumps({"compilerOptions": {"incremental": True}}), encoding="utf-8"
        )
        (project / "index.ts").write_text("export const value = 1;\n", encoding="utf-8")
        result = MODULE._run_corpus(
            corpus,
            {"reference": str(self.reference), "tsrs-default": str(self.tsrs), "tsrs-fast": str(self.tsrs)},
            None,
            {},
            [],
            self.root,
            ("cold", "warm"),
        )
        self.assertFalse(result["ok"])
        self.assertFalse(result["projects"][0]["ok"])

    def test_exact_corpus_count_rejects_extra_negative_diagnostics(self) -> None:
        entry = {
            "expected": "errors",
            "minimum_count": 1,
            "exact_count": 1,
            "required_codes": ["TS2322"],
        }
        one = [MODULE.NormalizedDiagnostic("index.ts", 1, 1, "TS2322", "error", "x")]
        self.assertTrue(MODULE._corpus_expectation_ok(entry, one))
        extra = one + [MODULE.NormalizedDiagnostic("index.ts", 2, 1, "TS2339", "error", "y")]
        self.assertFalse(MODULE._corpus_expectation_ok(entry, extra))

    def test_warm_only_comparison_is_rejected(self) -> None:
        code, report = self._compare(["--cache-state", "warm"])
        self.assertEqual(code, 2)
        self.assertFalse(report["ok"])
        self.assertTrue(any("warm-only" in item for item in report["failures"]))

    def test_compare_lint_writes_partial_report_on_failure(self) -> None:
        output = self.root / "lint-report.json"
        code = MODULE.main(
            [
                "compare-lint",
                "--web-root",
                str(self.web_root),
                "--config",
                "missing-config.mjs",
                "--output",
                str(output),
            ]
        )
        self.assertEqual(code, 2)
        report = json.loads(output.read_text(encoding="utf-8"))
        self.assertFalse(report["ok"])
        self.assertFalse(report["complete"])
        self.assertTrue(report["failures"])

    def test_benchmark_writes_partial_report_on_failure(self) -> None:
        output = self.root / "benchmark"
        code = MODULE.main(
            [
                "benchmark",
                "--web-root",
                str(self.web_root),
                "--output",
                str(output),
                "--pairs",
                "1",
                "--tsc",
                str(self.root / "missing-tsc"),
            ]
        )
        self.assertEqual(code, 2)
        report = json.loads((output / "report.json").read_text(encoding="utf-8"))
        self.assertFalse(report["ok"])
        self.assertFalse(report["complete"])
        self.assertTrue(any("incomplete evidence" in item for item in report["failures"]))

    def test_required_corpus_must_exist(self) -> None:
        missing = self.root / "missing-corpus"
        with self.assertRaises(MODULE.EvidenceError):
            MODULE._resolve_corpus(
                argparse.Namespace(corpus=str(missing), no_corpus=False)
            )
        original = MODULE._default_corpus_dir
        MODULE._default_corpus_dir = lambda: missing
        try:
            with self.assertRaises(MODULE.EvidenceError):
                MODULE._resolve_corpus(argparse.Namespace(corpus=None, no_corpus=False))
        finally:
            MODULE._default_corpus_dir = original
        self.assertEqual(
            MODULE._resolve_corpus(argparse.Namespace(corpus=None, no_corpus=True)),
            (None, "diagnostic-only"),
        )

    def test_exit_two_with_only_stderr_is_rejected(self) -> None:
        os.environ["FAKE_STDERR_FAIL"] = "1"
        try:
            code, report = self._compare()
        finally:
            os.environ.pop("FAKE_STDERR_FAIL", None)
        self.assertEqual(code, 2)
        self.assertFalse(report["ok"])
        self.assertTrue(
            any("failure exit without complete diagnostics" in item for item in report["failures"])
        )

    def test_diagnostics_with_success_exit_are_rejected(self) -> None:
        os.environ["FAKE_SUCCESS_DIAGNOSTICS"] = "1"
        try:
            code, report = self._compare()
        finally:
            os.environ.pop("FAKE_SUCCESS_DIAGNOSTICS", None)
        self.assertEqual(code, 2)
        self.assertFalse(report["ok"])
        self.assertTrue(
            any("diagnostics with a success exit" in item for item in report["failures"])
        )

    def test_truncated_output_is_rejected_and_read_is_bounded(self) -> None:
        payload = self.root / "large.txt"
        payload.write_bytes(b"x" * 100)
        text, truncated = MODULE._read_bounded(payload)
        self.assertEqual(len(text), 100)
        self.assertFalse(truncated)
        original = MODULE.MAX_OUTPUT_BYTES
        MODULE.MAX_OUTPUT_BYTES = 20
        try:
            text, truncated = MODULE._read_bounded(payload)
            self.assertEqual(len(text), 20)
            self.assertTrue(truncated)
            with self.assertRaises(MODULE.EvidenceError):
                MODULE.collect_type_inventory(
                    self.web_root, str(self.tsc), project="tsconfig.json"
                )
        finally:
            MODULE.MAX_OUTPUT_BYTES = original

    def test_fake_tool_truncation_is_rejected(self) -> None:
        original = MODULE.MAX_OUTPUT_BYTES
        MODULE.MAX_OUTPUT_BYTES = 50
        os.environ["FAKE_LONG_OUTPUT"] = "1"
        try:
            attempt = MODULE.run_process(
                "long", [str(self.tsrs), "--noEmit"], self.web_root
            )
            self.assertTrue(attempt.stdout_truncated)
            with self.assertRaises(MODULE.EvidenceError):
                MODULE._validate_compiler_attempt(attempt, self.web_root, "long")
        finally:
            MODULE.MAX_OUTPUT_BYTES = original
            os.environ.pop("FAKE_LONG_OUTPUT", None)

    def test_truncated_diagnostic_output_is_rejected(self) -> None:
        attempt = MODULE.Attempt(
            label="truncated",
            argv=["fake", "--noEmit"],
            cwd=str(self.web_root),
            started_at="now",
            exit_code=2,
            timed_out=False,
            interrupted=False,
            elapsed_seconds=0.0,
            cpu_seconds=None,
            max_rss_kb=None,
            stdout="index.ts(1,1): error TS2322: x",
            stderr="",
            stdout_truncated=True,
            stderr_truncated=False,
        )
        with self.assertRaises(MODULE.EvidenceError):
            MODULE._validate_compiler_attempt(attempt, self.web_root, "truncated")

    def test_inventory_rejects_non_path_output(self) -> None:
        os.environ["FAKE_INVENTORY_NOT_A_PATH"] = "1"
        try:
            with self.assertRaises(MODULE.EvidenceError):
                MODULE.collect_type_inventory(
                    self.web_root, str(self.tsc), project="tsconfig.json"
                )
        finally:
            os.environ.pop("FAKE_INVENTORY_NOT_A_PATH", None)

    def test_median(self) -> None:
        self.assertEqual(MODULE._median([]), None)
        self.assertEqual(MODULE._median([3.0]), 3.0)
        self.assertEqual(MODULE._median([1.0, 3.0, 2.0]), 2.0)
        self.assertEqual(MODULE._median([1.0, 2.0, 3.0, 4.0]), 2.5)


class LegacyCompilerCorpusTest(unittest.TestCase):
    """The installed legacy compiler proves the fixture corpus is real."""

    def test_corpus_projects_compile_as_declared(self) -> None:
        tsc = Path(__file__).resolve().parents[2] / "apps" / "web" / "node_modules" / ".bin" / "tsc"
        if not tsc.exists():
            self.skipTest("workspace TypeScript compiler is not installed")
        fixtures = Path(__file__).resolve().parent / "fixtures" / "web-toolchain" / "types"
        valid = subprocess.run(
            [str(tsc), "--noEmit", "--project", str(fixtures / "valid" / "tsconfig.json"), "--pretty", "false"],
            capture_output=True,
            text=True,
            timeout=120,
            check=False,
        )
        self.assertEqual(valid.returncode, 0, valid.stdout + valid.stderr)
        invalid = subprocess.run(
            [str(tsc), "--noEmit", "--project", str(fixtures / "invalid" / "tsconfig.json"), "--pretty", "false"],
            capture_output=True,
            text=True,
            timeout=120,
            check=False,
        )
        self.assertNotEqual(invalid.returncode, 0)
        diagnostics = MODULE.parse_diagnostics(invalid.stdout, fixtures / "invalid")
        self.assertGreaterEqual(len(diagnostics), 8)
        self.assertIn("TS2322", {item.code for item in diagnostics})
        instantiation = subprocess.run(
            [
                str(tsc),
                "--noEmit",
                "--project",
                str(fixtures / "instantiation" / "tsconfig.json"),
                "--pretty",
                "false",
            ],
            capture_output=True,
            text=True,
            timeout=120,
            check=False,
        )
        self.assertNotEqual(instantiation.returncode, 0)
        codes = {
            item.code
            for item in MODULE.parse_diagnostics(instantiation.stdout, fixtures / "instantiation")
        }
        self.assertIn("TS2589", codes)


if __name__ == "__main__":
    unittest.main()
