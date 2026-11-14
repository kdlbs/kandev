---
created: 2026-11-04
status: in_progress
requirements:
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003
  - REQ-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004
system_design:
  - ../../specs/agents/system-design/grouped-model-catalog.md
legacy_specs: []
---

# Implementation Plan: Model catalog selector repair

## Overview

Repair [issue #5053](https://github.com/kdlbs/kandev/issues/5053) and finish the model-selection gaps from [issue #5026](https://github.com/kdlbs/kandev/issues/5026).
First remove redundant ordinary headings. Then qualify the native Fusion join, connect its acquisition, and prove the owning flows on desktop and phone.
Agents owns the package because it owns provider catalog meaning and executable model identity.

This package resumes the unfinished [Task 04](../first-draft-profile-discovery/task-04-fusion-qualification.md) and [Task 07](../first-draft-profile-discovery/task-07-combined-acceptance.md).
Their completed siblings remain historical evidence. Do not repeat the first-profile resolver or permission implementation.
The existing [catalog decision](../../decisions/2026-11-03-provider-neutral-model-catalog.md) already defines the required boundaries.
This provider qualification needs no separate architecture decision.

The package is in progress.
Task 01 is implemented, and Task 02's pure normalizer is implemented and deterministically tested.
Task 02's captured-fixture qualification gate is still open, so Task 03's production acquisition is not enabled and Task 04 has not started.

## Evidence and root cause

Investigation baseline: `69b07e585b6406a458a7a78e87e886f8db11afdc`, also the observed remote `main` head.
The issue has no attached images or comments. Its historical screenshot findings are described in its body.

1. `components/model-catalog-groups.tsx:renderEntries` always creates a heading for an ordinary group.
   It never projects a singleton into one row.
2. `acpcompat.CatalogFromSelectOptions` and `CatalogFromRawOptions` preserve native groups, but `publishCatalog` supplies only nodes.
   Neither the live converter nor the utility probe provides native Devin relationship evidence.
3. The existing generic composition row requires a matching `catalog.compositions` entry.
   Without it, the ordinary pairing rows are expected fallback behavior.
4. `hostcli.ExecRunner.Output` combines streams and retains 64 KiB.
   It cannot safely acquire the reported native JSON or preserve an arbitrary profile/executor launch context.

A temporary component test opened the real `ModelConfigSelector` with a singleton and a two-variant group.
The singleton assertion failed because a group named `Kimi K2.7` still existed above the identically named row.
The multi-variant assertion passed. The temporary file was removed.

The issue reports a native `devin models list --format json` capture from CLI `3000.11.234 (92c83011d113)`.
Its two exact cross-checks matched all 61 current ACP Fusion choices to unique pairs and unique dimension tuples.
Only 395 of 590 native Fusion variants passed the same rule.
These counts describe that capture. They are neither production limits nor universal decoding evidence.

This workspace has no `devin` executable. No native capture, account access, prompt, inference, or live profile mutation occurred here.
The [qualification record](../first-draft-profile-discovery/fusion-qualification.md) names the remaining captured-fixture evidence.
The confirmed causes and exact-match fallback support a concrete repair without speculative model selection.

## Scope

### In scope

- Render ordinary singleton models once, retaining source order, identity, descriptions, and useful path context.
- Preserve meaningful multi-choice groups and composition entries during search.
- Qualify native family/variant records against exact current ACP choices.
- Acquire native data inside each owning live/probe launch context with bounded output and cleanup.
- Reuse the current generic editor, dependent option resolution, Apply, Save, and session confirmations.
- Prove profile, workflow, and live behavior with deterministic desktop/phone fixtures.
- Synchronize the original delivery gaps and public model-selection guidance after implementation.

### Out of scope

- Adding native-only models to availability, a static pairing list, general name splitting, or synthesized executable values.
- New pricing, entitlement, recommendation, or context-size features.
- New credentials, account enrollment, a host-global catalog cache, a new endpoint, or database migration.
- Changing permission modes, conversation identity, settings Save ownership, or live partial-application semantics.
- Reopening unrelated Devin permission scenarios or completed discovery work.

## Technical approach

### Presentation projection

Keep `buildCatalogTree` and the wire catalog lossless.
Change `CatalogCommandGroups` to project singleton subtrees from the full tree before filtering.
Check for compositions first. A verified one-binding composition remains a configuration entry.
Retain source order across loose leaves, collapsed groups, multi-choice groups, and unbound composition members.

Extend `ModelRow` with optional presentation identity and context detail as needed.
Use the occurrence key for cmdk identity and the provider value for callbacks and selected state.
Display removed meaningful paths separately from provider descriptions, with accessible detail and search keywords.
Keep repeated labels and repeated values as distinct occurrences.
Update the current `Standard` singleton expectation in the catalog component suite.

### Qualification and native join

Add the pure decoder in `internal/agentctl/acpcompat/devin_catalog.go`.
Use the exact native ID and label relationships from the [design](../../specs/agents/system-design/grouped-model-catalog.md#composition-qualification).
Lead uses the native family identity. Sidekick uses the exact native variant identity.
Both the source pair and the resulting dimension tuple must be unique.
If any tuple collides, keep every colliding member ordinary.
Intersect with the current ACP group on every enrichment, including after catalog updates.

Task 02 first captures a sanitized real fixture and qualifies the source's family identity and command shape.
Reported counts and synthetic fixtures cannot replace that gate.
Keep unknown members beside the configuration entry. Never reject a usable ACP catalog because optional enrichment failed.

### Acquisition and publication

Use one small `internal/agentctl/devincatalog` source with an injected owner runner and immutable evidence snapshot.
Carry its optional preparation/enrichment interface through internal adapter configuration.
`process.Manager` supplies the admitted live command context. `ACPInferenceExecutor.Probe` supplies the admitted settings context.
Use `prepareSessionArgs`, `prepareACPLaunch`, and the existing process ownership/cleanup paths.

Prepare once per live initialization and at most once per uncached probe.
Every notification and later publication uses pure enrichment without another process.
Reuse existing hostutility result caching, request flights, authorization scope, and context revisions.
Discard live evidence on launch replacement or reconfiguration.
Keep preparation outside adapter/configuration locks and fence late results by generation.

Bound native acquisition to 10 seconds, 2 MiB stdout, and 64 KiB stderr within the parent budget.
Detect overflow explicitly. Never parse a silently truncated successful prefix.
Keep stdout separate from diagnostics, cancel/reap descendants, and log only sanitized outcome codes.
Unqualified launch shapes retain ordinary selection without an unwrapped or host-account retry.

### Compatibility matrix

| Input and context | Result | Evidence and fallback |
| --- | --- | --- |
| Flat ACP provider | Existing exact selection | Existing flat component and converter tests |
| Native ordinary groups | Singleton projection, retained variants and paths | Mixed rendered fixture, including search and repeated occurrences |
| Qualified Devin, same live/probe context | Lead/Sidekick bindings for exact ACP leaves | Captured native fixture plus real subprocess-shaped deterministic tests |
| Partly qualified Devin group | One configuration entry plus ordinary unmatched leaves | Mixed bound/unbound and colliding-tuple regressions |
| Missing source, timeout, malformed JSON, overflow, or unsupported wrapper/flags | Ordinary ACP catalog remains selectable | Runner refusal/cancellation tests and browser fallback |
| A differently named synthetic composition | Same renderer and resolver | Existing pure tests plus desktop/phone neutral presentation fixture |
| Other providers, protocols, and ordinary host CLI suggestions | Existing behavior | No Devin source invocation, existing handshake and flat regressions |

## ASCII UI preview

These sketches define hierarchy and navigation. Names and spacing are illustrative provider data.
Kandev-owned copy remains localized. The existing selector shells and dimensions-first editor remain in use.

### UI-5053-01: Catalog, current and corrected desktop

Entry: profile Start model, workflow override, or task Model settings.

```text
Current                          Corrected
[ Filter models... ]             [ Filter models... ]
Kimi K2.7                        Kimi K2.7                 [check]
  Kimi K2.7 [check]              GLM-5.2
GLM-5.2                            Standard
  Standard                         1M context
  1M context                     Fusion                         >
Fusion                           Unknown pairing
  long pairing A ...               useful path / description
  long pairing B ...
```

The singleton heading disappears. Multi-choice headings remain even when search matches one choice.
Fusion's known pairs use one entry. An unmatched pairing remains selectable.
Secondary paths distinguish repeated labels without repeating a redundant singleton name.
Checks: AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.1, 002.2, 002.5, and 003.3.

### UI-5053-02: Desktop composition and resolution

```text
< Models                         Fusion
Lead        [ GPT-6.1 Sol            > ]
Sidekick    [ GPT-6 Luna High         > ]
Thinking    [ High                   > ]
Speed       [ Standard               > ]
                  [ Use configuration ]

Invalid:   Sidekick [ Choose a valid value > ]
           Use configuration is disabled
Resolving: Model options [loading]
           Old Thinking/Speed controls are hidden
Failure:   Model options unavailable [Retry]
```

Changing Lead retains a valid Sidekick or visibly requires its replacement.
Only one exact advertised value enables Apply.
Returned option availability controls Thinking/Speed. No option value comes from a model suffix.
Checks: AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-003.1 through 003.4 and 004.1 through 004.3.

### UI-5053-03: Phone, one inset sheet

```text
            Model settings        [x]
< Models                         Fusion
---------------------------------------
Lead
[ GPT-6.1 Sol                        > ]
Sidekick
[ GPT-6 Luna High                    > ]
Thinking
[ High                              > ]
Speed
[ Standard                          > ]
---------------------------------------
[          Use configuration          ]
             safe area

Dimension body replaces the same sheet:
< Fusion                     Sidekick
[ Search choices...                   ]
  Valid choice A
  Valid choice B
```

The existing `ModelConfigSheet` is the shipped settings exemplar.
The task `MobilePickerSheet` contributes the same single-sheet navigation pattern.
This temporary, multi-step choice fits a drawer instead of another route or stacked overlays.
One body owns vertical scrolling. Header and Apply remain reachable with safe-area clearance and dynamic viewport containment.
Phone/coarse-pointer controls meet 44px targets. Ordinary desktop controls retain 28px density.
Back and viewport changes retain the draft. Dismissal returns focus and does not apply it.
Checks: AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-004.2 and 004.5.

## Tests

| Criteria | Planned evidence |
| --- | --- |
| 002.1, 002.2, 002.5 | `components/model-config-selector-singletons.test.tsx`: singleton plus real variants, nested paths, described leaves, source order, repeated labels/values, and search |
| 002.4, 003.1, 003.3 | `acpcompat/devin_catalog_test.go:TestDevinCompositionBindings` and `TestDevinCompositionTupleCollisions`: exact source join, current ACP intersection, mixed unsupported entries, and ambiguity |
| 002.3, 003.2, 004.4 | `adapter/transport/acp/adapter_catalog_enrichment_test.go` and `utility/devin_catalog_test.go`: all snapshot boundaries, independent provider options, and preserved state |
| 003.3, 004.2 | `devincatalog/source_test.go`, process and utility runner tests: context parity, bounded streams, cancellation, cleanup, and generation changes |
| 003.4, 004.1 through 004.3 | Existing catalog, profile/workflow Apply, and live Apply component suites, plus the neutral second-provider fixture |

Task 01 owns the singleton behavioral RED test.
Task 02 owns the absent-binding RED test against captured data.
Task 03 owns subprocess and publication integration RED tests.
Task 04 uses those behavioral RED results and adds end-to-end proof against rebuilt assets.

## E2E tests

Task 04 creates the four grouped-catalog files already named by the original Task 07.
Desktop uses `chromium`. Files prefixed `mobile-` use `mobile-chrome`.

| Flow | Files and acceptance |
| --- | --- |
| Existing and first Devin profile: discover, singleton/search, Fusion, Back, Apply, Save/reload | `settings/grouped-profile-workflow-models.spec.ts` and phone counterpart. 002.2, 002.5, 003.1 through 003.3, 004.1, 004.2, 004.5 |
| Workflow override: exact model/options remain draft until Save | Same settings files. 004.1 and 004.4 |
| Live: exact model/options, unsupported controls, rapid changes, partial failure/retry, retained conversation | `chat/grouped-model-selector.spec.ts` and phone counterpart. 002.3, 003.2, 004.1 through 004.4 |
| Unknown pairing/source failure plus synthetic different dimension names | Both file pairs. 003.3, 003.4, 004.2 |
| Phone: touch, one active sheet, Back/focus, internal scroll, safe area, resize draft retention, no horizontal overflow | Both phone files. 004.2 and 004.5 |

The disposable Devin descriptor and native-command shim exercise real backend probes and normalization.
Do not fulfill successful discovery responses in Playwright.
A separately labelled neutral app-store fixture proves second-provider presentation, not native qualification.
Capture persisted model/options and native session identity through API evidence, alongside the rendered assertions.

## Work orders

- [x] [Task 01: Collapse ordinary singleton presentation](task-01-singleton-presentation.md)
- [ ] [Task 02: Qualify and normalize native Fusion bindings](task-02-fusion-bindings.md), after Task 01. Pure normalizer implemented; captured-fixture gate open.
- [ ] [Task 03: Acquire and publish in the owning context](task-03-contextual-acquisition.md), after Task 02. Blocked on Task 02's captured fixture.
- [ ] [Task 04: Prove desktop and phone owning flows](task-04-browser-acceptance.md), after Task 03.

Execute sequentially in the primary session. No delegation or implementation is authorized by this design package.

## Verification results

Investigation on 2026-11-04:

- Temporary singleton component reproduction: one failed singleton assertion and one passed multi-variant assertion. File removed.
- `(cd apps/web && pnpm exec vitest run components/model-config-selector-catalog.test.tsx lib/model-catalog-selection.test.ts)`: 32 tests passed across two files.
- `(cd apps/backend && go test -trimpath ./internal/agentctl/acpcompat -run 'Test(Catalog|ClientCapabilityMeta)' -count=1)`: passed.
- `command -v devin`: no executable. Real-provider fixture capture was not run.
- `apps/node_modules` and `apps/web/node_modules` already exist. No dependency installation was necessary for these diagnostics.

Design-package validation on 2026-11-04:

- `python3 scripts/list-docs.py validate`: passed, with 431 decisions and 1918 specifications.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- Task 04's `validateCoverage` preflight: covered, with all seven affected work orders accepted and no reference errors.
  The preflight forces runtime coverage with two simulated paths. It proves documentation references, not implementation.
- `git diff --check` and the new package's link/whitespace checks: passed.
- Git status contains only the five new package files and seven existing documentation files.

Issue #5053 was assigned to `carlosflorencio`, then confirmed through the GitHub issue API.
No open PR mentioning #5053 was found during the assignment check.
The complete package remains unstaged and uncommitted.
At that planning handoff, all implementation and browser commands were pending in the work orders.

Implementation on 2026-11-04:

- Task 01 changed `components/model-catalog-groups.tsx` and `components/model-config-selector-rows.tsx`:
  ordinary singleton chains project into one row, entries render in one ordered traversal, cmdk identity uses the leaf occurrence key, and removed paths render as accessible secondary detail beside provider descriptions.
  `components/model-config-selector-singletons.test.tsx` is the new permanent regression suite; `components/model-config-selector-catalog.test.tsx` updates the former `Standard` singleton-heading expectation.
- `(cd apps/web && pnpm exec vitest run components/model-config-selector-singletons.test.tsx components/model-config-selector-catalog.test.tsx components/model-config-selector.test.tsx components/model-config-selector-custom-model.test.tsx lib/model-catalog-selection.test.ts)`: 5 files, 65 tests passed.
- The wider selector/ownership component set (`profile-composed-config`, `workflow-composed-config`, `task/model-selector-composed-config`, `profile-model-fields`, `workflow-session-config-editor`, `lib/apply-model-configuration.test.ts`): 11 files, 97 tests passed.
- `(cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings <changed files>)`: clean.
- `(cd apps/web && pnpm run typecheck)`, `pnpm run i18n:check`, and `pnpm run i18n:ratchet`: passed.
- Task 02 added `internal/agentctl/acpcompat/devin_catalog.go`: a bounded native parser, the exact identifier/label join, tuple-collision rejection, and non-mutating validated enrichment.
  `internal/agentctl/acpcompat/devin_catalog_test.go` covers parsing bounds, exact bindings, current-catalog intersection, tuple collisions, ambiguous pairs, unexplained leaves, and original-catalog preservation with synthetic fixtures only.
- `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/acpcompat ./internal/common/modelcatalog -count=1)`: passed.
- `bash scripts/lint-go-changed <changed Go files>`: 0 issues.
- `(cd apps/backend && go build -trimpath ./cmd/agentctl ./cmd/kandev)`: passed.

Review fixes on 2026-11-04:

- Composition command identity is collision-free: it is allocated per render from the validated `group.id` (compositions) or leaf occurrence key (rows), with a kind prefix and counted suffix, instead of the unescaped `buildCatalogTree` path key.
  A slash-ambiguous fixture (root group `a/b` plus nested `b` under `a`) now proves distinct `data-value`, a single keyboard-selected entry, Enter opening the nested configuration, and a click opening the root one.
- The removed-path detail wraps on phone and coarse-pointer layouts so long shared-prefix paths stay distinguishable; desktop keeps the single-line density and the provider description stays on its own line.
  A phone regression covers two repeated `Default` labels under long paths.
- The catalog `Command` root scores only declared keywords, so command-identity prefixes and counted suffixes never participate in fuzzy search; a focused regression types `row` and `composition` and expects no results.
- A top-level loose choice that follows a group keeps the `Model` heading, because the heading attaches to the first top-level row run regardless of preceding groups; a regression covers the trailing-row case.
- `EnrichDevinCatalog` validates injected native evidence before indexing it and leaves repeated advertised values unbound, so a duplicate identifier or duplicated leaf can never produce a first-match binding; both have Go regressions.
- A group whose children are all nested groups force-mounts its cmdk wrapper, so a search that matches a nested choice no longer hides the parent heading and its content; a nested-group regression covers it.
- Re-verified: `(cd apps/web && pnpm exec vitest run <selector and ownership suites>)`: 11 files, 102 tests passed; `(cd apps/backend && go test -trimpath -tags fts5 ./internal/agentctl/acpcompat ./internal/common/modelcatalog -count=1)`: passed; oxlint and Prettier clean.

Remaining blocker on 2026-11-04:

- This workspace still has no `devin` executable, authorized account, or sanitized captured fixture.
- Task 02's qualification gate therefore stays open, and Task 03 was not started because it requires that qualification and its owning launch context.
- Task 04 stays blocked on Task 03. No synthetic row or reported issue count was relabelled as captured evidence.

## Documentation impact

This turn changes internal requirements, design, qualification evidence, and delivery records only.
The active requirement adds explicit singleton behavior as AC-AGENTS-DYNAMIC-PROVIDER-OPTIONS-002.5.
Existing criteria 003 and 004 already define Fusion and owning-flow behavior.
Task 04 updates `docs/public/agents-and-profiles.md` when the implementation exists.
No screenshot or README change is currently required.

## Risks

- The issue's native capture is reported evidence. Task 02 still needs sanitized observed inputs before enabling the production adapter.
- Family-level Lead identity can collide across effort variants. Every colliding tuple must fall back, with no first-match selection.
- Unknown command flags or wrappers cannot justify a different account context. Ordinary selection remains the supported fallback.
- Optional metadata can disappear in a later config snapshot unless every live/probe publication uses the same enrichment boundary.
- Mock browser success proves Kandev behavior, not provider authentication or a universal native schema.
- Keep the original Fusion qualification and browser gaps open until the corresponding new work orders actually pass.
