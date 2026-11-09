---
id: "06-background-work-snapshot-race"
title: "Preserve live workloads during snapshot reads"
status: done
wave: 5
depends_on:
  - "05-durable-plugin-failures"
plan: "plan.md"
requirements:
  - REQ-AGENTS-BACKGROUND-WORK-005
acceptance_criteria:
  - AC-AGENTS-BACKGROUND-WORK-005.1
  - AC-AGENTS-BACKGROUND-WORK-005.4
  - AC-AGENTS-BACKGROUND-WORK-005.8
  - AC-AGENTS-BACKGROUND-WORK-005.9
system_design:
  - ../../specs/agents/system-design/background-work.md
---

# Task 06: Preserve live workloads during snapshot reads

## Scope

Repair the missing running-job chip observed in PR CI. A delayed initial
HTTP snapshot must preserve jobs added or updated live after the request began.
Unchanged rows omitted by a complete snapshot still disappear. Keep existing
revision, capability, terminal-state, and output-window reconciliation.

Use a store-owned read generation for the latest request per session across
consumers. Clear and purge invalidate outstanding reads without affecting
sibling sessions. An obsolete response must neither recreate rows nor settle
a replacement read. A valid read can populate the shared cache after its
initiating consumer unmounts.

The shared state hook supplies both viewports. Existing composer chips,
desktop popovers, phone summaries, and independent inspection entries retain
their composition, labels, touch targets, and scroll owners.

```text
Desktop / phone chat
  Running background job -> existing compact chip
  Stale HTTP response   -> chip remains
  Process ends          -> chip clears; history remains reachable
```

## Validation

1. Use deferred API reads and the real runtime store to prove the loss before
   production changes. Cover live discovery, terminal updates, snapshot
   omissions, overlapping consumers, A-B-A session changes, clear/purge,
   superseded failures, and initiator unmount.
2. Hold a real initial empty list response in Playwright, publish a workload
   through the backend, and observe the real WebSocket update. Check the chip
   before and after HTTP publication settles on desktop and phone.
3. Rerun the failed phone process-end case and its desktop counterpart,
   focused store/hook/consumer tests, type checking, and changed-file lint.
4. Validate the owning design and this package. Track push and fresh CI in the
   external delivery record.

## Results

The real-store regression failed because an accepted running job became an
empty list after the deferred HTTP response. After correction, the focused
eight-file frontend run passed 72 tests. Type checking passed. The controlled
phone browser regression also failed at the final chip assertion on unchanged
CI production code, after the initial live chip had appeared. The original
isolated CI case passed, which did not waive the failed remote check.

The corrected browser runs passed six desktop tests and five phone tests,
including both process-end regressions and both controlled snapshot races.
They used fresh production assets, one worker, and zero retries. Changed-file
lint, the catalog (432 decisions and 1921 specifications), all-spec lint,
complete PR-documentation preflight (six work orders), and whitespace passed.
The external task plan retains exact commits, logs, and pending remote CI evidence.

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/session/use-background-work-race.test.ts hooks/domains/session/use-background-work.test.ts lib/state/slices/session-runtime/background-work-state.test.ts lib/state/slices/session-runtime/background-work-clear-window.test.ts lib/state/slices/session-runtime/purge-session.test.ts components/task/chat/background-work/background-work.test.tsx components/task/chat/background-work/background-work-overview-loading.test.tsx components/task/chat/background-work/background-work-clear-window.test.tsx)
(cd apps/web && pnpm run typecheck)
(cd apps/web && pnpm e2e:run --host --project chromium tests/task/background-work-viewer.spec.ts tests/task/background-work-process-end.spec.ts -- --retries=0)
(cd apps/web && pnpm e2e:run --host --no-build --project mobile-chrome tests/task/mobile-background-work-viewer.spec.ts tests/task/mobile-background-work-process-end.spec.ts -- --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```
