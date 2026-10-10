---
id: "01-preserve-current-target"
title: "Preserve current target profiles during creation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-TARGET-CREATION-CATALOGUE-001
acceptance_criteria:
  - AC-AGENTS-TARGET-CREATION-CATALOGUE-001.1
  - AC-AGENTS-TARGET-CREATION-CATALOGUE-001.2
  - AC-AGENTS-TARGET-CREATION-CATALOGUE-001.3
  - AC-AGENTS-TARGET-CREATION-CATALOGUE-001.4
  - AC-AGENTS-TARGET-CREATION-CATALOGUE-001.5
  - AC-AGENTS-TARGET-CREATION-CATALOGUE-001.6
system_design:
  - ../../specs/agents/system-design/target-profile-creation-catalogue.md
---

# Task 01: Preserve current target profiles during creation

## Summary

Publish only newly accepted additional-profile identities over the owner's current
membership, including partial MCP results. Independently prove real page/coordinator/
adapter/normalizer/WS/store/picker behavior before fixing the creation callback.

## In scope

- Exact existing-owner create-mode publication in `page.tsx` and the accepted-new
  subset supplied by `saveExistingAgent`/`saveExistingProfiles`/partial reconciliation.
- Independent T1-T3 causal RED and controls, then T4-T6 from the manifest.
- Current sibling metadata, accepted normalized identity, partial MCP draft/remap,
  navigation/error and same-ID/missing-owner boundary compatibility.
- Accurate lifecycle/status/results in the four package artifacts after execution.

## Out of scope

Ordinary saves/sibling105 PR4390, other creation writers, dynamic writers, deletes,
duplicates, global handlers/store policy, backend/API/server ordering, eligibility
or lifecycle frameworks, protected source access, historical plan edits, UI changes.
No production/permanent tests/install/statics/hooks/commit before ROOT's later release.

## Acceptance

1. T1 independently fails causally after ACK in nested membership and real picker;
   T2/T3 pass before production edits. Test isolation is external transport only.
2. Creation publication preserves current target siblings and metadata, adds only
   accepted new identities once, and retains normalized data and partial MCP/draft/
   error/navigation behavior. All .1-.6 criteria and T4-T6 have scoped evidence.
3. All exact checks pass with actual terminal joins and owned cleanup, documentation
   coverage includes tracked/untracked changes, and package results match execution.

## Implementation sequence

1. After explicit release, re-read paired contracts, actual moving base and PR4390;
   obtain ROOT local-heavy lease. Install once only if workspace dependencies absent.
2. Author T1/T2/T3 independently in the new suite from the existing shipped creation
   test pattern. Use real authenticated administrator state, existing configured
   claude-code owner, discovery/available-agent metadata, loaded secrets, both slices,
   real providers/saveAll and history. Hold the exact profile-create POST; exercise
   real owner PATCH adapter if changed submission fields require it. Deliver the
   registered same-owner created event and select its actual Combobox choice before
   ACK. Observe nested catalogue, projected choice, selected label and selectable
   option after ACK. Report one causal RED and two controls; fix fixture-only failures
   before treating RED as evidence. Settle all deferred work and cleanup.
3. Carry actual accepted submitted-profile results/correlation into an explicit
   creation-only publication context for full/partial existing-owner creation and
   remapped partial retries. Merge only these entries against current target;
   never republish captured old sibling profiles. Preserve
   current owner metadata except submitted owned patch fields on successful saves.
4. Cover T4/T5 and changed publication guards T6. Reuse `compareTimestamps` only for
   an existing copy of an accepted ID; retain pending MCP draft without regressing
   newer persisted data. Missing-owner publication does not reinsert captured owner.
   Do not invent transport, global conflict policy or draft semantics.
5. Run minimum GREEN/scoped checks and update actual results. ROOT's later
   implementation release grants one exclusive local lease through affected checks,
   normal active hooks, ordinary commit, push and PR creation. Return it only after
   actual local joins and current known owned cleanup, then continue hosted handling
   directly in the same turn with the original observer. No extra publication handoff.

## Verification

Run heavy commands serially under ROOT's lease. Use `/bin/bash`, login false,
installed mise Node24/pnpm, `NODE_OPTIONS=--max-old-space-size=4096`, Vitest one
worker and a bounded owned process group. Record argv/cwd/PID/start/group/bounds,
original native handle, actual terminal join/exit and fresh known group absence
before the next heavy command. Do not kill foreign processes or alter shared caches.

From repository root, set the tool path for later released execution:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:$PATH"
export NODE_OPTIONS=--max-old-space-size=4096
```

Conditional one-time workspace prerequisite, after release and lease only:

```bash
(cd apps && pnpm install --frozen-lockfile)
```

RED after T1-T3, before production edits:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 'app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx')
```

GREEN after implementation and T4-T6:

```bash
(cd apps/web && pnpm exec vitest run --project browser-locales --maxWorkers=1 'app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx' 'app/settings/agents/[agentId]/agent-create-catalogue.test.tsx' 'app/settings/agents/[agentId]/agent-save-helpers.test.ts' 'app/settings/agents/[agentId]/agent-save-helpers-provider.test.ts' 'hooks/domains/settings/use-agent-creation-store-sync.test.tsx')
(cd apps/web && pnpm exec prettier --check 'app/settings/agents/[agentId]/page.tsx' 'app/settings/agents/[agentId]/agent-save-helpers.ts' 'app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx' 'hooks/domains/settings/use-agent-creation-store-sync.ts' 'hooks/domains/settings/use-agent-creation-store-sync.test.tsx')
(cd apps/web && pnpm exec eslint --max-warnings 0 'app/settings/agents/[agentId]/page.tsx' 'app/settings/agents/[agentId]/agent-save-helpers.ts' 'app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx' 'hooks/domains/settings/use-agent-creation-store-sync.ts' 'hooks/domains/settings/use-agent-creation-store-sync.test.tsx')
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm run i18n:check)
```

Add exact paths for any changed helper/test file to its owning test/lint/format
commands; place any newly extracted hook in `hooks/domains/settings`. Existing
file-local functions may remain. Do not rerun unaffected full passing suites after
fixture-only changes. A hook extraction requires its affected suite and lint path.

Documentation checks from repository root under the limited docs lease:

```bash
python3 scripts/list-docs.py specs --system agents --format paths
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short --untracked-files=all
```

Use Node24 `.github/scripts/pr-docs.cjs` exported `validateCoverage` with actual
tracked staged/unstaged and untracked paths and actual file contents. Verify one
order; requirement/AC definitions, owning design declarations, plan membership and
relative links. Check untracked-file whitespace explicitly because `git diff --check`
does not inspect it. Record that actual coverage receipt with the validators.

## Files likely touched

- `apps/web/app/settings/agents/[agentId]/page.tsx`
- `apps/web/app/settings/agents/[agentId]/agent-save-helpers.ts`
- `apps/web/app/settings/agents/[agentId]/agent-create-target-catalogue.test.tsx` (new independent suite)
- `apps/web/hooks/domains/settings/use-agent-creation-store-sync.ts` (extracted store hook).
- `apps/web/hooks/domains/settings/use-agent-creation-store-sync.test.tsx` (publication guards).
- Existing helper tests if callback/partial contracts need affected assertions.
- The four package artifacts for lifecycle/status/results.

Read-only integration dependencies: contributor, API actions/normalizer, WS handlers,
settings types/setters, providers/coordinator, picker/Combobox, existing creation
suite. Changes to these boundaries require a ROOT scope checkpoint.

## Dependencies and parallelism

None. Sequential in this existing primary; no delegates, new tasks or sessions.

## Mobile and documentation

Pure state/data publication within unchanged UI satisfies mobile-parity's narrow
exception with real component proof. No new mobile E2E, ASCII layout or whole-app
build. Internal contract/package only; no public procedure or terminology changes.

## Delivery after later release

Use active ordinary hooks with no bypass, ordinary push and repository PR template
through a body file. Return the local lease after actual joins/known cleanup and
continue hosted in the same turn. Retain ONE
`timeout --signal=TERM --kill-after=10s 91m scripts/pr-await <PR> --deadline-min 90 --interval-sec 60 --format json`
observer and original handle/clock across pushes. Read automatic feedback while CI runs;
batch actual findings before one corrective push. Do not relaunch solely for a push
or require a manual full review per addition/SHA. A bounded material later diff may
need focused re-review. Optional style/metrics/docstring polish never gates delivery.

READY requires the known six checks plus actual Backend/Frontend/E2E parent success
for current head, a fresh complete/error-free state, no actionable visible/hidden/
human findings or changes requested, a clean head and adequate substantive review.
ROOT grants static compatibility and a separate serial expected-head normal squash
merge. Verify actual merge and only current owned cleanup; ROOT owns independent
fast-forward/archive/protected-source cleanup. No merge authority is implicit here.

## Results

ROOT reviewed all four final design artifacts and explicitly released implementation.
The bounded production change and independent permanent regression tests are complete.
Requirement status is active, design current, manifest implemented, and this single
implementation order done. Hosted delivery remains governed by the separate gates above.

Independent T1/T2/T3 joined `6bbcde`: one causal RED and two controls pass before
production edits. The first GREEN joined `c69920` with 56 passing cases and a real
T5 partial-retry duplicate-draft failure. Replacing the remapped create-mode baseline
entry corrected that failure; `631fd4` passed 47 affected page/helper cases. Six
existing creation compatibility cases passed in the initial run. Final page and
hook runs joined `598124`/`28b38b`, passing five page cases and five guards, including
owned patch fields, newer copies, pending MCP and absent owner.

Scoped lint/format, final typecheck, i18n and actual tracked/untracked documentation
checks pass as detailed in the [manifest](plan.md#verification-results). Preflight
covers all nine actual paths, not hypothetical source: one order, six criteria,
thirteen valid links and clean whitespace. One conditional frozen-lockfile install
ran. All local check handles joined and known owned process groups are absent.
A short format check overlapped a retained typecheck; both joined, the test format
was corrected, and subsequent heavy checks were serial. Normal active hooks and
ordinary publication follow under the same lease, with live task-plan receipts;
no bypass or extra ROOT publication handoff is authorized or needed.


### Review correction results

The same order also covers the reproduced review composition: newer accepted-ID
name/model during partial MCP failure, then retry, with and without a genuine
in-flight draft edit. RED `b93827` failed those two cases while all five original
page controls passed. Malformed timestamp RED `77bfd9` failed four inputs with five
hook controls passing. Precision RED `f4aa0c` failed one submillisecond case with
nine controls passing. No setup, unhandled or cleanup failure supplied these REDs.

The callback now returns the actual published target for partial creation draft
reconciliation; only fields changed since submission override that current copy.
The guard compares strict parsed epoch-nanoseconds, omitting invalid recency.
Ordinary draft merging remains unchanged. GREEN `e732fb` passed 64 cases across
five suites; final `3fc3f4` passed 17 affected page/hook cases after the precision
correction. Final lint `32cefa`, formatting `7abc07` and typecheck `df4879` pass.
The [manifest](plan.md#bounded-review-correction) records the scoped results;
normal hooks, the one batched corrective push and review-thread dispositions are
recorded in the live task plan without another planning or publication handoff.
