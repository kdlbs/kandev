---
created: 2026-10-28
status: implemented
requirements:
  - REQ-AGENTS-NATIVE-SUGGEST-001
  - REQ-AGENTS-NATIVE-SUGGEST-002
  - REQ-AGENTS-NATIVE-SUGGEST-003
  - REQ-UI-PROMPT-SUGGEST-001
  - REQ-UI-PROMPT-SUGGEST-002
  - REQ-UI-PROMPT-SUGGEST-003
  - REQ-UI-PROMPT-SUGGEST-004
system_design:
  - ../../specs/agents/system-design/native-prompt-suggestion-capability.md
  - ../../specs/ui/system-design/composer-prompt-suggestions.md
legacy_specs: []
---

# Implementation Plan: Native Prompt Suggestion Capability

## Overview

Normalize native support at the adapter boundary. Keep provider support, the
launch request, and effective enablement separate. Preserve the existing
composer and the verified Claude bridge.

This package was delivered after its design handoff. Implementation and focused
validation are recorded below and in the three work orders.

## Baseline and evidence

- Investigated current main: `3f3aabc53f23cc8b08f0bf2b1b766b88fbeaf7f2`.
- Original feature merge: `7351a48f44f996ea2c7dd91185ed889040323626`.
- Managed Claude bridge pin: `0.81.2`, source `5dbb453c63a89746627799b2b06b31ba01a1b674`.
- ACP Go SDK replacement already includes extension notifications at `48a57e4292e8`.
- Primary evidence and the current flow are in the
  [Agents design](../../specs/agents/system-design/native-prompt-suggestion-capability.md).
- [Original plan](../composer-prompt-suggestions/plan.md) and its Tasks 04/05
  remain historical delivery records. Their results are not follow-up validation.

Assumption check: product scope and behavior are explicit in the user request.
The absence of upstream advertisement is a verified fact. The chosen
compatibility rule and opt-in extension are design decisions in the
[ADR](../../decisions/2026-10-28-native-prompt-suggestion-capability.md).

## Scope

### In scope

- Feature-specific Claude evidence and a narrow compatibility exception.
- A generic support-only capability alongside existing effective enablement.
- Provider-specific metadata and notification translation inside the ACP adapter.
- New/load/resume/reset/recreate configuration and capability-loss coverage.
- Existing origin, literal-text, recovery, desktop, and phone regressions.
- Companion specification links and accurate user/developer documentation.

### Out of scope

- A new provider, provider registry, speculative framework, fallback model call,
  utility process, runtime feature flag, preference, or composer redesign.
- Upgrading the pinned bridge or ACP Go SDK.
- Upstream issue/PR publication, new persistent tasks, or implementation subagents.
- Broad verification suites, historical E2E reruns, and unrelated cleanup.

## Technical approach

### 1. Normalize support and preserve the effective contract

Replace `claudeCodeAgent` and `agentAdvertisesClaudeCode` with feature support.
Use small pure Claude integration functions in the planned
`dialect_claude_prompt_suggestions.go`. Keep existing queueing behavior independent.

Recognize the newly documented vendor declaration only as a new opt-in contract.
Neither inspected upstream release implements it. Do not claim otherwise in
tests, public docs, comments, or the PR.

When the declaration is absent, preserve the exact verified official package
identity/version and observed metadata shape. No name/model check escapes
the integration. A Kandev custom definition can have any name. A bridge that
also renames its wire identity needs the explicit declaration.

Add optional `native_prompt_suggestions_supported` to existing stream/runtime/
browser payloads. Preserve `supports_prompt_suggestions` as effective enablement
and `prompt_suggestion_source` as its persisted projection.
The [producer/consumer inventory](../../specs/agents/system-design/native-prompt-suggestion-capability.md#producers-and-consumers)
defines every field hop.

### 2. Keep configuration and recovery bounded

Supported/requested sessions receive the existing enable option and raw filter.
Supported/unrequested sessions receive an explicit disable option and no raw
filter. Unsupported sessions receive no suggestion metadata.

Use the same builder on new, load, and resume. Cover resume-to-load fallback
without changing its identity or error policy. Recreated connections recompute
support and launch preference. Retained adoption preserves its existing protocol
rules and never reinitializes only to discover this capability.

Keep suggestion windows and immutable generation-to-turn propagation. Reject
old session origins across replacement boundaries. Preserve existing state and
resume-attempt guards. Clear effective source on capability or request loss.

### Compatibility matrix

| Provider/integration | Transport and evidence | Intended behavior | Evidence/check | Insufficient shape |
| --- | --- | --- | --- | --- |
| Pinned official bridge | ACP; exact official wire identity, `0.81.2`, observed queueing shape | Native support preserved; preference gates enablement | Pinned source, prior native probe, exact tuple tests | Unknown version/name, empty or malformed metadata: unsupported |
| Custom bridge, arbitrary configured and reported names | ACP; literal new feature declaration | Supported after it adopts the documented options/raw-event contract | Synthetic wire integration test; provider adoption remains unproven | Namespace, queueing, or model alone: unsupported |
| Explicitly unsupported bridge | ACP; feature field false or malformed | No support, no legacy override | Negative table tests, including verified legacy identity | Fail closed |
| OpenCode or Codex with a Claude model | ACP or native transport; no implemented suggestion integration | Ordinary composer | Adapter negatives and existing unsupported E2E | Model/name never qualifies |
| Unadvertised official bridge `0.99.0` | ACP; source differs from the pin | No newly assumed support or automatic upgrade | Source investigation only | Requires an explicit declaration or separately verified compatibility change |
| Other transport | Generic capability/event boundary only | Remains unsupported until integrated | Optional-field compatibility tests | No speculative implementation |

The official compatibility exception is deliberately narrow. Unadvertised,
renamed wire forks and other versions can lose eligibility from the old heuristic.
That is a documented compatibility restriction, not a claim that they cannot
implement suggestions. Current pinned Claude support remains available.

## Tests

Existing tests are extended rather than copied. New test names below are planned.

| Criteria | Focused evidence |
| --- | --- |
| `AC-AGENTS-NATIVE-SUGGEST-001.1`, `.3`, `.4`, `.5` | New `TestNativePromptSuggestionSupport` in `acp/prompt_suggestion_test.go`: explicit true/false, missing/empty/malformed shapes, unrelated vendor, queue-only, arbitrary names, Claude models |
| `AC-AGENTS-NATIVE-SUGGEST-001.2`, `.6` | Same test's exact legacy tuple cases; source links and ADR distinguish verified versus proposed wire evidence |
| `AC-AGENTS-NATIVE-SUGGEST-002.1`, `.2`, `.3`, `.5` | New `TestPromptSuggestionCapabilitiesJSON` in `streams`; `TestPromptSuggestionCapabilitiesForwarded` in lifecycle/orchestrator; all request/support combinations |
| `AC-AGENTS-NATIVE-SUGGEST-002.4`, `.6` | Existing `TestPromptSuggestionsForLaunch`; add missing excluded-path assertions only where coverage is absent; existing immediate-preference hook test |
| `AC-AGENTS-NATIVE-SUGGEST-003.1` | New `TestPromptSuggestionSessionConfiguration` beside adapter session/resume tests; new `TestPromptSuggestionCapabilityRecreation` beside lifecycle launch/recreate tests |
| `AC-AGENTS-NATIVE-SUGGEST-003.2` | Existing `TestHandleAgentCapabilitiesEvent_RecordsNativePromptSuggestions` extended for raw/effective distinctions and loss; `agent-capabilities-prompt-suggestions.test.ts` loss/reload selection |
| `AC-AGENTS-NATIVE-SUGGEST-003.3` | Existing `prompt_suggestion_origin_test.go`, including delayed A-after-B, active prompt, unknown generation, duplicate window, and handed-off overlap; new failed-transition and same-ID restore regressions |
| `AC-AGENTS-NATIVE-SUGGEST-003.4` | Existing `prompt_suggestion_turn_test.go` and orchestrator rejection tests; assert both no write and no publish for unknown/foreign/stale turns and lost state guard |
| `AC-AGENTS-NATIVE-SUGGEST-003.5` | Generic event/type/selection tests; source inspection keeps provider logic confined to adapters |
| UI preference, stale, recovery, literal-text, and normal-composer criteria | Existing helper, hook, WS, TipTap, and hint tests named in Task 03 |

Use TDD for changed logic. RED must assert the old namespace-only behavior is
wrong. A missing symbol or selector alone does not prove a behavioral regression.

## E2E tests

Reuse these exact files and their existing projects:

- `e2e/tests/chat/prompt-suggestions.spec.ts`, `chromium`:
  ghost text, accept/send/dismiss, reload, no suggestion while off, stale turn,
  Quick Chat dismissal, and normal unsupported composer.
  Map to UI `001.2`, `002.1-.4`, `003.1/.3`, and `004.1-.3/.5-.6`.
- Change its unsupported fixture to retain queueing but explicitly deny suggestion
  support. This proves the corrected gate through the real adapter.
- Add a request observer to an existing typing flow. Assert typing produces no
  extra prompt or utility request during a bounded negative observation window,
  while retaining the whole-observation no-utility assertion for the automatic
  post-turn path. Reuse `dwell` and existing WebSocket observers, without a
  duplicate suite.
- `e2e/tests/settings/prompt-suggestion-settings.spec.ts`, `chromium`:
  existing preference persistence/help, UI `001.1/.3/.4`.
- `e2e/tests/chat/mobile-prompt-suggestions.spec.ts`, `mobile-chrome`:
  existing Quick Chat Use reply touch acceptance and actual 44px bounds,
  UI `004.4/.8`.

No layout or touch behavior changes. No new UI preview is required.
Existing UI previews remain in the original package; the configured send
shortcut in current tests is authoritative.

## Work orders

- [x] [Task 01: Normalize feature evidence and capability projection](task-01-normalize-capability.md), wave 1, no dependency.
- [x] [Task 02: Preserve session configuration and origin continuity](task-02-session-continuity.md), wave 2, depends on 01.
- [x] [Task 03: Verify composer compatibility and document limits](task-03-composer-compatibility.md), wave 3, depends on 02.

All work orders are `sequential`. Their exact commands start from repository root.
Install workspace dependencies once before the first pnpm command:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

Task 01 owns focused support/projection Go checks. Task 02 owns race-enabled
session/recreation/origin checks and changed-Go lint. Task 03 owns the exact
Vitest, typecheck, managed desktop/phone E2E, and documentation checks.
Do not run the full hosted or local E2E suite for this follow-up.

## Documentation validation commands

Run from repository root during design and after delivery-document updates:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check -- docs/specs docs/decisions docs/plans
git status --short -- docs/specs docs/decisions docs/plans
```

Run the repository `.github/scripts/pr-docs.cjs` `validateCoverage` preflight with
actual changed paths and linked files, including untracked work orders.
The design handoff records its result. That structural check does not replace
the manual AC-to-test mapping above.


The following preflight uses current changed paths, including untracked files.
Its second result checks the planned runtime package without claiming a code change.

```bash
node <<'NODE'
const fs = require('node:fs');
const cp = require('node:child_process');
const validator = require('./.github/scripts/pr-docs.cjs');
const packageDir = 'docs/plans/native-prompt-suggestion-capability';
const documents = [
  'docs/specs/agents/requirements/native-prompt-suggestion-capability.md',
  'docs/specs/agents/system-design/native-prompt-suggestion-capability.md',
  'docs/specs/ui/requirements/composer-prompt-suggestions.md',
  'docs/specs/ui/system-design/composer-prompt-suggestions.md',
  ...fs.readdirSync(packageDir).filter(name => name.endsWith('.md'))
    .map(name => packageDir + '/' + name),
];
const fileContents = Object.fromEntries(
  documents.map(path => [path, fs.readFileSync(path, 'utf8')]),
);
const paths = [...new Set([
  ...cp.execFileSync('git', ['diff', '--name-only', '-z', 'HEAD'],
    {encoding: 'utf8'}).split('\0'),
  ...cp.execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'],
    {encoding: 'utf8'}).split('\0'),
].filter(Boolean))];
for (const [mode, changedFiles] of [
  ['actual paths', paths],
  ['planned reference coverage', [
    ...paths,
    'apps/backend/internal/agentctl/server/adapter/transport/acp/adapter.go',
  ]],
]) {
  const result = validator.validateCoverage({changedFiles, fileContents});
  console.log(JSON.stringify({
    mode, status: result.status, ok: result.ok,
    workOrders: result.workOrders, errors: result.errors,
  }));
  if (!result.ok) process.exitCode = 1;
}
NODE
```

## Verification results

Design validation on 2026-10-28:

- `python3 scripts/list-docs.py validate`: passed, 414 decisions and 1,800 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- The exact Node preflight above: actual document-only paths are exempt.
  Planned runtime reference coverage is covered, with all three work orders
  accepted and no reference errors.
- `bash -n` over all six documented shell blocks: passed.
  The documented Node preflight also ran successfully from its extracted block.
- Relative Markdown links in all ten changed documents resolve.
- `git diff --check -- docs/specs docs/decisions docs/plans`: passed.
- At design handoff, `git status --short` showed only the ten intended
  documents as changed or untracked; they were unstaged and uncommitted at that
  checkpoint.

Implementation validation on 2026-10-28:

- Backend focused tests passed for ACP, streams, lifecycle, orchestrator, and
  mock-agent. The race-enabled suggestion block passed for ACP, lifecycle, and
  orchestrator. Failed new/load/reset and same-ID restore regressions preserve
  suggestion-origin ambiguity.
- Full ACP, streams, and mock-agent package tests passed.
- `go build ./...` passed; changed-Go lint reported 0 issues.
- Focused Vitest passed (7 files, 64 tests); web typecheck passed.
- Chromium E2E passed for desktop suggestion flows and settings; the
  unsupported-agent case retains whole-observation utility coverage. Mobile
  Chrome E2E passed (phone touch acceptance).
- Public-doc validation passed (63 tests, 54 pages); spec catalog and all-spec
  lint passed.
- The Node documentation preflight reported `covered` for both actual changed
  paths and planned reference coverage, with no reference errors.
- `git diff --check` passed.

Historical results from the original package are not copied here.

## Risks

- The explicit extension needs provider adoption. Mock success does not prove it.
- Exact legacy compatibility excludes other versions and renamed wire forks.
  Expanding it requires evidence, without guessing a version range.
- Provider silence remains valid and cannot prove lack of support.
- A raw support field mistakenly used as effective enablement would bypass opt-in.
  Payload and selection tests must pin their distinct meanings.
- Source/effective capability loss must propagate even when false is omitted.
- Session changes must not weaken existing origin, replay, or recovery guards.

## Documentation impact and completion

This phase updated internal requirements, design, the ADR, and delivery records.
Task 03 updated the public explanation in `docs/public/developer-tools.md` with
the actual shipped evidence limits. It also corrected scoped agentctl guidance.

All work orders passed. This plan and the work-order results are current. The
new Agents requirement is `active` and its design is `current`. The linked UI
contract was reconciled without changing historical test results. No commit,
push, or upstream publication was performed.

