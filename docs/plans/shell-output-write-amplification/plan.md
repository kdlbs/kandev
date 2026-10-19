---
created: 2026-10-19
status: implemented
requirements:
  - REQ-PLATFORM-INCREMENTAL-SHELL-001
system_design:
  - ../../specs/platform/system-design/incremental-shell-delivery.md
legacy_specs: []
---

# Implementation Plan: Shell-output write amplification

## Overview

Replace repeated accumulated foreground shell output with versioned patches and
bounded checkpoints before durable acceptance. Persist canonical live tails
incrementally so savings survive the journal/inbox boundary. Deliver dormant codec
and storage support first, activate the complete path together, then prove
endurance, compatibility and browser output parity. Implementation was explicitly authorized on 2026-10-19. All four work orders
are implemented and verified; their results record the exact validation scope.

Platform owns the durable transport/storage guarantee. ACP normalization remains
agent-owned and the browser consumes its current summary/detail contract.
The assumption check found no unresolved user question: preserve
supported tails and result semantics, keep current cadence, preserve all accepted
recovery evidence, and explicitly version incompatible encodings. Base64, chunk
size and checkpoint thresholds are the reviewed engineering choices.

## Diagnosis and evidence

### Supplied historical investigation

The parent investigated `/home/jcfs/.kandev/data/kandev.db` read-only. For October
18 session `bda950b7-85dc-4224-b76d-a4bd59801e6c`, task “Review and merge PR 3948”
using codex-acp, it reported:

- 28,872 tool updates originally accounted for 4,964,682,890 bytes.
- 14,239 updates at least 250,000 bytes accounted for 3,767,892,633 bytes.
- During 14 `monitor-replacements.py` windows, 25,254 updates accounted for
  4,513,916,679 bytes. The watcher printed matching file-write EVIDENCE lines
  and idle checkpoints.
- The 05:26:16–07:26:16 UTC example had 8,359 updates and 1,934,675,684 bytes;
  the retained watcher stdout was a 262,144-byte capped tail.
- Main DB size was 7,372,718,080 bytes; roughly 1 GB was reusable free pages
  after pruning.

Original inbox payloads have been pruned. These are parent-supplied aggregate
byte counts and temporal correlations, not exact per-tool attribution or proof
of 5 GB of distinct output. This subtask did not reopen the live DB, alter it,
or restart an instance. Physical allocation, historical writes and retained
unique output are different quantities.

### Isolated source-path reproduction, 2026-10-19

A temporary `zz_shell_amplification_repro_test.go` used the existing
`newTestAdapter`, `seedSnapshotTool`, `shellSnapshotNotification`, `shellDelta`
and `drainEvents` helpers in the ACP package. With `testing/synctest`, seed stdout
with 262,144 `x` bytes; for indexes 0..7199 send `fmt.Sprintf("%016d", index)`
through `handleACPUpdate`, marshal each emitted event, drain it, then advance
one second. Flush using `syncNotifQueue`. The temporary test was removed after
measurement; no permanent test or production code changed.

| Real adapter fixture | New bytes | Emitted events | Marshaled bytes |
| --- | ---: | ---: | ---: |
| Existing 10,000 chunks, 1 ms spacing, saturated stdout | 160,000 | 41 | 10,763,402 |
| Temporary 7,200 chunks, 1 second spacing, saturated stdout | 115,200 | 7,200 | 1,890,158,400 |

The slow fixture's emitted JSON/new-byte ratio was 16,407.62. Its test passed
because it asserted the current behavior as diagnostic evidence, not the proposed
budget. Permanent regression tests now assert the new byte budget; this diagnostic
measurement remains the unchanged pre-fix baseline. These measurements cover real adapter output serialization, not journal,
SQL, compression, filesystem bytes or a simulated implementation of v2.

Command actually run from the repository root:

```bash
(cd apps/backend && go test -trimpath ./internal/agentctl/server/adapter/transport/acp -run 'TestReproShellSlowWriteAmplification|TestShellSnapshotCoalescingStorageBound' -count=1 -v)
```

Result: PASS, package 13.434 seconds. Fast test 1.04 seconds, slow test 12.38
seconds of wall time; the latter represented two hours. Current source confirms
full serialization before journal and full inbox receipt insertion; canonical
shell output then takes the digest/gzip path independently.

## Scope

### In scope

- Typed patch/checkpoint codec, output-presence semantics and revision guards.
- V2 negotiation and journal upgrade/read guards, while accepting retained v1.
- Producer encoding after cadence selection but before acceptance/serialization.
- Incremental canonical live output, atomic tool projection and detail hydration.
- Typed retention proof, deletion/reset integration, bounded recovery and tests.
- Targeted desktop/phone output checks and rollout/rollback documentation.

### Out of scope

- Live DB maintenance, VACUUM, retention-grace reduction and instance restart.
- Conversation-fetch volume, provider-side loss, unlimited transcripts, terminal emulator.
- Optimizing unrecognized shapes, non-ACP transports or background-work events.
- A new feature toggle, UI layout, locale keys, public output API or metrics dashboard.
- Updating old runtime producers in place or claiming they satisfy v2 budgets.

## Technical approach

[The system design](../../specs/platform/system-design/incremental-shell-delivery.md)
owns the exact contract, projection and recovery rules. Preserve all existing
normalizer tests and the coalescer's initial, quiet-flush and barrier semantics.

Codec files under `internal/agentctl/types/streams` are a separate internal
representation, not a new provider-specific payload. Apply the codec only after
logical coalescing. Output carriers cannot remain duplicated in `ToolCallContents`.
Base revisions advance only with accepted writes. No generic string-prefix guess
can be treated as proof of provider stream semantics.

Canonical persistence adds a bounded current-tail head/chunk representation,
not a second full-output blob per event. Tool projection and effect/cursor commit
share a transaction. Integrate the existing orchestrator callbacks rather than
leaving them to persist full snapshots a second time. Include opt-in payload
removal, late terminal updates, session deletion and schema-conformance fixtures.

V2 is a real protocol/journal compatibility boundary. New backend v1 support is
explicit, including old exact-version comparison sites; migration never rewrites
accepted events. Every data endpoint rejects an unnegotiated compact reader.
Retained old agentctl still produces v1 and keeps the old write cost until a
safe owned replacement. No forced replacement is part of this package.

### Compatibility matrix

| Producer/shape | Transport/identity | Intended result | Evidence/fallback |
| --- | --- | --- | --- |
| Codex recognized `terminal_output_delta`, matching optional terminal ID | ACP, v2, exact tool/parent/generation | Compact after normalizing every delta | Saturated fast/slow fixture; wrong ID is ordinary barrier |
| Claude recognized terminal output | ACP, v2, typed normalized foreground shell | Compact selected normalized result | Delta/cumulative fixtures; unknown shape uses unchanged path |
| OpenCode cumulative text content | ACP, v2, typed normalized foreground shell | Replace/overlap without duplicate content carrier | Repeated/reset/capped content fixtures |
| Auggie final XML-like result | ACP, v2, normalized shell | Authoritative terminal checkpoint | Existing parser + final replacement tests; no inferred live support |
| OMP leading command echo | ACP, v2, normalized shell | Same echo-stripped result including split echo | Existing echo tests plus patch round trip |
| Supported retained old agentctl | Durable v1 identity and hash | Adopt/replay unchanged snapshots | V1/v2 binary/fixture matrix; no v2 byte claim |
| New agentctl, old backend/unknown codec | V2 descriptor/journal | Typed incompatibility; preserve Stop/evidence | Data endpoints fail closed; no legacy bypass or automatic replacement |
| Legacy non-durable/other transport/background/unknown metadata | Existing contract | Preserve behavior and ordering | No inferred optimization support; test no accidental data stripping |

### Budget acceptance

Use S=512 KiB, A=changed text bytes, N=changed output events, K=checkpoint
epochs and E=actual envelope/non-output serialization. Assert each journal/inbox
payload stream P <= 12A + E + 2S(K + floor(N/512)). Assert canonical chunk write
inputs <= 2A + 8 KiB*N + 2S*K; report small row metadata separately. Include the
seed checkpoint in K rather than pretending the fixture starts from an empty tail.
For the fixed-identity stdout-only 7,200 x 16-byte slow fixture, require each
journal/inbox payload stream below 32 MiB as an additional clear regression
threshold, versus the current approximately 1.89 GB of emitted event JSON.
The measured v2 journal and inbox results are below this threshold, as recorded
in Task 04.

Measure file sizes and WAL separately; they are not the proof of asymptotic logical
write bounds. Use fresh fixtures and identical checkpoint/retention settings.
No cleanup or compression may be necessary for the byte assertion to pass.

## Tests

The regression names below map acceptance to implemented evidence; existing
patterns and exact commands are named in work orders. Run the new failing byte assertion before implementing
activation. Use controlled time and disposable stores.

| Acceptance | Planned regression evidence |
| --- | --- |
| .1, .2 | `TestShellOutputPatchRoundTrip`, `TestShellOutputPatchSaturatedOverlap`, `TestIncrementalShellSlowStorageBound`, `TestIncrementalShellInterleavedStorageBound` |
| .2, .3 | `TestIncrementalShellNoopAndTerminal`, `TestIncrementalShellProjectionCrashReplay`, `assertShellEnduranceReplay`, UTF-8/empty/omitted table cases |
| .3, .5 | `TestIncrementalShellRetainedRuntimeCompatibility`, `TestIncrementalShellJournalUpgradeCrash`, `TestIncrementalShellUnsupportedReader` |
| .4 | `TestIncrementalShellDetailSnapshot`, `TestIncrementalShellRetentionRemoval`, `TestIncrementalShellRemovedPayloadCannotResurrect` |
| .6 | `TestIncrementalShellStorageEndurance`, actual serialized/write-input counters plus fresh journal/DB/WAL measurements |

Preserve existing `TestShellSnapshotCoalescingStorageBound`, shell normalization,
permission/background, durable stream, retention and output endpoint coverage.
Each work order's exact commands include its changed packages, without a broad
repository verification task. PostgreSQL tests use `KANDEV_TEST_POSTGRES_DSN` on
an isolated test database, including real concurrent connections.

## E2E tests

Add `apps/web/e2e/tests/chat/incremental-shell-output.spec.ts` (chromium) and
`mobile-incremental-shell-output.spec.ts` (mobile-chrome), both using isolated
mock runtime fixtures that prove v2 was negotiated. Cover .2/.3/.4/.5: open running
output, sparse append after saturation, reload, reconnect, authoritative final
replacement and nullable/nonzero/cancel status. Assert collapsed summaries carry
no body; expanded polling fetches the current committed snapshot and one final
result. Mocking an HTTP body alone does not test the new transport/storage path.

No rendered UI or interaction changes are proposed. Reuse the existing shell
row/disclosure on desktop and phone; no ASCII UI preview is necessary.

## Work orders

- [x] [Task 01: Lossless compact output codec](task-01-output-codec.md)
- [x] [Task 02: Incremental canonical output storage](task-02-canonical-storage.md)
- [x] [Task 03: Activate versioned end-to-end delivery](task-03-versioned-delivery.md)
- [x] [Task 04: Endurance, recovery and delivery documentation](task-04-endurance-recovery.md)

Execute 01 -> 02 -> 03 -> 04 sequentially. No subagents are authorized.

## Verification results

The pre-fix diagnostic passed as recorded above. Product implementation checks
pass as recorded in each work order. No live database or instance checks were
performed. Documentation checks include:

- `python3 scripts/list-docs.py validate`: PASS, 389 decisions / 1658 specifications.
- `python3 scripts/lint-spec-files.test.py`: PASS, 36 tests.
- `python3 scripts/lint-spec-files.py --all`: PASS.
- Local `.github/scripts/pr-docs.cjs:validateCoverage` preflight: PASS. The actual implementation diff
  validates all four changed work orders as covered with zero errors; the earlier
  design-only synthetic trigger also passed.
- `git diff --check -- docs/specs docs/decisions docs/plans/shell-output-write-amplification`:
  PASS. New untracked Markdown also received an explicit whitespace/link check.
- The design checkpoint left the manifest and all four work orders uncommitted
  for review. All four work orders are now implemented; PR delivery and review
  remediation are tracked separately until the remote merge is verified.

Node was absent from PATH; preflight ran successfully using the installed
`/home/jcfs/.nvm/versions/node/v24.18.0/bin/node`. Its reproducible stdin program:

```javascript
const fs = require('node:fs');
const cp = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const paths = cp.execFileSync('git', [
  'ls-files', '--modified', '--others', '--exclude-standard',
], { encoding: 'utf8' }).trim().split('\n').filter(p => p.startsWith('docs/'));
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const changedFiles = paths.map(filename => ({ filename, status: 'modified' }));
for (const synthetic of [false, true]) {
  const result = validateCoverage({ fileContents, changedFiles: synthetic
    ? [...changedFiles, {
      filename: 'apps/backend/internal/agentctl/server/process/delivery_batch.go',
      status: 'modified',
    }] : changedFiles });
  console.log(JSON.stringify({ synthetic, ...result }));
  if (!result.ok) process.exitCode = 1;
}
```

## Risks

- V2 activation touches all version equality gates, not just initial negotiation.
- Tool callbacks currently persist separately from generic projection; marking a
  patch projected before its canonical tool commit would lose output on restart.
- A patch codec alone leaves digest-blob churn and is not completion.
- Highly changing authoritative snapshots can contain an entire new tail each
  time; charge those replacement bytes honestly rather than claiming unique output.
- Old retained producers preserve behavior but retain old write cost. Downgrade of
  running compact SQL output is unsupported; matching backup rollback is not
  lossless for post-backup external actions.
- Physical DB/WAL writes depend on engine/page/transaction settings and require
  independent fixture measurement. No total DB-size ceiling is promised.

## Delivered results

The real durable producer's 7,200-update saturated stdout fixture emits about
11.7 MB of event JSON from 115,200 new bytes, versus the pre-fix real adapter's
1.89 GB. These two serialization boundaries are distinguished: accepted journal
records additionally encode their authenticated envelope and payload. The real
journal/inbox/canonical fixture records about 15.1 MB journal, 10.2 MB inbox and
18.6 MB chunk write inputs, with metadata and physical allocations reported
separately. No cleanup or compression is needed for the bound. Two saturated
fields retain their baseline across 1,100 revisions and three checkpoint
intervals below 4 MiB of producer event serialization.

Full producer/API race tests, targeted runtime/lifecycle/orchestrator/SQL race
regressions, isolated PostgreSQL projection and store conformance, SQL guard,
changed-Go lint, web typecheck and desktop/mobile E2Es passed. Existing shell
echo scenarios also passed. Implementation verification performed no live DB write, instance restart or
VACUUM. The user subsequently authorized commit, PR remediation and merge;
remote delivery is tracked in the PR and task plan. The owned PostgreSQL fixture was stopped after testing.
Retained v1 producers remain supported but retain their old storage cost.
