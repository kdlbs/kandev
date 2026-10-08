---
id: "01-confirm-review-delivery"
title: "Confirm Review delivery before removing feedback"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-UI-REVIEW-COMMENT-DELIVERY-001
acceptance_criteria:
  - AC-UI-REVIEW-COMMENT-DELIVERY-001.1
  - AC-UI-REVIEW-COMMENT-DELIVERY-001.2
  - AC-UI-REVIEW-COMMENT-DELIVERY-001.3
  - AC-UI-REVIEW-COMMENT-DELIVERY-001.4
  - AC-UI-REVIEW-COMMENT-DELIVERY-001.5
  - AC-UI-REVIEW-COMMENT-DELIVERY-001.6
  - AC-UI-REVIEW-COMMENT-DELIVERY-001.7
  - AC-UI-REVIEW-COMMENT-DELIVERY-001.8
  - AC-UI-REVIEW-COMMENT-DELIVERY-001.9
system_design:
  - ../../specs/ui/system-design/review-comment-delivery.md
---

# Task 01: Confirm Review delivery before removing feedback

## Summary

Keep pending Review feedback available until the real message.add request
acknowledges it. Implement the local pending/settlement boundary through the
actual shared desktop/phone Review composition with independently authored
rendered regressions and exact store/persistence evidence.

## In scope

- Independently author the nine scenario names in the [plan's test matrix](plan.md#tests)
  through both actual production mounts, mocking only external transports.
- Make `useReviewDialog` the captured-owner admission/acknowledgement/clear/close
  boundary and propagate `Promise<boolean>` plus pending state through the
  existing dialog/top bar/button. Remove invocation-time clears/closes.
- Preserve row metadata and exact current drafts, retry, explicit dismissal,
  same-session successful close, and original wire semantics.
- After GREEN, replace the existing public loss warning and reconcile the
  review-file-comments design's failure paragraph with a link to the new design.
- Record reached RED/GREEN and exact checks; promote requirement/design and
  manifest/work-order statuses only after every scoped gate actually passes.

## Out of scope

Backend/API/WS schema changes, formatting/routing/UUID policy changes, other
composer fixes, global store action changes, automatic resend, exactly-once or
rollback claims, generic draft/coordinator/navigation framework, new copy/CSS/
layout/touch controls, browser/build/E2E/full suite, delegation or new sessions.

## Acceptance

1. Both actual mounts prove rejected/missing-client retention, reached
   persistence, retry, deferred no-clear/no-duplicate admission, and acknowledged
   unchanged success through the real ReviewDialog send-button chain.
2. In-flight text/metadata edits, new/unrelated notes, deletion and changed
   session settlement are preserved; exact original payload and captured owner
   remain correct. No invocation-time clear/close remains on this chain.
3. Exact scoped gates below pass with retained original native receipts and
   joined owned resources; public docs/specs describe the implemented boundary
   truthfully. Existing positive controls remain intact.

## ASCII UI preview

UI-01: Existing Review delivery states, [full preview](plan.md#ascii-ui-preview).

```text
Desktop (sidebar | diff): [Fix comments (2): disabled while pending] [Close]
Phone (diff): overview-first tap, then same pending/settlement control
Failure: notes stay, Fix comments enabled, existing toast, no auto-close
Success: remove unchanged sent notes; keep new/edited notes for next send
```

Applies to AC-UI-REVIEW-COMMENT-DELIVERY-001.1 through `.9`. The drawing is a
state annotation; existing labels, geometry, scrolling and touch behavior are
retained. The scoped state/data mobile exception forbids presentation expansion.

## Files likely touched

- `apps/web/components/task/use-review-dialog.ts`
- `apps/web/components/task/dockview-review-dialog.tsx`
- `apps/web/components/review/review-dialog.tsx`
- `apps/web/components/review/review-dialog-surface.tsx`
- `apps/web/components/review/review-top-bar.tsx`
- `apps/web/components/review/review-fix-comments-button.tsx`
- New `apps/web/components/task/dockview-review-dialog.delivery.test.tsx`
- `docs/public/sessions-and-review.md` (existing paragraph only, after implementation)
- `docs/specs/ui/system-design/review-file-comments.md` (failure paragraph/link only)
- Owning requirement/design and this manifest/work order (lifecycle/results).

Read-only inputs include `mobile/session-mobile-review-dialog.tsx`, actual
desktop/tablet layout callers, CommentsStore/types/persistence/format, and
existing scoped test controls. Add a test helper only if required by the test
file's lint limit; include it in changed ESLint and results, keep it test-only.

## Dependencies and release barriers

None. Execution is sequential in task `7f4d60cb-9de6-4151-8514-8b8443340a24`,
primary `62e1b1e9-fa72-4310-87d0-2df5074684b8`. This order stays pending during
design. ROOT must send a LATER explicit implementation interrupt and grant the
ONE global local-heavy lease before install, tests, lint, typecheck or hooks.
No operator approval/model switch prompt and no delegation.

## TDD and receipt discipline

After release, read the owning pair and set this order in_progress. Author the
full first-party rendered regression independently of ROOT's protected 0400
candidate; never copy/import/replay/edit/chmod/release it. Establish causal
retention RED for rejection/missing client with actual success control PASS.
Missing required providers, unrelated read fixtures and runtime warnings do
not count as causal failures. Then implement the minimum correction and run
GREEN with the retained controls and remaining criteria.

Use existing Node 24.21.0 PATH, Bash login=false. Worktree dependencies are
absent at design. Under the heavy lease run exactly one pnpm9.15.9 frozen install
from apps if still absent. Do not change lockfiles or retry an unexpected install
failure. Each command below runs serially, independently rooted at repository
root; retain EVERY original native start/full response/session and actual final
chunk/exit. Track wrapper PID/PGID/start identity, wait/reap and fresh absence of
every owned group. Never infer status from logs or replay a passing check.

Routine affected fixture/lint repairs need only their affected checks.
Resource, timeout, transport, unknown or out-of-scope failures require a durable
ROOT checkpoint before alternatives, retry, cache wipe or process changes.
No foreign resources may be killed. All original handles must actually join
before explicit global-heavy RETURN. No backend lint for this TS/docs-only fix.

## Verification

The bounded commands below describe later implementation checks, not permission
to run them during design. Start each shell at repo root and export:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export NODE_OPTIONS="--max-old-space-size=4096"
```

Verify `node --version` is v24.21.0 and `(cd apps && pnpm --version)` is 9.15.9.
If apps/node_modules remains absent, run this once under the lease:

```bash
(cd apps && timeout --kill-after=10s 300s pnpm install --frozen-lockfile)
```

RED (new independently authored suite only), then GREEN (new suite plus scoped
unchanged compatibility controls; do not replay GREEN absent a new concern):

```bash
(cd apps/web && timeout --kill-after=10s 180s pnpm exec vitest run --maxWorkers=1 components/task/dockview-review-dialog.delivery.test.tsx)
(cd apps/web && timeout --kill-after=10s 180s pnpm exec vitest run --maxWorkers=1 components/task/dockview-review-dialog.delivery.test.tsx components/task/use-review-dialog.test.ts lib/state/slices/comments/review-file.test.ts lib/state/slices/comments/format.test.ts)
```

Changed-file lint, typecheck (including existing required generation hooks),
and repository i18n gates run once serially after the implementation:

```bash
(cd apps/web && timeout --kill-after=10s 180s pnpm exec eslint components/task/use-review-dialog.ts components/task/dockview-review-dialog.tsx components/review/review-dialog.tsx components/review/review-dialog-surface.tsx components/review/review-top-bar.tsx components/review/review-fix-comments-button.tsx components/task/dockview-review-dialog.delivery.test.tsx)
(cd apps/web && timeout --kill-after=10s 180s pnpm run typecheck)
(cd apps/web && timeout --kill-after=10s 180s pnpm run i18n:check)
(cd apps/web && timeout --kill-after=10s 180s pnpm run i18n:ratchet)
```

No new user-facing strings are planned; reuse the existing localized error.
If a helper or existing test callback needs a type correction, include every
actually changed TS/TSX file in the final scoped ESLint and relevant Vitest run.
Do not broaden testing to unrelated passing suites.

Dependency-free documentation gates from repository root:

```bash
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
git status --short
```

Run this real repository documentation evaluator on actual tracked/untracked
paths after implementation, using raw Git output via execFileSync. During
design, the same evaluator may instead use the six prospective production
paths plus these four artifacts; label the result prospective, not actual-code
coverage.

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const docs = [
  'docs/specs/ui/requirements/review-comment-delivery.md',
  'docs/specs/ui/system-design/review-comment-delivery.md',
  'docs/plans/preserve-review-comments/plan.md',
  'docs/plans/preserve-review-comments/task-01-confirm-review-delivery.md',
];
const gitPaths = args => execFileSync('git', args, {encoding: 'utf8'}).split('\0').filter(Boolean);
const paths = [...new Set([
  ...gitPaths(['diff', '--name-only', 'HEAD', '-z']),
  ...gitPaths(['ls-files', '--others', '--exclude-standard', '-z']),
])];
const fileContents = Object.fromEntries(docs.map(p => [p, fs.readFileSync(p, 'utf8')]));
const result = validateCoverage({changedFiles: paths.map(filename => ({filename, status: 'modified'})), fileContents});
console.log(JSON.stringify({paths, ok: result.ok, status: result.status, errors: result.errors}));
if (!result.ok) process.exitCode = 1;
NODE
```

Before completion, verify actual changed-path coverage, every declared AC/REQ
and design link, and current source alignment. Promote draft requirement to
active, design to current, manifest to implemented and order to done only after
its behavior and checks have been demonstrated. Record all failures and
affected reruns truthfully in Results and the durable platform plan.

## Standing delivery after implementation

The task's durable platform plan retains ROOT's delivery rules and exact
task/session/system marker. After scoped checks, normal active hooks,
Conventional Commit/no bypass/amend, push and ready PR are authorized. Preserve
clean exact local/upstream/authoritative remote/PR head and task-bound canonical
repository association; link only if absent and read back all five automation
settings false. Return global-heavy only after all original native joins and
fresh owned-group absence.

One original `scripts/pr-await --mode all-terminal --deadline-min 90 --interval-sec 60`
collector under GNU91m/kill10 follows; no duplicate timer polling, replacement,
hosted retry or moving-head polish. All six required contexts and actual
Backend/Frontend/E2E parents must succeed at the frozen head. Require fresh
complete/error-free checks, zero failed/pending/actionable/human gates and
authenticated CodeRabbit App347564 substantive FULL current-head all-file
review (sourceCommitId=coveredCommitId=head, kind reviewed). Sufficient automatic
FULL requires no duplicate request; only a proved gap permits at most one
necessary full request/head. Disposition every actual thread/grouped finding.

No merge occurs without ROOT's separate serial expected-head squash grant and
static incoming compatibility evidence. Verify actual MERGED SHA/tree/owned
blobs/remote inclusion and join only-owned cleanup. Keep managed worktree/deps,
protected ROOT proof and foreign resources. Completion requires verified merge,
joined cleanup and END; ROOT independently verifies/archiveABSENT/proof release
and slot refill. Parent notification is optional; never interrupt ROOT or retry
a full queue. Persist checkpoints for ROOT to read directly.

## Risks

See the [plan](plan.md#risks). In particular, preserve full first-party fixture
fidelity, immutable submitted row ownership, and truthful uncertain-outcome
limits. Do not trade those boundaries for tests of an implementation predicate.

## Parallelism

`sequential`

## Inputs

- [Owning requirements](../../specs/ui/requirements/review-comment-delivery.md)
- [Owning system design](../../specs/ui/system-design/review-comment-delivery.md)
- [Existing review file-comment design](../../specs/ui/system-design/review-file-comments.md)
- Scoped `apps/web/AGENTS.md` and `components/review/AGENTS.md`.
- Actual store/type/persistence/formatter and all shared mounts/callers.
- Existing `hooks/use-session-file-reviews.test.tsx` actual-provider transport
  fixture pattern, use-review-dialog tests and review-file/format controls.
- ROOT proof inspection is evidence only; never use its candidate as a fixture.

## Results

Implementation passed the bounded scoped gates after ROOT's explicit release.
The initial nine scenarios ran through both actual mounts (18 tests); 32 unchanged
controls passed. Corrective coverage adds a registered-disconnected scenario
on both mounts, bringing the mounted matrix to 20 tests. Rejection and missing-client RED reached actual store and
persistence; acknowledged controls passed. Fixture failures and affected
repairs/reruns are recorded in the [manifest](plan.md#verification-results).
ESLint, typecheck, i18n, documentation validators, actual-path coverage and
whitespace checks passed. Production scope is exactly six existing callback
modules; no global store/backend changes.

The PR review's offline-client finding was confirmed by actual WebSocketClient
queuing and dispatch-only timeout behavior. Its two mounted regressions failed
before the local status guard while two acknowledged controls passed. After
the guard, all 20 mounted cases passed in two disjoint affected runs. Changed
ESLint, typecheck, i18n, docs validators and full 14-path coverage passed.
The public retry guide now tells users to inspect the conversation before
resending an uncertain result. Historical full-review evidence at the original
head is retained; current-head review and merge gates remain delivery work.

No production/permanent test edits, product runs, install, staging, commit, PR
or merge occurred during design. Original design receipts are recorded
in the [manifest](plan.md#verification-results), durable platform plan and
`/tmp/kandev-child87-design-receipts-20261008/`. Catalog/spec lint, 36 dependency-free
spec-validator tests, prospective real documentation coverage and design hygiene
passed. Every design native command terminated; zero active handles and no
global-heavy lease acquired. These results do not release implementation or
claim actual-code/persistence RED/GREEN for this worktree. Implementation receipts
are retained separately in `/tmp/kandev-child87-receipts-20261008/`.
Delivery continues under the standing gates; merge requires ROOT's separate
grant and completion requires verified merge and joined cleanup.
