# Message queue E2E consolidation

Scope: `apps/web/e2e/tests/chat/message-queue.spec.ts`. Retain real queue
admission, cancellation, FIFO delivery, edit leases, workflow transitions and
separate mobile interaction coverage. This is a setup consolidation, not a
change to queue behavior, retry policy, timeouts or the mock agent.

## Candidate evidence

| Disposition | Existing case | Keeper and retained regression |
| --- | --- | --- |
| Consolidate | `full queue scrolls internally without hiding the composer` | Run the same `expectFullQueueScrolls` helper before `automatic merge accepts a compatible message into a full queue`. Both use `seedFullQueueTask` with ten rows. Keep scroll overflow, geometry, composer containment and last-row visibility assertions before enabling automatic merge. Then retain the count and combined tail-content assertions. |
| Consolidate | `queue editor textarea scrolls when content is long` | Run its long-content sizing assertions inside `saving the held head after its turn completes resumes Auto-run`, after opening the editor and before filling the final scripted message. Keep the 30-line input, scroll-height oracle, 200px maximum and automatic overflow. Then retain real turn completion, pending queue count, acknowledged save, delivered edited content and empty queue assertions. |
| Retain | Other thirteen cases | They cover distinct entry points or state transitions: Quick Chat keyboard/button admission, task and plan-mode submission, manual merge, automatic merge opt-out, clear-all, responsive row controls, session switching, later-row edit leases, Auto-run reload, targeted Send Now and workflow cancellation. |

Production owners are `queued-ghost-list.tsx`, `queued-ghost-edit-view.tsx`,
their row composition, and the queue/edit-protection hooks. Requirements remain
in `docs/specs/ui/requirements/message-queue-management.md` and
`message-queue-auto-merge.md`; adaptive row presentation remains in
`docs/specs/ui/system-design/message-queue-row-controls.md`.

History: full-queue scrolling entered in `1e1b6c03a6` (#1932), editor sizing in
`b090a0157e` (#1049), and held-head save coverage in `00683ae43f` (#3480).
The scrolling helper is also used by mobile tests and must remain. No helper,
production seam or mobile case becomes obsolete. Existing task-page submission
coverage continues to exercise queuing through the button; API-seeded held-head
coverage does not replace that entry point.

Risks: the scrolling helper leaves the panel open, so close it again before
the original closed-panel admission and chip reopening assertions. Editor sizing must
finish before the final scripted value is entered; the edit lease remains held
through the original turn's completion. Assertions stay in named steps so the
two moved regressions remain visible in reports.

## Verification

Run the complete file before and after the change, using the guarded managed
runner with one worker and retries disabled. Keep all seventeen scenario
identities in fifteen cases. Preserve any baseline failure as separate evidence.
No production assertion is changed or replaced by a mock-provided outcome.

The complete baseline passed 17/17 in 254.992 seconds. The candidate passed
15/15 in 250.417 seconds. Both used the same builds, host and one-worker managed
runner with retries disabled; neither skipped or retried a case. The JSON
reports prove all seventeen original scenario names remain as either retained
case titles or the two named moved steps. All original geometry, content,
delivery and count assertions remain. The edit-label visibility assertion and
the full-queue keeper's closed-panel admission are also retained.

This single pair saves 4.576 seconds (1.79%) across the complete file. It is a
small local sample, not a demonstrated stable percentage or evidence that the
full-suite 50% target is met. Raw reports, the original source copy and the
comparison receipt are under ignored `apps/backend/.build/ci-half/e2e-focus/`.
