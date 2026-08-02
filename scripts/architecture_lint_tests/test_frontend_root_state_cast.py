"""Tests for ARCH-FRONTEND-ROOT-STATE-CAST."""

from support import ArchitectureFixture, ROOT_STATE_RULE


class FrontendRootStateCastTest(ArchitectureFixture):
    def test_new_unsafe_cast_reports_typed_replacement(self) -> None:
        path = "apps/web/lib/state/store.ts"
        self.write(path, "const unsafe = state as unknown as RootState;\n")
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn(f"{path}:1", result.stdout)
        self.assertIn(ROOT_STATE_RULE, result.stdout)
        self.assertIn("typed domain state, actions, and defaults", result.stdout)
