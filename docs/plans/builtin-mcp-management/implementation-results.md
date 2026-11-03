# Built-in MCP implementation results

## User-reported package save feedback

Fixed two UI-02 omissions found during isolated manual testing: saved future
package additions now appear separately from current agent connections, and
task choices use settings dirty-field/group styling with the shared compact
Reset/Save surface. Effective package values are visible in the package list.
Returning to implicit inheritance clears dirty state. Saving keeps the current
agent's accepted manifest intact.

Rebased onto main `c060076d6723a611ebc260371aa80b555442e589`, preserving both
upstream fixture improvements and this branch's synchronization guards. The
normal and gated follow-up paths pass 20 race repetitions; Go lint is clean.
Post-rebase validation: 69 focused frontend tests, nine desktop browser journeys,
seven phone journeys, full typecheck, changed-file ESLint, i18n, public-doc and
spec validation. Browser retries are disabled and suites run sequentially.
The preview's web assets were updated without restarting its backend or changing
the user's saved task preferences. Exact-head remote CI remains pending the
checkpoint push. Earlier CI receipts below describe their recorded heads.

Status: all three work orders are complete, 2026-10-25. This record supersedes
only the completion claims in the historical partial publication checkpoint.

## Delivered behavior

- Five separately registered native packages, with Core required and explicit
  human Configuration consent for regular tasks. Fixed configuration-chat and
  restricted-surface contracts remain distinct.
- Versioned workspace policy and task choices, two-revision CAS, atomic initial
  creation consent, next-process-start binding, reserved identities, and dispatch
  guards. Existing retained processes keep their accepted packages.
- Prompt discovery follows the binding. Accepted manifests, desired selection,
  connection status, and observed catalogs remain separate. Foreign and retired
  execution evidence cannot replace the current owner's evidence.
- Typed Walkthrough intent survives queues, merges, retries and handoffs, and is
  checked again at dispatch. UI requests stop after owner/authentication changes.
- Workspace Built-in detail, task Customize and New Task choices, phone controls,
  localized copy, and a public package guide.

## Completed checks

- Twenty focused backend runtime packages passed, plus normal application wiring,
  atomic creation integration and final resolver refactor checks.
- Affected Go lint against `cc687bdce96bc22fb8a9ca87abe571b8309d3125`: zero issues.
- Eight frontend files: 102 tests passed, including ownership changes, CAS draft
  recovery, collision identity, creation payloads, and acceptance reconciliation.
- Full frontend typecheck and changed-file ESLint with zero warnings passed.
- Seven-language keys, placeholders, pseudo catalog, copy rules and i18n ratchet
  passed. Unrelated generator changes to existing pseudo and Traditional Chinese
  task/startup entries were excluded.
- Public documentation validator: 55 pages. Documentation catalog and all
  specification structure/size checks passed.

- Five focused backend race packages passed (streams, policy, orchestrator,
  queue handlers, and task handlers). Creation regression/error checks passed
  after the missing-primary-session and HTTP error classification fixes.
- Existing MCP explorer: three desktop/tablet and four phone cases passed.
- New desktop journeys: workspace save, task consent/pruning, denied draft,
  initial Configuration consent, and fixed configuration-chat catalog passed.
- Final phone run: workspace save, task consent/pruning, initial consent with a
  real Configuration read, and translated responsive detail passed. Layout checks
  cover 320/393/767/768/900 px, 44 px touch targets, horizontal containment, Back
  focus, one task customization scroller, and light/dark states.
- Six fresh PR screenshots cover desktop/phone workspace policy, task choices,
  and initial consent. The disposable capture tests passed and were removed.

Browser runs were sequential, one worker, retries zero. No full E2E suite or
protected runtime was used. Controlled mock-agent transport exercises real native
catalog and dispatch; it does not certify every installed provider.

## Reproduction and environment

Run the managed focused E2E commands from the work orders with
`KANDEV_SERVER_HOST=127.0.0.1`. This host has an operator config that binds to its
network interface, while the isolated fixture probes localhost. The test process
also needs the Nix Chromium shared-library paths in `LD_LIBRARY_PATH`; the
Playwright-downloaded headless shell cannot resolve them by itself. These are
command-local environment corrections, not repository or main-runtime changes.

The initial browser attempt stopped on stale backend artifacts after a source edit.
A fresh `make -C apps/backend build` resolved that. Later fixture-only attempts
confirmed the bind and Chromium library issues before any UI assertions ran.

A broad handler test attempt timed out at 600 seconds during a large existing
suite. The focused built-in handler tests pass; the broad suite is not reported
as a passing audit. The native subprocess cancellation fixture initially failed
because its fixed `/usr/bin:/bin` PATH cannot find `sleep` on this Nix host. The
follow-up below resolves that fixture limitation.
The earlier SQLite/PostgreSQL store conformance run, including upgrades, passed
with `-race` in 618 seconds; this continuation did not alter its schema.

## Final browser evidence

The last desktop Configuration/catalog and creation checks passed in 29.9 seconds
(`/tmp/kandev-run.e2e.lH9xH44D.log`). The final mobile run passed all five cases,
including its capture case, in 55.5 seconds (`/tmp/kandev-run.e2e.65LcYDX3.log`).
Desktop capture passed separately in 23.8 seconds. Successful assets and their
merged manifest are under ignored `apps/web/.pr-assets/` and preserved in the
PR-scoped `/tmp/kandev-pr4762-captures-h0m6a9tg/` directory.

An initial test used a Core-only tool on the Configuration endpoint; the runtime
correctly rejected it. The final real read uses `list_shared_prompts_kandev`.
The phone test uses the native Add task control and Send button. Failed test-only
selectors and incorrect catalog paths are not treated as product regressions.

Final visual inspection found content beside the sticky Back control during
scrolling. A full-width opaque header fixes this. Fresh desktop and phone capture
checks passed in 21.1 and 21.5 seconds, including header geometry and task
selection. All six screenshots were refreshed after this correction. Final
assets are backed up in `/tmp/kandev-pr4762-final-captures-pohidv5d/`.

The first commit attempt found two newly staged orchestrator imports that
bypassed the public runtime seam. Both now use the existing runtime history
loader. The staged architecture check exposed this gap in the earlier check of
tracked files; all other hooks passed on that attempt.

## Validation follow-up: portable cancellation fixture

The coordinator supplied the focused test-only change from assignment commit
`d44e77e7157a019f2dbb553a12a6dcaad56a97ce`. Its one-line diff was inspected through
the GitHub API and applied by intent: the subprocess fixture now receives the
host PATH instead of `/usr/bin:/bin`. No sibling feature commits or branch base
were imported. Production behavior and cancellation assertions are unchanged.

Before the edit, the targeted cancellation test reproduced the expected failure
(`runner error = <nil>, want context cancellation`). After the edit, the complete
`internal/agent/mcpconfig` package passed with `-race` in 3.029 seconds:

```bash
GOMAXPROCS=2 GOMEMLIMIT=4GiB go -C apps/backend test -trimpath -race -p=1 ./internal/agent/mcpconfig -count=1 -timeout=5m
```

The historical automatic-prompt and Office inventory failures were already
resolved in the completed WO02 implementation. The passing prompt-producer run
covered sysprompt, MCP server, orchestrator executor, orchestrator and task
handlers (`/tmp/kandev-builtin-prompt-producers-green.log`). The earlier partial
publication checkpoint is historical; it does not describe remaining work.

## CI repair: 26 October 2026

The failed checks on `6119a3436d13e156f41e406686f8d9576785dd21`
exposed runtime admission gaps and outdated test fixtures. The branch integrates
verified main `6a1a2ae10003c6bccc754ab144c6035ac38158d1`; the merge preserves
both built-in package controls and the new catalog Usage presentation. A backup
ref preserves the previous task head. An unrelated corrupt shared Git ref
prevented fetch connectivity verification; the main tip and its complete object
closure were verified separately, without changing that ref.

Runtime repairs cover these boundaries:

- The message-route wrapper forwards built-in intent and prompt preparation.
- Prepared-workspace starts bind packages before process admission, including
  the fast path that does not call lifecycle Launch again. Failed preparation
  prevents the start.
- Fresh workspace helpers discard the previous process binding. Retained helpers
  verify their original binding and do not overwrite it with desired policy.
- Helper creation preserves whether an agent command was explicit or inherited.
  An inherited default does not block initial binding. Explicit configuration,
  prior managed process use, and prior passthrough generations still reject it.

Each runtime defect has a failing regression before its production correction.
The explorer tests now supply the production state provider. Optional-tool mock
calls use their actual package server names. Automation fixtures record their
owner before launch; workflow controls are scoped to the active chat. The shared
sidebar fixture waits for durable archive cleanup before unarchive.

The first browser reruns were diagnostic: 18 passed and 4 failed, then 26 passed
and 3 failed. They identified the additional prepared-workspace admission path
and the archive fixture race. They are not final verification receipts.

The final affected Go lint run reports zero issues. Race-enabled helper
config/instance/process tests and lifecycle/executor/backendapp admission tests
pass. The explorer/management tests pass (22 tests), as do the related state and
selection tests (14 tests). Full web typecheck, affected ESLint, localization,
architecture, documentation catalog, and specification checks pass.

The rebuilt desktop run passes all 15 cases in 2.3 minutes, including restart
with a queued direct message, walkthrough requests and live replacement,
prevent-auto-start, and sidebar archive/unarchive state:

```bash
pnpm --dir apps/web e2e:run --project chromium \
  tests/session/session-resume-turn-start.spec.ts \
  tests/review/walkthrough.spec.ts \
  tests/task/sidebar-shared-state.spec.ts \
  tests/settings/prevent-auto-start-on-open.spec.ts -- --retries=0
```

Local receipts are `/tmp/kandev-4762-helper-race.log`,
`/tmp/kandev-4762-prepared-start-race.log`,
`/tmp/kandev-4762-go-lint-final.log`, and
`/tmp/kandev-4762-desktop-final.log`. Browser runs use command-local host library
paths and isolated backend ports. No full suite runs overlap.

The final phone run passes all 25 cases in 3.6 minutes. It covers Rich Output
image/file/replay views, automation detail and exact-run stop, Walkthrough touch
navigation and requests, messages during restart with Auto-run on/off, and
shared sidebar archive/unarchive state. Receipt:
`/tmp/kandev-4762-mobile-final.log`.

```bash
pnpm --dir apps/web e2e:run --project mobile-chrome \
  tests/review/mobile-walkthrough.spec.ts \
  tests/chat/mobile-rich-output.spec.ts \
  tests/session/mobile-session-resume-turn-start.spec.ts \
  tests/mobile-automation-detail.spec.ts \
  tests/task/mobile-sidebar-shared-state.spec.ts -- --retries=0
```


## Main rebase: 30 October 2026

The user requested a rebase onto main and promotion of PR #4762 from draft.
The verified fetched base is `74a91cfc161916f0d5264b86762adae14e43119e`.
Backup branch `backup/builtin-mcp-before-rebase-20261030` preserves published
head `6cddf6b2586a3e2570626d09c4a550049ed6a73e` before history changes.
The task branch replays only its three feature/test commits onto main; the
26 October merge's CI repairs are preserved in a separate follow-up commit.

Conflict resolutions retain main's registration assignment flow, session
exclusions, native task-creation recovery, attachments, and persistence catalog.
Built-in package choices accompany both normal and no-agent task creation.
The settings component extracts its built-in page state without moving or
removing the registration state. All locale additions from both sides remain.

The nine affected backend package checks pass with the race detector, including
binding admission, helper command provenance, retained bindings, stored policy,
required-store discovery, and task creation. The affected frontend unit tests
pass: 154 tests across seven files. Full web typecheck, localization checks,
documentation catalog validation, and architecture lint pass.

The rebuilt desktop browser run passes 16 cases. The sequential phone run passes
13 cases using the same build. These cover built-in workspace policy, task
selection and creation consent, configuration-chat bindings, denied writes,
translated phone detail, external registration assignment, and inherited-server
exclusion controls. Both managed runs use one worker and isolated backend ports.
Receipts are `/tmp/kandev-4762-e2e-desktop-oct30.log` and
`/tmp/kandev-4762-e2e-phone-oct30.log`.

Go lint first reached its package-loading timeout. The retry exposed four
constructor calls in two newer main-branch tests that still expected one return
value. Those tests now check the error before asserting background-work and
prompt-suggestion propagation. The complete agentctl config package passes with
the race detector, bringing the affected backend verification to ten packages.
This correction changes tests only. Receipt:
`/tmp/kandev-4762-config-rebase-race-oct30.log`.

The final affected Go lint run passes with zero issues. Receipt:
`/tmp/kandev-4762-rebase-go-lint-green-oct30.log`. Specification lint and the
cached diff whitespace check also pass. These are local verification results;
new GitHub checks are not treated as complete before publication.


## Main rebase and discovery parity: 2 November 2026

Rebased the four task commits onto verified main
`0392429c7bec1fc7df41d71220006f22f26d20e0`. Backup
`backup/builtin-mcp-before-rebase-20261102` preserves prior published head
`35a0c6045720125b623558efd4755e7e232cd723`.

The resolutions preserve resource-circuit identity alongside built-in bindings,
workspace collaboration alongside package policy, both required-store additions,
and both built-in intent and the observed turn-error stamp in message replay
identity. All eight locale catalogs retain webhook and package entries.

The default-union regression exposed a pre-existing package ownership gap:
`list_executors_kandev` was available to ordinary tasks in the combined catalog
but assigned to Configuration. It now belongs to Core with shared discovery.
Executor mutation tools remain in Configuration. Existing visibility and launch
authorization checks are unchanged. The regression now compares tool names for
clear diagnostics, and explicitly checks Core discovery and configuration-chat
membership. The design and public package guide reflect the corrected ownership.

Configuration chats receive Core and Configuration through the shared package
resolver and native package endpoints. Configuration-specific registration and
schema/authorization rules remain; this change does not claim their removal.
Retained legacy processes keep the combined server until their next normal start.

Validation passes: focused race-enabled checks in seven backend packages,
including package parity, configuration defaults, executor-discovery integration,
lifecycle metadata, persistence catalog, and message replay. The initial parity
failure and final correction are captured in
`/tmp/kandev-4762-parity-red-nov2.log` and
`/tmp/kandev-4762-parity-green-nov2.log`. The other package results are in
`/tmp/kandev-4762-rebase-tests-nov2.log`. Full web typecheck, localization, locale
formatting, specification and documentation catalog checks pass. The four
conflicted Go packages pass affected lint with zero issues. No browser suite or
GitHub CI waiter runs for this rebase; UI logic is unchanged.


## Package-owned registration cleanup: 2 November 2026

Completed the cleanup requested after the rebase caveat above. Native callers
now select package builders directly. Configuration chats default to Core and
Configuration through the same package path as regular tasks. Removed the old
surface-first group registry, configuration task registration, and temporary
Configuration assembly/copy server. Shared Core definitions are registered once;
caller eligibility and schema limits remain explicit in Core. The fixed external
and automation catalogs reuse these definitions without gaining new tools.

The ownership regression failed on the original mixed builders and now passes.
Full profile/server suites pass with `-race`; focused agentctl package/route tests
and affected Go lint pass. A before/after comparison across all seven legacy caller
types preserves full tool definitions. See work order 01's cleanup results for
exact commands and logs. The temporary catalog capture test was removed before
publication. The public package how-to now explains that Configuration uses the
same tools for both caller types and does not change task permissions.

Existing authorization checks and retained-process combined transport are
intentional compatibility boundaries. This cleanup removes duplicate assembly;
it does not turn package selection into a permission grant or restart live agents.


## CI, conflicts, and review remediation: 2 November 2026

Rebased onto verified main `f648e79f9ecc7d5db0db301f20d0b23c7cb44d97`, preserving
both ACP stream support and package permission normalization. Removed the
reproducible duplicate E2E helper import that broke catalog generation. Back
is explicitly a non-submit action. Reopening Customize refreshes clean saved
choices/revisions while preserving a dirty draft.

Validation passes: full profile/server/ACP race suites; affected Go lint; ten
management-hook tests; full TypeScript checking; changed-file ESLint/Prettier;
4,622-case E2E catalog generation; five desktop and five phone package journeys;
documentation, specification, public-page and whitespace checks. Work order 03
records exact commands, host environment setup, review IDs, and the original
failing CI run. No full local E2E suite or merge is performed.


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


## Package detail and unavailable acceptance repair (2026-11-03)

User task 1 saved Configuration On at revision 5, while its session returned
binding_state legacy and no accepted manifest. The next-start comparison helper
required accepted evidence and returned no rows. It now exposes the owned desired
selection with a neutral Selected label and current-agent Unknown explanation.
Valid accepted differences retain Add/Remove labels, and foreign desired owners
remain hidden. Actual attachment rows, health and observed tools stay unchanged.

Package detail now labels its type, supplies Back to packages, uses a compact
choice/authority card and one timing notice, and collapses tools and metadata.
The list names Built-in packages. Exclusion help appears only with inherited
inventory or saved exclusion intent; dormant/disabled restore remains available.
Translations cover every shipped locale and pseudo. Traditional Chinese settings
were generated; unrelated generator changes to automation catalogs were removed.

RED: `pnpm --dir apps/web exec vitest run components/task/chat/mcp-explorer/builtin-mcp-view-model.test.ts`
failed the three new selection cases. The managed desktop new-case run with
`--no-build --project chromium e2e/tests/settings/builtin-mcp-management.spec.ts
-- --retries=0 -g 'identifies package details|keeps a saved package visible'`
failed both UI regressions before implementation.

GREEN (standard isolated environment, one worker, retries disabled):
- Focused Vitest: eight files, 76 tests, passed. Includes built-in view/choice,
  management/session hooks, explorer, exclusion and shared save coverage.
- Full web typecheck and changed TS/TSX ESLint with zero warnings passed.
- `pnpm --dir apps/web run i18n:check` passed all shipped languages and pseudo.
- Managed chromium `e2e/tests/settings/builtin-mcp-management.spec.ts`: nine
  passed (38.5s) after updating the intentional timing-copy expectation.
- Managed mobile-chrome `e2e/tests/settings/mobile-builtin-mcp-management.spec.ts`:
  nine passed (54.9s), including translated layouts and 320px touch controls.
- Managed chromium inherited/dormant exclusion and detail cases: two passed
  (26.7s). The corresponding mobile cases: two passed (29.8s).
- Final dark desktop detail capture: one passed (6.1s). Phone dark detail is
  captured at 393px after its minimum-width checks and metadata collapse.
- Public-doc test runner and validator, doc catalog and full spec lint passed.

Read-only instance evidence is retained in the isolated root under
`{management,session}-<task-id>-ux-baseline.json`. Original RED and final GREEN
logs use `/tmp/kandev-4762-package-ux-*.log`. Images are ignored files in
`.pr-screenshots/builtin-mcp-4762/` and delivered through Kandev FilePreview.
The existing manual instance receives refreshed SPA assets after publication;
its backend and user choices are preserved. No main runtime or sibling workspace
is changed. No merge is authorized.
