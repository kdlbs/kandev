---
id: "01-preserve-intent"
title: "Preserve immediate preference intent"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-PLUGINS-MARKETPLACE-004
acceptance_criteria:
  - AC-PLUGINS-MARKETPLACE-004.1
  - AC-PLUGINS-MARKETPLACE-004.2
  - AC-PLUGINS-MARKETPLACE-004.3
system_design:
  - ../../specs/plugins/system-design/marketplace.md
---

# Task 01: Preserve immediate preference intent

## Summary

Serialize mounted global preference changes and reconcile failures against
accepted data. Prove actual switch and inherited/overridden row behavior.

## In scope

- Hook orchestration, real API transport and consumer regression tests.
- Disposable desktop/phone causal persistence and failure flows.

## Out of scope

Backend, schema, generic settings ownership, per-plugin mutation orchestration,
worker/installation policy and cross-mount or transport guarantees.

## Acceptance

- AC-PLUGINS-MARKETPLACE-004.1: inherited rows follow optimistic/accepted global
  state; explicit overrides remain independent.
- AC-PLUGINS-MARKETPLACE-004.2: serial persistence retains latest pending intent
  across rapid changes and older successful receipts.
- AC-PLUGINS-MARKETPLACE-004.3: failures restore accepted data or the unconfirmed
  off fallback, toasts remain per failed write, and later writes can recover.

## ASCII UI preview

UI-01, [combined preview](plan.md#ascii-ui-preview), desktop and phone:

```text
Automatically update plugins       [on/off]
Installed plugin (inherits)        [on/off]
Installed plugin (override)        [on/off] Reset
```

Existing responsive composition and touch controls remain; only state settles
differently. Maps to all three criteria above.

## Verification

Run from repository root with Node 24 and shared resource guard:

```bash
(cd apps/web && pnpm exec vitest run hooks/domains/plugins/use-auto-update-settings.test.ts app/settings/plugins/page-auto-update.test.tsx app/settings/plugins/page.test.tsx)
(cd apps/web && pnpm exec eslint hooks/domains/plugins/use-auto-update-settings.ts hooks/domains/plugins/use-auto-update-settings.test.ts app/settings/plugins/page-auto-update.test.tsx e2e/tests/plugins/plugin-auto-update*.ts e2e/tests/plugins/mobile-plugin-auto-update.spec.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=8192 pnpm run typecheck)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --host --project chromium e2e/tests/plugins/plugin-auto-update.spec.ts --retries=0)
(cd apps/web && GOMAXPROCS=2 GOFLAGS=-p=2 ../../scripts/run-quiet e2e --summary -- pnpm e2e:run --host --no-build --project mobile-chrome e2e/tests/plugins/mobile-plugin-auto-update.spec.ts --retries=0)
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.test.py
python3 scripts/lint-spec-files.py --all
git diff --check
```

After the normal-hook commit, verify exact-head documentation coverage using
Git object contents (repository root, installed Node 24 on the search path):

```bash
node <<'NODE'
const { execFileSync } = require("node:child_process");
const { validateCoverage } = require("./.github/scripts/pr-docs.cjs");
const git = (...args) => execFileSync("git", args, { encoding: "utf8" });
const base = "9bfb7da156b25155626ead19f8eceaff9b1ea4e0";
const head = git("rev-parse", "HEAD").trim();
const changedFiles = git("diff", "--name-only", "-z", base, head).split("\0").filter(Boolean);
const paths = [
  "docs/plans/plugin-auto-update-intent/plan.md",
  "docs/plans/plugin-auto-update-intent/task-01-preserve-intent.md",
  "docs/specs/plugins/requirements/marketplace.md",
  "docs/specs/plugins/system-design/marketplace.md",
];
const fileContents = Object.fromEntries(paths.map(p => [p, git("show", `${head}:${p}`)]));
const result = validateCoverage({ changedFiles, fileContents });
console.log(JSON.stringify({ base, head, ...result }, null, 2));
if (!result.ok) process.exitCode = 1;
NODE
git diff 9bfb7da156b25155626ead19f8eceaff9b1ea4e0 HEAD --check
```

No new copy: i18n catalogs unchanged; normal commit hook ratchets still apply.

## Files likely touched

- `apps/web/hooks/domains/plugins/use-auto-update-settings.ts` and `.test.ts`
- `apps/web/app/settings/plugins/page-auto-update.test.tsx`
- `apps/web/e2e/tests/plugins/plugin-auto-update-helpers.ts`
- `apps/web/e2e/tests/plugins/plugin-auto-update.spec.ts`
- `apps/web/e2e/tests/plugins/mobile-plugin-auto-update.spec.ts`
- Existing marketplace requirement/design and this plan package.

## Dependencies

None. Workspace dependencies installed with frozen lockfile before tests.

## Risks

Only accepted backend data defines rollback; the failed-read fallback is unknown.
Held HTTP tests are synthetic; browser successful writes must reach the real
managed backend. All owned command handles must reach terminal state.

## Results

- Baseline RED: actual hook/API plus real page controls, 6 intended failures
  among 11 tests. A separate late-GET regression fails false versus true on the
  original hook. No import or fixture failure in these RED runs.
- Browser RED on original production build: 2 failures, independently admitted
  second HTTP request while the first receipt is held. An earlier ambiguous
  toast locator was corrected and is excluded from causal evidence.
- Rebuilt fixed desktop GREEN: 2 passed. Successful requests reach disposable
  SQLite-backed settings; HTTP 503 failures are injected. Fixture inventory is
  local-only with no matching catalog release. Finally restores and reads the
  original global preference, then removes the local fixture.
- Typecheck: passed under Node 24, shared guard and 8 GiB cap (handle 15580).
- Catalog validation, 36 specification-linter tests, full specification lint,
  working-tree whitespace and documentation coverage preflight passed.
- Final hook/API/consumer plus unchanged page suites: 37 passed across 3 files
  (handle 81258). Scoped ESLint passed with no warnings (handle 70825).
- Phone: 2 passed with touch taps, real backend readback and reload, on the same
  rebuilt fixed production assets (handle 40525). Two earlier phone setup runs
  attempted a hidden desktop navigation link; final scenarios enter directly
  through the existing fixture Plugins route. No production navigation change.
- Phone capture inspected: existing settings hierarchy, direct global switch,
  single vertical page flow and unchanged fixture card match UI-01. Screenshot
  retained at `/tmp/attempt11-phone-success.png` for this validation handoff.
- Final desktop rerun: 2 passed (12.0s, handle 19420), with the strengthened
  immediate HTTP-admission counter and explicit first-commit readback. Phone:
  2 passed (16.0s). Both managed projects used one worker/shard and retries 0.
  Production assets were rebuilt for the fixed hook; later runs reused those
  exact assets, with no subsequent production source change.
- All owned installation, baseline RED, GREEN, formatting, lint, typecheck and
  managed browser handles are terminal. Failed setup attempts are distinguished
  above from behavioral RED; no retries, guard bypass or foreign process changes.
- Normal-hook commit and exact committed-head coverage/whitespace receipts are
  the remaining delivery gate, reported to the parent before ending the turn.
  No push or PR is authorized at this checkpoint.
- Duplicate check: main advanced from the original base to `6b7ffd69d8120f74b133072f0ea87f15e31c7e3e`
  with no target overlap. Open PRs 1512/4534 have no matching target paths;
  PR 3793 target patch is unchanged and does not repair this preference flow.
- Public docs need no change: existing opt-in/override instructions, labels,
  layout and public API remain accurate. Internal marketplace pair updated.
