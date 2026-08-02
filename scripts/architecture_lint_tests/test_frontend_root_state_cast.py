"""Tests for ARCH-FRONTEND-ROOT-STATE-CAST."""

from support import ArchitectureFixture, ROOT_STATE_RULE


class FrontendRootStateCastTest(ArchitectureFixture):
    def test_new_unsafe_cast_reports_typed_replacement(self) -> None:
        path = "apps/web/lib/state/store.ts"
        self.write(path, "const unsafe = state as unknown as RootState;\n")
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 1)
        self.assertIn(ROOT_STATE_RULE, result.stdout)
        self.assertIn("typed domain state, actions, and defaults", result.stdout)

    def test_multiline_double_cast_reports_first_cast_line(self) -> None:
        path = "apps/web/lib/state/store.ts"
        self.write(path, "const unsafe = (state as unknown)\n  as RootState;\n")
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 1)
        self.assertIn("as unknown as", result.stdout)

    def test_comments_between_cast_tokens_are_detected(self) -> None:
        path = "apps/web/lib/state/store.ts"
        self.write(
            path,
            "const unsafe = state as /* explanation */ unknown /* explanation */ as RootState;\n",
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 1)
        self.assertIn("as unknown as", result.stdout)

    def test_template_interpolations_are_scanned_but_template_text_is_ignored(self) -> None:
        path = "apps/web/lib/state/store.ts"
        self.write(
            path,
            "const text = `literal state as any ${state as any} ${state as unknown as RootState}`;\n",
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout.count(ROOT_STATE_RULE), 2)

    def test_duplicate_violation_lines_get_distinct_occurrences(self) -> None:
        path = "apps/web/lib/state/store.ts"
        marker = "const unsafe = state as any;"
        self.write(path, f"{marker}\n{marker}\n")
        self.write_baseline(
            root_state=[
                {
                    "escape": "as any",
                    "marker": marker,
                    "occurrence": 1,
                    "path": path,
                }
            ]
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 2)
