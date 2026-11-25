---
status: draft
system: platform
created: 2026-11-03
requirements:
  - REQ-PLATFORM-CI-PERFORMANCE-009
  - REQ-PLATFORM-CI-PERFORMANCE-010
  - REQ-PLATFORM-CI-PERFORMANCE-011
owners:
  - kandev
---

# Native web verification system design

## Purpose and boundaries

Platform owns shared repository validation and execution efficiency.
This continuation changes the web verification tools within existing local and hosted entry points.
The [CI performance design](ci-performance.md) retains capacity, measurement, and workflow boundaries.
The [CI system](../../ci/README.md) retains contributor trust ownership.

The selected targets are full Oxlint with migrated plugins and `tsrs --fast`.
The hybrid linter and ordinary tsrs mode remain comparison candidates, not substitutes for the selected delivery.
There are no new jobs, runners, runtime flags, public APIs, or application features.
Vite 8 and Rolldown remain unchanged.

JSX entity normalization preserves the same rendered characters on desktop and phone.
This package changes no layout, interaction, localization policy, or responsive composition.
It needs no new UI surface or ASCII UI preview.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| REQ-PLATFORM-CI-PERFORMANCE-009 | Lint host, Compatibility repairs, Verification corpus |
| REQ-PLATFORM-CI-PERFORMANCE-010 | Compiler boundary, Compatibility repairs, Verification corpus |
| REQ-PLATFORM-CI-PERFORMANCE-011 | Entry points, Measurement and adoption, Failure and rollback |

## Components and responsibilities

| Component | Responsibility |
| --- | --- |
| `apps/web/package.json` | Stable lint/typecheck command names and exact native tool pins. |
| `apps/web/oxlint.config.mjs` (new) | Required native rules, JS plugin registrations, scopes, and zero-warning policy. |
| Existing `eslint-rules/*.mjs` | First-party guard implementations and named exports for existing API tests. |
| Existing i18n options and wait exemptions | Shared source for lexical exceptions and permitted guard exclusions. |
| Existing ESLint configurations | Compatibility for RuleTester and API-based ratchets and inventory tools. |
| `.github/scripts/web-toolchain-evidence.py` (new) | Bounded compiler/linter comparison, inventory validation, and measurement reports. |
| `.pre-commit-config.yaml` | Existing changed-file hook, routed through the accepted native lint command. |
| Existing CI jobs | Required package commands, existing reporting, and native gate-contract tests. |

The evidence helper uses the Python standard library and explicit executable arguments.
It does not become a general task runner or a production service.
It joins every started process and preserves each original exit.
No helper holds a runner while it waits for another job.
Linux measurements use child resource data and label peak RSS as a process measurement.
Unavailable CPU or memory evidence remains unknown on other hosts.

## Compiler boundary

The tested candidate is `tsrs@0.20.1-ts7.1.0-beta` with `--noEmit --fast`.
Its source reference is TypeScript `7.1.0-beta`, commit `3e9236361fa9`.
Implementation starts with those exact pins and records any later pin change with new evidence.

The web `typecheck` command retains its existing pretypecheck generation and `rich-output:check` invocation.
Only the final compiler invocation changes.
The existing `tsconfig.json` owns project inclusion, strictness, `skipLibCheck`, libraries, and incremental behavior.
No compiler option narrows the checked project or disables errors.

The workspace retains TypeScript 5 for JavaScript API consumers and unchanged package compilers.
ESLint, typescript-eslint, AST inventory tools, and RuleTester continue to resolve their compatible API.
The standalone plugin SDK, desktop, and other workspace packages retain their current `tsc` commands.

Reference compilers use a separate temporary installation with an exact version and recorded package integrity.
Their `tsc` binaries must not replace the workspace binary through a package alias or symlink collision.
The evidence helper provisions this installation only for an explicit comparison operation.
Its report separates provisioning from compiler execution and includes both costs where adoption requires them.

Fast mode changes checker scheduling, caches, lazy members, and some diagnostics near implementation limits.
The [upstream behavior notes](https://github.com/oxc-project/tsrs/blob/main/docs/USAGE.md#side-effects) define that limitation.
Passing a clean project alone does not establish diagnostic equivalence.
The paired corpus and current project must pass before the required command changes.
Version and configuration changes require a fresh comparison.

## Lint host

The tested candidate is `oxlint@1.91.0` with exact dependency resolution.
The temporary migration used `@oxlint/migrate@1.90.0` as a converter, not a runtime dependency.
The final command loads the checked-in configuration and fails for warnings.

Native Oxlint rules replace supported core and TypeScript rules.
JS plugins preserve SonarJS, unused imports, TanStack Query, i18next, and first-party guards where native equivalents lack proven coverage.
Existing explicit React rule enablement remains authoritative.
The migration must not enable an unrelated recommended preset or omit a configured rule silently.

The rule ledger records each prior rule, severity, options, file scope, exclusions, suppressions, target rule, and executable fixture.
It also records rules that remain disabled and their established reasons.
Scope equivalence includes JavaScript, TypeScript, JSX, TSX, MJS, and CTS/MTS inputs that the current configuration selects.

The Oxlint configuration imports existing i18n options and wait exemption lists.
It does not copy their regular expressions into a second authority.
First-party rule modules can add default exports while retaining existing named exports.
Plugin registration and per-file enablement remain separate.
Scoped JS rules default to off outside their registered owner paths.
Explicit later exclusions replace unsupported conversion of override ignore lists.

The existing four Query rules retain their severities and parser-service exclusion.
The [Query lint contract](../requirements/web-tanstack-query-eslint.md) remains authoritative.
The [System Query owner guard](../../architecture-lint/system-design/migrated-system-query-owner.md) retains its four owner paths and supported syntax.
The Python architecture engine remains independent.

ESLint stays installed for API consumers and existing focused adapters.
Normal full web lint does not run a residual ESLint CLI pass.
Existing adapter tests remain, and new tests exercise the actual Oxlint configuration.
Shared options and the rule ledger prevent different policies between those consumers.

## Compatibility repairs

The [investigation evidence](../../../plans/native-web-verification/evidence.md) binds the initial findings to their source revision.
Current discovery must refresh that inventory before implementation.
Diagnostic counts are observations, not immutable acceptance counts.

### TypeScript migration

The initial 12 findings cover six files.
Observer mocks need the required `scrollMargin` member.
WebSocket test doubles need the closed `readyState` type.
The terminal resize buffer needs a type that reflects its actual `ArrayBuffer` allocation.

The implementation preserves mock behavior, socket state transitions, and every assertion.
It does not add broad `any`, double casts, new suppression comments, or relaxed compiler settings.
The buffer fix narrows the declared allocation type rather than adding a runtime copy or changing frame bytes.

### Oxlint migration

The initial 37 findings comprise 23 JSX-entity reports, 12 optional-chain reports, and two duplicate-method parser errors.
Punctuation entities can become their identical Unicode characters without changing labels or catalogs.
Callback fixtures must assert the required recorded call before invocation.
A missing callback remains an error rather than an optional no-op.

The duplicate declarations are `ApiClient.createSentryInstance` and `SessionPage.prStatusChip`.
The implementation preserves the effective last implementation's request payload, workspace scope, return value, and selector.
It removes only the shadowed declaration after focused tests establish that behavior.
No file exclusion or parser-error suppression replaces those repairs.

## Verification corpus

Compiler comparison uses identical non-dependency input sets and the same configured project.
The initial inventory contained 7,635 non-dependency files.
Version-specific bundled libraries remain explicit differences in the evidence.
Comparison includes complete normalized diagnostic tuples, message text, multiplicity, failure status, and source identity.
Order-only differences remain visible and do not excuse a missing error.

The corpus includes the eight injected basic errors from the investigation.
It also includes valid counterparts and relevant generic, conditional, union, mapped-type, recursive, inference, and instantiation-limit cases.
Negative fixtures must establish that the compiler performs type verification.
An empty corpus, incomplete inventory, crash, or mismatched required diagnostic fails the comparison.

Fast/default/reference comparisons cover cold verification and fresh incremental caches.
Warm rechecks include a changed invalid source file to reject stale success.
Checker counts and effective fast settings appear in the report.
Comparisons at supported one-, four-, and eight-checker settings use the same reference count.
Known reference history differences remain explicit, not assumed tsrs regressions.

Lint fixtures exercise native rules, plugin options, every first-party guard, all four System owner files, and all four Query rules.
They cover valid and invalid JSX text, entities, attributes, test-file exemptions, helper exemptions, and inline suppressions.
They include add/rename cases and selected-file CLI execution with paths containing spaces.
The required lint command itself must reject each forbidden shape.
Standalone RuleTester success cannot replace host-wiring evidence.

## Entry points

The normal commands remain `pnpm run lint`, `lint:i18n`, `lint:e2e-sleeps`, and `typecheck` from `apps/web`.
Focused native configurations reuse the main configuration's shared policy.
The existing i18n ratchets, wait ratchet, wait inventory, and non-JSX copy checks retain their behavior.
Classic API adapters remain explicit until they have an equivalent supported replacement.

The existing web pre-commit hook forwards the same changed filenames to Oxlint.
Package commands and hooks use the same version and rule scopes.
The routing may land before hosted acceptance; the prior commands remain a tested rollback path,
and Task 05 owns the hosted performance claim and final adoption decision.
The targeted ESLint CLI invocation in `upgrade-regression.yml` moves to the accepted native configuration.
Existing workflow contract tests protect required command exits and dependency joins.
Helper tests join the existing action-pinning workflow job without adding a job or permission.

Contributor guidance names the primary native commands and the retained API-only role of ESLint.
The documented `make typecheck` entry point remains unchanged.
Public runtime documentation needs no change unless implementation changes an exposed contributor command.

## Measurement and adoption

The existing investigation is diagnostic evidence, not passing migration acceptance.
Both tools reported unresolved errors in their fastest full-project experiments.
The hybrid linter is excluded from the selected delivery.

Baseline and candidate measurements use the same repaired application source and test assertions.
They differ only in the declared tool configuration and executable.
The helper preserves old command recipes, not old application snapshots.
Its isolated baseline restores the prior script routing on the repaired source.
Both sides retain the same generation and prechecks.
Three alternating successful pairs measure full lint and full package typecheck.
Compiler-only timings supplement, not replace, package-command measurements.
Cold dependency installation, cache restore, plugin startup, generation, reporting, and temporary reference costs remain attributable.

Adoption requires at least 50% lower median full-lint execution and 75% lower median typecheck execution.
The combined occupied frontend job time must decrease without a verification or resource regression.
Reports include CPU, peak RSS and its measurement scope, source hashes, versions, checker settings, cache state, exits, and individual samples.
Failed, cancelled, and incomplete attempts remain in the record.
No percentile or hosted speed claim follows from the local three-run sample.

Hosted acceptance uses three comparable baseline/candidate pairs from natural runs or separately authorized measurements.
The existing CI measurement helper reports setup, execution, runner-minutes, and waiting separately.
The shared 60-job limit is an accepted constraint.
Waiting time is not an optimization target or an acceptance failure for this package.
Earlier fifteen-minute CI and cache-rollout requirements remain separate.

## Failure and rollback

Missing diagnostics, lost file coverage, unresolved rule differences, incompatible APIs, or a missed performance bound prevent the performance adoption claim and removal of the tested rollback.
The conditional command routing may remain in service only with that rollback documented; the implementer records the exact unmet condition.
The implementer does not substitute the hybrid linter, ordinary tsrs mode, or disabled fast features silently.
An incompatible pin needs a corrected upstream release and new evidence, or a new explicit scope decision.

Rollback restores the last accepted dependency pins and command routing together.
Compatible source repairs and stronger fixtures can remain after rollback.
The rollback retains mandatory checks, existing job topology, and failed measurement evidence.
No runtime data, persisted configuration, or database migration is involved.

## Related decisions and delivery

- [Native verification decision](../../../decisions/2026-11-03-native-web-verification.md).
- [Existing architecture guard decision](../../../decisions/2026-10-08-bounded-frontend-eslint-architecture-guard.md).
- [Capacity-aware verification decision](../../../decisions/2026-10-08-capacity-aware-ci-verification.md).
- [Implementation package](../../../plans/native-web-verification/plan.md).
