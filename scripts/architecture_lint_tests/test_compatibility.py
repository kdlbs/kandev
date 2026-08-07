"""Compatibility-ledger validation tests."""

import json
import tempfile
from pathlib import Path

from support import ArchitectureFixture


class CompatibilityLedgerTest(ArchitectureFixture):
    def test_malformed_and_duplicate_entries_fail(self) -> None:
        entry = self.valid_ledger_entry()
        entry.pop("owner")
        self.write_ledger([entry])
        self.track_all()
        malformed = self.run_cli("--all")
        self.assertEqual(malformed.returncode, 1)
        self.assertIn("owner", malformed.stdout)

        entry = self.valid_ledger_entry()
        self.write_ledger([entry, entry])
        duplicate = self.run_cli("--all")
        self.assertEqual(duplicate.returncode, 1)
        self.assertIn("duplicate compatibility id", duplicate.stdout)

    def test_invalid_and_expired_dates_fail(self) -> None:
        entry = self.valid_ledger_entry()
        entry["introduced_on"] = "2026-99-99"
        self.write_ledger([entry])
        self.track_all()
        invalid = self.run_cli("--all")
        self.assertEqual(invalid.returncode, 1)
        self.assertIn("invalid introduced_on date", invalid.stdout)

        entry = self.valid_ledger_entry()
        entry["target_removal_date"] = "2000-01-01"
        self.write_ledger([entry])
        expired = self.run_cli("--all")
        self.assertEqual(expired.returncode, 1)
        self.assertIn("expired on 2000-01-01", expired.stdout)

    def test_missing_path_or_marker_fails(self) -> None:
        self.write_ledger([self.valid_ledger_entry()])
        self.track_all()
        missing_path = self.run_cli("--all")
        self.assertEqual(missing_path.returncode, 1)
        self.assertIn("referenced path is not tracked", missing_path.stdout)

        self.write("apps/backend/internal/example/compat.go", "package example\n// different\n")
        self.track_all()
        missing_marker = self.run_cli("--all")
        self.assertEqual(missing_marker.returncode, 1)
        self.assertIn("marker no longer exists", missing_marker.stdout)

    def test_valid_registered_entry_passes(self) -> None:
        marker = "// compat: old-name"
        self.write("apps/backend/internal/example/compat.go", f"package example\n{marker}\n")
        self.write_ledger([self.valid_ledger_entry(marker=marker)])
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_semver_accepts_prerelease_and_build_metadata(self) -> None:
        marker = "// compat: semver"
        self.write("apps/backend/internal/example/compat.go", f"package example\n{marker}\n")
        entry = self.valid_ledger_entry(marker=marker)
        entry.pop("introduced_on")
        entry["introduced_version"] = "1.2.3-alpha+build.7"
        entry.pop("target_removal_date")
        entry["target_removal_version"] = "2.0.0"
        self.write_ledger([entry])
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_semver_rejects_leading_zero_and_empty_identifiers(self) -> None:
        marker = "// compat: semver"
        self.write("apps/backend/internal/example/compat.go", f"package example\n{marker}\n")
        for invalid in ("01.2.3", "1.2.3-..", "1.2.3-alpha..1"):
            with self.subTest(invalid=invalid):
                entry = self.valid_ledger_entry(marker=marker)
                entry.pop("introduced_on")
                entry["introduced_version"] = invalid
                self.write_ledger([entry])
                self.track_all()

                result = self.run_cli("--all")

                self.assertEqual(result.returncode, 1)
                self.assertIn("invalid introduced_version", result.stdout)

    def test_ledger_outside_repository_uses_absolute_label(self) -> None:
        marker = "// compat: external-ledger"
        self.write("apps/backend/internal/example/compat.go", f"package example\n{marker}\n")
        self.track_all()

        with tempfile.TemporaryDirectory() as tempdir:
            ledger_path = Path(tempdir) / "compatibility-ledger.json"
            ledger_path.write_text(
                json.dumps({"version": 1, "entries": [self.valid_ledger_entry(marker=marker)]}) + "\n",
                encoding="utf-8",
            )
            result = self.run_cli("--all", "--ledger", str(ledger_path))

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
