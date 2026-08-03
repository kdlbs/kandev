"""Cross-rule baseline behavior tests."""

from support import ArchitectureFixture, ROOT_STATE_RULE, RULE_FILES, RUNTIME_IMPORT, RUNTIME_RULE


class BaselineTest(ArchitectureFixture):
    def test_removed_violation_requires_its_rule_baseline_to_shrink(self) -> None:
        path = "apps/backend/internal/example/removed.go"
        self.write(path, "package example\n")
        self.write_baseline(runtime=[self.runtime_entry(path)])
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn(RUNTIME_RULE, result.stdout)
        self.assertIn("stale baseline entry", result.stdout)
        self.assertIn("runtime_import.json", result.stdout)

    def test_comparison_rejects_growth_but_allows_shrinkage(self) -> None:
        old_path = "apps/backend/internal/example/old.go"
        new_path = "apps/backend/internal/example/new.go"
        self.write(old_path, f'package example\nimport "{RUNTIME_IMPORT}"\n')
        self.write_baseline(runtime=[self.runtime_entry(old_path)])
        self.track_all()
        self.git("commit", "-qm", "base")
        base = self.git("rev-parse", "HEAD").stdout.strip()

        self.write(new_path, f'package example\nimport "{RUNTIME_IMPORT}"\n')
        self.write_baseline(runtime=[self.runtime_entry(old_path), self.runtime_entry(new_path)])
        self.track_all()
        growth = self.run_cli("--all", "--baseline-base-ref", base)

        self.assertEqual(growth.returncode, 1)
        self.assertIn("baseline may only shrink", growth.stdout)
        self.assertIn(new_path, growth.stdout)

        (self.repo / new_path).unlink()
        (self.repo / old_path).write_text("package example\n", encoding="utf-8")
        self.write_baseline(runtime=[])
        self.track_all()
        shrink = self.run_cli("--all", "--baseline-base-ref", base)
        self.assertEqual(shrink.returncode, 0, shrink.stdout + shrink.stderr)

    def test_new_rule_can_bootstrap_its_own_missing_base_baseline(self) -> None:
        relative = f"config/architecture-lint/{RULE_FILES[ROOT_STATE_RULE]}"
        (self.repo / relative).unlink()
        self.track_all()
        self.git("commit", "-qm", "base without future rule")
        base = self.git("rev-parse", "HEAD").stdout.strip()
        self.write_baseline()
        self.track_all()

        result = self.run_cli(
            "--all",
            "--baseline-base-ref",
            base,
            "--allow-missing-base-baseline",
        )

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)
