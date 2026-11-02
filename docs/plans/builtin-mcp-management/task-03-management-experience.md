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

## Results

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
