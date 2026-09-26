"""Explicit source-deprecation ledger rule tests."""

from support import ArchitectureFixture
from architecture_lint.rules.deprecation_ledger import find_declarations, scan


class DeprecationLedgerTest(ArchitectureFixture):
    def test_new_unregistered_typescript_declaration_fails(self) -> None:
        path = "apps/web/lib/new-api.ts"
        self.write(
            path,
            """
            /** @deprecated Use replacementApi instead. */
            export function oldApi(): void {}
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("ARCH-DEPRECATION-LEDGER", result.stdout)
        self.assertIn("function:oldApi#1", result.stdout)
        self.assertIn("add a matching compatibility-ledger entry", result.stdout)

        repeated = self.run_cli("--all")

        self.assertEqual(repeated.stdout, result.stdout)

    def test_matching_ledger_registration_satisfies_the_rule(self) -> None:
        path = "apps/web/lib/new-api.ts"
        self.write(
            path,
            """
            /** @deprecated Use replacementApi instead. */
            export function oldApi(): void {}
            """,
        )
        self.write_ledger(
            [
                {
                    "id": "old-api",
                    "locator": {
                        "path": path,
                        "declaration": "function:oldApi#1",
                        "marker": "@deprecated",
                    },
                    "reason": "Existing callers still use the old API.",
                    "owner": "web maintainers",
                    "introduced_on": "2026-01-15",
                    "removal_condition": "All callers use replacementApi.",
                    "target_removal_version": "2.0.0",
                }
            ]
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_incomplete_ledger_registration_does_not_satisfy_the_rule(self) -> None:
        path = "apps/web/lib/new-api.ts"
        self.write(
            path,
            """
            /** @deprecated Use replacementApi instead. */
            export function oldApi(): void {}
            """,
        )
        entry = {
            "id": "old-api",
            "locator": {
                "path": path,
                "declaration": "function:oldApi#1",
                "marker": "@deprecated",
            },
            "reason": "Existing callers still use the old API.",
            "owner": "",
            "introduced_on": "2026-01-15",
            "removal_condition": "All callers use replacementApi.",
            "target_removal_version": "2.0.0",
        }
        self.write_ledger([entry])
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("required field owner", result.stdout)
        self.assertIn("ARCH-DEPRECATION-LEDGER", result.stdout)

    def test_ledger_registration_with_wrong_declaration_fails(self) -> None:
        path = "apps/web/lib/new-api.ts"
        self.write(
            path,
            """
            /** @deprecated Use replacementApi instead. */
            export function oldApi(): void {}
            """,
        )
        self.write_ledger(
            [
                {
                    "id": "old-api",
                    "locator": {
                        "path": path,
                        "declaration": "function:otherApi#1",
                        "marker": "@deprecated",
                    },
                    "reason": "Existing callers still use the old API.",
                    "owner": "web maintainers",
                    "introduced_on": "2026-01-15",
                    "removal_condition": "All callers use replacementApi.",
                    "target_removal_version": "2.0.0",
                }
            ]
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("locator declaration does not match", result.stdout)

    def test_plain_prose_and_strings_do_not_create_annotations(self) -> None:
        source = '''
        // Legacy fallback compatibility remains part of the domain model.
        const ordinaryComment = "Deprecated: this is only text";
        const docString = "/** @deprecated not a JSDoc comment */";
        const templateText = `/** @deprecated also only text */`;
        // @deprecated is not a JSDoc tag.
        '''

        findings = find_declarations("apps/web/lib/notes.ts", source)

        self.assertEqual(findings, [])

    def test_detached_deprecation_comments_do_not_attach_to_later_declarations(self) -> None:
        go_source = """\
        package example
        // Deprecated: historical note, detached from the declaration.

        func Current() {}
        """
        typescript_source = """\
        /** @deprecated Historical note, detached from the declaration. */

        export function currentApi(): void;
        """

        self.assertEqual(
            find_declarations("apps/backend/internal/example/current.go", go_source),
            [],
        )
        self.assertEqual(
            find_declarations("apps/web/lib/current.ts", typescript_source),
            [],
        )

    def test_generated_test_fixture_and_third_party_sources_are_excluded(self) -> None:
        source = """\
        /** @deprecated Old API. */
        export function oldApi(): void;
        """

        paths = [
            "apps/web/lib/generated/old-api.ts",
            "apps/web/lib/__tests__/old-api.ts",
            "apps/web/lib/fixtures/old-api.ts",
            "apps/web/node_modules/vendor/old-api.ts",
            "apps/web/lib/old-api.test.ts",
            "apps/web/lib/old_api_test.ts",
            "apps/web/lib/old-api.fixture.ts",
            "apps/web/lib/old-api.gen.ts",
        ]

        for path in paths:
            with self.subTest(path=path):
                self.assertEqual(find_declarations(path, source), [])

        self.assertEqual(
            find_declarations(
                "apps/backend/vendor/example.go",
                "// Deprecated: Old\nfunc Old() {}",
            ),
            [],
        )

        self.assertEqual(
            find_declarations(
                "apps/web/lib/old-api.ts",
                "// @generated file\n" + source,
            ),
            [],
        )

    def test_go_annotations_cover_fields_but_ignore_prose_and_strings(self) -> None:
        source = '''
        package example

        // compatibility legacy fallback prose is not an annotation
        type Payload[T any] struct {
          // Deprecated: use Current instead.
          Old string
          Current string // Deprecated: retained for old clients.
        }
        var note = "Deprecated: this is only a string"
        '''

        findings = find_declarations("apps/backend/internal/example/payload.go", source)

        self.assertEqual(
            findings,
            [
                (6, "field:Payload.Old", "Deprecated:"),
                (8, "field:Payload.Current", "Deprecated:"),
            ],
        )

    def test_go_interface_method_annotation_uses_qualified_identity(self) -> None:
        source = """\
        package example
        type Service interface {
          // Deprecated: use CloseContext.
          Close() error
        }
        """

        findings = find_declarations("apps/backend/internal/example/service.go", source)

        self.assertEqual(findings, [(3, "method:Service.Close", "Deprecated:")])

    def test_go_top_level_type_and_function_annotations_are_detected(self) -> None:
        source = """\
        package example
        // Deprecated: use CurrentType.
        type OldType string
        // Deprecated: use CurrentFunc.
        func OldFunc() {}
        """

        findings = find_declarations("apps/backend/internal/example/old.go", source)

        self.assertEqual(
            findings,
            [
                (2, "type:OldType", "Deprecated:"),
                (4, "func:OldFunc", "Deprecated:"),
            ],
        )

    def test_existing_unregistered_declaration_passes_its_exact_baseline(self) -> None:
        path = "apps/web/lib/old-api.ts"
        self.write(
            path,
            """
            /** @deprecated Use replacementApi instead. */
            export function oldApi(): void {}
            """,
        )
        self.write_baseline(
            deprecation_ledger=[
                {"path": path, "declaration": "function:oldApi#1", "marker": "@deprecated"}
            ]
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_removed_declaration_makes_its_baseline_entry_stale(self) -> None:
        path = "apps/web/lib/old-api.ts"
        self.write_baseline(
            deprecation_ledger=[
                {"path": path, "declaration": "function:oldApi#1", "marker": "@deprecated"}
            ]
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1)
        self.assertIn("stale baseline entry", result.stdout)

    def test_baseline_growth_after_rollout_is_rejected(self) -> None:
        path = "apps/web/lib/old-api.ts"
        self.write(
            path,
            """
            /** @deprecated Use replacementApi instead. */
            export function oldApi(): void {}
            """,
        )
        self.track_all()
        self.git("commit", "-m", "baseline")
        self.write_baseline(
            deprecation_ledger=[
                {"path": path, "declaration": "function:oldApi#1", "marker": "@deprecated"}
            ]
        )
        self.track_all()

        result = self.run_cli("--all", "--baseline-base-ref", "HEAD")

        self.assertEqual(result.returncode, 1)
        self.assertIn("baseline may only shrink", result.stdout)

    def test_identity_does_not_change_with_comment_or_line_formatting(self) -> None:
        first = """\
        /** @deprecated Use anotherApi. */
        export function oldApi(): void;
        """
        reformatted = """\
        /**
         * @deprecated Keep the explanation here.
         */
        export function oldApi(): void;
        """

        first_identity = scan("apps/web/lib/old-api.ts", first)[0].identity_dict()
        reformatted_identity = scan("apps/web/lib/old-api.ts", reformatted)[0].identity_dict()

        self.assertEqual(first_identity, reformatted_identity)

    def test_generated_go_and_test_sources_are_excluded(self) -> None:
        source = """\
        // Deprecated: use New instead.
        func Old() {}
        """

        self.assertEqual(
            find_declarations("apps/backend/internal/example/old_test.go", source),
            [],
        )
        self.assertEqual(
            find_declarations(
                "apps/backend/internal/example/generated/old.go",
                "// Code generated by tool. DO NOT EDIT.\n" + source,
            ),
            [],
        )

    def test_typescript_module_extensions_are_scanned(self) -> None:
        source = """\
        /** @deprecated Use New instead. */
        export function Old(): void;
        """

        self.assertEqual(
            find_declarations("apps/web/lib/old-api.mts", source),
            [(1, "function:Old", "@deprecated")],
        )

    def test_multiline_jsdoc_uses_tag_line_and_member_identity(self) -> None:
        source = """\
        export type Payload = {
          /**
           * @deprecated Use currentField instead.
           */
          oldField?: string;
        };
        """

        findings = find_declarations("apps/web/lib/payload.ts", source)

        self.assertEqual(findings, [(3, "property:Payload.oldField", "@deprecated")])

    def test_repeated_declarations_in_one_file_have_distinct_identities(self) -> None:
        source = """\
        /** @deprecated Use currentApi instead. */
        export function oldApi(): void;
        /** @deprecated Use currentApi instead. */
        export function oldApi(value: string): void;
        """

        findings = scan("apps/web/lib/old-api.ts", source)

        self.assertEqual(
            [finding.identity_dict()["declaration"] for finding in findings],
            ["function:oldApi#1", "function:oldApi#2"],
        )
