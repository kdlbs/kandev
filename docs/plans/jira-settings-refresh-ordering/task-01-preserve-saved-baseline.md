---
id: "01-preserve-saved-baseline"
title: "Preserve saved Jira baseline"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-SETTINGS-MANUAL-SAVE-001
acceptance_criteria:
  - AC-UI-SETTINGS-MANUAL-SAVE-001.4
system_design:
  - ../../specs/ui/system-design/settings-manual-save.md
---

# Task 01: Preserve Saved Jira Baseline

## Summary

Fence obsolete Jira configuration GET publications at the mounted caller.
Keep acknowledged Save/removal baselines and later eligible reads working.

## In scope

Jira settings read ownership, acknowledged writes, meaningful real-boundary
regressions, desktop/phone browser proof, internal design and delivery records.

## Out of scope

Query/global coordination, other integrations, server ordering, cross-tab writes,
backend/API/schema/auth/credential changes, initial-GET recovery expansion,
sibling row rename fixture, additional agents/tasks/sessions, push and PR.

## Acceptance

1. Old successful/null GETs cannot replace an acknowledged Save/removal baseline;
   failed reads/writes retain truthful existing behavior. Latest eligible reads,
   later fresh null and health metadata continue to publish on the actual timer.
2. Canonical POST normalization and secret identity/reuse remain unchanged;
   in-flight edits survive, Reset uses the accepted baseline, next Save works.
3. Workspace/lifetime and overlapping-read tests pass; desktop and phone use the
   real disposable backend with mock Jira only, and cleanup/seed guards pass.

## ASCII UI preview

UI-01: Existing Jira settings, desktop and phone. State semantics only.
Entry: Settings > Workspace > Integrations > Jira. Existing content scrolls;
the shared safe-area-aware Save surface stays fixed when dirty.

```text
[Jira integration]  [Enabled]
[Site] [Authentication] [Project: SAVED]
[Saved credential] [Auth health]
Before: old GET arrives -> [Reset] [Save changes] (incorrectly dirty)
After:  old GET arrives -> clean, no dirty surface
Later edit: [Project: NEWER] -> [Reset] [Save changes]
Reset: [Project: SAVED] -> clean
```

Phone uses the shipped dedicated workspace integration settings route and
existing touch Save/Reset controls. No layout, navigation, or hitbox changes.

See [full package](plan.md). AC-UI-SETTINGS-MANUAL-SAVE-001.4 applies throughout.

## Verification

```bash
(cd apps/web && pnpm exec vitest run components/jira/jira-settings-refresh.test.tsx components/jira/jira-settings.test.tsx components/jira/jira-form-state.test.ts)
(cd apps/web && pnpm exec eslint --max-warnings 0 components/jira/jira-settings.tsx components/jira/use-jira-config-publication.ts components/jira/jira-settings-refresh.test.tsx e2e/tests/integrations/*jira-settings-refresh*)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet)
GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/integrations/jira-settings-refresh.spec.ts -- --retries=0
GOMAXPROCS=2 GOFLAGS=-p=2 scripts/run-quiet e2e --summary -- pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/integrations/mobile-jira-settings-refresh.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Also run the repository PR-documentation coverage preflight against the complete
changed-path set, normal active pre-commit and commit-msg hooks, and inspect
screenshots. Retain all handles until terminal and external logs/receipts.

## Files likely touched

- `apps/web/components/jira/jira-settings.tsx`
- `apps/web/components/jira/use-jira-config-publication.ts`
- `apps/web/components/jira/jira-settings-refresh.test.tsx`
- `apps/web/e2e/tests/integrations/jira-settings-refresh-helpers.ts`
- `apps/web/e2e/tests/integrations/jira-settings-refresh.spec.ts`
- `apps/web/e2e/tests/integrations/mobile-jira-settings-refresh.spec.ts`
- `docs/specs/ui/system-design/settings-manual-save.md`
- This plan and work order; public integrations explanation only if needed.

## Dependencies and parallelism

None. Sequential in the same primary session; preserve all other work.

## Inputs

[Existing requirement](../../specs/ui/requirements/settings-manual-save.md),
[owning design](../../specs/ui/system-design/settings-manual-save.md), and source
receipt in the parent-provided `attempt24-source` directory. Main is
`f99ac9086e8f7c6e9cab6d6e5151227a20a68833`; open PR 1512 head remains
`a8655ad92faa050a68140a904500fc99ebc0a04b`, with no equivalent pending-read fence.

## Risks

StrictMode disposal, stale loading finalizers, failed mutation recovery, and
preserving successful write admission independently of read ordering.

## Results

Implemented the mounted Jira owner in `use-jira-config-publication.ts` and wired
all settings GET producers plus Save/removal acknowledgements to it. No other
integration, backend, cache, API, schema, credential, or shared coordinator change.
The normal 90-second timer starts after form-seeding loads settle, so a very slow
initial GET remains able to initialize the form; initial-failure recovery is not
expanded. Successful write feedback and admission remain independent of read order.

- Permanent source RED: `pnpm exec vitest run components/jira/jira-settings-refresh.test.tsx`
  failed on two actual dirty-state assertions (old 200/204 after Save); old 500
  control passed. No import/setup failure. External `red.log`.
- Managed desktop RED: real disposable backend, mock Jira, real 90-second timer,
  held pre-save HTTP snapshot. After Save, releasing it changed
  `data-settings-dirty` to true. One causal failure; cleanup and seed/source guards
  passed. External `browser-red-full.log`.
- A focused slow-initial-load RED caught lost form seeding in the first correction;
  the final timer gate preserves it. External `initial-seeding-red.log`.
- Final focused Vitest command above: 3 suites, 30 tests passed (23 new caller/API
  cases plus 7 existing settings/form cases). Covers old success/null/error,
  overlapping reads and failed latest read without replay, Save during reads,
  reads during Save, canonical ACK, metadata, later edits, Reset, second Save,
  failed Save/removal, successful removal, fresh reads/null, workspace replacement,
  unmount/remount, StrictMode and slow initial load. `green-final.log`.
- Final changed-file ESLint with `--max-warnings 0`: passed. Full web typecheck
  with the CI 8 GiB Node cap: passed (`typecheck-final.log`). An earlier typecheck
  caught unsupported Testing Library `exact` options in the tests; corrected.
- Final desktop (chromium) and native phone (Pixel 5 `mobile-chrome`) capture
  commands each passed 1 test, one worker, one shard, retries 0, strict WS checks.
  Desktop tests a stale 200 snapshot; phone tests a stale real 204 snapshot.
  Both exercise Save, clean state after release, Reset, second Save, and reload.
  Poll-time config snapshots may also serve the existing availability consumer;
  only those pre-write GET responses are held, and production APIs remain real.
  Fresh backend/web/plugin production build preceded final product checks; phone
  and screenshot runs intentionally reused those exact unchanged assets through
  managed `--no-build`. Final capture reruns were needed because body-only
  attachments were not retained by the list reporter; screenshots now persist.
- Inspected `desktop-saved.png` and `mobile-saved.png`: saved NEXT project is
  visible without dirty markers or horizontal overflow, using existing layouts.
  Phone is browser/device emulation, with no physical keyboard/OS/device or real
  Jira-provider/incident claim. These are focused passes, not a zero-flake gate.
- Each browser case owns and finally removes a workspace, repository and explicit
  worktree executor. Shared seed HEAD/full status and source HEAD/full status
  remained identical. No operator runtime/data mutation or shared seed reset.
- `pnpm run i18n:check`: passed, seven complete locale catalogs/pseudo unchanged.
  No new user-facing UI copy. `i18n:ratchet`, documentation preflight, whitespace,
  and normal commit hooks are recorded in the external final checkpoint receipt.
- `python3 scripts/list-docs.py validate`: passed, 396 decisions/1701 specifications.
  `python3 scripts/lint-spec-files.py --all`: passed. Public-doc tests: 63 passed;
  public-doc validator: 54 pages validated. Public integrations how-to adds the
  acknowledged baseline/Reset explanation; root README/screenshots need no change.

Two intermediate caller-test runs failed only on fixture steps: confirmation is
asynchronous, and a clean removal-failure control does not render Reset. Corrected
by awaiting actual confirmation settlement and staging a local edit before Reset;
these are not additional causal source failures. One early root-shell invocation
could not find pnpm/Node; package commands use the installed Node 24 runtime from
`apps`, and root doc validators use that same Node executable explicitly.

External durable evidence directory:
`/home/jcfs/.kandev/tasks/preserve-saved-jira_9z9wmeu2/receipts/`.

Original source-proof/commit base remains
`f99ac9086e8f7c6e9cab6d6e5151227a20a68833`. Live main refresh reached
`5aa31bec1a8a5e9cdb679811b02aa3e7b78271c2` through unrelated row-rename,
settings-return and agent-runtime changes; exact incoming paths do not intersect
this patch. No equivalent Jira fix appeared in the refreshed 68-open-PR inventory.
PR 1512 remains `a8655ad92faa050a68140a904500fc99ebc0a04b`; inspected actual
paginated patches show Query replacement without an equivalent pending-read ACK
fence. PRs 4375/4517 heads are unchanged and have no matching settings/API/form
producer paths. No backlog fishing or Query migration is included.

Implementation and local verification are done. Stop at the clean committed
`ready_for_parent_validation` checkpoint with all owned handles terminal. No push,
PR, CI monitor, merge, archive, follow-up task, or other worker is authorized here.


## PR review remediation

Review remediation is limited to the browser helper: arm workspace-scoped POST
response waits before both Save actions, wait for the released GET body, observe
its rejected publication with a named negative-assertion dwell, and reject the
capture barrier when fetching/asserting/fulfilling fails. The existing production
owner and Jira wiring are unchanged. Strengthened-helper source RED and final
focused browser results are retained in the external delivery receipt.

The optional toast suggestion is not adopted: an acknowledged backend write
still succeeds when its mounted owner is gone, and feedback must remain truthful.
The optional loading-finalizer suggestion is not adopted: publication freshness
and live-owner loading settlement have different lifetimes. A successful write
can invalidate a read without disposing its owner; `isCurrent()` in `finally`
could leave loading set after that invalidation. No speculative concurrent-load
or initial-GET recovery redesign is included.

Native hosted CI for the reviewed implementation passed both owned Jira cases
on their first attempts. Its retry-summary artifact recorded four unrelated
cases passing after retry; that is qualified evidence, not a zero-flake claim.
Fixture-only follow-up invalidates the prior exact-head CI/review snapshot.
Final exact-head gates and current-base synthetic proof remain external delivery
requirements; no record-only commit is needed after those gates finish.


The strengthened helper reproduced the original dirty-state assertion against a
temporary original producer in the owned detached checkout. Final desktop and
phone checks each passed one case, retries 0, after the helper changes. A local
phone failure and bounded diagnostic isolated an unresolved body-completion wait
for the held real 204 response; 204 now uses its response event (no body), while
200 responses still await their bodies. Timeout cleanup also runs after the page
closes. Both failed diagnostic runtimes were removed by managed teardown, and
all temporary instrumentation/producer edits were removed. Strict changed-helper
and sanctioned-wait lint passed. See external `hardened-helper-*` receipts.
