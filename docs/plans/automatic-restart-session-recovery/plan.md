---
created: 2026-11-12
status: complete
requirements:
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-001
  - REQ-PLATFORM-DURABLE-RESTART-RECONCILIATION-002
system_design:
  - ../../specs/platform/system-design/session-recovery-convergence.md
  - ../../specs/platform/system-design/durable-restart-reconciliation.md
legacy_specs: []
---

# Implementation plan: Automatic session recovery after restart

## Overview

Restore the same saved conversation automatically after an eligible restart.
The normal composer returns with one informational transcript message.
First repair obsolete recovery ownership, then connect automatic recovery and
publication, then verify desktop and phone behavior.

Platform owns this repair because it owns shared restart and durable-delivery
reconciliation. Agent capability and native identity contracts remain unchanged.
The user confirmed automatic recovery, an ordinary chat input, and a chat notice.
No user decision remains open for the eligible recovery path.

The first startup after this change must recover the tasks already stranded
before upgrade. This includes the diagnosed failed session. The user must not
create a new interruption or send a message to trigger that recovery.
Use the same saved ACP conversation when the provider can resume it and existing
ownership and workspace checks pass. This is an upgrade acceptance gate.

Operational expectation: after upgrade and restart, session
`aef9eae1-2efc-4de5-ad36-9215e5329eb2` must recover through this normal path
when its provider, workspace, and ownership evidence remain available. No SQL
surgery or manual recovery action is required. Live-instance repair remains out of scope.

## Confirmed defect

The affected session contains an earlier failed prompt and a later interrupted
prompt in the same native conversation. The earlier incident left a `settled`
notice and a `prepared` restore attempt with no completion time.

- `repairRestartRecoveryPresentation` returns before reconstructing the later
  incident because the earlier notice is outside the active recovery phases.
  `loadSilentRestoreSession` then rejects that notice as a current candidate.
- Native Resume loads the saved conversation and retires the later journal
  submission. `finishPreparedNativeAttemptTx` rejects the unrelated old attempt.
  Independently, `restoreAuthenticatedNativeNoticeTx` rejects the settled notice.
- SQL leaves the generic `unresolved_durable_work` block open. The browser
  receives errors and keeps the recovery surface instead of the composer.

The repair must correct automatic recovery. Making either manual button appear
successful is insufficient.

### Read-only incident evidence

All times are UTC on 2026-11-12.

- Task `21440e51-8179-4052-acc6-a86d1ab32e98`, session
  `aef9eae1-2efc-4de5-ad36-9215e5329eb2`, native conversation
  `01a196c1-fb77-7470-92cd-8ecfaf88c11f`.
- At 02:38 the earlier prompt failed with a delivery disk-space error. Its SQL
  terminal, journal terminal, and resolved block retain that result. Its silent
  restore attempt remained pending with a `prepared` checkpoint.
- The later prompt started at 10:07. The retained backend log ends its previous
  process at 10:22:03 with an abnormal workspace stream closure. A new backend
  starts at 11:04. These logs do not identify who or what killed the old process.
- At 11:09:18 native load reports idle. A stable read-only journal snapshot
  shows the later submission retired, still `interrupted_unknown`.
  Received, projected, acknowledged, and journal high-water cursors reach 52980.
- Manual Resume requests at 11:09:30 and 11:09:37 reach the backend. Browser
  diagnostics record an error response for each. SQL still holds the generic block.

Sources are the live database opened with `mode=ro`, the session-scoped raw ACP
file, and retained backend files under `/root/.kandev/logs/`. Backend bundle
`379204eccaa4f79d9c8f765a5f67f43e` is partial because of its archive byte limit.
Frontend bundle `61c746a393fade9edffdc889107051da` includes both failed requests.
The frontend manifest reports a bounded history and capacity loss. Kernel logs
are inaccessible. No live recovery action or database write was performed.

### Disposable reproduction

`TestReproHistoricalTerminalRecoveryStrandsLaterResume` used a fresh SQLite
repository with an old proven terminal incident and a later authenticated proof.
It ran with:

```bash
(cd apps/backend && go test -trimpath -tags fts5 ./internal/task/repository/sqlite -run '^TestReproHistoricalTerminalRecoveryStrandsLaterResume$' -count=1 -v)
```

The control passed. The stale-notice, stale-attempt, and combined cases each
failed the expected successful-resolution assertion. In every case the current
snapshot and block matched. Each stale record independently refused resolution.
The temporary test was removed. No production or permanent test changes exist.

## Scope

### In scope

- Guarded retirement of obsolete prepared attempts for proven terminal sources.
- Selection of the current interrupted incident despite older settled metadata.
- Automatic startup recovery without task focus or a recovery-button click.
- Shared disposition for automatic recovery, Resume, and Retry connection.
- One durable, localized chat notice and the ordinary composer after recovery.
- Notice publication for ordinary interrupted restarts and explicit native Resume,
  including recovered-notice restoration without earlier terminal history.
- Existing persisted rows, repeat restart, crash boundaries, desktop, and phone.
- First startup after upgrade with existing restart-interrupted failed tasks.

### Out of scope

- Identifying or changing the process that caused the original interruption.
- Live-instance repair, SQL surgery, deployment, or restarting the user's runtime.
- Repeating the interrupted prompt, tools, or queued work.
- New native conversations, provider/model changes, or removing ownership guards.
- Office, workflow, automation, archive, recipient, or capacity-policy changes.

## Technical approach

### Historical ownership

Extend `native_resume_attempt.go`, `native_resume_binding.go`, and terminal
settlement with the exact historical-source predicate in the convergence design.
Use `SilentRestoreStageTerminal` with the existing `blocked` outcome for obsolete
prepared work, following `completeSupersededContextAttemptTx`. Add a typed
`SilentRestoreOutcomeBlocked` constant and use reason `source_terminal_settled`
when no earlier reason exists. Preserve the source checkpoint and genuine terminal result.
Repeat the checks during recovery to repair existing records without a migration.
SQLite and PostgreSQL use their existing admission locks and compare-and-set rules.

### Automatic convergence

Classify the historical source before phase-based exits in
`restart_recovery_presentation.go` and `silent_restart_recovery_inputs.go`.
Reconstruct the current incident from authenticated inventory and the persisted
prompt binding. Reuse the existing bounded worker and an eligible loaded native
successor. Keep source death, native identity, every unresolved row, and unrelated
admission restrictions as separate gates.

Resume and Retry connection consume the same committed result. Neither operation
can claim success from process readiness while an unresolved incident remains.
Retry connection remains state-only and cannot schedule restoration. Automatic
recovery and explicit Resume retain their existing restoration authority.
Reuse existing recovery publication and persist a deterministic `status` notice.
Use the existing lifecycle-only turn convention. A publication retry uses the
same message identity, including after a successful SQL commit and process loss.
Follow `writeCeilingSurfaceNote` and `ceilingSurfaceMessageID` in
`apps/backend/internal/orchestrator/ceiling_surface.go`. Use the convergence
design's stable source tuple, metadata kind, and canonical English content fallback.
An automatic restore and a later explicit restore of one incident share one ID.

### Compatibility

| Runtime evidence | Behavior | Verification | Unsupported or incomplete shape |
| --- | --- | --- | --- |
| Live retained agent, compatible journal | Reattach and preserve its turn | Existing survival tests | Preserve its existing attachment error |
| Dead source, saved native state, complete authenticated journal/SQL proof | Restore automatically and return composer | New backend and desktop/phone regressions | Keep the specific blocker |
| Idle successor with matching saved native identity | Reuse and finish the same recovery | Loaded-successor integration case | Protect active, foreign, or unproven successor |
| Remote executor | Apply existing authenticated executor proof | Existing supported-executor tests | Do not infer absence from a missing reply |
| Legacy peer, missing native state, actual history gap | Keep its existing supported recovery | Existing negative controls | No automatic downgrade or new conversation |

Shared code does not establish new provider or executor support. No new runtime
flag, public endpoint, table, column, or journal format is required.

### Presentation and mobile contract

Use one shared disposition and transcript renderer. The informational notice
does not own admission or offer Resume and Retry connection. Independent blockers
retain their existing actionable presentation. Preserve drafts across all states.

The phone entry point remains the task's Chat view. The nearest exemplar is
`apps/web/components/task/task-layout.tsx`: one transcript scroller and the
existing bottom composer. Use that composition and safe-area/keyboard behavior.
An inline notice fits this short status update. No temporary choice needs a drawer.
Phone Send retains its existing touch target. Desktop preferences remain unchanged.

## ASCII UI preview

UI-01: Eligible restart, current failure. The screenshot and logs establish this
recovery surface. The normal composer is unavailable.

```text
Transcript
[Connection interrupted]
[Continue from saved history] [Resume] [Retry connection] [Stop]
```

UI-02: Automatically restored conversation, desktop and phone.

```text
Transcript
The session was interrupted. Your conversation is restored. You can continue.

[Type your next instruction...                         ] [Send]
```

The transcript scrolls. The normal composer keeps its existing fixed placement.
On phones the notice wraps within the transcript and the composer clears the
safe area and keyboard. During pending recovery the draft remains editable,
but Send waits for committed readiness. Genuine blockers keep specific guidance.
Structure is required. Spacing is illustrative. The notice uses the canonical
English string above and its translations.
Maps to AC-PLATFORM-DURABLE-RESTART-RECONCILIATION-001.4 and 002.10.

## Tests

- `native_resume_historical_terminal_test.go`:
  `TestHistoricalTerminalRecovery` and its PostgreSQL counterpart cover the
  four reproduced combinations, full terminal proof, mixed live work,
  allocated candidates, changed identities, rollback, and repeat resolution.
  Maps to 001.3, 001.5, 002.6, and 002.9.
- `automatic_restart_historical_terminal_test.go`:
  `TestAutomaticRestartAfterHistoricalTerminalRecovery` covers startup without
  browser participation, absent/generic current blocks, and loaded successors.
  Its upgrade cases seed the old stranded state before the corrected backend starts.
  Maps to 001.1, 001.2, 002.3, 002.9, and 002.11.
- `manual_recovery_historical_terminal_test.go`:
  `TestManualRecoveryAfterHistoricalTerminalRecovery` covers Resume and state-only
  Retry connection reading automatic recovery's committed result. It also proves
  that pending Retry cannot launch or schedule restore. Maps to 002.3 and 002.9.
- `session_recovery_notice_test.go`:
  `TestRecoveryNoticePublicationIdempotent` covers ordinary interrupted restart,
  historical-terminal recovery, explicit Resume including 002.6, same-incident
  deduplication, distinct later incidents, and publication after a commit/restart
  boundary. Maps to 001.4, 001.5, and 002.10. Task 02 owns persistence/publication;
  Task 03 owns the visible notice and composer.
- `session-recovery-notice.test.tsx`, recovery-model tests, and revision-merge
  tests cover localized notice rendering, draft retention, late snapshots,
  ordinary composer selection, and independent blockers. Maps to 001.4 and 002.10.

## E2E tests

New `automatic-restart-recovery.spec.ts` and
`mobile-automatic-restart-recovery.spec.ts` use the existing real restart fixture.
Both include a previous terminal incident followed by a later interrupted turn.
Both also include an ordinary interrupted restart with no earlier terminal incident.
An additional upgrade case starts from an already-failed persisted session.
It must recover on the first corrected startup, without another crash or prompt.
Close every page in the test-owned browser context before restarting the backend.
Assert that `context.pages()` is empty. Keep it empty until the read-only API
probe described in Task 03 proves committed readiness and notice persistence.
Then create a replacement page and navigate. The backend must recover before task focus.
Assert native identity, one
notice, normal input, retained draft, no required recovery buttons, reload,
and successful submission of a distinct new instruction. Count the old prompt's
dispatches independently. Add an unrelated blocked sibling as a negative control.
Use `chromium` and `mobile-chrome` respectively, with fresh production builds.
Maps to 001.1 through 001.5, 002.9, 002.10, and 002.11.

## Work orders

- [x] [Task 01: Separate obsolete terminal recovery ownership](task-01-terminal-history.md)
- [x] [Task 02: Restore eligible sessions automatically](task-02-automatic-recovery.md)
- [x] [Task 03: Show recovered chat and verify both viewports](task-03-restored-chat.md)

All work is sequential. This package does not authorize delegation.
The existing journal-growth, session-convergence, and journal-shutdown packages
remain historical delivery records. Their results do not verify this repair.

## Verification results

Diagnostic reproduction: one passing control and three expected failing cases.
The temporary test was removed after diagnosis.

- `python3 scripts/list-docs.py validate`: passed, 450 decisions and 1995 specifications.
- `python3 scripts/lint-spec-files.test.py`: passed, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: passed.
- `validateCoverage` from `.github/scripts/pr-docs.cjs`: all three work orders
  passed the linked requirement, acceptance, plan, and design checks. The
  preflight included the planned backend path to require coverage validation.
- `git diff --check -- docs/specs docs/plans/automatic-restart-session-recovery`:
  passed. New work orders also passed direct whitespace and local-link checks.

Tasks 01, 02, and 03 are complete. Task 01's SQLite/PostgreSQL matrix, SQL guard,
store-conformance suite, changed-Go lint, and diff check passed. Task 02's
race-enabled orchestrator, lifecycle, and SQLite recovery matrices and changed-Go
lint passed. Task 03's desktop and phone E2E suites each passed all three restart
scenarios. Focused UI tests, typecheck, i18n validation, public documentation
validation, specification catalog validation, complete specification lint, and
diff checks passed. The live instance was not repaired or restarted.

## Risks

- A broad exemption for settled notices can hide a current unresolved incident.
- Completing an allocated attempt can release a live or unproven candidate.
- Journal retirement can precede SQL resolution. A retry must retain exact proof.
- A frontend-only dismissal can reveal an input whose submission still fails.
- Duplicate publication can create repeated notices after restart or reload.
- The original process death remains unattributed. This repair cannot explain it.

## Peer-review follow-up results

- Preserved unrelated and newer frontend failures while clearing only guidance
  tied to the restored submission. Added regressions for stale incident A,
  later session error B, and a newer request-local failure.
- Bound notice publication to the exact authenticated committed recovery block
  and source identity. Completed source labels and source settlement after SQL
  commit no longer strand a pending notice.
- Preserved the historical execution owner in restored recovery metadata.
- Switched startup candidate selection to dialect-aware JSON key and string
  predicates. SQLite and PostgreSQL pending-only cases pass after an ordinary
  metadata update.
- Created a distinct pending marker for a later incident in both same-generation
  and earlier-generation restored-notice paths, while retaining the older
  recovery snapshot.
- Focused frontend tests, web typecheck, localization checks, backend build,
  changed-Go lint, and desktop/phone production E2E passed. The four required
  PostgreSQL recovery suites and pending-marker candidate test passed, along
  with final race-enabled SQLite and orchestrator checks.
