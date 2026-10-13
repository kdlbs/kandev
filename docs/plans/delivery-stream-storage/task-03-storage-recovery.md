---
id: "03-storage-recovery"
title: "Prove combined storage and recovery"
status: done
wave: 3
depends_on: ["01-shell-snapshots", "02-control-retirement"]
plan: "plan.md"
requirements:
  - REQ-PLATFORM-BOUNDED-SESSION-HISTORY-002
  - REQ-PLATFORM-DELIVERY-RETENTION-002
  - REQ-PLATFORM-DELIVERY-RETENTION-005
  - REQ-PLATFORM-DELIVERY-RETENTION-006
acceptance_criteria:
  - AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.4
  - AC-PLATFORM-BOUNDED-SESSION-HISTORY-002.5
  - AC-PLATFORM-DELIVERY-RETENTION-002.1
  - AC-PLATFORM-DELIVERY-RETENTION-002.2
  - AC-PLATFORM-DELIVERY-RETENTION-005.2
  - AC-PLATFORM-DELIVERY-RETENTION-005.4
  - AC-PLATFORM-DELIVERY-RETENTION-005.7
  - AC-PLATFORM-DELIVERY-RETENTION-006.3
  - AC-PLATFORM-DELIVERY-RETENTION-006.5
  - AC-PLATFORM-DELIVERY-RETENTION-006.6
system_design:
  - ../../specs/platform/system-design/delivery-stream-storage.md
  - ../../specs/platform/system-design/backend-agent-delivery-retention.md
---

# Task 03: Prove combined storage and recovery

## Summary

Verify both fixes through real durable persistence using sanitized synthetic output,
and confirm that history/recovery remains usable on desktop and phone. Record measured
bytes and retirement progress, update public explanations, then deliver one PR.

## In scope

- Small ACP-to-journal-to-task-repository integration fixture using real persistence
  components and normal receive/projection, with known bootstrap controls and bounded
  shell snapshots. Reuse existing process/lifecycle test seams; do not create a new
  generic production integration layer just for testing.
- Measure emitted/journal/inbox payload bytes, event/effect counts and retired prefix.
  Assert canonical final stdout/stderr, terminal state and replay/conflict behavior.
- Existing desktop/mobile retention E2E and public explanation of periodic snapshots,
  unchanged replay guarantees and explicit physical compaction.
- Keep all plan/spec statuses and exact check results synchronized for one PR.

## Out of scope

Live DB copies or cleanup, new UI, new mocks for unrelated providers, benchmark suites,
full verification audits, releases and automatic PR polling/delegation.

## Acceptance

1. `TestShellSnapshotDurableStorage` records bounded marshaled bytes through real
   journal and SQL paths, with contiguous assigned sequences and unchanged canonical
   final output. After replay-window expiry the proved prefix and generic receipts
   retire, while protected evidence survives.
2. Restart/replay of committed snapshots remains ordered and idempotent; changed
   content fails and expired events remain expired. Existing desktop and phone
   retention flows pass with Stop and detail/history recovery intact.
3. Public docs distinguish snapshot cadence, logical cleanup and physical reclamation.
   All task-defined checks and traceability gates pass; both fixes ship in one PR.

## TDD evidence

Create `shell_snapshot_storage_test.go` in the ACP test package, with
`TestShellSnapshotDurableStorage`. This location drives the real notification owner without a production test export,
then uses the real bbolt journal and task repository receive/projection APIs.
Use aggregate measurements and controlled clocks. No original private output or
multi-gigabyte generated fixture is required. Introduce the end-to-end regression
before altering integration code if Tasks 01/02 expose a wiring gap.

## Verification

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/adapter/transport/acp -run '^TestShellSnapshotDurableStorage$' -count=1 -v -timeout=2m)
: "${KANDEV_TEST_POSTGRES_DSN:?Set an isolated PostgreSQL test DSN}"
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle -run 'TestShellSnapshot|TestDurableDelivery|TestDeliveryRetention|TestDeliveryEvent' -count=1 -timeout=10m)
(cd apps && pnpm install --frozen-lockfile)
make build-backend build-backend-linux-helpers build-web-e2e build-e2e-plugin-package
(cd apps/web && pnpm e2e:run --project chromium tests/session/delivery-retention.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/session/mobile-delivery-retention.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run browser projects sequentially with the guarded runner, using isolated E2E data.
Apply the PR-documentation coverage validator to changed work orders and their
requirements/designs, including a projected production-file change during planning.
Do not run the GitHub status publisher as a local structural preflight.

## Files likely touched

- `apps/backend/internal/agentctl/server/adapter/transport/acp/shell_snapshot_storage_test.go` (new)
- `apps/backend/internal/agentctl/server/process/submission_delivery_test.go`
- `apps/backend/internal/agent/runtime/lifecycle/durable_delivery_replay_projection_test.go`
- `docs/public/sessions-and-review.md`
- `docs/public/operations.md`
- `docs/specs/platform/system-design/delivery-stream-storage.md`
- `docs/plans/delivery-stream-storage/plan.md`
- `docs/plans/delivery-stream-storage/task-01-shell-snapshots.md`
- `docs/plans/delivery-stream-storage/task-02-control-retirement.md`
- `docs/plans/delivery-stream-storage/task-03-storage-recovery.md`

## Dependencies

Tasks 01 and 02, with their recorded red/green evidence and both-engine checks.

## Risks

Process and lifecycle integration fixtures may need a focused test seam to bridge
packages; keep production APIs unchanged. PostgreSQL skips are incomplete proof.
Public explanation must not suggest that unacknowledged journal events are discarded
or that this repair automatically compacts the database.

## Parallelism

`sequential`

## Inputs

- Stream storage design and prior work orders.
- Existing process durable submission tests and lifecycle projection/replay fixtures.
- Existing delivery-retention Playwright specs and public docs-maintainer guidance.

## Results

Completed. The combined fixture lives in the ACP test package so it drives the real private notification owner without adding a production test export. It persists emitted snapshots in the real bbolt journal, reopens/replays it byte-for-byte, receives/projects them through the real SQLite task repository, and writes/rehydrates the canonical shell message using normal message APIs. It uses single-candidate retention leases to preserve the 250 ms admitted SQL budget under race instrumentation, rather than increasing the budget.

The saturated 2,000-update fixture persisted eleven events totaling approximately 2.64 MB. All eleven expired records and their exact generic effects retired. Canonical bounded stdout, stderr and exit status survived, identical receives remained duplicates, and retired replay was rejected.

Final checks after the last production edit:

- Full ACP race package (includes every new snapshot and journal/storage regression): passed, 194.885 seconds.
- Process/lifecycle task-defined race command: passed, 44.060 seconds and 4.063 seconds respectively.
- Backend/helper/web/plugin build targets: passed; managed desktop runner rebuilt after the final source changes.
- Chromium delivery-retention E2E: one passed, one skipped (optional frozen pre-retention binary gate), 58.2 seconds. Final phone command reused that fresh verified build with --host --no-build: one passed, 53.1 seconds.
- Public docs validator tests: 63 passed; public docs validation: 48 pages passed.
- Specification tests: 36 passed; full catalog and specification validation passed (381 decisions, 1,583 specifications).
- Full backend changed-code lint against base 1cb568ac5f7a50ce3183ef31fa9c9f04b7c1d57a: zero issues.
- PR documentation coverage preflight: covered; whitespace checks passed.

Public docs updated: sessions-and-review.md (explanation) and operations.md (how-to/reference context). No rendered UI changes or screenshots are required. Existing phone history/recovery coverage satisfies mobile parity. Live database cleanup, deployment and compaction remain excluded.

PR review clarified the public explanation: output-only snapshots have a minimum
250 ms spacing, with ordered boundaries allowed to flush sooner; later retained
output means later events in the contiguous sequence. This changes documentation
wording only. Public docs validator tests and page validation were rerun and passed
(63 tests, 48 pages); production race and E2E results above remain applicable.
