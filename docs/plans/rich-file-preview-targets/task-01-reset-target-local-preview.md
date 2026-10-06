---
id: "01-reset-target-local-preview"
title: "Reset target-local file preview state"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-AGENT-RICH-OUTPUT-001
acceptance_criteria:
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.9
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.10
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.11
  - AC-AGENTS-AGENT-RICH-OUTPUT-001.12
system_design:
  - ../../specs/agents/system-design/agent-rich-output.md
---

# Task 01: Reset Target-local File Preview State

## Summary

Scope disclosure and preview state to one session/repository/path descriptor
inside the real rich-output consumer. Replacing that target remounts the file
card collapsed and idle, preserving explicit lazy expansion and same-target
cache reuse.

## Admission

ROOT reviewed this package and explicitly released implementation with the
exclusive GLOBAL LOCAL-HEAVY lease on 2026-10-05. Local implementation/checks are done in this
same primary session. The grant covers serial real-consumer RED/GREEN, scoped
gates, one pinned frozen install if absent, active hooks, ready publication,
and live readback. No merge lease is granted. Return the local-heavy lease only
after all local/publication handles are joined, before the sole 45-minute
all-terminal hosted collector.

Historical DESIGN-only restrictions were observed before this release. No
delegation, recursive tasks, new sessions/tabs, model change, or operator question.
Parent callback queue remains full; never retry messages/questions. Save durable
receipts in the own platform plan, which ROOT watches. The [manifest](plan.md#admission-and-checkpoint)
records identity and recovery rules. Persistent completion still requires the
separate ROOT MERGE lease, verified actual merge, and only-owned joined cleanup.

## In scope

- Add the transport-only-mocked full-consumer regression file named below.
- Key the actual `FilePreviewBlock` in `RichOutputBlockView` by an unambiguous
  ordered session/repo/path tuple. Preserve outer type/index keys.
- Maintain the owning pair and update actual implementation/check results in
  this order and the manifest after release.

## Out of scope

- Hook, parser, transport, backend/schema, request coordination, automatic reads,
  aborts, global cache, speculative callback/effect fixes.
- Other block identities or chart work, settings/persistence, dependencies,
  public copy/API/config, geometry, touch, navigation, scroll, breakpoint work.
- Broad suites, app/browser/DB allocation, historic proof replay/removal.

## Acceptance

1. Permanent real-consumer RED demonstrates each same-position path/repo/session
   replacement defect before the production edit; GREEN proves collapse,
   discarded old state, no automatic read, and correct explicit new-target read
   (AC .9-.10).
2. Rendered controls prove same-target metadata/rerender stability, lazy success
   and explicit failure retry, old deferred success/error isolation, and
   independence of duplicate-target sibling cards (AC .10-.12). Only client
   acquisition/file-content transport is mocked.
3. Targeted gates pass under the granted lease with every owned handle joined;
   documents carry actual results. Source changes stay in the file branch.
   Public lazy-preview contract and mobile state-only exception remain valid.

## ASCII UI preview

UI-01: shared inline card, unchanged controls and phone stacking. Full context:
[manifest preview](plan.md#ascii-ui-preview).

```text
Target A expanded: [Report] alpha:a.txt [Hide preview] [Open file]
                  old content
Replace with B:   [Report] beta:b.txt  [Preview]      [Open file]
                  collapsed; zero automatic reads
Expand B:         [Report] beta:b.txt  [Hide preview] [Open file]
                  B loading, then B content or unavailable
```

AC .9-.10 govern the state transitions. Existing `FilePreviewBlock` markup,
`aria-expanded`, `min-[420px]` header composition, localized controls, bounded
preview, and viewer callback provide the actual rendered evidence. Under
[`mobile-parity` line 118](../../../.agents/skills/mobile-parity/SKILL.md#mobile-e2e-expectations),
this purely local state normalization uses rendered component tests and an
explicit note without browser/build/mobile E2E. Neither viewport composition
nor pointer behavior changes. Escalate any causal visual scope change to ROOT
before adding browser work.

## Implementation sequence after release

1. Read this order, the owning pair, and manifest. Mark `in_progress` only after
   explicit release and lease. Read current source for causal compatibility if
   it changed; do not replay ROOT proof or repeat its design blob audit.
2. If workspace dependencies are absent, do exactly one pinned pnpm 9.15.9
   frozen install from `apps/`. Current design checkout has no `apps/node_modules`.
3. Write `rich-output-renderer-file-targets.test.tsx`. Use a real renderer
   harness, valid persisted arguments, real parser/card/hook, and the repository
   locale/React setup. Only mock `@/lib/ws/connection` and
   `@/lib/ws/workspace-files` detail transport. Keep the original parse-mocked
   renderer test separate.
4. Run only this new file for causal RED. Assert same block index and unchanged
   title for independent path/repo/session replacement. Verify rendered collapse,
   no old content/loading/error, request count unchanged, and exact tuple on
   subsequent explicit expansion. RED must name the state defect, not a fixture,
   missing import, or resource failure.
5. Add the target tuple key to the actual `FilePreviewBlock` return, preferably
   inline `JSON.stringify([sessionId, block.repo, block.path])`. Leave the
   component and hook implementation intact. Do not key the outer wrapper.
6. Complete the [bounded test matrix](plan.md#tests): old deferred success/error
   before expansion and after new content; unchanged/fresh args and metadata;
   same-target error retention; lazy successful cache; explicit failure retry;
   two identical-target cards with only the first replaced. Settle every owned
   promise in `act` and clean real React fixtures. Avoid Cartesian matrices or
   assertions of implementation key calculations.
7. Run the targeted GREEN and gates below serially. Routine causal own-fixture,
   test, and lint corrections are allowed inside the approved lease. No generic
   replay of a passing suite or broader tests absent a new corrective finding.
8. Record actual commands/results, synchronize manifest, and follow later
   publication gates. Local work-order `done` is not persistent completion.

## Verification

Run from repository root, using `tools.exec_command` with `shell="/bin/bash"`
and `login=false`. Retain each returned `session_id`; poll it to a terminal
result before proceeding. One global local-heavy lease covers installation,
tests, eslint, typecheck, i18n, and normal active hooks; do not overlap children.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/pnpm/9.15.9:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:/usr/local/bin:/usr/bin:/bin"
export NODE_OPTIONS="--max-old-space-size=4096"
ulimit -t 600
node --version
(cd apps && pnpm --version)
# Only once if apps/node_modules is absent. Stop on failure; do not autoretry.
if [ ! -d apps/node_modules ]; then
  (cd apps && timeout --kill-after=15s 900s pnpm install --frozen-lockfile) || exit
fi
```

Require actual Node 24 and pnpm 9.15.9 output. Explicit PATH/heap/CPU bounds
apply to every subsequent shell. OS wall-time bounds below allow checkpoints;
resource/time-limit failures require ROOT recovery, not a retry with relaxed
limits. Installation and checks never allocate an app, browser, or database.

RED, before the production edit (expected causal nonzero result):

```bash
(cd apps/web && timeout --kill-after=15s 600s pnpm exec vitest run --maxWorkers=1 components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx)
```

GREEN and later gates, each awaited separately; stop on failure:

```bash
(cd apps/web && timeout --kill-after=15s 600s pnpm exec vitest run --maxWorkers=1 components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx components/task/chat/messages/kandev/rich-output/rich-output-renderer.test.tsx)
(cd apps/web && timeout --kill-after=15s 600s pnpm exec eslint components/task/chat/messages/kandev/rich-output/rich-output-renderer.tsx components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx)
(cd apps/web && timeout --kill-after=15s 600s pnpm run typecheck)
(cd apps/web && timeout --kill-after=15s 600s pnpm run i18n:check)
(cd apps/web && timeout --kill-after=15s 600s pnpm run i18n:ratchet)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

Use this lightweight repository reference-coverage preflight from root during
DESIGN and again after implementation. It intentionally checks the planned
production path against the linked package; it is not hosted CI evidence:

```bash
/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node <<'NODE'
const fs = require('node:fs');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const documents = [
  'docs/specs/agents/requirements/agent-rich-output.md',
  'docs/specs/agents/system-design/agent-rich-output.md',
  'docs/plans/rich-file-preview-targets/plan.md',
  'docs/plans/rich-file-preview-targets/task-01-reset-target-local-preview.md',
];
const result = validateCoverage({
  changedFiles: [...documents,
    'apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.tsx'],
  fileContents: Object.fromEntries(documents.map(p => [p, fs.readFileSync(p, 'utf8')])),
});
console.log(JSON.stringify(result, null, 2));
process.exitCode = result.ok && result.status === 'covered' ? 0 : 1;
NODE
```

No public guide edit is expected: its existing lazy expansion and viewer routing
remain accurate. No locale additions are planned. Normal active commit hooks
run during later authorized delivery; preserve their gates and report actual
results. No broad unit suite, build, browser, E2E, backend, or desktop check.

## Files likely touched

- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer.tsx`
- `apps/web/components/task/chat/messages/kandev/rich-output/rich-output-renderer-file-targets.test.tsx` (new after release)
- `docs/specs/agents/requirements/agent-rich-output.md`
- `docs/specs/agents/system-design/agent-rich-output.md`
- `docs/plans/rich-file-preview-targets/plan.md`
- This work order.

Read-only inputs include the file card, shared hook, parser, existing renderer
and hook tests, `apps/web/AGENTS.md`, and ROOT's preserved proof artifacts.

## Dependencies

No work-order dependencies. Later implementation INTERRUPT and global
local-heavy lease are mandatory admission prerequisites; a separate ROOT MERGE
lease governs eventual squash merge.

## Risks and recovery

Wrapper-level keys would reset unrelated rendering; delimiter-based keys could
alias targets; parse/hook mocks would hide the defect. Keep the real consumer
and file-instance boundary.

For resource/timeouts, transport/unknown causes, out-of-scope work, or actual
hosted failures, preserve receipts/handles and checkpoint WAITING for ROOT's
bounded recovery. No callback retries, cache wipe, foreign process kill, weakened
checks, or broad passing replay. Preserve foreign worktrees/caches/refs/processes,
paused oversized child, unproved volume, managed worktree/deps, and ROOT proof.
Freeze any later published SHA except actual corrective findings; follow the
manifest's CI, current-head CodeRabbit, merge lease, independent merge evidence,
joined-handle and only-owned cleanup gates before persistent completion.

## Parallelism

`sequential`; no agents or additional sessions.

## Inputs

- [Active requirement](../../specs/agents/requirements/agent-rich-output.md), AC .9-.12.
- [Current system design](../../specs/agents/system-design/agent-rich-output.md#file-preview-lifetime).
- [Accepted native rich-output ADR](../../decisions/2026-08-14-kandev-native-agent-rich-output.md), existing explicit-read boundary.
- [Manifest evidence and test matrix](plan.md#confirmed-evidence-and-assumptions).

## Results

Local implementation and task-defined checks complete under ROOT's exclusive lease. DESIGN
history: no permanent test/production edits or product checks/publication preceded
release. Current production diff is only the file-instance session/repo/path key.

Actual receipts: one frozen apps install pnpm9.15.9 (joined71025, exit0); permanent
full-consumer RED (joined28337, exit1, three causal failures/one positive); new
consumer GREEN13/13 (final joined54271, exit0); existing memoization control1/1
passed in combined run. Combined run also found an own-test closing-brace error;
corrected it and reran only the affected suite. Corrected own-test ESLint warnings;
final scoped eslint exit0 with no warnings. Typecheck joined63700 exit0; i18n check
joined40963 exit0. I18n ratchet joined93642 exit0. Catalog/spec lint exit0
(351 decisions/1366 specs); actual six-path repository reference coverage exit0
`covered`, no errors. `git diff --check` exit0. All local product handles joined.
Normal active hooks and ready publication remain the next delivery step; their
receipts are recorded in the own platform plan. Local work-order done does not
complete the persistent task: verified actual merge/cleanup is still required.

The full consumer tests cover AC .9-.12 with actual parser/card/hook/React and
transport-only mocks. No browser/build/E2E/screenshot is needed under the reviewed
mobile-parity line118 state-normalization exception. Public lazy-preview guide
remains accurate; no public copy/API/config changes.
