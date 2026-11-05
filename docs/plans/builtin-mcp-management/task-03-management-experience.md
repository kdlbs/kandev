---
id: "03-management-experience"
title: "Package selection and responsive management"
status: completed
wave: 3
depends_on:
  - "02-policy-runtime-binding"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BUILTIN-MCP-001
  - REQ-AGENTS-BUILTIN-MCP-002
  - REQ-AGENTS-BUILTIN-MCP-003
  - REQ-AGENTS-BUILTIN-MCP-004
acceptance_criteria:
  - AC-AGENTS-BUILTIN-MCP-001.1
  - AC-AGENTS-BUILTIN-MCP-001.2
  - AC-AGENTS-BUILTIN-MCP-001.3
  - AC-AGENTS-BUILTIN-MCP-001.4
  - AC-AGENTS-BUILTIN-MCP-002.1
  - AC-AGENTS-BUILTIN-MCP-002.3
  - AC-AGENTS-BUILTIN-MCP-002.4
  - AC-AGENTS-BUILTIN-MCP-002.5
  - AC-AGENTS-BUILTIN-MCP-003.1
  - AC-AGENTS-BUILTIN-MCP-003.3
  - AC-AGENTS-BUILTIN-MCP-003.4
  - AC-AGENTS-BUILTIN-MCP-003.5
  - AC-AGENTS-BUILTIN-MCP-004.1
  - AC-AGENTS-BUILTIN-MCP-004.2
  - AC-AGENTS-BUILTIN-MCP-004.3
  - AC-AGENTS-BUILTIN-MCP-004.4
  - AC-AGENTS-BUILTIN-MCP-004.5
system_design:
  - ../../specs/agents/system-design/builtin-mcp-management.md
---

# Task 03: Package selection and responsive management

## New-task placement correction (2026-11-04)

Completed the user's finding under AC-AGENTS-BUILTIN-MCP-001.1 and UI-02.
The form previously rendered a package disclosure above Advanced settings. Its
summary and the shared list both rendered the same title. Move the package
content into the existing MCP server picker and remove the extra disclosure.
The shared list owns the single heading. Preserve the package draft across
picker/Advanced close and reopen, and clear it at the form's new-task/workspace
reset boundary. Keep existing additional-server selection and package authority.

Own `task-create-dialog.tsx`, `task-create-dialog-advanced-settings.tsx`,
`task-create-dialog-mcp-select.tsx`, `task-create-builtin-packages.tsx`, and the
form reset/state wiring, and shared package-row responsive height. Reuse package creation E2E helpers/specs on
desktop and phone. Add reset regressions in the existing form lifecycle test.
No sibling exclusions, runtime injection, or new policy work.

Mobile design: New Task > Advanced settings > MCP servers is the entry point.
Use the existing task-create MCP drawer, its header Close, one safe-area-aware
scroll body, and the existing form submit footer after dismissal. The low-use
advanced option stays beside other MCP choices. The desktop uses its existing
popover with the same list/detail. One heading identifies Built-in packages.
Back returns focus to the package row. Preserve package choices across closure.

Acceptance: no standalone package section; list reachable through MCP even
without catalog servers or while their request is pending; one title; explicit On survives picker and Advanced
reopen and is saved by task creation; Back never submits; phone controls remain
touch-sized and contained, including wrapped row labels; new task/workspace clears previous consent.

PR review repair: only the additional-server content waits for catalog and
selection data. Built-in packages retain their independent loading and editing
path in the same picker. A deferred catalog response must not disable that
entry. Desktop and minimum-width phone regressions choose Configuration On
before releasing the real server response, then verify that consent survives.

Focused commands (managed host environment/resource bounds from results):

```sh
pnpm --dir apps/web exec vitest run components/task-create-dialog-reset-effects.test.ts components/task-create-dialog-form-reset.test.ts components/task-create-dialog-advanced-settings.test.tsx components/task-create-dialog-composition.test.tsx
pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/settings/builtin-mcp-management.spec.ts -- --retries=0 -g 'at task creation|Back from package'
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-builtin-mcp-management.spec.ts -- --retries=0 -g 'at task creation|Back from package'
```

Also run changed-file lint/format, web typecheck, and affected docs validators.
Update the public package how-to's new-task entry instructions.

Hosted follow-up CI also exposed sequencing defects in the existing navigation
and workflow-model E2E cases. Delivery includes their test-only repair and
focused phone/desktop checks recorded in the implementation results. This does
not reopen product scope or change package policy, UI, or runtime behavior.

## Summary

Expose built-in package policy and task choice through existing MCP management.
Complete Configuration opt-in, per-package session evidence, desktop/phone parity,
localization, dependent action guards, and public documentation.

## In scope

Typed API clients/domain hooks; Built-in package section and detail; workspace
limits/defaults with shared Save/Discard/Reset; task Inherit/On/Off selector in
Customize and authorized task creation; fixed configuration-chat explanation;
read-only/blocked/denied/conflict flows; per-package connection/evidence detail;
Walkthrough guard/queued intent; locales; focused unit/rendered/E2E tests; docs.

## Out of scope

Global-profile package picker, independent session override, general management
hub/navigation changes, inherited-server exclusions, assignment at registration,
new probes, automatic reconnect, or modifying sibling workspaces.

## Acceptance

1. UI-01/UI-02 make Configuration explicit for regular tasks and automatic in
   configuration chats, with human authority and shared-resource scope explained.
   Workspace blocks and Core reasons are visible. Save/discard/reset/conflict/
   denial retain correct drafts and task/workspace ownership.
2. Desired, bound, and observed packages stay separate across saves, reloads,
   execution/session changes, legacy agents, and failures. One package's health
   never appears as another's evidence. Walkthrough requests use bound eligibility;
   historical content remains readable. Collisions offer safe remediation.
3. Desktop and phone finish opt-in and pruning with equivalent capability,
   localized copy, one phone scroller, safe areas, correct targets, keyboard and
   focus return. Public docs explain names, defaults, authority, and exact timing.

## UI preview contract

Use [UI-01 and UI-02](plan.md#ui-preview-contract). The explicit task instruction
requires native rich-output previews rather than ASCII.

UI-01 desktop: Built-in package list > inline package detail; purpose/owner,
registered tools/contexts, workspace policy choices, fixed required contexts,
next-start notice, shared Save/Discard. Configuration offers Blocked or Allow
explicit task selection; its configuration-chat default remains visible. Phone:
list replaced by full-height detail, fixed navigation/save, one scrolling body.

UI-02 task: Customize > Built-in packages, with Core Required and each optional
package Inherit/On/Off plus effective value/origin. Configuration initially reads
Inherit (Off); choosing On shows authority inline before Save. Connections >
package shows desired selection, bound manifest, pending state, and actual
observations separately. Desktop master/detail; phone list > detail > tool in one
drawer with Back/Close. Conflict keeps draft, denial stays unsaved, read-only
shows values/reasons. Map to AC-001.1/.2/.3/.4 and AC-004.1 through .5.

## Verification

Run from repository root; install once before pnpm checks in this fresh worktree.
Create named tests via TDD. Managed E2E builds use isolated fixtures, sequential
runs, and no protected runtime/preview ports.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run hooks/domains/workspace/use-builtin-mcp-management.test.ts hooks/domains/session/use-builtin-mcp-session.test.ts hooks/domains/session/use-request-changes-walkthrough.test.ts components/task-create-dialog-helpers.test.ts components/task/chat/mcp-explorer/builtin-mcp-view-model.test.ts components/task/chat/mcp-explorer/mcp-explorer-view-model.test.ts lib/state/slices/session-runtime/builtin-mcp-attachment.test.ts lib/ws/handlers/session-mcp-status.test.ts)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm run i18n:zh-hant)
(cd apps/web && pnpm run i18n:check)
(cd apps/web && pnpm run i18n:ratchet)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/builtin-mcp-management.spec.ts tests/settings/builtin-mcp-configuration.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-builtin-mcp-management.spec.ts -- --retries=0)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run scoped eslint on actual changed TS/TSX paths and any changed tests not listed.
Inspect light/dark, long locales, 320/393/767px phone, 768px boundary, 900px touch,
and desktop. Record screenshots and measured hitboxes/overflow against UI-01/02.
No full E2E audit. Check actual MCP tool catalogs/dispatch through controlled real
built-in requests; mocked healthy rows are presentation-only evidence.

## Files likely touched

- `apps/web/components/settings/workspaces/mcp-settings.tsx`, new detail and tests;
  `hooks/domains/workspace/use-builtin-mcp-policy.ts`; typed API clients/types.
- `apps/web/components/task/chat/mcp-explorer/` selector, detail, navigation and
  view-model; `hooks/domains/task/use-builtin-mcp-preferences.ts`; task creation
  selection/save integration; session snapshot projection and domain hooks.
- `apps/web/hooks/domains/session/use-request-changes-walkthrough.ts`, Changes,
  mobile Changes, Review callers, and typed queued intent payload.
- `apps/web/e2e/tests/settings/{builtin-mcp-management,builtin-mcp-configuration,mobile-builtin-mcp-management}.spec.ts`,
  fixture helpers and `apps/web/src/locales/`.
- `docs/public/agents-and-profiles.md` and `docs/public/automation-and-mcp.md`;
  affected README task-agent MCP and screenshots. Reconcile active workspace MCP,
  rich-output, discovery, and configuration requirements/design wording where
  it assumes unconditional availability or a single injected server.

## Dependencies

Task 02. The UI only exposes selection once runtime and dispatch enforce it.

## Inputs

Full design, parent testing guide, shipped settings save contributor and explorer,
mobile-parity language/sizing references. Use `/tdd`, `/e2e`, `/mobile-parity`,
`/better-ui`, and `/docs-maintainer`. Keep one draft/controller across viewports.

## Risks

Task settings affect future starts of all its normal sessions; a session editor
must not label the scope session-only. Optional server entries are not catalog
records or healthy merely because selected. Qualified names and legacy reports
need clear labels. All seven locales are required; generate Traditional Chinese.

## Parallelism

`sequential`

## Current user-review repair (2026-11-03)

Implementation proceeds within the reviewed UI-02 package and existing
AC-003.3, AC-004.1 through .5. The user requested a clearer package detail,
visible saved Configuration selection, and no empty exclusions section.
This is presentation repair, not a new injection or exclusion authority.

Read-only evidence: isolated task 1 has preference revision 5, Configuration On,
and an owned desired manifest, but its session reports legacy with no bound
manifest. The existing pending-difference helper returns no rows without a
valid accepted snapshot. That hides the saved selection. Existing regression
coverage explicitly expects this empty result, so it misses the user's state.
The exclusions introduction is currently gated only on callback presence.
Detail has a generic Back button, a Built-in badge, repeated timing copy and
expanded technical ownership/context text; its sticky background mismatches
the desktop dialog.

Sequential work:
1. Add a failing pure regression for owned selected packages with unknown or
   legacy acceptance; preserve foreign-owner and no-fabricated-delta coverage.
   Extend desktop/phone browser coverage for saved opt-in with missing
   acceptance, package identity/navigation, and no empty exclusion help.
2. Keep next-start selections visible with a neutral current-agent Unknown
   explanation. Retain the valid accepted comparison and actual evidence path.
   Recompose package detail around a labelled header, compact choice card,
   concise authority/timing, and collapsed technical metadata. Keep one draft
   and shared dirty/save controls. Show exclusion help only when inherited
   inventory or saved exclusion intent exists; preserve dormant restore.
3. Run focused units, typecheck, changed lint, i18n and the desktop/phone package
   journeys plus inherited/dormant exclusion journeys. Use single workers and
   retries disabled. Capture real desktop and phone detail/Connections views
   through file previews. Update public behavior documentation and results.

UI-02 desktop: compact Back to packages > Built-in package / Configuration >
purpose > task-choice card with effective value and shared mutation warning >
one next-start explanation > Tools and Package details disclosures. The sticky
navigation matches the surrounding surface. The panel owns its save bar.
Phone uses the existing full-height explorer and the same hierarchy, one content
scroller, 44px active targets, reachable Back/Close/Save, and no nested drawer.
Nearest exemplar: the existing explorer detail drill-down and settings change-
aware save surface. Logic, choice state, error handling and permissions are
shared across viewports. Verify 320/393/767px plus the 768px boundary and touch
900px using the existing translated-detail case; preserve focus and drafts.

Connections with known acceptance retains next-start additions/removals. With
unknown acceptance it shows saved selected packages, explicitly says the current
agent set is unknown, and makes no claim that an agent connected or changed.

Repair completed. The pure regressions first failed in unknown, legacy and
foreign-acceptance cases; both desktop regressions failed on the expected empty
exclusion help and missing saved Configuration row. After repair, 76 focused
frontend tests pass across eight files. All nine desktop package journeys and
nine phone journeys pass. The existing inherited/dormant exclude/restore journey
also passes on desktop and phone, preserving saved restore actions. Dark detail
captures passed separately; phone capture uses 393px after 320px control checks.
The translated-detail case covers 320/393/767/768/900px and dark/light. All runs
use one worker and retries disabled. An old timing-copy assertion was updated to
the intentional shorter wording before the final desktop pass.

Full typecheck, changed-file ESLint, seven-language/pseudo validation, public-doc
validation/tests, document catalog and spec lint pass. Shared exclusion help was
extracted into its existing component owner to stay within complexity limits.
No backend, injection, startup, permission or stored-preference behavior changed.
Current screenshots live under ignored `.pr-screenshots/builtin-mcp-4762/` and
are delivered through task FilePreview. Exact logs use the
`/tmp/kandev-4762-package-ux-` prefix. See implementation-results for commands.

## Results

### User-reported save feedback correction (2026-11-02)

Reopened under the existing approved UI-02 scope and AC-003.3, AC-004.1 through
.5. The isolated task accepted the Configuration preference, but Connections
rendered only the retained process's original attachment rows. The package list
also omitted effective choices, and the task editor used an always-visible
footer without the shared dirty-field and dirty-group treatment.

Sequential repair: first prove missing pending-addition feedback and the clean
editor's visible Save action with desktop browser regressions. Then show separate
next-start additions/removals beside actual connections, retain actual-only
health/tool evidence, show effective choices in package rows, and reuse the
settings dirty-field/group styling and compact change-aware save surface inside
the explorer. Phone retains one scrolling body and reachable fixed save actions.
Reset in that save surface discards the local edit; reset-to-default remains a
separate draft operation. Do not restart agents or alter the user's seeded task.

Targeted checks: the existing desktop and phone management specs, including
`shows a saved package addition` and `shows change-aware package controls`;
built-in view-model, management-hook and session-hook Vitest tests; web typecheck,
changed-file lint, i18n check, and public/spec validation. These repairs use the
existing reviewed design package and do not change package authority or timing.

UI-02 correction: Connections shows a labelled next-start change list, followed
by actual current-process servers. Selecting a pending package opens its existing
Customize detail. Customize shows effective On/Off in each row, green highlights
only on changed controls/groups, and a compact Unsaved changes / Reset / Save
surface while dirty. The phone uses the same data and actions in its full-height
drawer, with the save surface outside the content scroller.

The earlier completed results below remain historical evidence.

Repair results: both desktop regressions first failed on the expected missing
pending row and always-visible Save control. The inheritance hook test first
failed because implicit Inherit remained dirty. After correction and rebase,
69 focused frontend tests pass; nine desktop browser cases pass (seven management
journeys plus queue/history conflict fixtures), and seven phone cases pass,
including 320px save controls and a dirty draft preserved across 767px and back.
All browser checks use one worker and no retries. Typecheck, changed-file ESLint,
i18n, public-doc validators, and specification checks pass. Mock-agent timing
checks pass 20 race repetitions and Go lint reports zero issues.

The rebase preserves main's longer ungated follow-up observation window and its
content-settlement/snapshot fixture, plus this branch's explicit follow-up release
gate and transactional pause/claim fencing. The ungated fake-clock test now waits
for the final output, so its goroutine drains. The isolated preview receives only
new web assets; task 1's preference revision and running agent remain unchanged.

Completed. Workspace policy, task customization and
initial creation choices, package evidence, Walkthrough guards, localization, and
public docs are implemented. Tests use domain hooks and pure view models plus
focused browser journeys; React component unit tests were replaced with rendered
browser checks under the current repository testing convention. Actual commands
and results are in [implementation-results.md](implementation-results.md).

Final results: 102 focused frontend tests pass, with full typecheck, zero-warning
changed-file ESLint, seven-language/i18n checks, documentation validation, and
focused browser journeys. Desktop Configuration chat catalog and task creation
consent pass real package calls. Phone creation, opt-in/pruning, workspace policy,
and Portuguese dark/light layout at 320/393/767/768/900 px pass. Existing desktop
and phone MCP explorer tests pass. Six fresh desktop/phone screenshots cover
workspace policy, task choices, and initial consent. Disposable capture specs were
removed after the successful managed run; assets remain ignored.


## CI and review remediation (2026-11-02)

Rebased onto main `f648e79f9ecc7d5db0db301f20d0b23c7cb44d97`. The Claude ACP
import conflict keeps main's `streams` subagent support and package-aware native
permission identity. Backup `backup/builtin-mcp-before-fixup-20261102-79e882f`
preserves the previously published head.

Fixed the E2E Build failure in run 41438927330 (head 79e882f454d6): a duplicate
`waitForCascadeArchiveCleanup` import prevented native test-catalog discovery.
The exact Playwright listing reproduced that failure, then discovered 4,622
cases after removing the duplicate. No runtime behavior changed for this fix.

Review 4422789201: Back now declares `type="button"` in package detail. Both
real task forms verify that Back returns to the list, preserves focus and leaves
the submit count at zero. The existing synchronous Back path also passed this
browser check before the explicit type was added; this is a semantic HTML guard.

Review 4422789233: reopening the explorer refreshes clean built-in choices and
both saved revisions. Unsaved drafts remain intact. A deferred hook test failed
before this fix and now proves current revisions reach the next save. Desktop
and phone journeys also change saved preferences through the API, reopen without
a page reload, and retain an unsaved choice across a later close/reopen.

Checks passed:

```bash
GOMAXPROCS=2 GOMEMLIMIT=4GiB go -C apps/backend test -trimpath -race -p=1 ./internal/mcp/profile ./internal/mcp/server ./internal/agentctl/server/adapter/transport/acp -count=1 -timeout=5m
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/adapter/transport/acp/dialect_claude.go
pnpm --dir apps/web exec vitest run hooks/domains/workspace/use-builtin-mcp-management.test.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec playwright test --config e2e/playwright.config.ts --project=chromium --project=mobile-chrome --project=containers --list --reporter=list
pnpm --dir apps/web e2e:run --host --project chromium tests/settings/builtin-mcp-management.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome tests/settings/mobile-builtin-mcp-management.spec.ts -- --retries=0
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
node scripts/validate-public-docs.mjs
git diff --check
```

The hook suite passes 10 tests; browser checks pass 5 desktop and 5 phone cases,
sequentially with one worker and retries disabled. E2E host invocations clear
`KANDEV_INTERNAL_CONFIG_FILE`, `KANDEV_INTERNAL_CONFIG_HOME_FILE`, and
`KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG`, and set
`LD_LIBRARY_PATH=/home/zeval/.cache/kandev-builtin-browser-libs/lib` for Nix-host
Chromium. Builds use `GOMAXPROCS=2`, `GOMEMLIMIT=4GiB`, `GOFLAGS=-p=2`, and
`NODE_OPTIONS=--max-old-space-size=4096`. The first two browser attempts exposed
host configuration/library setup issues, which were resolved before behavioral
verification. No protected runtime was used.

Changed TypeScript files also pass Prettier and zero-warning ESLint. Go lint
2.9.0 reports zero issues. Logs are `/tmp/kandev-4762-fixup-{desktop,phone,go-race,
go-lint,typecheck,eslint,format-check}.log`, `/tmp/kandev-4762-reopen-{red,green}.log`,
and `/tmp/kandev-4762-catalog-{red,green}.log`. This remediation changes form
semantics and refresh logic; visual composition and existing screenshot scenes
remain unchanged. The design and public how-to describe the refresh rule.


### Ambient-environment CI cleanup follow-up

Run 41456338052 at head 93f82770208e exposed
`TestGitOperatorCreatePRPublishesEmptyRemoteBaseBeforeProviderRequest`: TempDir
cleanup found a nonempty `.git` directory. The fixture created a WorkspaceTracker
but did not register Stop. Git operations trigger asynchronous status enrichment;
Stop cancels and joins those observations before TempDir removes the repository.
The shared empty-remote fixture now registers `t.Cleanup(tracker.Stop)` immediately
after construction. This is a test-only ownership fix, with no production change.

The exact test passed 20 local race-enabled attempts before the fix, so the local
run does not claim a deterministic reproduction of the CI scheduling race. After
the cleanup fix, the affected empty-remote cohort passes 20 race-enabled attempts
under the same poisoned GitLab environment. Go lint reports zero issues.

```bash
env -u KANDEV_INTERNAL_CONFIG_FILE -u KANDEV_INTERNAL_CONFIG_HOME_FILE -u KANDEV_INTERNAL_AGENTCTL_STARTUP_CONFIG KANDEV_GITLAB_HOST=https://gitlab.ambient-ci.invalid GITLAB_HOST=https://gitlab.ambient-ci.invalid GITLAB_TOKEN=glpat-ambient-ci-not-a-real-token GOMAXPROCS=2 GOMEMLIMIT=4GiB go -C apps/backend test -trimpath -race -p=1 ./internal/agentctl/server/process -run 'EmptyRemote|PublishesEmptyRemote' -count=20 -timeout=5m
bash scripts/lint-go-changed apps/backend/internal/agentctl/server/process/git_empty_remote_test.go
```

CI log: `/tmp/kandev-4762-ambient-job.txt`; local results:
`/tmp/kandev-4762-empty-base-{red,green,lint}.log`. The file named red contains
the original passing local repeat run; the failure evidence is the CI log.


### Queue follow-up CI fixture follow-up

The same head also failed E2E shard 22/28 in run 41456337865:
`queue-after-background-followup.spec.ts` missed its short busy window. The mock
completed the follow-up on a fixed timer while the browser was still checking or
submitting queue state. CI reported either an empty durable queue after submit
or an already-direct composer. The exact focused local run passed, so this is a
scheduling-sensitive fixture defect, not evidence that the CI failure disappeared.

The existing file-release helper now optionally holds this mock follow-up using
`KANDEV_E2E_ASYNC_FOLLOWUP_RELEASE_FILE`. The test owns the marker and releases it
only after the durable queue and chip are observed. Cleanup releases the marker
and restores backend environment even after failure. The spec now has zero
retries. No production agent, orchestration, or queue behavior changed.

A fake-clock regression failed before the gate because the follow-up finished
without release. It now passes, including the ungated default. The exact browser
case passes five sequential repetitions with retries disabled after fresh builds.

```bash
GOMAXPROCS=2 GOMEMLIMIT=4GiB go -C apps/backend test -trimpath -race -p=1 ./cmd/mock-agent -run 'AsyncFollowup|AwaitFile' -count=20 -timeout=2m
pnpm --dir apps/web e2e:run --host --project chromium tests/chat/queue-after-background-followup.spec.ts -- --retries=0 --repeat-each=5
bash scripts/lint-go-changed apps/backend/cmd/mock-agent/async_followup.go apps/backend/cmd/mock-agent/async_followup_test.go
pnpm --dir apps/web exec eslint --max-warnings 0 e2e/tests/chat/queue-after-background-followup.spec.ts
```

The browser command uses the same clean host environment, Nix library path, and
resource bounds recorded above. Logs: `/tmp/kandev-4762-followup-gate-{red,green}.log`,
`/tmp/kandev-4762-queue-followup-{red,green}.log`, and
`/tmp/kandev-4762-followup-{lint,eslint}.log`. The mock-only rebuild was rejected
by the freshness guard; the successful run rebuilt all required artifacts through
the managed runner.

The first complete remote run finished with 83 passed, 16 skipped, 1 neutral,
and 4 failed checks. These represent the two leaf failures above plus their
backend/E2E aggregate gates. No other leaf failure or unresolved review thread
was reported. Both fixes are included in the next published checkpoint.


### Additional current-head fixture remediation

Full CI at c3371301851e exposed three more fixture/assertion defects and one
external download failure. The original ambient-environment job now passes.

- E2E shard 6/28, job 125330545523: the historical pending-content test displayed
  ready content while expecting loading. It waited for turn completion but not
  content preservation, and its direct database pause did not fence a current
  export. The test now waits for `content_complete`; the helper atomically
  removes links/marks pending and advances the export claim token before deferring
  export. Ready inline content may still have a queued export state, so the
  precondition correctly uses content completeness. The test retains its real
  backend pending/resume path and passes five retry-free repetitions.
- Backend shard 2/4, job 125328683896: the lifecycle package hit its 15-minute
  limit inside TestSSHExecutorCloseSSHClientFallsBackToTheRealClose. The stack
  shows NewSession racing transport teardown after Close. The test now observes
  client.Wait with a bounded deadlock guard, and joins its waiter in cleanup,
  instead of opening a new channel on the closing connection. The old exact
  test passed 100 local repetitions; the race evidence is the CI stack. The
  corrected close cohort passes 100 race-enabled repetitions.
- E2E shard 11/28, job 125330545723: the direct-cancel fixture used an
  uninterruptible `/sleep` turn and reached the correct recovery UI after the
  cancellation could not settle. The successful-cancel test now uses the
  existing cancelable file hold and waits for a provider acceptance marker.
  Its direct and detached-background assertions remain unchanged; both cases
  pass five repetitions each with retries disabled.
- Backend Static Checks, job 125328683955: the SDK consumer's fresh dependency
  download failed at sum.golang.org with HTTP/2 INTERNAL_ERROR. No repository
  assertion failed. A same-head job rerun was rejected while the parent workflow
  was still active. The next published checkpoint reruns this unchanged check;
  no SDK or dependency source was changed for the transport error.

Commands (using the clean host environment and Nix browser library path above):

```bash
GOMAXPROCS=2 GOMEMLIMIT=4GiB go -C apps/backend test -trimpath -race -p=1 ./internal/agent/runtime/lifecycle -run '^TestSSHExecutorClose' -count=100 -timeout=2m
pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/git/turn-changes-history-pending.spec.ts -- --retries=0 --repeat-each=5
pnpm --dir apps/web e2e:run --host --no-build --project chromium tests/chat/cancel-turn-availability.spec.ts -- --retries=0 --repeat-each=5
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/executor_ssh_lifecycle_test.go
pnpm --dir apps/web exec eslint --max-warnings 0 e2e/helpers/turn-changes-history.ts e2e/tests/git/turn-changes-history-pending.spec.ts e2e/tests/chat/cancel-turn-availability.spec.ts
```

Logs: `/tmp/kandev-4762-ssh-close-{red,green,lint}.log`,
`/tmp/kandev-4762-turn-pending-{red,final-green}.log`,
`/tmp/kandev-4762-cancel-availability-{red,green}.log`. The original focused
browser runs passed; the CI assertions and artifacts provide the failure
observations. The rejected export-state precondition was corrected to the
content-completeness contract before the successful repeated checks. All changes
in this batch are test-only. Public behavior and visual composition are unchanged.


### CI and merge-conflict repair (2026-11-03)

Rebased onto main `8d0186aa77ef`, preserving package behavior and adopting
main's per-prompt async follow-up release marker. Replaced the phone menu's
in-motion measurement with the existing finite-animation wait. Reproduced the
OpenCode stale-card assertion under browser slowdown and synchronized the test
with the exact notice deletion event. Disabled the desktop smoke job's shared
Go-cache restore after its release-shaped build exhausted runner disk space.
No production or public contract changed.

Affected race checks, six desktop cases, six phone cases, 28 workflow contract
checks, ESLint and formatting pass. Exact commands, failure evidence and limits
are in [implementation results](implementation-results.md#ci-delivery-and-main-integration-2026-11-03).
Publication must be followed by fresh-head CI; local checks do not prove the
hosted desktop runner has enough disk space.


### Reload readiness in CI navigation fixture (2026-11-03)

Published-head CI exposed a desktop Files restoration test that checked Git
hydration before its reloaded Dockview layout was ready. The same failure
reproduces with temporary browser slowdown. One existing layout-readiness wait
fixes the sequence without increasing the Git assertion timeout. The full five
case desktop file and the phone journey pass with retries disabled; diagnostic
instrumentation is removed. See the [implementation results](implementation-results.md#navigation-fixture-follow-up-from-published-head-ci-2026-11-03).
No package UI or runtime behavior changes. Fresh-head CI remains required.

### Current-main integration checks (2026-11-04)

All 16 owned patches are preserved on main `e180953eae3e`. Five backend packages
pass race-enabled checks, 65 frontend tests pass, and typecheck, localization,
and Go lint pass. A fresh managed build passes seven desktop and three phone
cases, including Files restoration, executor discovery, package opt-in/pruning,
and main's clarification attachments. See the [current-main receipt](implementation-results.md#current-main-compatibility-and-publication-2026-11-04).
No new package behavior or public documentation change. Publication and fresh
remote CI remain separate delivery gates.
