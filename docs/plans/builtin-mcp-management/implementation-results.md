# Built-in MCP implementation results

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
