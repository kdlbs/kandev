---
id: "01-current-profile-publication"
title: "Reconcile standalone dynamic saves against current profiles"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001
acceptance_criteria:
  - AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.9
system_design:
  - ../../specs/agents/system-design/dynamic-agent-routing-01.md
---

# Task 01: Reconcile standalone dynamic saves against current profiles

## Summary

Preserve current profile collections when the standalone dynamic editor's HTTP
save completes after a WebSocket mutation. Write independent real-state
regressions first, observe meaningful RED, then apply the smallest current-state
publication correction and verify narrow GREEN with the existing controls.

## Entry gates and identity

Task `bc2f7b48-5579-4a9d-a8bf-b2ed13fa8103`, same primary session
`a3aa6a7a-11f2-40de-9b01-ad0b4fa347aa`, title
`Preserve profiles during dynamic saves`. Preserve the `<kandev-system>` marker,
question barriers, user edits, title ownership and normal final-action gates
in the version-safe MCP task plan. Do not create tasks, sessions, tabs, delegate
or change models. ROOT observes the actual conversation and plan directly;
do not depend on callbacks or repeated ACK messages.

ROOT reviewed the concrete four artifacts and sent the later same-primary
implementation release. Implementation and scoped product checks are complete;
docs validation and normal delivery are in progress. Current GLOBAL LOCAL-HEAVY
is exclusively CHILD56 until explicit return after ready publication. MERGE
remains NONE. The original design checkpoint alone supplied no execution grant.

## In scope and ownership

- Production: `apps/web/components/settings/dynamic-agent-profile-editor-state.ts`.
- Optional immediate helper: `apps/web/components/settings/agent-profile-page-state.ts`
  only if existing option reconciliation genuinely requires an adjustment.
- New permanent regression, authored independently after the interrupt:
  `apps/web/components/settings/dynamic-agent-profile-editor-save-concurrency.test.tsx`.
- If the optional helper changes, extend its existing
  `agent-profile-page-state.test.ts` only for that changed logic; keep real-state
  consumer regression primary.
- Existing owning requirement/design plus this manifest/work order for accurate
  statuses and actual results. Preserve their previous history and user edits.

## Out of scope

No production edits to draft/coordinator/store/WS/picker/API/action/normalizer,
other-profile save rewrites, or the unused `useProfileEnabledToggle`. No new
callers, API/backend/persistence/SDK/permissions/transport/environment/timer/
framework/retry/ordering/owner-lifetime/feature-rollout policy. No UI markup,
copy, layout, touch, scrolling, navigation, breakpoint or new browser/E2E/build
work without causal necessity and a later ROOT grant. Preserve managed worktree,
dependencies, logs, shared refs/caches, foreign processes, paused giant and
unproved volume; cleanup applies only to proven owned resources.

## Acceptance

1. The post-response publication reads current store state, replaces only an
   existing owned target under existing revision rules, and preserves every
   unrelated accepted profile and latest option, including deletion and absent
   Settings-owner cases.
2. Independent deferred-request regression fails causally before the correction
   and passes afterward through real store/coordinator/registered-WS/picker.
   Supported preservation assertions use ID, `label`, enabled and revision;
   complete optional-field equality is not an oracle.
3. Loaded editor/candidates/draft/ACK/error/toast/saved revision/onDraftChange
   semantics remain intact. All exact task checks pass with original processes
   joined and physically gone, and durable results truthfully distinguish
   implementation, PR readiness, merge and actual completion.

## Implementation sequence

1. Read this package and the current scoped guidance. Resolve exact current
   checkout/base and affected source contracts without a main-only rebase.
   Mark this work order and MCP phase in progress only after the interrupt.
2. Independently author the test below; do not open/copy/import/replay/mutate
   ROOT's protected archives listed in [the manifest](plan.md#evidence-and-assumption-check).
3. Render real StateProvider/createAppStore, ToastProvider, SettingsSaveProvider,
   the actual hook with current store-derived owner/profile, and an
   AgentProfilePicker subscribed to real options. Capture `useAppStoreApi` and
   `useSettingsSaveCoordinator` from inside those providers. Use the registered
   `registerAgentsHandlers` events, real updateAgentProfileAction and normalizer;
   partial mock only fetchJson, retaining real ApiError/isHandledApiError.
   Use full supported snake-case wire snapshots and valid dynamic candidates.
4. Edit the draft, await contributor registration, invoke `saveAll`, inspect
   PATCH URL/method/body while its promise is deferred, deliver WS, and prove
   it updated live state before resolving HTTP. Await the original save and
   resulting renders. Always settle/reject deferred promises during cleanup;
   no sleeps, mocked state/provider/coordinator, manual save substitute, or
   tests that merely echo a new helper.
5. Run the exact causal RED command. Expected cause is another owner's profile
   removed by captured-list publication, not setup or fixture failure. Positive
   controls must distinguish normalized ACK/request behavior from preservation.
6. In the owned hook, obtain the store API and read it after awaited response.
   Map its current settings agents, replacing only the matching existing target
   under the editor owner if `isProfileRevisionNewer` allows it. Missing owner
   or target means no insertion; do not republish a captured row. Reuse
   `reconcileAgentProfileOptions` or `useSyncAgentsToStore` for current options.
   Preserve the current draft response acceptance and all surrounding behavior.
7. Run GREEN matrix and existing controls, then the remaining exact checks.
   Update results with actual argv, logs, exit codes, timing and cleanup evidence.
   Do not promote the entire existing multi-part dynamic-routing draft or mark
   its broad historical plan complete because this bounded work order passes.

## Regression matrix

Use describe name `dynamic profile standalone save concurrency` and these exact
test names for anchored selection. Map the first five and failure test to
`@covers AC-AGENTS-DYNAMIC-AGENT-ROUTING-001.9` in test names or nearby comments.

| Test name | Stimulus and observable oracle |
| --- | --- |
| `retains a concurrent known-owner create` | Registered create for another loaded owner during deferred save; profile and option remain after completion. Causal RED anchor. |
| `retains a concurrent known-owner update` | Another owner's existing row gets newer name and enabled=false; current row/option retain revision, label and disabled state. |
| `retains a concurrent known-owner deletion` | Registered deletion of another owner's profile; neither collection resurrects it after ACK. |
| `retains mixed represented and unrepresented options` | Loaded other-owner row plus valid option whose Settings owner is absent; retain both, and preserve a newer represented option over an older Settings snapshot. |
| `retains the rendered unrepresented picker label` | Select valid unrepresented option in actual AgentProfilePicker before save; accepted label remains afterward and is not replaced by Unavailable. |
| `preserves a newer targeted profile revision` | Newer target WS update while request is pending; older HTTP response cannot regress current target or newer flattened option; existing dirty/conflict behavior remains. |
| `does not insert an absent target` | Remove owned target during deferred request using registered delete; completing ACK does not append it to Settings or rebuild a deleted option. Characterize surviving option separately through mixed case. |
| `does not insert an absent owner` | Current Settings owner is absent at response; completion does not recreate owner from props or introduce a target row. No global lifetime guarantee. |
| `accepts an ordinary current response and request` | No concurrent mutation; one real PATCH with trimmed name, enabled, dynamic version and ordered snake-case policies; current normalized target/option, saved revision and success toast remain supported. |
| `accepts its own websocket acknowledgement` | Matching current target ACK delivered by registered WS before HTTP response; no external conflict, duplicate target or draft regression. Use fully normalized supported ACK fixture. |
| `preserves state on a failed response` | Reject deferred PATCH after unrelated WS change; no destructive collection publication or success toast, existing error toast/draft/saving cleanup and actual coordinator outcome. Do not alter swallowed-error semantics. |
| `blocks an invalid policy through the coordinator` | Dirty invalid transient/hard policy vetoes saveAll; no fetch or store publication. Ordinary-success case proves a valid policy is admitted. |
| `keeps embedded drafts parent-owned` | onDraftChange receives edited name/enabled/candidate patch; embedded contributor stays non-standalone, no request/publication through saveAll. |

Where a same-profile response ties or lacks timestamps, characterize
`isProfileRevisionNewer`'s editable comparison and `mergeOptionsByNewest`'s
rebuilt-option tie, without a new version or field-merging policy. Existing
reconciliation tests remain the focused controls for these rules. Do not assert
new global cross-owner, deleted-row, transport or session-order guarantees.

## Mobile and rendered verification

State-only correction inside the shipped dynamic profile editor and shared Save
surface. Desktop and phone consume the same state; layout, interactions and
viewport logic are unchanged. The real rendered picker test plus unit/component
matrix satisfies mobile-parity's pure state/data exception. No ASCII preview,
browser startup, Playwright or build is required. If implementation changes that
scope, end to ROOT before expanding verification.

## Verification and resource protocol

Run serially only under explicit GLOBAL LOCAL-HEAVY lease. Use bash with
`login=false`, Node24 and pinned pnpm9.15.9 via explicit PATH; do not change
system tooling. Verify versions before product checks. Every command gets an
upfront UTC start and cutoff, exact argv, separate owned log and recorded
original tool session_id/chunk plus PID/PGID. Retain and actually join every
original handle; record exit and prove its owned PID/PGID physically gone.
Use GNU timeout TERM at the limit and KILL after 10s. Never infer completion
from a second run or discard a handle. These are required execution records,
not design-turn jobs.

From repository root, initialize each bash invocation with:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
node --version
pnpm --version
```

Only if `apps/node_modules` or the required local tool binaries are absent,
perform ONE conditional installation, under the lease, from apps:

```bash
(cd apps && timeout --signal=TERM --kill-after=10s 120s pnpm install --frozen-lockfile)
```

No installation in this design turn, no repair install loop or cache wipe.
If versions/tooling/setup/resource/timeout fail, actually join/clean only the
owned command, checkpoint and END to ROOT without autoretry.

Exact causal RED (before production edits):

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism components/settings/dynamic-agent-profile-editor-save-concurrency.test.tsx -t '^dynamic profile standalone save concurrency retains a concurrent known-owner create$')
```

Exact GREEN matrix:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism components/settings/dynamic-agent-profile-editor-save-concurrency.test.tsx -t '^dynamic profile standalone save concurrency ')
```

Exact existing controls:

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm exec vitest run --project=browser-locales --maxWorkers=1 --no-file-parallelism components/settings/dynamic-agent-profile-editor-state.test.ts components/settings/agent-profile-reconciliation.test.ts components/settings/agent-profile-page-state.test.ts components/settings/agent-profile-picker.test.tsx components/settings/settings-save-provider.test.tsx app/actions/agents.test.ts)
```

Changed-file ESLint (add `components/settings/agent-profile-page-state.ts` and
its test only if actually changed; no whole-tree lint):

```bash
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm exec eslint components/settings/dynamic-agent-profile-editor-state.ts components/settings/dynamic-agent-profile-editor-save-concurrency.test.tsx)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm run typecheck)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm run i18n:check)
(cd apps/web && timeout --signal=TERM --kill-after=10s 120s pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Use the normal project typecheck, including its legitimate ignored pretypecheck
release-note/changelog assets. No direct tsc, generated-file/setup hacks or
changes to tracked configuration. Re-run only affected checks for routine own
causal fixture or lint corrections. Unknown, resource, setup, timeout,
transport or out-of-scope failures cause actual END to ROOT; no passing replay,
foreign kills or uncontrolled retries. Actual reference/link/PR-doc coverage
preflight must validate this work order, its plan, owning requirement and design
against the eventual actual changed paths. Document-only exemption alone does
not establish implementation coverage.

## Delivery boundaries after implementation

Use normal active hooks, a new Conventional Commit, push and ready PR after
authorized checks pass. No bypass or amend. Load current commit/push/pr skills
and PR template at delivery. Freeze the published head except actual findings;
no main-only rebase, synthetic merged tests or optional polish.

Explicitly return LOCAL-HEAVY to ROOT before ONE original all-terminal
`scripts/pr-await <PR> --mode all-terminal --deadline-min 45 --format json`
observer under GNU `timeout --signal=TERM --kill-after=10s 46m`. Retain that original observer across
actual fixup heads. ROOT may authorize replacement only after it has joined and
its PID/PGID is physically gone. No self-successor, manual parallel GitHub timer
probe or second observer. Use actual current scripts and skills; verify the
deadline/CLI contract before starting, and END to ROOT if it differs.

Readiness requires the current six required contexts plus actual Backend,
Frontend and E2E parent SUCCESS; fresh complete exact-head evidence with
`errors=[]`; disposition of visible and hidden actionable human findings and
an empty resolver. Require authenticated configured App347564 substantive FULL
CURRENT HEAD ALL-files review. Inspect automatic processing/complete/skip;
sufficient automatic FULL review is accepted. At most ONE necessary true-gap
request; no ACK comments, optional duplicates, Claude extra or settings changes.
Any valid finding needs focused remediation and a new actual head/check record.
Acquire a later serial heavy lease for remediation checks.

Retry only the exact ROOT-named failed-job budget after terminal parent, fresh
frozen OPEN/aligned head and all reads joined, with ordinary causal dependencies.
No passing/all/blind/unknown duplicate POST. At actual MERGE-ready END, return
to ROOT; do not merge. ROOT performs static current-main/head/merge-tree/owned
blob/contract assessment and must send a SEPARATE serial MERGE interrupt.

If later authorized, use normal expected-head squash, no admin/bypass. Verify
independently the actual REST/Git merged SHA, tree, owned blobs and main
reachability. Join every owned handle and clean owned resources only. Explicit
MERGE RETURN precedes actual completion END. ROOT owns independent archives,
checksum proof release and loop refill. No machine-uptime guarantee: retain an
honest durable checkpoint and recover in this same session.

## Dependencies and parallelism

None. `sequential`. No native delegation, new persistent tasks, sessions or tabs.

## Inputs

- [Requirement](../../specs/agents/requirements/dynamic-agent-routing.md),
  `REQ-AGENTS-DYNAMIC-AGENT-ROUTING-001`, criterion `.9`.
- [Owning design](../../specs/agents/system-design/dynamic-agent-routing-01.md#standalone-settings-save-publication).
- [Existing Platform editor contract](../../specs/platform/system-design/agent-settings-parity.md#editor-reconciliation).
- Existing source: AgentProfilePage, DynamicAgentProfileEditor and its state/draft,
  StateProvider, SettingsSaveProvider, updateAgentProfileAction,
  normalizeAgentProfile, registerAgentsHandlers, settings slice and AgentProfilePicker.
- Existing similar implementation: reconcileAgentProfileOptions,
  useSyncAgentsToStore, mergeOptionsByNewest and their existing tests.
- ROOT's accepted proof summary in the manifest; protected archives remain untouched.

## Risks

Response/WS normalization differences can make whole-object equality misleading.
Use supported oracles and fully supported ACK payloads. Keep revision ties and
absent-row cases bounded by the owning design. Preserve caught-error behavior
and newer local edits; a separate failure-transport or coordinator repair is
out of scope. Resource or authority boundaries override delivery convenience.

## Results

Implementation executed in the same primary session on base
`95c040e84951773dc8ed6f684fff0425d7b63f96`, after ROOT reviewed the four exact
design hashes. Only the owned dynamic state hook changes production: it reads
current settings agents after HTTP, updates the existing owned target only when
existing revision reconciliation accepts it, and reuses `useSyncAgentsToStore`.
The existing helper and all other production consumers remain unchanged.

The new test was authored independently without reading or copying ROOT's
protected archives. It exercises the actual state provider, coordinator, hook,
registered WS handlers, API action/normalizer and rendered picker, partially
mocking only fetchJson. The known-owner-create RED failed on the lost row and
option after an accepted live create, with production unchanged. Ordinary
request/ACK controls passed before the correction.

| Check | Actual result | Original handle / first chunk / terminal chunk |
| --- | --- | --- |
| Conditional pinned frozen install | PASS; Node24.21.0 / pnpm9.15.9, 2.1s | 93961 / a2207e / 7ce362 |
| Causal RED exact anchor | Expected exit1, 1 causal failure / 12 skipped | 82076 / 12a811 / 0bfacf |
| Pre-fix current request/own ACK controls | PASS, 2 / 11 skipped | 31283 / 011d6b / be6a27 |
| Initial fixed matrix | PASS, 13/13 | 5468 / 8221a1 / bb93ea |
| Six existing controls | PASS, 54/54 | 36004 / f88aed / 4ef731 |
| Final independently authored matrix | PASS, 13/13, 10.47s | 46895 / 1f0f88 / 9b223d |
| Changed ESLint, zero warnings | PASS | 55021 / 409202 / 4b600c |
| Normal project typecheck | PASS, including legitimate pretypecheck assets | 73483 / f76126 / b5c629 |
| i18n check | PASS; existing catalog orphan notice only | 51061 / cfe95d / 241d26 |
| i18n new-code ratchet | PASS | 86952 / f475bc / 53592c |

Routine own-fixture repairs followed actual diagnostics: extracted long test
callbacks and duplicate literals; supplied canonical feature/options state;
narrowed event dispatch and included required legacy WS payload fields. The
own-ACK control was strengthened to assert accepted revision and clean
coordinator before HTTP resolution. Per-test increasing revisions prevent
prior deletion tombstones from suppressing that control. No causal assertion
was weakened and no production WS/type/framework behavior changed. Typecheck
attempts 72337 and 75491 exited2 only on these own fixture diagnostics; all
original handles joined and PID/PGIDs disappeared before affected reruns.
Existing passing 54 controls were not replayed after fixture-only repairs.

Every product command ran serially under the exclusive lease, Bash login=false,
explicit pinned PATH, Node4GiB, GNU120s TERM/kill10 bounds. The original records
under `/tmp/kandev-child56-execution/<name>.json` and `.log` retain exact argv,
UTC start/cutoff/kill cutoff/join, PID/PGID, exit and physical disappearance;
`original-ledger.json` adds original tool handles/chunks. The version-safe MCP
plan retains the full durable receipt handoff. Formatting changes only layout.
No foreign resources, archives, tracked setup or runtime flags were changed.

Public guidance remains accurate after the documented audit. This pure state
correction changes no markup/layout/copy or viewport interaction; the rendered
picker regression supplies the mobile parity evidence. No browser/E2E/build
was requested or run.

Implementation and task verification are complete. Catalog validation
(357 decisions/1408 specs), all-spec lint and all 36 linter tests pass.
Actual six-file reference coverage is `covered`, `errors=[]`, accepting the
owning Agents requirement/design and this ONE work order. All 25 local Markdown
links/anchors, 32KiB ceilings and whitespace checks pass. `git diff --check`
passes. Requirement/design retain their broader existing draft statuses.

Normal hooked commit/push and ready publication are next. CI/reviews, observer
and merge are pending; done here means bounded implementation/check completion,
not merge or overall task completion. The later delivery boundaries above
remain mandatory. Current publication/CI/merge receipts belong in the durable
MCP plan so the published head remains frozen except actual findings.
