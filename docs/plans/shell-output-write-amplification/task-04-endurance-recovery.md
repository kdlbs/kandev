---
id: "04-endurance-recovery"
title: "Endurance, recovery and delivery documentation"
status: done
wave: 4
depends_on:
  - "03-versioned-delivery"
plan: "plan.md"
requirements:
  - REQ-PLATFORM-INCREMENTAL-SHELL-001
acceptance_criteria:
  - AC-PLATFORM-INCREMENTAL-SHELL-001.1
  - AC-PLATFORM-INCREMENTAL-SHELL-001.2
  - AC-PLATFORM-INCREMENTAL-SHELL-001.3
  - AC-PLATFORM-INCREMENTAL-SHELL-001.4
  - AC-PLATFORM-INCREMENTAL-SHELL-001.5
  - AC-PLATFORM-INCREMENTAL-SHELL-001.6
system_design:
  - ../../specs/platform/system-design/incremental-shell-delivery.md
---

# Task 04: Endurance, recovery and delivery documentation

## Summary

Prove the lifetime write bound and browser compatibility with isolated fixtures. Publish operational upgrade/rollback limits only after behavior is implemented.

## In scope

- Add controlled-time fast/slow/sparse/interleaved endurance tests, with seeded saturated tails, high-entropy/multibyte/escaped data, no-op cumulative updates and many ordered barriers. Record actual serialized journal/inbox, canonical write inputs and fresh journal/DB/WAL bytes, independently of compression or cleanup.
- Add retained v1 and v2 binary/fixture tests, mid-upgrade failure, producer/backend replacement, prefix pruning, lost ACK, duplicate/conflicting events and canonical removal/reload coverage.
- Add isolated desktop/mobile end-to-end scenarios and public explanation of v2, v1 retained limits, forward repair and matched backup rollback. Reconcile legacy no-table wording and proposed/current document lifecycle after all work orders pass.

## Out of scope

No full E2E suite, live DB/instance, manual compaction, retention-policy change or new frontend layout.

## Acceptance

- `TestIncrementalShellStorageEndurance` reports all byte categories, satisfies the design formulas and the slow fixture <32 MiB journal/inbox thresholds, and reconstructs identical supported output after restart/duplicate replay.
- Desktop and mobile tests prove negotiated v2 live output, reload/reconnect and final results through existing output APIs; permission, late-terminal and summary no-body contracts remain covered.
- Public docs and all required checks pass; actual evidence is recorded in work orders, draft specs become current/active only when implementation matches, and no unsafe downgrade or old-runtime byte claim is published.

## Verification

Run from the repository root. Add the regression tests and show their expected
red result before making production changes. Commands below define the required checks; actual results are recorded below. Run PostgreSQL only against an isolated configured test database.

```bash
(cd apps/backend && go test -trimpath -race ./internal/agentctl/server/process ./internal/agent/runtime/lifecycle ./internal/task/repository/sqlite -run 'IncrementalShell' -count=1 -v)
(cd apps/backend && test -n "$KANDEV_TEST_POSTGRES_DSN" && go test -trimpath -race ./internal/task/repository/sqlite -run 'IncrementalShell.*Postgres' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm e2e:run --project chromium tests/chat/incremental-shell-output.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/chat/mobile-incremental-shell-output.spec.ts)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

Run `bash scripts/lint-go-changed` with every changed Go file in this task before
completion, plus `git diff --check`. Record those exact paths/results here.

## Files likely touched

- New `incremental_shell_*_test.go` fixtures under process, journal, lifecycle and task repository packages
- `apps/web/e2e/tests/chat/incremental-shell-output.spec.ts` (new)
- `apps/web/e2e/tests/chat/mobile-incremental-shell-output.spec.ts` (new)
- `apps/web/e2e/fixtures` and `apps/backend/cmd/mock-agent` fixture wiring, as needed
- `docs/public/operations.md`, `docs/public/sessions-and-review.md`
- `docs/specs/ui/requirements/acp-shell-command-output.md` historical storage scope
- This package, paired specs and accepted ADR

## Dependencies

03-versioned-delivery.

## Risks

Physical file growth is not logical bytes written. Record unsupported PostgreSQL/binary execution as unverified and do not mark required evidence done without running it.

## Parallelism

`sequential`. The primary session owns integration and shared status updates.

## Inputs

- [Requirement](../../specs/platform/requirements/incremental-shell-delivery.md).
- [System design](../../specs/platform/system-design/incremental-shell-delivery.md).
- [Plan](plan.md), including diagnosis, compatibility matrix and test names.
- [Decision record](../../decisions/2026-10-19-incremental-shell-delivery.md).
- Existing `adapter_shell_snapshot_test.go`, `shell_output_test.go`,
  `agent_delivery_test.go`, `message_payload_test.go` and `durable_delivery_stream_test.go`
  patterns in their respective implementation packages.

## Results

Implemented and verified on 2026-10-19. All data came from disposable fixtures;
no live DB or instance was used. The owned PostgreSQL container was stopped.

### Storage and recovery

- Current-source SQL race suite (`IncrementalShell|AgentDelivery`): passed in 143.915s, including endurance (131.24s), cold-cache lost-ACK/conflicting replay, post-grace pruning and removed-output late checkpoints. The receipt hash still rejects changed replay after the payload is pruned.
- Full metadata-instrumented endurance run (69.934s without race): 115,200 changed text bytes; 15,127,635 journal serialized bytes; 10,172,802 inbox bytes; 18,620,160 chunk write-input bytes; 2,388,403 head metadata bytes; 1,792,800 message metadata bytes; 7,200 generic effect rows. Envelope bytes: 7,334,775. Journal/main DB/WAL sizes: 33,558,528 / 18,124,800 / 4,659,752. File sizes are allocation snapshots, not cumulative physical writes. Timestamp serialization can vary slightly between runs.
- The endurance fixture generates codec records and uses the actual journal, inbox and canonical projector. The independent real-producer 7,200-update fixture verifies its checkpoint schedule; browser tests cover the complete ACP-to-detail path. Seed checkpoint bytes are charged separately from new text. No cleanup/compression is used to pass the budgets.
- Both saturated fields, no-op checkpoint boundaries, sparse updates, interleaving/barriers, terminal replacement, escaped/high-entropy/multibyte fields and omitted/unknown exits have focused coverage. The two-field regression first failed because a checkpoint counted toward its own successor; the corrected 1,100-revision fixture spans three checkpoint intervals below 4 MiB and passes with race instrumentation.
- Isolated PostgreSQL compact projection passed with race instrumentation (3.351s), including distinct concurrent connections. Full store-conformance results are recorded in Task 02.
- Journal migration tests inject a failure before transaction commit, verify the v1 header remains intact, reopen read-only and then upgrade/replay unchanged records. Retained-runtime compatibility uses v1 capability/journal fixtures; no historical agentctl binary was launched. Old-reader rejection is exercised through actual HTTP/WebSocket endpoints.

### Browser and tooling

- Discovery: three tests each in chromium/mobile-chrome. Final fresh managed host build passed all three desktop tests (27.7s); the same artifacts passed all three phone tests (24.7s). Coverage includes saturated live output, a sparse append, reload while running, authoritative final stdout/stderr, nonzero/unknown exit and UI cancellation with preserved output after reload. Phone controls use touch input.
- The four existing desktop echo scenarios also passed beside the two terminal-result scenarios (six tests, 50.2s). Components and public output APIs are unchanged.
- The producer/storage TDD failures are the data-regression red gate. Browser assertions were added after persistence wiring because the user-facing contract stays the same and the v2 fixture requires that wiring. Mock streaming fixture tests first failed with zero emitted frames, then passed after adding controlled ACP frames; their race tests pass (1.063s). Gates belong to the disposable E2E temporary root and are released in `finally`.
- Focused E2E ESLint passed. Web typecheck passed with `NODE_OPTIONS=--max-old-space-size=8192`; the default 4 GiB run exhausted Node's heap without TypeScript diagnostics. No project configuration was changed.
- Changed-Go lint passed with zero issues, followed by sequential focused zero-issue checks of later edits. An attempted overlapping lint was refused by golangci-lint's lock; the sequential retry passed. Exact checked paths are recorded in [verification-go-files.txt](verification-go-files.txt). Repeat the combined scope with `xargs -a docs/plans/shell-output-write-amplification/verification-go-files.txt bash scripts/lint-go-changed`.
- Public docs tests (63), 48-page public-doc validation, catalog validation (389 decisions / 1658 specifications), spec-linter tests (36), full spec lint, PR-documentation coverage and whitespace checks pass.
- Interrupted commands with no completion result are not counted as passes. Full process/API tests used a disposable HOME with existing Go caches and installed Node/npm on PATH because the host login profile references a missing cargo file. No user profile was changed.

The paired requirements/design and ADR now describe the implemented contract.
Retained v1 runtimes keep their original write cost. Forward repair and matched
backup rollback limits are documented in Operations; retention grace and
unacknowledged recovery evidence are unchanged. Commit, PR remediation and merge were subsequently authorized by the user;
remote delivery is tracked in the PR and task plan.


### PR remediation verification

The shared desktop/phone helper now verifies the final exit status and absence
of earlier live output after the final reload, in addition to exact stdout and
stderr. A fresh managed host build passed all three Chromium cases in 33.0s;
the same artifacts passed all three mobile-chrome cases in 29.8s. Each run used
one shard and one worker. Commands:

```bash
pnpm --dir apps/web e2e:run --host --shards 1 --project chromium tests/chat/incremental-shell-output.spec.ts
pnpm --dir apps/web e2e:run --host --no-build --shards 1 --project mobile-chrome tests/chat/mobile-incremental-shell-output.spec.ts
```

No UI composition, interaction or copy changes were introduced. Both viewport
checks reuse the shipped command row and output disclosure.
