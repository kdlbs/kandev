---
id: "01-enable-browser-compatible-fork"
title: "Enable browser-compatible native fork"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-CODEX-NATIVE-004
  - REQ-AGENTS-CODEX-NATIVE-006
acceptance_criteria:
  - AC-AGENTS-CODEX-NATIVE-004.1
  - AC-AGENTS-CODEX-NATIVE-006.1
system_design:
  - ../../specs/agents/system-design/codex-app-server.md
---

# Task 01: Enable browser-compatible native fork

## Summary

Use the shared UUID utility for the existing native fork request identity.
Prove that desktop and touch confirmation submit the existing request and
select its returned session when `crypto.randomUUID` is absent.

## In scope

- Independently write real MessageActions/store regressions with the real fork
  service and transport-only fork mock; select desktop and touch compositions.
- Replace the native action's raw UUID call with `generateUUID`, preserving
  lazy request-ref lifetime, retry identity and accepted-success reset.
- Run the bounded checks below and synchronize plan/work-order results.

## Out of scope

No generic fork rewrite, new API/protocol/backend/registry/helper behavior,
flag promotion, layout/copy/i18n changes, private-fixture replay, broad local
suites, new E2E gate, delegation, or another session. Do not touch any other
worktree, private proof, dependencies, refs, caches or processes.

## Acceptance

1. Without randomUUID, desktop dialog and touch drawer confirmation each send
   exactly one valid-UUID request with the unchanged task/session/turn/request
   envelope, then the real store selects the response task and session.
2. With capability present, the same outcome remains; disabled native support
   keeps the action absent. Rejected RPC preserves localized feedback and the
   source session; explicit retry retains the ID, success selects the destination,
   and a later operation initializes a new ID.
3. Only the native UUID call/import and independently authored consumer tests
   change in runtime scope; targeted checks pass and documentation results
   record actual evidence and limitations.

## Verification

ROOT released implementation to this same primary on 2026-10-18 and granted
the global local-heavy lease. Read the owning requirement/design pair before
execution and preserve all other contributors' changes.

Before package commands, obtain ROOT's global local-heavy lease and set a
shell-local PATH to `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin`.
If that binary is unavailable, checkpoint ROOT instead of changing global
configuration. From repository root, run installation once for this fresh
worktree before pnpm commands or hooks:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
(cd apps && pnpm install --frozen-lockfile)
```

Write `message-actions-native-fork.test.tsx` independently, using the test names
and outcomes in [the plan](plan.md#tests). Run the new tests before the fix;
confirm desktop and touch fail because missing randomUUID prevents submission,
not because of imports, mocks, store setup or unrelated assertions. Then make
the helper substitution and run the affected suites together:

```bash
(cd apps/web && NODE_OPTIONS=--max-old-space-size=2048 timeout --kill-after=10s 180s pnpm exec vitest run components/task/chat/messages/message-actions-native-fork.test.tsx components/task/chat/messages/message-actions.test.tsx components/task/chat/messages/action-message-actions.test.tsx --maxWorkers 1)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=2048 timeout --kill-after=10s 180s pnpm exec eslint --max-warnings 0 components/task/chat/messages/message-actions.tsx components/task/chat/messages/message-actions-native-fork.test.tsx)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 timeout --kill-after=10s 300s pnpm run typecheck)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git diff --check -- docs/plans/native-fork-browser-capability
git status --short -- docs/plans/native-fork-browser-capability
```

Retain and join each original command handle; serialize heavy commands and do
not duplicate or automatically retry runs. Restore original crypto descriptors
and every timer/listener in test cleanup. No global browser security changes.
Run normal active commit hooks during authorized delivery; do not bypass them.
Run the repository PR-documentation coverage validator before delivery. This
local preflight validates the planned runtime path as well as all four linked
artifacts; the eventual hosted check uses the actual published diff:

```bash
node <<'JS'
const fs = require("node:fs");
const { validateCoverage } = require("./.github/scripts/pr-docs.cjs");
const paths = [
  "docs/specs/agents/requirements/codex-app-server.md",
  "docs/specs/agents/system-design/codex-app-server.md",
  "docs/plans/native-fork-browser-capability/plan.md",
  "docs/plans/native-fork-browser-capability/task-01-enable-browser-compatible-fork.md",
];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, "utf8")]));
const changedFiles = [
  "apps/web/components/task/chat/messages/message-actions.tsx",
  ...paths,
].map(filename => ({ filename, status: "modified" }));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify(result, null, 2));
if (!result.ok || result.status !== "covered") process.exitCode = 1;
JS
```

Record exact commands and results, including any pre-existing or out-of-scope blocker.

No local Playwright run is required for this state/data-only helper substitution;
see [the existing consumer coverage and mobile justification](plan.md#e2e-tests).
No local runtime verification is claimed during the design-only phase.

Return the local-heavy lease and proceed directly in the same turn to hosted
validation during authorized delivery. Preserve ROOT's delivery constraints:
one owned CI observer (90-minute observation, GNU timeout 91 minutes with
10-second kill grace, 60-second cadence), no overlapping/replayed observers or
automatic CI retries, persistent job-name budgets, and no routine full bot
requests on a new SHA. Freeze published head absent correctness/required CI
findings; reuse adequate substantive reviews plus later diff assessment.
READY requires the known six required checks plus actual current Backend,
Frontend and E2E parent SUCCESS, complete error-free snapshot, zero actionable
visible/hidden/human findings, and clean aligned head. ROOT owns the current-main
static compatibility check and separate expected-head normal squash grant.
Never archive/clean this task or touch protected STOPPED128.

## Files likely touched

- `apps/web/components/task/chat/messages/message-actions.tsx`
- `apps/web/components/task/chat/messages/message-actions-native-fork.test.tsx` (new)
- The four design-package artifacts for status/results only.

## Dependencies

None. ROOT's later explicit implementation release and global local-heavy lease
are execution prerequisites. PR3897 shares the TSX and service paths: use the
admitted exact-patch assessment, refresh if moved, and preserve its generic action.

## Risks

Shared-file integration overlap; crypto/timer/listener test contamination;
transport mocks do not prove live remote-browser or native-provider behavior.
The original qualification is ROOT-only evidence and must not be replayed.

## Parallelism

`sequential`

## Inputs

- [Requirements](../../specs/agents/requirements/codex-app-server.md): existing
  AC-AGENTS-CODEX-NATIVE-004.1 and AC-AGENTS-CODEX-NATIVE-006.1.
- [System design](../../specs/agents/system-design/codex-app-server.md#browser-request-identity).
- [Plan evidence and scope](plan.md), public mobile-remote-access VPN guidance.
- Existing MessageActions, StateProvider, `message-actions.test.tsx`, UUID helper,
  fork service and WebSocket transport mock patterns. No private candidate input.

## Results

Completed on 2026-10-18 after ROOT's explicit release and lease grant.

- `pnpm install --frozen-lockfile` from `apps/`: exit 0.
- New regression suite before substitution, `--project browser-locales
  --maxWorkers 1`: exit 1; two causal failures, three controls passed.
- Combined three-file invocation with that project filter: exit 0, five new
  tests passed. Existing files were excluded by project selection, so the exact
  follow-up was `pnpm exec vitest run
  components/task/chat/messages/message-actions.test.tsx
  components/task/chat/messages/action-message-actions.test.tsx --maxWorkers 1`:
  exit 0, 24 tests passed. Commands used GNU 180-second timeout/10-second kill
  grace and `NODE_OPTIONS=--max-old-space-size=2048`. The verification block
  above removes the project filter for future combined runs.
- Exact scoped ESLint command above: exit 0. Prettier found the test unchanged.
- Exact single typecheck command above: exit 0, including rich-output check and
  TypeScript, `NODE_OPTIONS=--max-old-space-size=8192`, 300-second timeout.
- Catalog validation, spec lint, six-path PR-documentation preflight, diff and
  plan status checks: passed. Detailed original handles and counts are in
  [plan results](plan.md#verification-results).

Production changes are limited to the shared UUID import and native action call.
The new suite uses the real responsive hook with media inputs, real confirmation
surfaces, store, UUID helper and fork service; only the transport is mocked.
Crypto property descriptors and media spy are restored after unmount, which
releases the real responsive listeners; the suite creates no timers/listeners
of its own. No private fixture access, delegation, flag promotion or other
runtime edits occurred. Normal hooks and hosted validation follow as delivery
steps; receipts accompany the published head.
