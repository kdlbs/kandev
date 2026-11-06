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

## Secondary adversarial review (2026-11-03)

The user requested another session to review the PR and follow valid suggestions.
Session `cc25a267-a0c6-432d-aafc-ef6445812b60` reviewed the package architecture,
authority, lifecycle, evidence and UI paths, then expanded coverage to changed
integration hunks and fixtures. It reported three actionable P2 findings:

- Trusted legacy Auggie evidence uses `show_walkthrough`. Admission now accepts
  this exact alias alongside the canonical name while rejecting lookalikes,
  foreign servers and foreign sources.
- Stopped/suspended execution inventory was treated as a live intent target.
  Admission now uses the existing runtime probe, preserves live grants, and
  checks desired packages for a future start. Probe errors block admission and
  project Unknown. Historical acceptance is preserved. Final runtime execution
  and manifest checks remain mandatory; package availability does not promise
  that a missing-inventory session can resume.
- The executor-listing config E2E called Configuration instead of Core and could
  pass on a title emitted before failure. It now opens that call's output and
  requires the seeded executor ID, and calls `kandev/list_executors_kandev`.

RED receipts: `/tmp/kandev-4762-adversarial-alias-red.log`,
`/tmp/kandev-4762-adversarial-resume-red.log`, and
`/tmp/kandev-4762-adversarial-executors-red.log`. The browser failure showed the
actual wrong-package tool-not-found error. Focused unit GREEN and the reviewer's
read-only patch recheck passed before the main rebase.

The branch reconciles main's agent-group termination proof change without
conflicts. Range-diff confirms all twelve existing feature commits retain their
patches. Backups preserve the published branch and complete working checkpoint.
No shared Git metadata repair, main runtime or manual instance changes occur.

This review is not an exhaustive full-file audit of all 298 files. The reviewer
read new small owners and traced changed integration hunks; it did not audit all
legacy owner bodies, translations, docs, installed providers or screenshots, and
ran no tests. Primary-session post-rebase validation and publication follow.

Post-rebase verification passes:

- Race-enabled focused checks in orchestrator, executor, intent, MCP server and
  lifecycle packages. Coverage includes built-in admission/evidence, native tool
  name presentation, cold resume and main's agent-process group checks. After
  extracting retained-binding validation to meet complexity limits, the affected
  orchestrator intent/session-view race tests pass again.
- Rebuilt desktop configuration catalog and executor profile lifecycle: two
  passed in 11.0s. The strengthened executor output assertion sees the real
  `exec-local` result through Core.
- Sequential phone package opt-in/pruning: one passed in 13.8s. Browser runs use
  one worker, retries zero, and the clean isolated environment described above.
- Changed E2E ESLint and Prettier, 55-page public-doc validation and its tests,
  documentation catalog (429 decisions, 1895 specifications), full spec lint and
  whitespace checks pass.

Receipts: `/tmp/kandev-4762-adversarial-race.log`,
`/tmp/kandev-4762-adversarial-orchestrator-final-race.log`,
`/tmp/kandev-4762-adversarial-desktop-final.log`, and
`/tmp/kandev-4762-adversarial-phone-final.log`. Earlier lint identified the
complexity extraction and a test switch cleanup; a later attempt collided with
another workspace's active lint process and was retried without bypassing it.
The manual test instance and its existing preferences are unchanged. No new UI
markup or layout is introduced by these review fixes, so existing UI captures
remain representative. Remote CI is pending publication of this checkpoint.

Final affected Go lint passes with zero issues after the extraction and test
switch correction: `/tmp/kandev-4762-adversarial-go-lint-green.log`. Normal commit
hooks and exact-head publication evidence are preserved in the saved task plan.


### CI delivery and main integration (2026-11-03)

Rebased the owned branch from `8cd31a0896ab` onto main `8d0186aa77ef`.
The backup branch `backup/builtin-mcp-before-ci2-main-20261104` retains the
published `e830f1790c1d` checkpoint. Range comparison preserves all package
changes. The async follow-up conflicts use main's newer per-prompt release
marker, bounded failure/timeout handling, and causal queue-admission test.
This supersedes the backend-wide environment gate in the historical receipts.
Main and sibling workspaces were not changed.

Exact-head E2E run `41692938913`, attempt 1, used synthetic merge
`9dc41444e0f4` (published head `e830f1790c1d` into base `8cd31a0896ab`).
Three leaf failures required repair:

- The phone Dismiss action was measured during the menu's entrance scale.
  The unchanged test failed locally at 43.072px. A diagnostic capture proved
  the menu transform was below 1 while the computed control height was 44px;
  its settled rendered height was 44px. The test now waits for finite menu
  animations before measuring. The existing target-size assertion and real
  dismissal interaction remain.
- OpenCode exhaustion passed in isolation and in its full five-case file.
  Temporary fourfold browser CPU throttling reproduced the CI failure.
  Frame capture and the browser store showed retry 9/10 while the API had
  already recorded exhaustion; the viewer had not received the deletion yet.
  The test now arms a wait for that exact session/notice deletion before the
  final dispatch and awaits delivery before asserting the UI. No assertion
  timeout, retry budget, provider, or runtime behavior changed. The same
  throttled reproduction passes with the causal wait. Temporary throttling,
  frame capture, and diagnostic output are absent from the committed test.
- Desktop E2E Smoke failed during the release-shaped native rebuild with
  `no space left on device`, before any release-shaped smoke assertion.
  Its setup-go restore unpacked a shared 1,161MB compressed cache alongside
  Rust artifacts. This job now disables that restore to retain build space.
  Other job caches and all smoke assertions remain. A fresh hosted run must
  confirm that the runner has sufficient space; local checks cannot prove it.

Final affected checks, all with normal hooks enabled and no test retries:

```sh
GOMAXPROCS=2 GOMEMLIMIT=4GiB go -C apps/backend test -trimpath -race -p=1 ./cmd/mock-agent ./internal/orchestrator ./internal/orchestrator/executor -run 'AsyncFollowup|AwaitFile|BuiltinIntent|BuiltinSessionView' -count=1 -timeout=5m
pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/session/opencode-capacity-retry.spec.ts e2e/tests/chat/queue-after-background-followup.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts -- --retries=0 --repeat-each=3
python3 .github/scripts/e2e-tests-workflow-contract_test.py
pnpm --dir apps/web exec eslint --max-warnings 0 e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts e2e/tests/session/opencode-capacity-retry.spec.ts
pnpm --dir apps/web exec prettier --check e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts e2e/tests/session/opencode-capacity-retry.spec.ts
```

The Go mock-agent and orchestrator cohorts pass under the race detector; the
executor package compiles with no matching tests. Desktop E2E passes six cases;
phone E2E passes six cases (both cases repeated three times). All 28 workflow
contract checks pass. ESLint and formatting pass. Managed artifacts were rebuilt
after the rebase; later runs reused those current artifacts. Browser commands
use the clean launch-variable environment, Nix browser libraries, and resource
bounds recorded above. Only one E2E process ran at a time. Main port 9998 and
the user's manual preview were untouched.

This repair changes test synchronization and CI cache behavior. It adds no
product behavior, public configuration, UI markup, copy, or screenshots.
Public docs, durable requirements, and system design therefore need no change.
Fresh published-head CI remains the delivery gate; historical green receipts
are not proof for the rewritten head.


### Follow-up integration with current main (2026-11-03)

Published checkpoint `622cc1fda88b` cleared the three CI leaf failures:
E2E run `41718286174` passed Desktop E2E Smoke and shards 3/28 and 8/28.
Its complete snapshot had 89 passed, zero failed/pending, 12 skipped, one
neutral, and no unresolved review threads. Main then advanced to
`77e6460916b2`, introducing fresh conflicts. Those counts are historical.
The backup `backup/builtin-mcp-before-main3-20261103` preserves that checkpoint.

The next rebase retains both the built-in preference tables and main's new
`harness_native_session_rebinds` readiness/upgrade contract. It also retains
main's UI observation of all 11 retry ordinals together with the exact-notice
deletion barrier. Main's Send Now fixture build/artifact identity additions
coexist with the desktop job's cache setting. Package selection, registration,
authority, and retained execution binding remain unchanged.

A semantic merge correction was required in the built-in recovery branch.
Main now counts classified recovery candidates through a per-input ledger.
The older package branch bypassed it with a direct progress increment, even
if the recovery-block write failed. The new real-Manager/SQLite/reporter test
injects that exact write failure: before the correction it fails with
`done = 1, want 0`; the persisted-refusal positive control passes.
The branch now defers a confirmed persisted refusal to the ledger's stop-work
settlement. A failed block write stays incomplete. The returned outer recovery
error supplies that confirmation; a nested binding error inside a failed-write
wrapper does not. Dispatch and recovery authorization are unchanged.

The focused lifecycle cohort and queue/task/persistence integration cohorts
pass under the race detector. Workflow contract checks (28), changed-file
ESLint, and formatting also pass. Exact commands:

```sh
GOMAXPROCS=2 GOMEMLIMIT=6GiB go -C apps/backend test -trimpath -race -p=1 ./internal/agent/runtime/lifecycle -run 'Builtin|RecoveryProgress|SettingsSourceRecovery' -count=1 -timeout=5m
GOMAXPROCS=2 GOMEMLIMIT=6GiB go -C apps/backend test -trimpath -race -p=1 ./internal/orchestrator ./internal/orchestrator/messagequeue ./internal/task/handlers ./internal/persistence/requiredstores ./internal/persistence/storeconformance -run 'Builtin|SendNow|QueuedMessage|PlanComment|InitialTaskBrief|Catalog|Upgrade|StoreConformance|OwnerBehavior' -count=1 -timeout=5m
python3 .github/scripts/e2e-tests-workflow-contract_test.py
pnpm --dir apps/web exec eslint --max-warnings 0 e2e/tests/session/opencode-capacity-retry.spec.ts e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts
pnpm --dir apps/web exec prettier --check e2e/tests/session/opencode-capacity-retry.spec.ts e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts
```

After the final production edit, changed-package Go lint reports zero issues.
The managed runner rebuilt all native and browser fixtures against the rebased
source. Eight desktop cases and four phone cases pass with one worker and zero
retries, covering queue follow-up delivery, Send Now not-delivered handling,
OpenCode capacity recovery, Core executor discovery from Configuration chats,
phone package opt-in/pruning, and phone error dismissal. Browser commands use
the same clean launch-variable environment and bounded resources recorded above.
The user's manual test instance and main port 9998 remain untouched.

```sh
bash scripts/lint-go-changed apps/backend/internal/agent/runtime/lifecycle/manager_lifecycle.go apps/backend/internal/agent/runtime/lifecycle/builtin_mcp_recovery_progress_test.go apps/backend/internal/persistence/requiredstores/catalog.go
pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/session/opencode-capacity-retry.spec.ts e2e/tests/chat/queue-after-background-followup.spec.ts e2e/tests/chat/send-now-not-delivered.spec.ts e2e/tests/settings/config-management.spec.ts -- --retries=0 -g 'desktop|delivers a message queued|marks an unrestorable|executor listing'
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/session/mobile-acp-prompt-error-continuity.spec.ts e2e/tests/chat/mobile-send-now-not-delivered.spec.ts e2e/tests/settings/mobile-builtin-mcp-management.spec.ts -- --retries=0 -g 'phone:|not-delivered Send Now|opts in and prunes'
```

The startup correction conforms to the existing platform progress contract;
it adds no package option, permission, recovery action, UI layout or public API.
The public package guide already documents recovery-required behavior and needs
no change. Existing package screenshots still represent the product surfaces.


### Navigation fixture follow-up from published-head CI (2026-11-03)

CI on `724bab647927` completed with 86 passed, 13 skipped, one neutral,
and three failed checks: two leaf jobs plus the E2E summary. The leaf jobs
were shard 5/28 and Kubernetes compatibility v1.34.8. No review threads or
approval-required runs were open. All other checks, including the original
three CI failures, passed.

Shard 5's Files restoration journey entered its Git-state assertion immediately
after reload, before the desktop layout had mounted. The isolated case passed;
temporary eightfold browser CPU throttling around reload reproduced the same
five-second false hydration predicate. A diagnostic capture shortly afterward
showed Git ready, commits loaded, and loading false. The fixture now waits for
the existing positive-width Dockview readiness signal before checking Git state.
The Git timeout, folder retry, held-sibling assertions, and task-return checks
are unchanged. The same slowdown reproduction passes with this one added wait.
Temporary throttling and diagnostic capture are absent from the final source.
The phone path has no Dockview dependency.

After the final helper edit, the complete five-case desktop file and the phone
journey pass with one worker and retries disabled. ESLint and formatting pass.
Current artifacts were built against the published source; the fix changes
only test synchronization and does not require a product rebuild.

```sh
pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/task/task-navigation-responsiveness.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-task-navigation-responsiveness.spec.ts -- --retries=0
pnpm --dir apps/web exec eslint --max-warnings 0 e2e/tests/task/task-navigation-helpers.ts
pnpm --dir apps/web exec prettier --check e2e/tests/task/task-navigation-helpers.ts
```

Kubernetes failed during `kind load docker-image`, before product assertions.
Its ignored stderr prevents a root-cause claim. The local host has no Docker
installation, so a single fresh-runner retry of that exact completed job was
requested against the unchanged head. It passed in attempt 2 (job
`126284732204`) on the same `724bab647927` source, including the agentctl/API
smoke. The original import failure's cause remains unknown because stderr was
discarded. No Kubernetes or product code changed; this does not replace the
separate navigation fixture repair. Fresh CI is still required after publication.
Public docs, package UI screenshots, and durable package contracts are unchanged.

### Current-main compatibility and publication (2026-11-04)

Rebased all 16 owned patches onto main
`e180953eae3e433ed91fd15844ad3fe4571b7b76`. The pre-rebase branch is preserved
in `backup/builtin-mcp-before-main4-20261103`; range-diff reports every patch
unchanged. The resulting code checkpoint is
`cb45990ad848151e58f71f1b4ab67bebbcd70a00`. Main's clarification-answer
attachment schema, caller checks, and idle-suspended prevent-auto-start behavior
are preserved. No new package behavior or permission was introduced in this
integration. The navigation fixture repair remains test-only.

Five affected backend packages pass under the race detector. Both frontend
test files pass (65 tests), full web typecheck passes, all locale checks pass,
and changed-package Go lint reports zero issues. All 11 changed JSON files
parse without duplicate keys. Exact commands:

```sh
GOMAXPROCS=2 GOMEMLIMIT=6GiB go -C apps/backend test -trimpath -race -p=1 ./internal/mcp/profile ./internal/mcp/server ./internal/mcp/handlers ./internal/agent/runtime/lifecycle ./internal/orchestrator -run 'Builtin|Package|Registration|AskUserQuestion|Clarification|AnswerAttachment|RecoveryProgress' -count=1 -timeout=5m
pnpm --dir apps/web exec vitest run hooks/domains/session/use-session-resumption.test.ts hooks/domains/workspace/use-builtin-mcp-management.test.ts
pnpm --dir apps/web run typecheck
pnpm --dir apps/web run i18n:check
bash scripts/lint-go-changed apps/backend/internal/mcp/server/server.go apps/backend/internal/agent/runtime/lifecycle/session.go
```

The managed runner rebuilt the backend, web, plugin fixtures, and Send Now
fixture from this source. Seven desktop cases and three phone cases pass with
one worker and retries disabled. These cover the complete desktop navigation
file, phone Files navigation, executor discovery, phone package selection and
pruning, and clarification attachments on both screen sizes. Commands use the
clean launch-variable environment and resource bounds recorded above.

```sh
pnpm --dir apps/web e2e:run --host --project chromium e2e/tests/task/task-navigation-responsiveness.spec.ts e2e/tests/settings/config-management.spec.ts e2e/tests/chat/question-answer-attachments.spec.ts -- --retries=0 -g 'navigation branch setup|navigation task setup|Files stays usable|mounted task panels|overview keeps|executor listing|a choice-only answer accepts'
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-task-navigation-responsiveness.spec.ts e2e/tests/settings/mobile-builtin-mcp-management.spec.ts e2e/tests/chat/mobile-question-answer-attachments.spec.ts -- --retries=0 -g 'phone Files remains|opts in and prunes|a choice-only answer uses'
```

Logs: `/tmp/kandev-4762-main4-{backend,web-unit,typecheck,i18n,go-lint,desktop,phone}.log`.
The user's preview and main runtime remain untouched. Kubernetes has no local
Docker host; its successful prior same-source retry is recorded above. Fresh
published-head CI, review-thread state, and mergeability remain delivery gates
tracked in the saved task plan, rather than claims made by this local receipt.


### New-task MCP placement correction (2026-11-04)

The user reported that package choices appeared above Advanced settings and the
package title appeared twice. The form rendered its own package disclosure,
while the shared package list rendered the same heading inside it. The new
regression fails before the correction because that top-level section is visible.

PR #4762 had already merged as `a1013e89fc460b39b17003e5022d756bf480e3d2`.
The correction starts from verified main `5d6adcbca18231a5aa3726ca9ee642bfa255cd4a`
on `fix/task-create-mcp-package-placement-6d1b`. The original merged branch stays
unchanged. The initial task-owned changes are preserved in a backup patch and
stash before transfer to this branch. All affected creation sources match the
merged checkpoint before this repair.

Packages now appear inside Advanced settings > MCP servers, beside existing
server choices in the desktop popover and phone drawer. The extra disclosure is
removed; the shared list owns one heading. Package consent remains in the form
across picker and Advanced close/reopen. Clearing consent moves from the picker
mount effect to the existing new-task/workspace reset boundary. Two hook tests
fail before that reset repair, including provisional locked-workflow hydration.
Back remains a non-submit action and returns focus to the package row.

Phone visual verification also exposed fixed-height responsive button classes
on package rows with wrapped descriptions. A rendered containment regression
fails before correction: the label's top inset is below the required eight
pixels. Package rows now override the phone/coarse fixed height with automatic
height, preserving their existing minimum size and padding. The test checks
label bounds inside every row. It waits for the existing finite drawer animation
before measuring; an earlier measurement during entrance was test setup timing,
not evidence of a settled viewport failure. No arbitrary delay or timeout change.

After the final production edit, all 22 focused component tests pass, full web
typecheck passes through the current native compiler, changed-file Oxlint has
zero warnings, and formatting passes. Three desktop and three phone cases pass
with one worker and zero retries. These verify consent persistence and actual
creation, non-submit Back, one title, row/viewport containment, and unchanged
additional-server selection through the existing picker. Native/backend fixtures
were rebuilt from the current base, then the web build was refreshed after each
UI edit. No main runtime or user preview was accessed.

```sh
pnpm --dir apps/web exec vitest run components/task-create-dialog-reset-effects.test.ts components/task-create-dialog-form-reset.test.ts components/task-create-dialog-advanced-settings.test.tsx components/task-create-dialog-composition.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec oxlint -c oxlint.config.mjs --deny-warnings components/mcp/builtin-package-list.tsx components/task-create-dialog.tsx components/task-create-dialog-advanced-settings.tsx components/task-create-dialog-mcp-select.tsx components/task-create-builtin-packages.tsx components/task-create-dialog-form-reset.ts components/task-create-dialog-reset-effects.ts components/task-create-dialog-state.ts components/task-create-dialog-form-reset.test.ts components/task-create-dialog-reset-effects.test.ts e2e/helpers/builtin-mcp-management.ts e2e/tests/settings/builtin-mcp-management.spec.ts e2e/tests/settings/mobile-builtin-mcp-management.spec.ts
pnpm --dir apps/web run build:e2e
pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/settings/builtin-mcp-management.spec.ts e2e/tests/task/task-mcp-selection.spec.ts -- --retries=0 -g 'at task creation|Back from package|matches neighboring Advanced'
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-builtin-mcp-management.spec.ts e2e/tests/task/mobile-task-mcp-selection.spec.ts -- --retries=0 -g 'at task creation|Back from package|keeps task additions inside'
```

Browser commands use the clean launch environment and resource bounds from the
previous receipts, with `CAPTURE_PR_ASSETS=1` for screenshot output. The final
fresh desktop/phone captures are visually inspected and compressed. The capture
manifest selects exactly those two assets. Public how-to instructions now give
the Advanced settings > MCP servers path. The owning requirement/design and
work order record this placement and draft ownership; no package default,
permission, payload or runtime apply-point change. All existing translated
labels are reused. Public-doc validator and its tests, catalog validation,
specification lint, and diff whitespace checks pass.

Logs: `/tmp/kandev-creation-fix-{unit-receipt,typecheck-verified,complete-lint,desktop-verified,phone-verified}.log`.
Failing evidence: `/tmp/kandev-4762-creation-{ui,reset}-red.log` and
`/tmp/kandev-creation-fix-row-red.log`. Publication/hosted CI is a separate gate
recorded in the saved task plan. No new tasks, sessions or workers were created.

### Placement follow-up PR review repair (2026-11-04)

Both review findings are valid. The older integration-base and feature-branch
receipts are now labelled historical. The placement correction at the top of
the plan remains the active branch/base record.

The MCP trigger previously waited for additional-server catalog/selection reads,
which made the independent built-in package editor unreachable during a slow
request. Only additional-server content now waits for those reads. Explicit
form disablement still applies to the whole trigger. The existing no-built-in
loading behavior is unchanged. The search draft remains in the picker owner,
so closing and reopening either native surface preserves the query.

The new deferred-response regression fails before the fix because the MCP
trigger is disabled. Desktop and a 320-pixel phone choose Configuration On
while the real catalog response is held. Releasing and draining that response
clears its loading status without discarding consent. The existing additional
server regression also caught a query reset during the initial helper
extraction; keeping query ownership in the original picker repairs it.

Final source verification: 22 focused unit tests, web typecheck, changed-file
Oxlint, formatting, and whitespace checks pass. Four desktop and four phone
cases pass with one worker and zero retries. These include the new pending-read
case, actual Configuration creation, Back without submission, and unchanged
additional-server selection/search. The first phone invocation passed all three
package cases but its additional-server fixture exited during backend restart,
before assertions. The same complete four-case scope passed with backend
diagnostics enabled; no production or test change was made for that startup
failure. The original failure is retained, not counted as a passing run.

```sh
pnpm --dir apps/web exec vitest run components/task-create-dialog-reset-effects.test.ts components/task-create-dialog-form-reset.test.ts components/task-create-dialog-advanced-settings.test.tsx components/task-create-dialog-composition.test.tsx
pnpm --dir apps/web run typecheck
pnpm --dir apps/web exec oxlint -c oxlint.config.mjs --deny-warnings components/task-create-dialog-mcp-select.tsx e2e/helpers/builtin-mcp-management.ts e2e/tests/settings/builtin-mcp-management.spec.ts e2e/tests/settings/mobile-builtin-mcp-management.spec.ts
pnpm --dir apps/web run build:e2e
pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/settings/builtin-mcp-management.spec.ts e2e/tests/task/task-mcp-selection.spec.ts -- --retries=0 -g 'keeps built-in choices available|at task creation|Back from package|matches neighboring Advanced'
E2E_DEBUG=1 pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/settings/mobile-builtin-mcp-management.spec.ts e2e/tests/task/mobile-task-mcp-selection.spec.ts -- --retries=0 -g 'keeps built-in choices available|at task creation|Back from package|keeps task additions inside'
```

Use the clean launch environment and resource bounds recorded above. Logs:
`/tmp/kandev-5047-loading-{red,desktop-final,phone-final,phone-diagnostic,unit,typecheck-final,lint-verified,format-check}.log`.
Public instructions retain the same entry path and package semantics, so no
additional public copy or translation is needed. Normal hooks and exact-head
CI/review/merge evidence remain separate delivery gates in the saved task plan.

### Hosted CI sequencing repair (2026-11-04)

At published head d19cbd2a6, E2E run 41845382225 attempt 1 finished with
Shard 26/28 and its dependent aggregate failed. All other checks were green.
The shard exposed two test sequencing errors outside the package editor. Both
remain part of delivery verification; their location does not waive the gate.

The mobile workflow test changed a session to mock-smart before waiting for its
initial mock-slow runtime. The set-model handler persists the chosen model
before acknowledging the request, so asserting the old model afterward races
the accepted write and launch completion. The test now waits for the workflow
move and initial idle runtime before changing the model. Its later assertions
still require reuse of that exact session with mock-smart.

The mobile navigation test polled the restored Git projection before waiting
for its commits read. The test now arms the existing WebSocket response watcher
before reload and awaits a correlated, ready commits response for the target
session before checking the projection. A ready:false envelope with an empty
array cannot satisfy that wait. The existing UI timeout and subsequent file
visibility, retry, cache and navigation assertions stay unchanged. The same
helper serves desktop, so both callers are verified. No production behavior,
public copy, fixture credentials, retry count or UI timeout changes.

The two exact failing titles passed an initial local reproduction; that did
not invalidate their CI failures. After repair, each phone journey passed three
repetitions with zero retries. After adding the explicit not-ready envelope
guard, phone navigation passed three more repetitions and all five desktop
navigation tests passed. Changed-file Oxlint, formatting and whitespace checks
pass. No broader local suite or concurrent full E2E run was started. Fresh
exact-head CI is still required after publication.

```sh
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-task-navigation-responsiveness.spec.ts e2e/tests/task/mobile-create-task-workflow-agent-overrides.spec.ts -- --retries=0 --repeat-each=3 -g 'phone Files remains usable|keeps touch controls usable'
pnpm --dir apps/web e2e:run --host --no-build --project chromium e2e/tests/task/task-navigation-responsiveness.spec.ts -- --retries=0
pnpm --dir apps/web e2e:run --host --no-build --project mobile-chrome e2e/tests/task/mobile-task-navigation-responsiveness.spec.ts -- --retries=0 --repeat-each=3
pnpm --dir apps/web exec oxlint -c oxlint.config.mjs --deny-warnings e2e/tests/task/task-navigation-helpers.ts e2e/tests/task/mobile-create-task-workflow-agent-overrides.spec.ts
```

Commands use the same clean launch environment and resource limits as above,
with E2E_DEBUG=1 for diagnostic backend logs. Logs:
`/tmp/kandev-5047-ci-sequencing-{phone,desktop-final,navigation-phone-final,lint-final,format-final}.log`.
Exact failed-job raw/plain logs, error snapshots, workflow trace and the shard
manifest are retained under `/tmp/kandev-5047-d19-*`. Existing final-head UI
captures still represent the unchanged rendering; this repair changes tests
and delivery records only.
