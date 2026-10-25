"""Tests for ARCH-FRONTEND-STATE-UI-IMPORT."""

from support import ArchitectureFixture


RULE = "ARCH-FRONTEND-STATE-UI-IMPORT"
COMPONENT_IMPORT = "@/components/app-sidebar/app-sidebar-constants"


class FrontendStateUIImportTest(ArchitectureFixture):
    @staticmethod
    def baseline_entry(path: str, import_path: str = COMPONENT_IMPORT) -> dict[str, str]:
        return {"path": path, "import": import_path}

    def write_frontend_baseline(self, entries: list[dict[str, str]]) -> None:
        self.write_json(
            "config/architecture-lint/frontend_state_ui_import.json",
            {"version": 1, "rule": RULE, "entries": entries},
        )

    def test_static_type_export_and_dynamic_alias_imports_report(self) -> None:
        path = "apps/web/lib/state/fixture-source.ts"
        self.write(
            path,
            '''import type { Sidebar } from "@/components/sidebar";
export { route } from "@/app/routes";
const lazyPanel = import("@/components/lazy-panel");
import "@/app/side-effect";
export * from "@/components/star-export";
const substituted = `${import("@/components/template-substitution")}`;
const directTemplate = import(`@/components/template-direct`);
''',
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout.count(RULE), 7)
        for line in range(1, 8):
            self.assert_diagnostic_location(result, path, line)
        self.assertIn("components and app layers consume state", result.stdout)

    def test_typescript_import_equals_resolves_alias_and_relative_ui_modules(self) -> None:
        path = "apps/web/lib/state/slices/ui/import-equals.ts"
        self.write(
            path,
            '''import Sidebar = require("@/components/sidebar");
import type AppRoute = require("../../../../app/routes");
import Local = require("./local");
''',
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout.count(RULE), 2)
        self.assert_diagnostic_location(result, path, 1)
        self.assert_diagnostic_location(result, path, 2)

    def test_nested_template_substitution_imports_are_scanned(self) -> None:
        path = "apps/web/lib/state/nested-template.ts"
        self.write(
            path,
            'const nested = `outer ${`inner ${import("@/components/nested")}`} tail`;\n',
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 1)

    def test_nested_template_text_is_not_scanned_as_code(self) -> None:
        path = "apps/web/lib/state/nested-template-text.ts"
        self.write(
            path,
            'const nested = `outer ${`inner import("@/components/template-text")`} tail`;\n',
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_escaped_direct_template_imports_resolve_to_ui_modules(self) -> None:
        path = "apps/web/lib/state/escaped-template.ts"
        self.write(
            path,
            r'''import(`\u0040/components/unicode-panel`);
import(`\x40/app/escaped-route`);
''',
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout.count(RULE), 2)
        self.assert_diagnostic_location(result, path, 1)
        self.assert_diagnostic_location(result, path, 2)

    def test_relative_components_and_app_modules_are_resolved(self) -> None:
        path = "apps/web/lib/state/slices/ui/relative.ts"
        self.write(
            path,
            '''import { Panel } from "../../../../components/app-sidebar/panel";
export { route } from "../../../../app/routes";
import { local } from "./local";
''',
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertEqual(result.stdout.count(RULE), 2)
        self.assert_diagnostic_location(result, path, 1)
        self.assert_diagnostic_location(result, path, 2)

    def test_comments_and_arbitrary_strings_are_ignored(self) -> None:
        path = "apps/web/lib/state/strings.ts"
        self.write(
            path,
            '''// import { Sidebar } from "@/components/sidebar";
const text = "export { route } from '@/app/routes'";
const template = `import("@/components/template")`;
/* export { route } from "@/app/comments" */
const local = import("./local");
const member = loader.import("@/components/member");
''',
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_tests_fixtures_and_generated_files_are_excluded(self) -> None:
        test_path = "apps/web/lib/state/ignored.test.ts"
        fixture_path = "apps/web/lib/state/fixtures/ignored.ts"
        generated_path = "apps/web/lib/state/generated.ts"
        forbidden = 'import "@/components/ignored";\n'
        self.write(test_path, forbidden)
        self.write(fixture_path, forbidden)
        self.write(generated_path, "// Code generated by test. DO NOT EDIT.\n" + forbidden)
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_generated_markers_in_strings_and_unrelated_comments_do_not_exclude(self) -> None:
        path = "apps/web/lib/state/generated-marker-lookalike.ts"
        self.write(
            path,
            '''const warning = "DO NOT EDIT while synchronization is running";
// This comment mentions Code generated but is not a generated header.
import "@/components/forbidden";
''',
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 3)

    def test_current_two_findings_pass_only_with_exact_baseline(self) -> None:
        first = "apps/web/lib/state/slices/ui/app-sidebar-actions.ts"
        second = "apps/web/lib/state/slices/ui/ui-slice.ts"
        import_line = f'import {{ APP_SIDEBAR_EXPANDED_WIDTH }} from "{COMPONENT_IMPORT}";\n'
        self.write(first, import_line)
        self.write(second, import_line)
        self.write_frontend_baseline([self.baseline_entry(first), self.baseline_entry(second)])
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_baseline_identity_includes_import_for_same_path_findings(self) -> None:
        path = "apps/web/lib/state/slices/ui/multiple-imports.ts"
        self.write(
            path,
            '''import "@/components/first";
import "@/app/second";
''',
        )
        self.write_frontend_baseline([self.baseline_entry(path, "@/components/first")])
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, path, 2)
        self.assertIn("@/app/second", result.stdout)

    def test_new_finding_cannot_grow_existing_baseline(self) -> None:
        old_path = "apps/web/lib/state/slices/ui/ui-slice.ts"
        new_path = "apps/web/lib/state/slices/ui/new-ui.ts"
        self.write(old_path, f'import "{COMPONENT_IMPORT}";\n')
        self.write(new_path, 'import "@/app/routes";\n')
        self.write_frontend_baseline([self.baseline_entry(old_path)])
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assert_diagnostic_location(result, new_path, 1)
        self.assertIn(RULE, result.stdout)

    def test_removed_finding_requires_baseline_cleanup(self) -> None:
        path = "apps/web/lib/state/slices/ui/ui-slice.ts"
        self.write(path, "export const state = {};\n")
        self.write_frontend_baseline([self.baseline_entry(path)])
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn(RULE, result.stdout)
        self.assertIn("stale baseline entry", result.stdout)
