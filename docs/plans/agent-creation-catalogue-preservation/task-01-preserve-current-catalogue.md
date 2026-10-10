---
id: "01-preserve-current-catalogue"
title: "Preserve current catalogue during agent creation"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CREATION-CATALOGUE-001
acceptance_criteria:
  - AC-AGENTS-CREATION-CATALOGUE-001.1
  - AC-AGENTS-CREATION-CATALOGUE-001.2
  - AC-AGENTS-CREATION-CATALOGUE-001.3
  - AC-AGENTS-CREATION-CATALOGUE-001.4
  - AC-AGENTS-CREATION-CATALOGUE-001.5
  - AC-AGENTS-CREATION-CATALOGUE-001.6
system_design:
  - ../../specs/agents/system-design/creation-catalogue.md
---

# Task 01: Preserve current catalogue during agent creation

## Summary

Independently prove and fix normal creation overwriting a later different-owner
profile in the current catalogue and actual picker. Bind publication to the
mounted provider's store while keeping accepted target assembly, partial MCP
results, draft remapping, shared save and navigation behavior.

## Entry gate and dependencies

Released by ROOT at 2026-10-09 03:07 UTC after its full four-artifact review;
receipt `/tmp/kandev-root-child92-design-review-release-20261009.json` binds this
task, original primary and exclusive GLOBAL LOCAL-HEAVY lease. Merge authority
remains NONE. Read the owning
requirement/design and [manifest delivery gates](plan.md#authority-and-delivery-checkpoints)
first; they are part of this work order. No agents, new tasks, tabs or sessions.
There are no prior work-order dependencies. Work sequentially in this original
primary. Use `/tdd`; no proof replay or speculative coverage audit.

## In scope

- `useAgentStoreSync` in the creation page: replace its captured catalogue
  selector with `useAppStoreApi` and read the catalogue inside `upsertAgent`.
- One independently authored `.test.tsx` suite with T1 through T6 from the
  [manifest test matrix](plan.md#tests), using real product boundaries.
- Targeted compatibility checks and accurate spec/plan/work-order results.

## Out of scope

Same-owner concurrency arbitration, orphan retention, other editors/writers,
backend/schema/API/WS/store changes, generic caches and timestamps. ROOT's
protected test must never be read/copied/imported/replayed/modified or removed.
Do not add production test seams or copy-scan exceptions. Test fixture strings
belong in recognized test files or test-owned helper parameters.

## Acceptance

1. T1 independently fails on the current page for both received catalogue and
   actual selected picker choice after real accepted creation. T2/T3 preserve
   meaningful success/rejection controls, with T3 verifying a newer draft.
2. The bounded creation callback reads the current owning store at publication;
   T1 through T6 pass for existing/new-owner success and MCP partial-result
   callbacks. Helpers, target assembly and deliberate branch navigation retain
   their existing behavior; no same-owner concurrency claim is added.
3. Exact affected checks pass, original handles are terminal/joined and owned
   groups gone, results are recorded before ROOT local-heavy RETURN. Only after
   later grants may delivery continue under the manifest's frozen-head/observer/
   review/merge gates.

## Sequential implementation

1. Re-read this package and current relevant source, without replaying ROOT's
   proof. Confirm explicit release/lease. Mark only this work order in_progress.
2. If actual dependencies are still absent, perform the single conditional
   pinned frozen install below. A failure/resource/timeout/transport/unknown
   issue is a ROOT checkpoint, not permission for a retry or alternative.
3. Author T1/T2/T3 through real Page/providers/saveAll/API/WS/picker. Stub only
   fetchJson and external editor capability. Create wire fixtures independently.
   Put a consumer probe inside the real provider for store/coordinator; control
   actual browser history and call actual registerAgentsHandlers.
4. Run the RED command. Require the expected T1 causal failure reaching both
   store/picker assertions and meaningful control observations; fixture/runtime
   errors are not RED evidence. Retain and actually join the original handle.
5. Complete T4/T5/T6 for the other callback outcomes. Add the minimal page
   correction, then run the GREEN suite plus existing focused helper coverage.
   Do not introduce target profile/revision merging. Do not change projection
   to hide current absent-owner behavior.
6. Run the exact lint/typecheck/ratchet checks. After a real failure, fix only
   affected scope and rerun the necessary failing checks. No broad passing
   replays, cleanup hacks, runtime changes or foreign-process kills.
7. Record exact results/handles/group receipts; mark this task done and the
   manifest implemented only when all defined acceptance is proved. Promote
   paired requirement draft to active and design draft to current if conformant.
   Run light spec/catalog/whitespace checks after those metadata changes.
8. The 03:07 release includes normal active hooks, commit, push and ready PR
   publication under this lease. Return local-heavy to ROOT after publication,
   actual joins and fresh exact owned groups gone. Follow later grants for one
   observer, semantic evidence and serial normal merge. No merge authority now.

## Verification

All shell commands use `/bin/bash`, loginfalse. After release, expose the
existing tools to child processes without a runtime/config override:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:$PATH"
```

ROOT's release refines resources before execution: set
`NODE_OPTIONS=--max-old-space-size=4096` for Vitest, ESLint and typecheck, keep
Vitest maxWorkers=1, and run no heavy commands concurrently. Wrap each heavy
command in an owned bounded process group; record original native handle,
cwd/argv, PID/start/group/bounds, actual terminal join and fresh exact group
absence before the next heavy command. Add changed i18n:check to the defined
ratchet and actual changed-path documentation coverage checks.

Initial actual dependencies were absent. Exactly one conditional install from
`apps/`, only if they are still absent under the lease:

```bash
pnpm install --frozen-lockfile
```

From `apps/web/`, RED after independently writing T1/T2/T3, before production
change (Vitest's actual project classifier places this suite in browser-locales):

```bash
pnpm exec vitest run --project browser-locales --maxWorkers=1 'app/settings/agents/[agentId]/agent-create-catalogue.test.tsx'
```

GREEN once T1 through T6 and the bounded correction are complete:

```bash
pnpm exec vitest run --project browser-locales --maxWorkers=1 'app/settings/agents/[agentId]/agent-create-catalogue.test.tsx' 'app/settings/agents/[agentId]/agent-save-helpers.test.ts' 'app/settings/agents/[agentId]/agent-save-helpers-provider.test.ts'
pnpm exec eslint --max-warnings 0 'app/settings/agents/[agentId]/page.tsx' 'app/settings/agents/[agentId]/agent-create-catalogue.test.tsx'
pnpm run typecheck
pnpm run i18n:ratchet
pnpm run i18n:check
```

If a test-owned helper is necessary, add its exact path to that lint command;
do not expand to unrelated files. No new broad UI/WS/picker test reruns are
needed when their implementation is unchanged. Normal active commit hooks
remain a later lease-owned delivery action; no bypass.

Light document checks from repository root after final spec metadata changes:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

## Files likely touched

- `apps/web/app/settings/agents/[agentId]/page.tsx` (production ownership).
- `apps/web/app/settings/agents/[agentId]/agent-create-catalogue.test.tsx` (new independent coverage).
- A same-directory test-owned helper only if needed for file/function limits;
  literal fixtures remain in the recognized test or explicit helper parameters.
- The four package artifacts for accurate lifecycle/status/results.

Read-only compatibility resources: `agent-save-helpers.ts`,
`agent-save-contributor.ts`, `app/actions/agents.ts`, `lib/ws/handlers/agents.ts`,
`components/state-provider.tsx`, `components/settings/settings-save-provider.tsx`,
`components/settings/agent-profile-picker.tsx`, settings types/slice actions,
and existing helper tests. A need to modify them is a ROOT scope checkpoint.

## Mobile and docs

Pure state/data normalization within unchanged composition/copy qualifies for
mobile-parity's narrow exception. Real component coverage exercises shared
profile availability; no new mobile Playwright test or ASCII layout is planned.
Public docs need no change because procedure, terminology, copy and commands
remain the same. This package records internal behavior and implementation.

## Risks

Read too early and the stale closure remains. Mock a product boundary and the
test can miss the failure. New-agent partial MCP errors have existing navigation
different from additional-profile partial errors; observe it honestly.

## Results

Implementation completed after ROOT's 03:07 release on 2026-10-09. Production
changes only the page import/store binding and callback-time catalogue read.
Real Page/providers/saveAll/API/WS/picker tests cover T1 through T6. Only external
fetchJson transport is mocked; no editor mock or production seam was necessary.
All six ACs are covered, with unchanged shared composition/copy as the mobile
exception. These are browser component integrations, not hosted backend E2E.

Commands are the exact Verification commands above, unless the row names an
affected-only rerun. Each serial command retained its original native handle,
actually joined terminal, and recorded an empty fresh owned process group before
the next heavy command. Full cwd/argv/PID/start/group/time bounds and raw logs
are in `/tmp/kandev-child92-execution-20261009/<label>.json` and `.log`; original
native handles and terminal chunks are in the corresponding `*-native.json`.

| Check / receipt label | Original native / terminal | Exit / result | Owned group, gone after join |
| --- | --- | --- | --- |
| Conditional frozen install, `install` | 74882 / b5d8dc | 0, single install | 4168362 |
| T1-T3 RED, `red` | 47142 / 3f0629 | 1; T1 causal store and picker failure, T3 passed; T2 fixture navigation expectation wrong | 4177003 |
| Affected T2 control, `red-success-control` | 52717 / 24d93a | 0, 1 passed / 2 skipped after fixture expectation correction | 4182019 |
| T1-T6 + focused helpers, `green` | 76333 / ee3eb6 | 0, 48 passed / 3 files | 4185913 |
| Prettier, `format` | terminal ed8c90 | 0; test formatted, page unchanged | 4188440 |
| Initial exact lint, `lint` | 34297 / 344f53 | 1; test describe exceeded 100 lines only | 4188786 |
| Affected test lint, `lint-test-grouping` | 75970 / 718c24 | 0 after splitting test groups | 4190230 |
| Initial typecheck, `typecheck` | 95932 / be7acc | 2; three test-owned transport/handler/wire typing errors | 4191890 |
| Fixture format, `format-type-fixture` | terminal dfc545 | 0 | 3654 |
| Final affected T1-T6, `green-final-fixtures` | 17630 / 879462 | 0, 6 passed / 1 file; passing helper suites not replayed | 4664 |
| Final typecheck, `typecheck-fixtures` | 8168 / 6d566e | 0; original handle recovered after interrupted tool call | 6375 |
| Final affected test lint, `lint-final-fixtures` | 27803 / f11582 | 0 | 16300 |
| `i18n-ratchet` | 16338 / 815f64 | 0; changed source clean, guard intact | 16849 |
| `i18n-check` | 31460 / ed106c | 0; catalog's 436 unreferenced-key warnings are non-failing | 17670 |

Actual changed-path documentation validator returned `covered`, `ok=true`,
`errors=[]` for the six owned files; accepted this work order and its owning
requirement/design. No proof replay, extra install, heavy overlap, runtime/cache
hack, foreign kill, delegation or protected-file access occurred.

Before publication, active pre-commit and commit-msg hooks and installed
commitlint were verified. Normal delivery receipts are recorded in the durable
Kandev task plan/conversation; no hook or publication result is inferred here.
Ready PR association requires post-link canonical readback and all five flags
false. The premature pre-link flag attempt failed with state_known=false and
was not retried; ROOT disposition permits only necessary scoped post-link
correction. Hosted CI/reviews and merge remain pending separate ROOT gates.
