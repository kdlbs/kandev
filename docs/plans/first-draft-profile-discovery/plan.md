---
created: 2026-11-03
status: in_progress
requirements:
  - REQ-AGENTS-PROFILE-DISCOVERY-001
  - REQ-AGENTS-PROFILE-DISCOVERY-002
  - REQ-AGENTS-PROFILE-DISCOVERY-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-001
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004
  - REQ-AGENTS-DEVIN-PERMISSIONS-001
system_design:
  - ../../specs/agents/system-design/profile-capability-discovery.md
  - ../../specs/agents/system-design/grouped-model-catalog.md
  - ../../specs/agents/system-design/devin-profile-permissions.md
legacy_specs: []
---

# Implementation Plan: Devin discovery and model selection

## Overview

Deliver [issue #5023](https://github.com/kdlbs/kandev/issues/5023) and [issue #5026](https://github.com/kdlbs/kandev/issues/5026) as one reviewed work package.
First allow draft discovery without a persisted parent. Then preserve grouped catalogs and support verified composition across profiles, workflows, and live sessions.
The previously requested Devin permission controls remain a linked part of the overall delivery.
Implementation was explicitly requested after the design review; the results below record what was delivered and what remains open.

Agents owns this repair because it owns profile membership and provider capability identity.
AC-AGENTS-PROFILE-DISCOVERY-001.2 already requires discovery from a new draft without a save.
The #5023 correction needs no new requirement or acceptance ID.
The profile-discovery design makes draft and saved identities explicit.
The grouped-catalog design adds requirements 002 through 004 under the existing dynamic-provider-options owner.

## Evidence and root cause

Investigation used source commit `8cd31a0896abc50e6d9683369cee92252c5d2e44`.
The issue reports a late CLI installation after backend startup.
Startup synchronization skips absent executables, while later availability listing can detect them without creating parent rows.

The current call path is:

1. The profile editor sends a complete draft snapshot without `profile_id`.
2. `FetchProfileDynamicModels` calls `resolveProfileProbeContext`.
3. `resolveProfileProbeAgent` requires `GetAgentByName` to return a persisted row.
4. A missing row returns `ErrAgentNotFound` before the utility probe starts.
5. `writeProfileDiscoveryError` maps that error to HTTP 404.
6. The form requires a model before `saveNewAgent` can persist the parent.

`ResolveAgentModelConfig` shares the same resolver and the same defect.
The draft branch in `resolveProfileDiscoveryProfile` runs too late to avoid the missing parent dependency.

A temporary `TestIssue5023Investigation` reproduced the current behavior with real controller code, the default registry, and disposable SQLite.
The utility boundary returned healthy deterministic responses.
Four subtests covered Devin and Qwen, each with `refresh=false` and `refresh=true`.
For each subtest, both endpoints returned `ErrAgentNotFound` without calling their utility method.
Creating only `models.Agent{Name: agentName}` made both endpoint calls succeed with the same draft.
Baseline responses then contained one model and model-option responses had status `ok`.

The command passed all four diagnostic subtests:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agent/settings/controller -run '^TestIssue5023Investigation$' -count=1 -v)
```

This diagnostic test asserts the defect, not the corrected behavior.
The investigation removed it after use.
No production code, permanent test, live database, or provider credentials changed.
No browser reproduction or real Devin subprocess ran in this investigation.
The issue separately records successful standalone ACP discovery on the affected host.

### Issue #5026: Group loss and unqualified composition

Live `convertACPConfigOptions` and utility `probeConfigOptions` both flatten native ACP groups.
The utility path uses `selectOptionsUngrouped`, and downstream types retain only flat choices.
Two temporary `TestIssue5026Investigation` tests reproduced this against the actual converter functions.
Both tests retained exact choice values and descriptions while dropping both native group IDs and labels.

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/utility -run '^TestIssue5026Investigation$' -count=1 -v)
```

Both diagnostic tests passed and were removed. They asserted current loss, not corrected behavior.
No model inference or live database access occurred.

The issue reports Devin's grouped-options handshake extension and model-dependent Thinking/Speed settings.
The installed SDK supports native groups containing choices, without arbitrary nesting or composition bindings.
Official Fusion documentation describes Lead/Sidekick controls but does not establish their wire mapping.
Task 04 must qualify that binding source before the Devin composition adapter is implemented.
Unknown mappings retain ordinary exact-model selection, and #5026 remains incomplete until verified Fusion selection works.

## Scope

### In scope

- Accept complete authorized drafts for registered agents without persisted parents.
- Use the same distinction for baseline discovery and dependent model options.
- Keep draft identity stable before and after parent creation.
- Preserve saved-profile membership, disabled-agent checks, secret validation, and cache separation.
- Prove first-profile creation through real HTTP discovery and Save on desktop and phone.
- Preserve native groups through live/probe normalization, DTOs, caches, events, existing snapshots, and boot hydration.
- Add provider-neutral hierarchical search and exact combination selection, including verified Devin Fusion and a synthetic second provider.
- Integrate profile drafts, workflow overrides, live session confirmation, restoration, and partial-application behavior.
- Deliver localized desktop/phone interactions and the linked permission controls.

### Out of scope

- Persisting agent rows during discovery or changing startup synchronization.
- Bypassing model validation or selecting a model automatically.
- Changing authentication, executable installation, runtime activation, or provider billing.
- Unverified model-ID synthesis, inferred effort/speed values, arbitrary metadata execution, or another selection store.
- Remote executor discovery, speculative pricing/recommendations, and general diagnostic-message improvements.

## Technical approach

### Shared settings resolver

Change `apps/backend/internal/agent/settings/controller/profile_discovery.go`.
Resolve the registered agent and inference support independently of persisted profile membership.
Branch on the presence of `profile_id` before a parent lookup.
For a new draft, use the canonical registry ID and require the complete snapshot.
For a saved profile, retain the parent row ID and existing profile ownership validation.
Treat repository failures as errors rather than permission to use the draft path.

Both `FetchProfileDynamicModels` and `ResolveAgentModelConfig` already call this resolver.
Keep that shared boundary.
Retain `requireProfileDiscoveryScope`, strict JSON decoding, optional-agent policy, secret scope, and launch validation.
Do not add an installation check before the existing utility boundary.
An unavailable executable must retain its ordinary probe result.

### Cache identity

Update `profileProbeScope` so every new draft uses `<authorization-scope>:draft:<registry-id>`.
Existing drafts with parents currently use a database UUID in that position.
This process-local identity change requires no migration or API field change.
Saved scopes remain `<authorization-scope>:<profile-id>`.
Keep HMAC identity, effective command, resolved secrets, refresh generations, TTL, and admission in `hostutility`.
Do not create a second cache or change opaque revision semantics.

### Compatibility matrix

| Target | Transport and identity | Result and evidence | Unsupported or invalid input |
| --- | --- | --- | --- |
| Devin and Qwen drafts without parents | ACP, canonical registry ID | Both endpoints reach a healthy utility in controller regressions | Preserve normal utility statuses |
| Draft with a parent | Existing supported probe, same registry ID | Same scope before and after parent creation | Complete snapshot still required |
| Saved concrete profile | Existing supported probe, persisted parent and profile IDs | Existing saved-launch tests and new refusal cases | Missing, foreign, dynamic, workspace, or role profile fails before execution |
| Unknown, disabled optional, or non-inference agent | No valid discovery target | Controller and HTTP refusal tables | Preserve current error classification |
| Other registered inference providers | Existing host utility implementation | Shared resolver only, existing capability tests | No new protocol support or fallback |
| Browser acceptance fixture | Mock ACP under an additional registered Amp identity | Real probe, resolve, save, and reload on desktop and phone | Mock responses do not establish real Amp or Devin authentication support |

### Browser proof without shared seed damage

Use `backend.restart({ KANDEV_MOCK_PROVIDERS: "amp-acp" })` in the owned worker.
Acquire `testPage` before setup so its initialization cannot overwrite the preconditions.
Retain the normal `mock-agent` seed and its workflow, task, Office, and utility references.
Remove only the disposable Amp row through `apiClient.rawRequest("DELETE", "/api/v1/agents/<id>")`.
Built-in deletion retains the registry entry, and the mock availability path continues to expose it.
Assert that Amp is available but absent from `listAgents` before opening the editor.

Open `/settings/agents/amp-acp?mode=create` through the existing profile entry flow.
Observe actual requests to the Amp probe and resolve endpoints.
Do not fulfill those requests in Playwright.
Refresh, choose an advertised model, save, and reload.
Require no parent row through successful discovery and dependent resolution.
After Save, assert the persisted parent and profile model through the API.
Always remove the created Amp data and call `backend.restart()` in cleanup.
If the worker already contains Amp data, preserve that baseline or use a fresh owned worker.

### Grouped catalogs and exact compositions

Follow [the grouped-catalog design](../../specs/agents/system-design/grouped-model-catalog.md) and its shared-type decision.
Add optional bounded catalog data to normalized config options and derive the legacy flat lookup from it.
Use neutral Go and TypeScript types. Do not duplicate provider interpretation in React.
Negotiate Devin's grouped extension through the existing shared `acpcompat.ClientCapabilityMeta` helper.
Preserve existing Cursor and other provider handshakes.

Use native group structure directly. Additional subgroups and composition bindings require supplied or verified relationships.
Task 04 records the source and qualified mapping before it enables Lead/Sidekick controls.
Dimensions resolve to one exact advertised model. Thinking and Speed remain returned configuration options.
The shared component and resolver must also work for a synthetic provider with different dimension names.

Live mutation stays session-authoritative. Settings Apply changes only the current draft, and existing Save persists it.
During live Apply, model and option calls retain separate confirmations.
A later failure displays confirmed state and a retryable partial error without resetting the conversation.
Extend the existing session selector snapshot and hydration, without another persistence record or selection store.

| Catalog input | Normalized behavior | Acceptance evidence |
| --- | --- | --- |
| Flat provider | Existing exact values and selection | Flat regression fixtures |
| Native ACP groups | Ordered group IDs, labels, descriptions, and choices | Live/probe/boot round trips |
| Verified Devin Fusion bindings | Lead/Sidekick resolve to one advertised entry | Qualified real fixtures plus generic UI tests |
| Unknown or ambiguous Fusion shape | Ordinary catalog remains usable | Negative fixtures. Does not complete Fusion acceptance |
| Synthetic second provider | Same renderer with different groups and dimension names | Resolver and desktop/phone tests |

## ASCII UI preview

UI-GROUP-01: Existing model trigger opens the shared desktop catalog.
Native groups and verified subgroups appear only when the catalog supplies them.

```text
Models
[ Search models...                  ]
Fusion                              >
Model family                        >
Ordinary model
```

UI-GROUP-02: Verified Fusion configuration in the same desktop picker.

```text
< Models                      Fusion
Lead        [ Lead model           v ]
Thinking    [ Provider value       v ]
Sidekick    [ Sidekick model       v ]
Speed       [ Provider value       v ]
              [ Use configuration ]
```

UI-GROUP-03: Phone uses one inset bottom sheet.
Dimension selection replaces the body with a choice list and Back action.

```text
          Model settings
< Models                      Fusion
Lead
[ Lead model                      > ]
Thinking
[ Provider value                  > ]
Sidekick
[ Sidekick model                  > ]
Speed
[ Provider value                  > ]
[         Use configuration         ]
```

UI-GROUP-04: Resolution, invalid pairing, and partial application.

```text
Resolving:       [ Loading configuration... ]
                 Stale dependent controls are hidden.
Invalid pairing: Choose an available Sidekick.
                 [ Use configuration (disabled) ]
Live partial:    Model changed. An option was rejected.
                 [ Retry from confirmed state ]
```

The phone sheet has a fixed header, one internal vertical scroller, safe-area clearance, and a reachable Apply action.
Controls have at least 44px touch targets. Desktop retains normal compact sizing.
The nearest exemplar is `MobilePickerSheet`, reused through an appropriate shared shell outside task-owned code.
The sketches show required hierarchy and states. Labels are illustrative localized copy, not provider constants.
Views 01/02 cover dynamic-provider criteria 002.1/002.2 and 003.1/003.2.
Views 03/04 cover criteria 004.1 through 004.5, including focus, cancellation, viewport changes, and failure recovery.

## Tests

| Acceptance criteria | Required evidence |
| --- | --- |
| 001.2, 001.3, 001.5 | New `controller/profile_discovery_first_draft_test.go`: `TestFirstDraftProfileDiscoveryWithoutAgentRow`, for Devin and Qwen, both endpoints and both refresh values, with no persisted rows |
| 001.1, 001.4 | `TestFirstDraftProfileDiscoveryValidation` and `TestSavedProfileDiscoveryRequiresParentOwnership`: authorization, registry policy, snapshots, secrets, and saved membership |
| 001.6, 002.1, 002.2 | `TestFirstDraftProfileDiscoveryScope`: stable draft scope across parent creation, separate agents/users/saved profiles, and forwarded refresh |
| 001.2, 001.4, 002.4 | New `handlers/profile_discovery_first_draft_test.go`: `TestFirstDraftProfileDiscoveryEndpoints`, with valid 200 responses and preserved 400/401/403/404 refusals |
| 002.1, 002.2, 002.3, 002.5 | Existing `hostutility` profile identity, cache, generation, and activation tests |

Existing controller tests provide saved launch settings, explicit clears, secret failures, and environment precedence evidence.
Extend the recording utility to capture model-option requests where necessary.
Use separate test files instead of enlarging an already large test file.

For #5026, Task 03 covers grouping/flat contract round trips and shared handshakes.
Task 04 covers verified bindings and unknown/ambiguous/new entries without model synthesis.
Task 05 covers generic navigation, search, combination filtering, and a differently named synthetic provider.
Task 06 covers stale requests, per-surface Apply, partial live application, snapshot hydration, restore, and historical attribution.
Task 07 supplies complete desktop/phone acceptance and public docs.

## E2E tests

Add `apps/web/e2e/tests/settings/first-profile-capability-discovery.spec.ts` for project `chromium`.
Add `mobile-first-profile-capability-discovery.spec.ts` for project `mobile-chrome`.
Share fixture setup through `apps/web/e2e/helpers/first-profile-capability-discovery.ts`.

Both tests cover AC-AGENTS-PROFILE-DISCOVERY-001.2, 001.3, 001.5 and 003.2, 003.3, 003.4.
Require the missing-parent precondition, successful real discovery, model selection, Save, API persistence, and reload.
Assert that probe and resolve requests omit `profile_id` and contain the complete launch snapshot.
Require a ready model selector and successful option reconciliation before Save.
Phone interactions use touch controls and the existing settings scroll container.
Assert reachable actions, at least 44px refresh and save targets, and no horizontal page overflow.

The existing phone profile-discovery spec is the mobile exemplar.
The repair reuses its selector, inline status, refresh action, page navigation, and scroll owner.
Tasks 01/02 change no rendered composition. Tasks 05/06/07 use the shared picker previews above.
Keep all existing user-facing copy localized.

The final matrix adds grouped/flat catalogs, ordinary/composed selections, and profile/workflow/live owners on desktop and phone.
Include verified Fusion, a synthetic second provider, unsupported options, loading/retry, cancellation, resizing, stale results, and partial application.
Retain the combined first-Devin-profile flow: no parent, grouped discovery, exact pairing, real options, Save, and reload.
An ordinary model fallback is compatibility evidence, not a substitute for verified Fusion acceptance.

## Work orders

- [x] [Task 01: Remove the draft parent dependency](task-01-draft-discovery-context.md)
- [x] [Task 02: Prove first-profile creation](task-02-first-profile-browser-proof.md), after Task 01
- [x] [Task 03: Preserve the structured catalog](task-03-catalog-contract.md)
- [ ] [Task 04: Qualify Devin composition bindings](task-04-fusion-qualification.md) (gate unresolved)
- [x] [Permission defaults](../devin-profile-permissions/task-01-permission-intent.md) and [controls](../devin-profile-permissions/task-02-permission-controls.md), after Tasks 01/02
- [x] [Task 05: Build the shared hierarchical picker](task-05-shared-catalog-picker.md)
- [x] [Task 06: Integrate configuration ownership](task-06-selector-integration.md)
- [ ] [Task 07: Prove both issues end to end](task-07-combined-acceptance.md) (partial)

Execute remaining work sequentially in the primary session.
Tasks 01, 02, 03, 05, and 06 plus the permission defaults/controls are implemented. Remaining gates: Task 04's Fusion qualification and Task 07's grouped/Fusion browser acceptance. Do not rerun completed tasks.
Task 04 blocks real Fusion composition until its evidence is complete.
Other independent completed outcomes remain valid while that gate is unresolved.
The completed profile-discovery, first-run setup, and generic-wrapper packages remain historical delivery records.
This package adds the missing-parent case without rewriting their completed scopes or results.

## Verification results

Investigation: four diagnostic subtests passed and confirmed the existing defect.
The later #5026 investigation passed two converter diagnostics and confirmed group loss.
Permanent implementation and browser checks remain pending.
Record actual implementation commands and results in each work order after execution.

Design-package validation on 2026-11-03:

- `python3 scripts/list-docs.py validate`: passed, 428 decisions and 1893 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `.github/scripts/pr-docs.cjs` `validateCoverage`: the actual documentation diff was exempt with no errors.
  A simulated runtime diff passed as covered, with both work orders accepted against their requirement and design references.
  This preflight validates package links, not a production change.
- `git diff --check -- docs/specs/agents docs/plans/first-draft-profile-discovery`: passed.
- New work-order links and whitespace: passed. All three new package files are present and untracked.
- At the original repair handoff, Git status contained only this package, its requirement backlink, and its system-design amendment.
  The later permission-settings design adds the companion files linked below.

The full package remains unstaged and uncommitted for user review.

Expanded package validation after adding #5026:

- `python3 scripts/list-docs.py validate`: passed, 429 decisions and 1896 specifications.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- The PR documentation preflight accepted all nine linked work orders against a simulated backend/frontend implementation diff.
- `git diff --check -- docs/specs docs/plans docs/decisions`: passed.
- Both group-loss diagnostics passed and their temporary test files were removed.
- Permanent implementation and browser verification remain pending, including the Task 04 Fusion-binding gate.

## Implementation results (2026-11-03)

Tasks 01, 02, 03, 05, and 06 are implemented with permanent tests. The permission companion's backend defaults/validation and settings control are implemented with unit/component tests.
Task 02's desktop and phone specs pass against the real backend. Task 04 remains an open qualification gate: no machine-readable Fusion binding source exists in the official documentation, and this environment has no Devin CLI or credentials, so no speculative decoder was added.
Task 07 is partial: first-profile specs exist, grouped-catalog browser coverage and the Fusion criterion remain open.

Verification actually run:

- `go build ./...`: passed.
- `go test -trimpath ./internal/common/modelcatalog ./internal/agentctl/acpcompat ./internal/agentctl/server/adapter/transport/acp ./internal/agentctl/server/utility -count=1`: all packages passed.
- `go test -trimpath -tags fts5 ./internal/agent/settings/... ./internal/agent/hostutility -count=1`: all packages passed.
- `go test -trimpath -tags fts5 ./internal/agent/runtime/lifecycle -count=1`: passed (159s).
- `NODE_OPTIONS=--max-old-space-size=12288 pnpm exec tsc --noEmit -p tsconfig.json`: passed. The default `pnpm run typecheck` OOMs in this 28 GB container at the generated rich-output catalog step, so the compiler was run directly.
- `pnpm exec vitest run` for the catalog, selector, hydration, apply, profile/workflow, and Devin settings suites: 187 tests passed across 18 files.
- `pnpm run i18n:check`: passed (pseudo locale regenerated, seven locales complete).
- `node scripts/validate-public-docs.mjs`: 54 pages validated.
- Desktop E2E: `pnpm e2e:run --host --shards 1 --project chromium tests/settings/first-profile-capability-discovery.spec.ts` passed (1 passed, 4.8s).
- Phone E2E: `pnpm e2e:run --host --shards 1 --project mobile-chrome tests/settings/mobile-first-profile-capability-discovery.spec.ts` passed (1 passed, 6.4s).
- Devin permission E2E: desktop `pnpm e2e:run --host --shards 1 --project chromium tests/settings/devin-profile-permissions.spec.ts` passed (2 passed, 10.1s); phone `--project mobile-chrome tests/settings/mobile-devin-profile-permissions.spec.ts` passed (1 passed, 8.0s).
- `bash scripts/lint-go-changed` on every changed Go file: `0 issues`.

Review remediation (2026-11-03):

- Saved-profile discovery validates the edited permission intent, not the saved mode. The launch snapshot carries the draft mode and the resolve request's mode overrides it; both Smart->Autonomous and Autonomous->Smart transitions reach the utility, while a genuinely conflicting pair still fails. Controller and HTTP regressions cover both endpoints.
- The completed configuration now reaches every owning surface. Profiles and workflow rules apply the exact model plus the candidate model's real provider options to their drafts. Live sessions resolve candidate options without mutation, then confirm the model followed by each option, fenced by the per-session request id and confirmed execution id; a later option failure keeps confirmed state, shows a partial-application toast, and retries without re-applying the model. Superseded requests confirm nothing.
- Catalog leaves keep their full group path in command-filter keywords, so a provider description cannot hide them from a group-name search.
- Navigating Back to the model list keeps each group's unfinished draft; re-entering the group restores it. The regression also asserts no owning model mutation.
- Back, dimension, choice, Apply, and provider-option controls use 28px fine-pointer desktop density and at least 44px below the 768px phone boundary and on coarse pointers.

Added verification: `pnpm exec vitest run` for the selector, catalog, apply, profile/workflow/live owning-surface, hydration, and Devin suites passed (209 tests across 22 files). `go test -trimpath -tags fts5 ./internal/agent/settings/... ./internal/agent/hostutility ./internal/common/modelcatalog ./internal/agentctl/acpcompat -count=1` passed. `bash scripts/lint-go-changed` on every changed Go file reported `0 issues`. Desktop first-profile E2E was re-run after the picker changes.

PR fixup remediation (2026-11-03):

- Live candidate options resolve through a session-scoped backend route that uses the session profile's saved launch context and never mutates the session; dynamic profiles fall back to the agent-wide probe.
- The picker binds candidate option state to the resolved model, resets drafts when the catalog or owning model changes, keeps custom-model and withdrawn-model rows in the catalog view, renders composition leaves that have no binding, and shows option descriptions with a sticky phone Apply action.
- Explicit "Use CLI settings" on a new Devin profile is carried as a request-only intent so creation does not re-insert the default disabled override; inherit also preserves an explicit Autonomous mode.
- Catalog validation now bounds composition counts and labels, rejects orphaned/duplicate bindings and prototype-like dimension keys, and enforces identifier lengths in both Go and TypeScript. Cached resolutions deep-copy the catalog.
- Applied live confirmations read the live session entry per mutation, require the captured execution id when present, report superseded late failures, and re-notify a second partial failure.
- Review-driven regressions include session-reset isolation, retry-only-pending options, catalog bounds, unbound choices, ambiguous pairings, missing-profile discovery, env-bound defaults, and header/phone assertions.

Known gaps:

- Fusion composition (Task 04, part of Task 07) remains unimplemented. Issue #5053 now identifies native JSON source evidence, but sanitized observed fixtures and contextual acquisition are still pending. Issue #5026's Fusion acceptance remains unmet.
- The Devin permission browser specs are added but their managed runs are recorded separately; the isolated capability scenarios (Smart present/absent, sandbox failure) still need a mock peer knob.
- An isolated Devin settings mock descriptor is added (`KANDEV_E2E_MOCK_DEVIN`), but the Smart-present/Smart-absent/sandbox-failure scenarios still need a deterministic mock-peer knob.

## Selector repair follow-up (2026-11-04)

[Issue #5053](https://github.com/kdlbs/kandev/issues/5053) confirms duplicate singleton headings and identifies a native Fusion relationship source.
The [selector repair package](../model-catalog-selector-repair/plan.md) refines the remaining delivery.
Its [Task 01](../model-catalog-selector-repair/task-01-singleton-presentation.md) owns the rendered singleton correction beyond the original Task 05 coverage.
Its [Task 02](../model-catalog-selector-repair/task-02-fusion-bindings.md) and [Task 03](../model-catalog-selector-repair/task-03-contextual-acquisition.md) complete the original Task 04's qualification and integration.
Its [Task 04](../model-catalog-selector-repair/task-04-browser-acceptance.md) owns the remaining grouped/Fusion Task 07 browser matrix.
All four were pending at the planning handoff; the [selector repair Task 01](../model-catalog-selector-repair/task-01-singleton-presentation.md) is now implemented and its component suite passes.
Task 02's pure normalizer is implemented, but its captured-fixture gate remains open, so Tasks 03 and 04 are still blocked.
Existing completed tasks and historical verification counts remain unchanged.
Do not close the original Fusion gates from this planning update or synthetic-only evidence.

## Documentation impact

The existing active requirement remains authoritative.
Its only edit links this delivery package.
The system design adds the draft identity distinction.
Public documentation already describes discovery from an unsaved draft in `docs/public/agents-and-profiles.md`.
This repair changes no public fields, labels, instructions, or screenshots.
The original #5023 correction requires no public documentation change by itself.
Task 07 updates public model-selection guidance for #5026. The permission companion owns its distinct Devin permission documentation.

## Related follow-up

The user also requested predefined Devin permission settings with Smart as the new-profile default.
That behavior is planned in the companion [Devin profile permissions package](../devin-profile-permissions/plan.md).
Complete Tasks 01/02 before that companion's integrated browser acceptance.
The companion package does not change this issue's root cause or the repair's production scope.

## Risks

- A broad missing-row fallback can admit invalid saved profiles. Only an omitted `profile_id` selects the new-draft path.
- Using an empty or random draft identity can merge contexts or defeat cache reuse. Use the canonical registry ID.
- The browser fixture must exercise the actual backend. A fulfilled probe response cannot detect this defect.
- Backend restart restores environment defaults but retains SQLite data. Cleanup must remove owned Amp data explicitly.
- Mock browser evidence proves Kandev ordering, not commercial provider authentication. Real-provider behavior retains its current status handling.
- Native grouping is verified, but the Fusion Lead/Sidekick binding source remains a qualification gate.
- Optional catalog data must survive every projection, especially live restore and boot hydration, without replacing existing state authority.
- Multiple ACP mutations can partially succeed. Apply must report confirmed state and retain the conversation.
