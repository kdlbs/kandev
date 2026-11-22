---
id: "04-ssh-e2e-and-docs"
title: "Prove SSH desktop and phone flows"
status: done
wave: 4
depends_on:
  - "03-recovery-and-lifecycle"
plan: "plan.md"
requirements:
  - REQ-EXECUTORS-SSH-PASSTHROUGH-001
acceptance_criteria:
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.1
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.2
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.3
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.4
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.5
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.6
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.7
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.8
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.9
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.10
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.11
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.12
  - AC-EXECUTORS-SSH-PASSTHROUGH-001.13
system_design:
  - ../../specs/executors/system-design/ssh-passthrough.md
---

# Task 04: Prove SSH desktop and phone flows

## Summary

Prove remote CLI execution and recovery through the existing browser terminal
against a disposable SSH target. Exercise the phone terminal with actual device
options and update public executor documentation after the behavior passes.

## In scope

- Desktop and phone SSH passthrough specs using `ssh-test-base` and remote helpers.
- A bounded opt-in mock TUI trace with PID, cwd, prompt IDs, and an MCP-call
  scenario. It remains test-only and contains no credentials.
- Verify device options on the SSH fixture. Playwright currently supplies them
  to the fixture-created context. Keep Docker-dependent phone
  tests in `containers`, and exclude SSH files from ordinary `mobile-chrome` discovery.
- No prepare script, absent host workspace, remote output and input, remote MCP
  tool call, reload/reconnect, sibling isolation, Stop, explicit Resume, and loss
  of transport without a duplicate CLI.
- Pixel 5 Chat/keybar input with `.tap()`, interrupt, resize, reconnect, and
  document overflow checks. Inspect the resulting phone screenshot.
- Ended passthrough sessions display the existing Chat recovery surface on
  desktop and phone. Hide the phone agent keybar until Resume starts the session.
- Public SSH setup and troubleshooting docs: remote CLI prerequisite, supported
  passthrough behavior, reconnect/resume limits, and authentication failures.

## Out of scope

Live paid-provider tests, a developer's main instance, real Proxmox administration,
Tailscale configuration changes, or a new terminal composition.

## Acceptance

1. Desktop E2E asserts both visible remote terminal results and remote PID/cwd
   evidence. No local marker exists. Reload preserves PID and prompt count.
   A subsequent unique prompt produces a new response on that same process.
2. Phone E2E asserts the actual Pixel 5 viewport and touch mode, enters a prompt,
   uses the existing keybar, and receives a remote response. Reconnect and Resume
   remain reachable. No required action depends on hover or document overflow.
3. Stop/recovery scenarios prove remote tree cleanup, preserved workspace, and
   no duplicate process on unknown liveness. Existing SSH and local terminal
   regressions pass. Docs state only behavior supported by this evidence.

## Terminal interaction preview

Reuse `UI-01` and `UI-02` from the [plan](plan.md#terminal-interaction-preview).

```text
UI-01 Desktop                 UI-02 Phone Chat
+----------------------+      +-------------------------+
| Agent terminal       |      | Task / Session          |
| Remote CLI output    |      | Remote CLI output       |
| > user input         |      | > user input            |
+----------------------+      | [Ctrl] [Esc] [Tab] [...] |
                              +-------------------------+
```

Desktop retains its layout. Phone Chat adds the existing 44-pixel keybar above
task navigation. The shell terminal is the mobile exemplar. Terminal content
owns scroll. Agent input uses its own session-bound sender and cannot fall back
to shell input. Closing or replacing a socket releases only its own sender. Preserve phone keybar viewport/safe-area behavior, manual input
routing, localized names, and touch targets. Map rendered evidence to .3, .4,
and .13. No visual polish changes are required by this work order.

## Verification

Run from the repository root with an available Docker daemon. A skipped SSH
suite is not passing evidence. The managed runner must build fresh artifacts.

```bash
(cd apps && pnpm install --frozen-lockfile)
(cd apps/backend && go test -trimpath -tags fts5 ./cmd/mock-agent -count=1)
(cd apps/web && pnpm exec playwright test --config e2e/playwright.config.ts --project=chromium --project=mobile-chrome --project=containers --list --reporter=json)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/ssh/passthrough.spec.ts tests/ssh/mobile-passthrough.spec.ts tests/ssh/recovery.spec.ts tests/ssh/agent-readiness.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/terminal/terminal-agent.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/terminal/mobile-passthrough-rendering.spec.ts tests/terminal/mobile-terminal-keybar.spec.ts)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Validate the new spec/fixture TypeScript files with the repository's targeted
lint command. Run the public-doc checks and the complete
[documentation preflight block](plan.md#documentation-coverage-preflight):

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

Record the exact invocations and all results. Confirm discovery includes both new files only in
`containers`, with actual phone emulation in the mobile file.

## Files likely touched

- `apps/web/e2e/tests/ssh/passthrough.spec.ts`, `mobile-passthrough.spec.ts`, and
  `passthrough-helpers.ts` (new specs and shared helpers).
- `apps/web/e2e/fixtures/ssh-test-base.ts` for device options and disposable profiles.
- `apps/web/e2e/playwright.config.ts` and `apps/web/e2e/README.md` for the guarded
  mobile SSH discovery exception.
- `apps/web/e2e/helpers/ssh.ts` only for reusable remote evidence helpers.
- `apps/backend/cmd/mock-agent/tui.go`, a focused TUI helper, and tests if needed
  for opt-in remote evidence and MCP invocation.
- `docs/public/executors.md` and relevant existing CLI-mode documentation.
- This plan and work-order results, plus paired spec statuses after delivery.

## Dependencies

Tasks 01-03 and their targeted checks. The managed runner builds the host
backend, remote agentctl helper, and Linux mock-agent fixture.

## Risks

The SSH fixture currently creates a fresh browser context directly. A mobile
project/device setting alone does not guarantee its page inherits those options.
Do not accept terminal visibility as remote execution proof or an old response
as acceptance of a later prompt. Restore shared profiles and terminate owned
targets through fixture cleanup even when assertions fail.

## Parallelism

`sequential`

## Inputs

- Existing SSH launch/recovery fixtures and `SessionPage` terminal methods.
- Existing local mobile passthrough and keybar specs.
- `/e2e`, `/mobile-parity`, `/better-ui`, and `/docs-maintainer` guidance.
- `apps/web/AGENTS.md`, `apps/backend/cmd/mock-agent/AGENTS.md`, and
  `docs/public/README.md`.

## Results

Implementation adds five desktop SSH passthrough tests and one Pixel 5 touch
test. The mock CLI records bounded, opt-in remote PID, workspace, and accepted
prompt evidence. Its MCP probe calls the authenticated task-session tool. No
prepare script or live provider account is required.

The phone test verifies input, Tab, reconnect with the same PID, interrupt,
explicit Stop/Resume, and no document horizontal overflow. Headless Chromium
does not open an OS keyboard; the test simulates the visual viewport change.
The captured phone terminal was inspected: output and the existing keybar fit
inside the viewport. Ended sessions expose the existing recovery controls and
hide the agent keybar. No new UI copy or translations were needed.

Completed checks:

- Four focused web test files: 79 tests passed.
- Web typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck`.
- Targeted ESLint, `pnpm run i18n:ratchet`, and `pnpm run i18n:check` passed.
- Existing phone terminal suites: all 16 tests passed in `mobile-chrome`.
- Existing local desktop terminal suite: the final repeat passed all five tests
  in `chromium` (33.9 seconds) after the shared Resume changes.
- Playwright discovery confirms the six new SSH tests run only in `containers`.
- Mock-agent package tests passed (2.570 seconds).
- Public-doc validator: 48 documents passed; its 63 tests passed.
- Specification catalog, specification lint, and all 36 lint tests passed
  after the final spec status update.

The first browser attempts found and drove fixes for initial-launch admission,
stale-cleanup client-lease deadlock, ACP acknowledgement on terminal Resume,
and lost Resume intent after Stop. Sibling tests now select the requested tab
and ignore hidden terminal panels. Final SSH revalidation passed all 14 tests
in 4.3 minutes, with no skips. This includes all five new desktop cases, the
Pixel 5 touch case, four existing recovery cases, and four readiness cases.

The shared Docker daemon has no usable default bridge. A task-owned network
and a task-local Docker wrapper provide fixture networking without changing
the daemon or other users' containers. The managed runner builds fresh host
and remote artifacts before the suite:

```bash
cd apps/web
PATH=/tmp/kandev-ssh-docker-bin.reml481u:$PATH GOCACHE=/home/zeval/.cache/kandev-ssh-ee8214d6-go KANDEV_E2E_CONTAINERS=1 E2E_DEBUG=1 pnpm e2e:run --project containers tests/ssh/passthrough.spec.ts tests/ssh/mobile-passthrough.spec.ts tests/ssh/recovery.spec.ts tests/ssh/agent-readiness.spec.ts
```

Managed runs accept one project at a time. Repeated `--project` options selected
only the last project, so the local and phone suites were run separately.

The final local repeat used the fresh artifacts from the managed SSH run:

```bash
cd apps/web
pnpm e2e:raw tests/terminal/terminal-agent.spec.ts --project=chromium
```

The passing phone screenshot is retained as task-owned evidence at
`/tmp/kandev-ssh-ee8214d6-evidence/phone-terminal.png`. Test resources were
disposable; no developer instance or user task data was changed.
The task-owned Docker network had no remaining containers after fixture cleanup
and was removed with the temporary wrapper. Shared daemon settings and images
were preserved. Final documentation coverage and `git diff --check` passed.

PR preparation captured and inspected desktop and phone terminal/recovery
surfaces in a disposable managed SSH run (two tests passed). All four assets
are compressed under ignored `.pr-assets`. The architecture hook identified a
new gateway import of lifecycle internals. The handler now uses the public
runtime facade's existing execution alias. Focused gateway race tests passed
(1.233 seconds), and architecture lint passed after the correction.

Evidence uses disposable Linux SSH targets and the mock CLI. The reporter's
Proxmox LXC, Tailscale transport, and authenticated provider CLIs were not
available. The reporter-specific missing-token cause remains unconfirmed.


PR review remediation: the SSH helper now polls the remote initial-prompt trace
before asserting its count. After rebuilding backend, helper, and frontend
artifacts, the focused managed SSH suite passed all 14 tests in 4.2 minutes,
including phone input, passthrough reconnect, remote MCP, Stop/Resume, backend
recovery, and agent readiness:

```bash
PATH=/tmp/kandev-pr-ee8214d6-tools-bumju97b/bin:$PATH GOCACHE=/home/zeval/.cache/kandev-ssh-ee8214d6-go KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --host --no-build --shards 1 --project containers tests/ssh/passthrough.spec.ts tests/ssh/mobile-passthrough.spec.ts tests/ssh/recovery.spec.ts tests/ssh/agent-readiness.spec.ts
```

The preceding broad SSH-folder run passed eight cases before an intentional
interrupt to narrow the selection. It is not counted as a full-suite pass.
Go race checks passed for all six affected packages; Go lint, targeted web lint,
documentation catalog validation, and specification lint also passed.


CI remediation also corrects the phone diff continuity fixture. Its mutable
version marker now stays outside the reading region, so the asserted content
anchor survives the edit. Both Pierre and Monaco cases pass without retries.
The initial toolbar replay passed all 34 tests without retries under two CPUs
and 4 GiB memory. A subsequent one-CPU replay reproduced all four phone toolbar
failures. The backend had moved the task to Review while the canonical browser
record retained its bootstrap placement. Shared task-navigation identity reads
now reconcile that record through the scoped overview journal. The same
one-CPU partition passed all 34 tests in 6.2 minutes after the fix. All 159
focused state and route tests passed, including missed movement, newer live
placement, and workspace-change regressions. The branch-switch case passed in
its original partition.

After rebuilding the native backend, delivery fixture, and plugin fixture, the
recovery partition passed all 33 tests in 4.4 minutes without retries under the
same resource limits. The previously failing read-only Resume completed in
17.2 seconds. Both phone diff renderer cases passed in 42.6 seconds. The full
lifecycle race suite passed in 303.752 seconds. Changed-package Go lint,
targeted frontend lint, formatting, and documentation checks passed.


A later CI run exhausted the shared 60-second deadline in the integration
shortcut test. The same failure reproduced with one CPU and 4 GiB memory.
Each of the six integration journeys now has its own test. All record, save,
reload, navigation, reset, and default-command assertions remain. All six
tests passed without retries in 2.2 minutes under the same resource limits.
Targeted ESLint and formatting checks passed.


The second Greptile review identified global terminal selection and extended UI
waits in the SSH tests. The input helper now requires the caller's terminal
locator. Desktop and phone Resume cases arm a correlated `session.recover` wait
before the action, await its response, and use the terminal assertion's default
timeout. The sibling test still verifies input reaches only the selected remote
PID. Targeted ESLint and formatting passed. The SSH browser suite is a required
delivery check after the remediation commit; results are recorded in PR evidence.
