---
created: 2026-11-02
status: done
requirements:
  - REQ-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001
system_design:
  - ../../specs/platform/system-design/session-config-cross-agent-reconcile.md
legacy_specs: []
---

# Implementation plan: Model selector availability with stale config

## Overview

Restore the composer's advertised model controls when historical runtime keys
coexist with a missing or false settlement marker. One sequential work order
covers the gate, input-path regressions, rendered behavior, and browser evidence.

Source: [issue #5005](https://github.com/kdlbs/kandev/issues/5005).
Assignee: `carlosflorencio`, assigned and verified on 2026-11-02.
Inspected workspace commit: `c060076d6723a611ebc260371aa80b555442e589`.

The [active requirement](../../specs/platform/requirements/session-config-cross-agent-reconcile.md)
already defines the outcome. This is a conformance repair for
`AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001.4` and `.7`.
Criterion `.5` protects subsequent model changes. Criteria `.1`, `.2`, `.3`,
`.6`, and `.8` retain backend replay filtering as a compatibility boundary.

## Confirmed root cause and reproduction

`hasCompleteDynamicConfig` takes the union of profile and persisted runtime keys.
It ignores unadvertised non-agent keys only when `configOptionsSettled === true`.
Both session hydration helpers map an absent persisted marker to false.
Thus historical keys can hold `configHydrated` false indefinitely.
`shouldHideModelSelector` then hides the entire control, regardless of available models.

The existing test named "still requires persisted-only unadvertised keys while
the catalog is unsettled" explicitly preserves this failure.

The smallest synthetic fixture has:

- A running session with current model `composer-2.5` and one advertised model choice.
- Advertised config options `mode`, `model`, and `fast`.
- Persisted-only `context`, `effort`, and `reasoning_effort` keys.
- No profile config options and no settlement marker in `acp_model_state`.

A temporary Vitest reproduction passed this fixture through both real hydration
helpers, then asserted selector availability. Both assertions failed with
`expected false to be true`. The existing 31 selector tests passed.
Each reproduction first proved that its marker was false. Each also proved
that a true marker in an in-memory copy released the gate.
The temporary file was removed after the run. No production state changed.

The incident's original missing-marker transition remains unknown. Source tracing
found intentional paths that wait for authoritative startup responses and live
payloads that omit settlement. Neither fact identifies the incident's exact history.
The package repairs the proven availability defect without fabricating settlement.

## Scope

### In scope

- Separate runtime-only historical keys from profile-required keys in the gate.
- Release historical non-agent keys when a usable advertised catalog exists.
- Cover boot, client-fetch, and live update inputs with absent or false settlement.
- Prove model selection and advertised option access on desktop and phone.
- Preserve startup hydration, legacy agent semantics, and mutation error behavior.

### Out of scope

- Backend settlement publication, response correlation, or persistence changes.
- Claiming or reconstructing the original incident's missing-marker transition.
- Marking unknown catalogs settled or weakening provider-confirmed configuration.
- Deleting saved keys, changing permission modes, or adding recovery controls.
- New provider-specific behavior, runtime flags, copy, or visual redesign.

## Technical approach

### Selector gate

Change `apps/web/components/task/model-selector.tsx`, primarily
`hasCompleteDynamicConfig`. Reuse `profileRequiredConfigKeys` and
`persistedRuntimeConfigKeys` to identify runtime-only keys.
Keep `requiredConfigKeys` useful for diagnostics.

Implement the additional rule from the
[system design](../../specs/platform/system-design/session-config-cross-agent-reconcile.md#selector-availability).
A usable catalog requires nonempty raw config options and at least one flat
model choice or usable model-shaped select option. Use the existing select
guards for category aliases. Do not use a static discovery catalog as evidence.

An unadvertised runtime-only key then stops blocking the selector. Exclude
`agent` and every key required by either profile source. Preserve existing
settled-catalog, flat-model, and legacy-agent branches.

Do not change `shouldHideModelSelector` to bypass all hydration checks.
Do not change hydration, WebSocket handlers, backend readiness, or setter behavior.

### Compatibility matrix

| Provider shape or transport | Intended behavior | Evidence | Unresolved fallback |
| --- | --- | --- | --- |
| Cursor ACP shape: flat models plus model/fast config | Historical runtime keys do not hide controls | Exact sanitized unit fixture and rendered test | No usable catalog retains the existing gate |
| Config-option-only ACP model, including category alias | A usable model select releases runtime-only historical keys | Parameterized gate tests | Empty choice list retains the existing gate |
| Flat-model-list ACP without config options | Existing model exception and other readiness rules remain | Existing flat-list and empty-catalog tests | No inferred settlement |
| Strict boot, REST fetch, or WebSocket entry | Same availability for the same accepted catalog | Both hydration helpers and real WS handler | Startup projection remains authoritative |
| Provider-restored entry | Same availability without replacing provider-restored values | Component test with provider-restored policy | Existing unknown-model handling remains |
| Profile-required or provider-defined `agent` key | Unsettled missing profile/agent keys still block | Mixed-source and legacy-agent regressions | Existing settled behavior remains |

These are contract shapes, not live-provider certifications. Browser tests use
the existing mock agent. No external Cursor service is required.

## ASCII UI preview

### UI-01: Desktop composer, populated catalog without settlement

Entry point: existing task conversation. The source-confirmed current gate hides
the model control. The proposed state restores the existing picker.

```text
Before
+--------------------------------------------------------+
| Message input                                          |
| [Mode v]                                      [Send]   |
+--------------------------------------------------------+

After
+--------------------------------------------------------+
| Message input                                          |
| [Mode v]                       [Composer 2.5 v] [Send]   |
+--------------------------------------------------------+
Open model picker: advertised models and Fast controls
```

### UI-02: Phone composer, same accepted catalog

```text
Before                       After
+-------------------------+  +------------------------------------+
| Message input           |  | Message input                      |
| [Mode v]         [Send] |  | [Mode v] [Composer 2.5 v]   [Send] |
+-------------------------+  +------------------------------------+
Tap model trigger: existing touch picker
```

The exact toolbar width and labels are illustrative. Required structure is
a visible model trigger in the existing composer actions, with access to all
advertised controls. Keep the current phone picker, action order, and grouping.
Preserve the current wrapping behavior and available action space.

The composer remains outside transcript scrolling. Existing picker content owns
its scrolling, focus handling, and safe-area treatment. Phone triggers retain
44px targets. Desktop retains its current density.
No new layout, navigation, or breakpoint behavior is required.
Both views map to AC-001.4, AC-001.5, and AC-001.7.

Nearest phone exemplar:
`apps/web/e2e/tests/chat/mobile-model-selector.spec.ts`.
Rendered checks open the picker and perform a model change, rather than only
asserting that a label exists.

## Tests

Add tests before production changes and observe the expected gate failure.
All criterion suffixes refer to
`AC-PLATFORM-SESSION-CONFIG-CROSS-AGENT-RECONCILE-001`.

| Criteria | Test file and named scenario |
| --- | --- |
| .4, .7 | `components/task/model-selector-stale-config-gate.test.ts`: "hydrates persisted-only stale keys from an advertised catalog without settlement" |
| .4, .7 | Same file: missing/false/true marker, runtime and override sources, config-only model, and category alias variants |
| .4, .7 | Same file: mixed stale runtime keys and unresolved snapshot/current-profile keys, absent/empty catalogs, flat-only catalog, and provider-defined agent key |
| .4, .5, .7 | New `components/task/model-selector-stale-config.test.tsx`: "keeps advertised controls usable after boot, fetch, and live model updates without settlement" |
| .5 | Existing `components/task/model-selector-consecutive-switch.test.tsx` and startup/confirmed-config handler suites |

The new component suite uses both real hydration functions and
`registerSessionModelsHandlers` with a store-shaped fixture. Reuse the nearest
selector component test's React/store mocks. Keep lower-level modules free of
component imports. Gate-only cases live in their own file so
`model-selector.test.ts` stays inside the web `max-lines` budget; the
one-contract-violation case remains in that file, now asserting availability.

Assert that the accepted entry retains absent/false settlement and its saved
historical keys. Opening the picker causes no setter call. Choosing an
advertised model sends only that choice. A later model update can remove a
previously advertised runtime-only key without hiding the control.

Retain existing tests for profile keys, legacy agent keys, flat lists, and settled
catalogs. Replace the one test that expects the proven contract violation.
Add a mixed-state regression: ignored historical keys plus one unresolved profile
key must still fail the universal gate.

## E2E tests

Extend the existing files rather than replacing their scenarios:

- Desktop: `e2e/tests/chat/model-selector-error.spec.ts`, project `chromium`.
- Phone: `e2e/tests/chat/mobile-model-selector.spec.ts`, project `mobile-chrome`.

Add a "stale runtime config without settlement" case to each file.
After initial hydration settles, use the E2E store bridge to add runtime-only
keys absent from the mock catalog. Clear the entry's
settlement marker while retaining the real model catalog and advertised options.
Use keys such as `context` and `reasoning_effort`, not mock-advertised `effort`.

Assert the seeded state before interaction, then open the actual picker.
Choose Mock Smart through the real mock backend and assert the persisted model.
Open the advertised effort option after the choice. Record outgoing setter
requests and assert that no historical key is sent. Phone uses `.tap()` and
asserts the existing trigger's 44px target and viewport containment.

These cases prove rendered interaction from the accepted store state. They do
not claim a live Cursor resume or database restart reproduction. Unit/component
tests prove the real snapshot and live-handler input paths.
Both browser cases cover .4, .5, and .7.

## Work orders

- [x] [Task 01: Restore advertised model controls](task-01-restore-advertised-controls.md)

Executed sequentially in the primary conversation.

## Verification results

Implementation completed on 2026-11-02.

- Gate: `hasCompleteDynamicConfig` now ignores an unadvertised key only when the
  key comes from runtime config, no profile source requires it, it is not
  `agent`, and the session entry has a usable advertised catalog (at least one
  raw config option plus a flat model or usable model-shaped select). Existing
  settled, flat-model, legacy-agent, and profile-key behavior is unchanged.
- TDD: the new gate assertions failed before the change (`expected false to be
  true`), and the component suite rendered no trigger. After the change,
  `model-selector.test.ts` 31 passed, `model-selector-stale-config-gate.test.ts`
  11 passed, `model-selector-stale-config.test.tsx` 7 passed (including the
  provider-restored entry), and `model-selector-consecutive-switch.test.tsx`
  2 passed.
- Browser: desktop `model-selector-error.spec.ts` 8 passed, including the new
  stale-runtime case; phone `mobile-model-selector.spec.ts` 3 passed, including
  the 44px trigger and viewport-containment stale-runtime case.
- Combined focused Vitest run: 9 files, 73 passed.
- `pnpm run typecheck`: passed with `NODE_OPTIONS=--max-old-space-size=8192`;
  the default heap exhausted memory in this workspace.
- `pnpm exec eslint` on the changed files: 0 errors, 0 warnings.
- `pnpm run i18n:check` and `pnpm run i18n:ratchet`: passed.
- `python3 scripts/list-docs.py validate`: passed, 425 decisions and 1888 specifications.
- `python3 scripts/lint-spec-files.py --all`: passed.
- Documentation coverage preflight: actual and prospective changes both covered.
- `git diff --check`: passed.

No unresolved difference from UI-01 or UI-02. The browser cases seed store state
after initial hydration and do not claim a live Cursor resume or database
restart reproduction.

## Documentation impact

The existing Platform requirement remains active and unchanged in meaning.
Its new links connect this package and the new design. The prior reconciliation
and resume-label packages retain their historical results and completion state.
This package supersedes only the prior gate expectation for runtime-only keys
in usable catalogs without settlement.

Public docs need no change. This correction restores an existing control without
changing terminology, commands, or APIs. No new ADR is required because the
active requirement already owns the behavior.

## Risks

- Incomplete catalogs can arrive during startup. Preserve all existing startup
  projections and profile-key barriers, and run their focused tests.
- Runtime and profile keys can overlap. Source membership must use both profile
  sources, not only the session snapshot.
- A model list alone is insufficient for the new non-model-key exception.
  Empty and flat-only controls must retain their existing outcomes.
- The incident's missing-marker origin remains unknown. This repair does not
  certify settlement publication across every provider or retained runtime.
