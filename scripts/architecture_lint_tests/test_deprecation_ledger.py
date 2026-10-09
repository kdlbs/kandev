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
        self.assertIn("function:oldApi[signature=", result.stdout)
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
                        "declaration": "function:oldApi[signature=( )]#1",
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
                "declaration": "function:oldApi[signature=( )]#1",
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

    def test_go_grouped_declarations_are_detected_by_declared_name(self) -> None:
        source = """\
        package example
        const (
          // Deprecated: use CurrentValue.
          OldValue, OldAlias = 1, 2
        )
        var (
          // Deprecated: use CurrentVar.
          OldVar struct{}
        )
        type (
          // Deprecated: use CurrentType.
          OldType int
        )
        """

        findings = find_declarations("apps/backend/internal/example/grouped.go", source)

        self.assertEqual(
            findings,
            [
                (3, "const:OldValue", "Deprecated:"),
                (3, "const:OldAlias", "Deprecated:"),
                (7, "var:OldVar", "Deprecated:"),
                (11, "type:OldType", "Deprecated:"),
            ],
        )

    def test_go_embedded_fields_are_detected_with_normalized_names(self) -> None:
        source = """\
        package example
        type Wrapper struct {
          // Deprecated: use CurrentType.
          OldType
          // Deprecated: use CurrentPointer.
          *pkg.OldPointer
          // Deprecated: use CurrentGeneric.
          OldGeneric[T]
        }
        type Service interface {
          // Deprecated: use CurrentService.
          OldService
        }
        """

        findings = find_declarations("apps/backend/internal/example/embedded.go", source)

        self.assertEqual(
            findings,
            [
                (3, "field:Wrapper.OldType", "Deprecated:"),
                (5, "field:Wrapper.OldPointer", "Deprecated:"),
                (7, "field:Wrapper.OldGeneric", "Deprecated:"),
                (11, "field:Service.OldService", "Deprecated:"),
            ],
        )

    def test_go_grouped_type_spec_keeps_struct_field_scope(self) -> None:
        source = """\
        package example
        type (
          // Deprecated: use CurrentPayload.
          OldPayload struct {
            // Deprecated: use CurrentField.
            OldField string
          }
        )
        """

        findings = find_declarations("apps/backend/internal/example/grouped_type.go", source)

        self.assertEqual(
            findings,
            [
                (3, "type:OldPayload", "Deprecated:"),
                (5, "field:OldPayload.OldField", "Deprecated:"),
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
                {
                    "path": path,
                    "declaration": "function:oldApi[signature=( )]#1",
                    "marker": "@deprecated",
                }
            ]
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 0, result.stdout + result.stderr)

    def test_removed_declaration_makes_its_baseline_entry_stale(self) -> None:
        path = "apps/web/lib/old-api.ts"
        self.write_baseline(
            deprecation_ledger=[
                {
                    "path": path,
                    "declaration": "function:oldApi[signature=( )]#1",
                    "marker": "@deprecated",
                }
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
                {
                    "path": path,
                    "declaration": "function:oldApi[signature=( )]#1",
                    "marker": "@deprecated",
                }
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
            [(1, "function:Old[signature=( )]", "@deprecated")],
        )

    def test_tsx_apostrophe_in_jsx_text_does_not_hide_later_deprecations(self) -> None:
        source = """\
        export const RetryLabel = () => <p>Don't retry this request</p>;
        /** @deprecated Use currentApi instead. */
        export function oldApi(): void;
        """

        self.assertEqual(
            find_declarations("apps/web/lib/retry-label.tsx", source),
            [(2, "function:oldApi[signature=( )]", "@deprecated")],
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

    def test_nested_typescript_type_literals_keep_their_declaration_scope(self) -> None:
        source = """\
        export interface Options {
          config: {
            /** @deprecated Use current instead. */
            old: string;
          };
        }
        export type Constrained<T extends {
          /** @deprecated Use current instead. */
          old: string;
        }> = T;
        """

        findings = find_declarations("apps/web/lib/options.ts", source)

        self.assertEqual(
            findings,
            [
                (3, "property:Options.config.old", "@deprecated"),
                (8, "property:Constrained.old", "@deprecated"),
            ],
        )

    def test_nested_typescript_type_literals_under_string_keys_keep_identity(self) -> None:
        source = """\
        export interface Options {
          "legacy-config": {
            /** @deprecated Use current instead. */
            old: string;
          };
        }
        """

        self.assertEqual(
            find_declarations("apps/web/lib/options.ts", source),
            [(3, 'property:Options["legacy-config"].old', "@deprecated")],
        )

    def test_typescript_declarations_after_decorators_are_detected(self) -> None:
        source = """\
        /** @deprecated Use CurrentClass instead. */
        @sealed
        export class OldClass {}
        export interface Service {
          /** @deprecated Use currentMethod instead. */
          @trace
          oldMethod(): void;
        }
        """

        findings = find_declarations("apps/web/lib/decorated.ts", source)

        self.assertEqual(
            findings,
            [
                (1, "class:OldClass", "@deprecated"),
                (5, "method:Service.oldMethod[signature=( )]", "@deprecated"),
            ],
        )

    def test_typescript_local_variables_are_not_api_declarations(self) -> None:
        source = """\
        export class Example {
          run() {
            /** @deprecated This is local state. */
            const oldValue = 1;
          }
        }
        """

        self.assertEqual(
            find_declarations("apps/web/lib/example.ts", source),
            [],
        )

    def test_typescript_member_identity_supports_string_numeric_and_computed_keys(self) -> None:
        source = """\
        export interface Keys {
          /** @deprecated Use currentKey instead. */
          "old-key": string;
          /** @deprecated Use currentIndex instead. */
          4: string;
          /** @deprecated Use currentIterator instead. */
          [Symbol.iterator](): Iterator<string>;
        }
        """

        findings = find_declarations("apps/web/lib/keys.ts", source)

        self.assertEqual(
            findings,
            [
                (2, 'property:Keys["old-key"]', "@deprecated"),
                (4, "property:Keys[4]", "@deprecated"),
                (6, "method:Keys[Symbol.iterator][signature=( )]", "@deprecated"),
            ],
        )

    def test_typescript_member_keys_have_format_stable_identities(self) -> None:
        double_quoted = '''\
        interface Keys {
          /** @deprecated Use current. */
          "old-key": string;
          /** @deprecated Use current. */
          [Symbol.iterator](): Iterator<string>;
        }
        '''
        reformatted = '''\
        interface Keys {
          /**
           * @deprecated Keep the explanation here.
           */
          'old-key': string;
          /** @deprecated Use current. */
          [Symbol . iterator](): Iterator<string>;
        }
        '''

        first = scan("apps/web/lib/keys.ts", double_quoted)
        second = scan("apps/web/lib/keys.ts", reformatted)

        self.assertEqual(
            [finding.identity_dict()["declaration"] for finding in first],
            [finding.identity_dict()["declaration"] for finding in second],
        )

    def test_repeated_declarations_in_one_file_have_distinct_identities(self) -> None:
        source = """\
        /** @deprecated Use currentApi instead. */
        export function oldApi(): void;
        /** @deprecated Use currentApi instead. */
        export function oldApi(value: string): void;
        """

        findings = scan("apps/web/lib/old-api.ts", source)

        declarations = [finding.identity_dict()["declaration"] for finding in findings]
        self.assertEqual(len(declarations), 2)
        self.assertEqual(len(set(declarations)), 2)
        self.assertTrue(all(declaration.endswith("#1") for declaration in declarations))
        self.assertTrue(all(declaration.startswith("function:oldApi[signature=") for declaration in declarations))

    def test_overload_identities_survive_reordering_and_signature_formatting(self) -> None:
        source = """\
        /** @deprecated Use currentApi instead. */
        export function oldApi(value: 'ready'): void;
        /** @deprecated Use currentApi instead. */
        export function oldApi(value: number): void;
        """
        reordered_and_reformatted = """\
        /** @deprecated Use currentApi instead. */
        export function oldApi(
          value : number
        ) : void;
        /** @deprecated Use currentApi instead. */
        export function oldApi(value: "ready"): void;
        """

        original = {
            finding.identity_dict()["declaration"]
            for finding in scan("apps/web/lib/old-api.ts", source)
        }
        reformatted = {
            finding.identity_dict()["declaration"]
            for finding in scan("apps/web/lib/old-api.ts", reordered_and_reformatted)
        }

        self.assertEqual(len(original), 2)
        self.assertTrue(all(declaration.endswith("#1") for declaration in original))
        self.assertEqual(reformatted, original)

    def test_overloaded_methods_keep_their_container_in_the_identity(self) -> None:
        source = """\
        export interface Service {
          /** @deprecated Use currentApi instead. */
          oldApi(value: string): void;
          /** @deprecated Use currentApi instead. */
          oldApi(value: number): void;
        }
        export interface Worker {
          /** @deprecated Use currentApi instead. */
          oldApi(value: string): void;
        }
        """

        declarations = [
            finding.identity_dict()["declaration"]
            for finding in scan("apps/web/lib/old-api.ts", source)
        ]

        self.assertEqual(len(declarations), 3)
        self.assertEqual(len(set(declarations)), 3)
        self.assertTrue(all(declaration.endswith("#1") for declaration in declarations))
        self.assertTrue(any(declaration.startswith("method:Service.oldApi[") for declaration in declarations))
        self.assertTrue(any(declaration.startswith("method:Worker.oldApi[") for declaration in declarations))

    def test_removed_overload_does_not_transfer_its_registration(self) -> None:
        path = "apps/web/lib/old-api.ts"
        original_source = """
            /** @deprecated Use currentApi instead. */
            export function oldApi(value: string): void;
            /** @deprecated Use currentApi instead. */
            export function oldApi(value: number): void;
        """
        self.write(
            path,
            original_source,
        )
        first_declaration = scan(path, original_source)[0].identity_dict()["declaration"]
        self.write_ledger(
            [
                {
                    "id": "old-api-string-overload",
                    "locator": {
                        "path": path,
                        "declaration": first_declaration,
                        "marker": "@deprecated",
                    },
                    "reason": "Existing callers still use the string overload.",
                    "owner": "web maintainers",
                    "introduced_on": "2026-01-15",
                    "removal_condition": "All callers use currentApi.",
                    "target_removal_version": "2.0.0",
                }
            ]
        )
        self.write(
            path,
            """
            /** @deprecated Use currentApi instead. */
            export function oldApi(value: number): void;
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("old-api-string-overload: locator declaration does not match", result.stdout)

    def test_identical_overload_signatures_are_reported_as_ambiguous(self) -> None:
        path = "apps/web/lib/old-api.ts"
        source = """
            /** @deprecated Use currentApi instead. */
            export function oldApi(value: string): void;
            /** @deprecated Use currentApi instead. */
            export function oldApi(value: string): void;
        """
        self.write(
            path,
            source,
        )
        declaration = scan(path, source)[0].identity_dict()["declaration"]
        self.write_ledger(
            [
                {
                    "id": "old-api-duplicate-overload",
                    "locator": {
                        "path": path,
                        "declaration": declaration,
                        "marker": "@deprecated",
                    },
                    "reason": "Existing callers still use oldApi.",
                    "owner": "web maintainers",
                    "introduced_on": "2026-01-15",
                    "removal_condition": "All callers use currentApi.",
                    "target_removal_version": "2.0.0",
                }
            ]
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("ambiguous repeated deprecation identity", result.stdout)

    def test_duplicate_go_annotations_on_one_field_are_reported_as_ambiguous(self) -> None:
        self.write(
            "apps/backend/internal/example/payload.go",
            """
            package example
            type Payload struct {
              // Deprecated: use Current.
              Old string // Deprecated: retained for old clients.
            }
            """,
        )
        self.track_all()

        result = self.run_cli("--all")

        self.assertEqual(result.returncode, 1, result.stdout + result.stderr)
        self.assertIn("ambiguous repeated deprecation identity", result.stdout)

    def test_go_multi_name_field_requires_registration_for_each_name(self) -> None:
        path = "apps/backend/internal/example/api.go"
        source = """\
        package example
        type T struct {
          // Deprecated: use C.
          A, B string
        }
        """
        self.write(path, source)

        declarations = [
            finding.identity_dict()["declaration"] for finding in scan(path, source)
        ]

        self.assertEqual(declarations, ["field:T.A#1", "field:T.B#1"])

        self.write_ledger(
            [
                {
                    "id": "field-a",
                    "locator": {
                        "path": path,
                        "declaration": "field:T.A#1",
                        "marker": "Deprecated:",
                    },
                    "reason": "Existing callers still use field A.",
                    "owner": "backend maintainers",
                    "introduced_on": "2026-01-15",
                    "removal_condition": "Callers use field C.",
                    "target_removal_version": "2.0.0",
                }
            ]
        )
        self.track_all()

        missing_b = self.run_cli("--all")

        self.assertEqual(missing_b.returncode, 1, missing_b.stdout + missing_b.stderr)
        self.assertIn("field:T.B#1", missing_b.stdout)
        self.assertIn("unregistered", missing_b.stdout)

        self.write(
            path,
            """\
            package example
            type T struct {
              // Deprecated: use C.
              B string
            }
            """,
        )
        self.track_all()

        removed_a = self.run_cli("--all")

        self.assertEqual(removed_a.returncode, 1, removed_a.stdout + removed_a.stderr)
        self.assertIn("field-a: locator declaration does not match", removed_a.stdout)
        self.assertIn("field:T.B#1", removed_a.stdout)
        self.assertIn("unregistered", removed_a.stdout)

    def test_regex_backticks_do_not_hide_typescript_deprecations(self) -> None:
        cases = {
            "apps/web/lib/regex.ts": (
                """\
                const message = `literal text
                /** @deprecated template text is not a comment */
                end`;
                const ratio = calculate() / scale;
                const matcher = /`/;
                function hasMatch(input: string) {
                  if (input) /`/.test(input);
                  return /`/.test(input);
                }
                /** @deprecated Use newApi instead. */
                export function oldApi(): void;
                """,
                10,
            ),
            "apps/web/lib/regex.tsx": (
                """\
                const label = () => <p>Don't parse this as a string</p>;
                const message = `literal text
                /** @deprecated template text is not a comment */
                end`;
                const ratio = calculate() / scale;
                const matcher = /`/;
                function hasMatch(input: string) {
                  if (input) /`/.test(input);
                  return /`/.test(input);
                }
                /** @deprecated Use newApi instead. */
                export function oldApi(): void;
                """,
                11,
            ),
        }

        for path, (source, marker_line) in cases.items():
            with self.subTest(path=path):
                self.assertEqual(
                    find_declarations(path, source),
                    [(marker_line, "function:oldApi[signature=( )]", "@deprecated")],
                )

    def test_nested_generic_closers_preserve_function_and_method_registrations(self) -> None:
        path = "apps/web/lib/generic-api.ts"
        compact = """\
        /** @deprecated Use currentFunction instead. */
        export function oldFunction<T extends A<B<C>>>(value: G<H<I>>): void;
        export interface Service {
          /** @deprecated Use currentMethod instead. */
          oldMethod<T extends A<B<C>>>(value: G<H<I>>): void;
        }
        """
        spaced = """\
        /** @deprecated Use currentFunction instead. */
        export function oldFunction<T extends A<B<C> > >(value : G<H<I> >) : void;
        export interface Service {
          /** @deprecated Use currentMethod instead. */
          oldMethod<T extends A<B<C> > >(value : G<H<I> >) : void;
        }
        """

        original_findings = scan(path, compact)
        reformatted_findings = scan(path, spaced)
        original_declarations = [
            finding.identity_dict()["declaration"] for finding in original_findings
        ]
        reformatted_declarations = [
            finding.identity_dict()["declaration"] for finding in reformatted_findings
        ]

        self.assertEqual(original_declarations, reformatted_declarations)

        entries = []
        for finding in original_findings:
            declaration = finding.identity_dict()["declaration"]
            entry_id = (
                "old-function"
                if declaration.startswith("function:oldFunction")
                else "old-method"
            )
            entries.append(
                {
                    "id": entry_id,
                    "locator": {
                        "path": path,
                        "declaration": declaration,
                        "marker": "@deprecated",
                    },
                    "reason": "Existing callers still use this declaration.",
                    "owner": "web maintainers",
                    "introduced_on": "2026-01-15",
                    "removal_condition": "All callers use the current declaration.",
                    "target_removal_version": "2.0.0",
                }
            )
        self.write(path, compact)
        self.write_ledger(entries)
        self.track_all()

        self.write(path, spaced)
        self.track_all()
        reformatted_registration = self.run_cli("--all")

        self.assertEqual(
            reformatted_registration.returncode,
            0,
            reformatted_registration.stdout + reformatted_registration.stderr,
        )

        changed_signature = spaced.replace("A<B<C> > >", "A<B<D> > >")
        self.write(path, changed_signature)
        self.track_all()
        stale_registration = self.run_cli("--all")

        self.assertEqual(stale_registration.returncode, 1)
        self.assertIn("old-function: locator declaration does not match", stale_registration.stdout)
        self.assertIn("old-method: locator declaration does not match", stale_registration.stdout)

    def test_generic_closer_normalization_preserves_shift_and_type_changes(self) -> None:
        compact = """\
        /** @deprecated Use currentApi instead. */
        export function oldApi<T extends A<B<C>>>(value: number = input >>> 1): void {}
        """
        spaced_closers = compact.replace("A<B<C>>>", "A<B<C> > >")
        changed_shift = spaced_closers.replace("input >>> 1", "input >> 1")
        changed_type = spaced_closers.replace("A<B<C> > >", "A<B<D> > >")

        compact_identity = scan("apps/web/lib/api.ts", compact)[0].identity_dict()["declaration"]
        spaced_identity = scan("apps/web/lib/api.ts", spaced_closers)[0].identity_dict()["declaration"]
        changed_shift_identity = scan("apps/web/lib/api.ts", changed_shift)[0].identity_dict()["declaration"]
        changed_type_identity = scan("apps/web/lib/api.ts", changed_type)[0].identity_dict()["declaration"]

        self.assertEqual(compact_identity, spaced_identity)
        self.assertNotEqual(compact_identity, changed_shift_identity)
        self.assertNotEqual(compact_identity, changed_type_identity)
