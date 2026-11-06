---
id: "01-honor-opened-selection"
title: "Honor Opened selection"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-OFFICE-AUTOMATIONS-SETTINGS-001
acceptance_criteria:
  - AC-OFFICE-AUTOMATIONS-SETTINGS-001.19
system_design:
  - ../../specs/office/system-design/automations-settings-02.md
---

# Task 01: Honor Opened selection

## Summary

Gate the real GitHub PR polling evaluator on the saved selected-opened predicate.
Independently reproduce unchecked firing, implement the minimum correction,
prove authoring/persistence/firing behavior, and update the known public limit.

## In scope

- The [settings design's event-selection section](../../specs/office/system-design/automations-settings-02.md#github-pr-event-selection)
  and the regression matrix in [the plan](plan.md#tests).
- Real evaluator, in-memory stores, credential routing/provider adapter,
  `Service.FireTrigger`, event subscriber, run/dedup observations; synthetic
  external HTTP transport only. Reuse `setupTestStore`, existing GitHub store
  setup, and `subscribeAutomationTriggered` patterns after inspecting them.
- Real `GitHubPRConfig` checkbox tests and existing draft forwarding tests.
- Replace only the inaccurate event-list sentences in the GitHub PR paragraph
  of `docs/public/automation-and-mcp.md`. State that Opened must be selected
  and clearing it stops PR polling/firing, while keeping the other limitations.

## Out of scope

No new event kinds, All repos, labels, hydration, pagination, provider/auth
changes, schema, migrations, retry/concurrency/cursor/lifecycle, production TS,
layout/copy, screenshots or unrelated cleanup. No live network/account/agent
claims. Private qualification source remains unread and untouched.

## Acceptance

1. Independent RED fails because a matching fixture with no selected opened
   still searches/fires; GREEN requires an exact existential opened match
   before provider/dedup/admission/firing. Cover empty/missing/null/unsupported,
   explicit default and mixed-list controls without resetting saved config.
2. Real stores/subscriber prove no negative-case effects and preserve enabled,
   ownership/repo/branch/author/draft filters and successful firing/dedup.
   Real editor clear/select outputs and service/store reload agree.
3. Exact checks pass; the public paragraph describes the corrected selection
   rule with existing limitations intact. Record actual results and mark this
   order done and the manifest complete only after implementation verification.

## Verification

Only run after ROOT review, a later explicit implementation interrupt and a
ROOT-granted global heavy lease. All commands below start at repository root.
Run them sequentially and retain original native execution handles through
actual completion. Record setup failures separately from causal test failures.

Set the toolchain for this shell; do not change cache locations or system HOME:

```bash
export PATH="/home/jcfs/.local/share/mise/installs/go/1.26.0/bin:/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
export GOTOOLCHAIN=local
export GOMAXPROCS=2
export GOMEMLIMIT=512MiB
node --version
pnpm --version
```

Require Node >=24.18 and pnpm 9.15.9. If and only if `apps/node_modules` is
missing, run `(cd apps && pnpm install --frozen-lockfile)` once.

After writing only the independent backend regression, run RED and retain its
actual expected failure. Do not read/replay ROOT's private qualification test.

```bash
(cd apps/backend && timeout --kill-after=10s 180s go test -race -trimpath -tags=fts5 -p=1 ./internal/automation -run '^TestGitHubPREventSelection' -count=1 -v)
```

Implement the eligibility gate, then run the exact same command for GREEN.
After all owned corrections are batched, run the owning package once:

```bash
(cd apps/backend && timeout --kill-after=10s 180s go test -race -trimpath -tags=fts5 -p=1 ./internal/automation -count=1)
```

Canonical changed-Go lint expands actual changed filenames to complete package
scope. The helper enforces its internal five-minute bound; the outer bound is
360 seconds. Include any additional actual changed Go test file as an argument.

```bash
GOMAXPROCS=2 GOMEMLIMIT=3GiB timeout --kill-after=10s 360s bash scripts/lint-go-changed apps/backend/internal/automation/evaluator.go apps/backend/internal/automation/evaluator_pr_events_test.go
```

After authoring the component test (no production TS changes):

```bash
(cd apps/web && pnpm exec vitest run components/automations/trigger-configs/github-pr-config.test.tsx components/automations/automation-trigger-drafts.test.ts)
(cd apps/web && pnpm exec eslint components/automations/trigger-configs/github-pr-config.test.tsx --max-warnings=0)
```

Typecheck is conditional on an actual production TS change, which would require
scope review; its command is `(cd apps/web && pnpm run typecheck)`. Component
test types are checked by their normal owning tools; do not broaden the suite.

After the public paragraph is corrected:

```bash
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short -- docs/plans/github-pr-event-selection
```

Run this local Node 24 coverage dry-run. It reads actual owned changed paths,
including the untracked manifest/order. The second call separately supplies the
planned evaluator path as input data so a Markdown-only design diff still checks
the complete reference chain; it does not edit source or post GitHub statuses.

```bash
node <<'NODE'
const fs = require('node:fs');
const { execFileSync } = require('node:child_process');
const { validateCoverage } = require('./.github/scripts/pr-docs.cjs');
const changed = new Set([
  ...execFileSync('git', ['diff', '--name-only', '-z'], { encoding: 'utf8' }).split('\0'),
  ...execFileSync('git', ['ls-files', '--others', '--exclude-standard', '-z'], { encoding: 'utf8' }).split('\0'),
].filter(Boolean));
const paths = [...changed];
const fileContents = Object.fromEntries(paths.map(p => [p, fs.readFileSync(p, 'utf8')]));
const actual = validateCoverage({ changedFiles: paths, fileContents });
const planned = validateCoverage({
  changedFiles: [...paths, 'apps/backend/internal/automation/evaluator.go'], fileContents,
});
console.log(JSON.stringify({ node: process.version, actual, planned }, null, 2));
if (!actual.ok || !planned.ok || planned.status !== 'covered') process.exitCode = 1;
NODE
```

After staging later, include `git diff --cached --name-only -z` in this path
inventory too, or run the preflight before staging. Keep all four referenced
documents in `fileContents` even if a later fixup changes only source. Stage only
after later authorization and run normal `(cd apps/web && pnpm run i18n:ratchet)`
over the normal staged diff containing all owned new files. Keep active ordinary
commit hooks; never bypass them. Do not rerun passing checks without a change,
failure, or unresolved concern that justifies it.

## Files likely touched

- `apps/backend/internal/automation/evaluator.go`
- `apps/backend/internal/automation/evaluator_pr_events_test.go` (new)
- `apps/web/components/automations/trigger-configs/github-pr-config.test.tsx` (new)
- `docs/public/automation-and-mcp.md` (exact GitHub PR limitation paragraph)
- Existing owning requirement/design and this manifest/order for results/status.

Production model/registry/editor/save/provider code is read-only context.

## Dependencies

None within the package. ROOT review, later explicit same-primary release and
heavy lease precede implementation. Keep normal commit/push/ready-PR delivery
in this session after task-defined checks. No merge is authorized by this order.

## Risks

Missing/null legacy configs stop accidental firing. A fake evaluator, stubbed
FireTrigger or unsuccessful routing fixture would not prove the correction.
Use explicit positive controls and real stores; do not change production client
or hydration behavior to compensate for incomplete synthetic responses.

## Parallelism

`sequential`; no delegation authorized.

## Inputs

- REQ-OFFICE-AUTOMATIONS-SETTINGS-001 and AC-001.19 in the existing owner pair.
- `evaluator.go`, `models.go`, `trigger_registry.go`, service create/add/update,
  store trigger persistence and run/dedup methods.
- `github_pr_merged_subscriber_test.go`, `fire_trigger_publish_test.go`, GitHub
  credential-routing/provider test fixtures and backend test reference.
- `github-pr-config.tsx`, `github-pr-merged-config.test.tsx`,
  `automation-trigger-drafts.ts`, and its existing tests.
- Scoped backend/web guides, `/tdd`, `/mobile-parity`, `/docs-maintainer`, and
  current ROOT delivery/resource constraints preserved in the external plan.

## Results

Done on 2026-10-17 after ROOT's later explicit implementation release.
All exact task checks passed; [the manifest](plan.md#verification-results)
records commands, actual results/counts, causal RED, setup/fixture corrections,
and the normal hook-required warning correction. Production code changes are limited to the
selected-opened gate, plus the exact public limitation paragraph. No broad
local audit, production TS change, browser/agent launch or live provider claim.
Ordinary active hooks and authorized ready-PR publication follow these checks;
merge remains subject to a later ROOT grant.
