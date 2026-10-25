---
id: "01-preserve-dispositions"
title: "Preserve acknowledged finding dispositions"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-NATIVE-CODE-REVIEW-001
acceptance_criteria:
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.4
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.9
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.10
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.11
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.12
  - AC-AGENTS-NATIVE-CODE-REVIEW-001.13
system_design:
  - ../../specs/agents/system-design/native-code-review-action-publication.md
---

# Task 01: Preserve acknowledged finding dispositions

## Summary

Implement shared local action-publication ownership and an acknowledged baseline
so an older completion cannot reopen a newer acknowledged dismissal. Prove the
rendered card/overview outcome and current error behavior through the real
provider and API path, with only transport/report boundaries mocked.

## In scope

- Add the store/task/finding group helper from the owning design and integrate
  `useFindingActions` without changing its public wrappers or toast semantics.
- Independently author the permanent real-provider fixture and cases in the
  [plan test matrix](plan.md#tests), including separate consumers and scope controls.
- Keep snapshot, clear, supersession, and unseen live-update behavior, using
  actual existing slice/handler actions in controls.
- Update this work order and manifest with actual RED/GREEN/check receipts.

## Out of scope

All exclusions in [plan scope](plan.md#scope) apply. Do not change production
cards, overview, providers, store schema, WS handlers, API protocol, backend,
package configuration, locale copy, or runtime flags. Do not copy/import/replay
or mutate ROOT's disposable proof. No browser, builds, or full suites.

## Acceptance

1. The first permanent rendered test reproduces the precise acknowledged
   Dismiss regression against the unchanged hook, while current success/failure
   controls pass. No missing-API, fixture/import, or collection failure counts.
2. All plan-matrix cases pass with shared ownership and the acknowledged
   baseline, including pending-newer failure and separate instances/scopes.
   Assert actual WS action/payload, card/overview, complete row metadata, and
   existing toast/report behavior. Do not weaken tests to match implementation.
3. All affected checks below complete with retained original handles and actual
   exit verdicts; outstanding requests/processes are joined and fresh owned
   groups are gone before explicit ROOT global-heavy RETURN. No delivery gate
   is inferred from wrapper success or an unjoined run.

## Implementation sequence

After ROOT review and explicit implementation INTERRUPT, reread the package and
current base, mark only this work order `in_progress`, and obtain the ROOT
global-heavy grant before install/test/lint/typecheck/i18n commands. Keep all
work in this primary session. Do not send an operator/model question.

Write the first regression and current-action controls before production edits.
Run the RED command and inspect the exact causal assertion plus actual exit.
Then write the minimum helper/hook correction and remaining matrix cases; run
GREEN and affected checks serially. Split a fixture helper only for size/lint
requirements. Minimal causal fixture, lint, and type corrections are permitted;
resource/timeout/unknown/transport/out-of-scope failures checkpoint to ROOT
without automatic retries, cache wipes, or foreign process kills.

## Verification

Commands are rooted independently and run only under the later explicit grant.
Use the configured Node 24 environment and pinned pnpm `9.15.9`; a version
mismatch is a ROOT checkpoint, not authorization to modify shared toolchains.
Skip install when this managed worktree already has workspace dependencies.

```bash
export PATH="/home/jcfs/.local/share/mise/installs/node/24.21.0/bin:$PATH"
test "$(corepack pnpm@9.15.9 --version)" = "9.15.9"
if [ ! -d apps/node_modules ]; then (cd apps && corepack pnpm@9.15.9 install --frozen-lockfile); fi

# RED: unchanged production hook; exact causal assertion and passing controls.
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run hooks/domains/review/use-finding-actions.test.tsx --maxWorkers=1 --no-file-parallelism)

# GREEN: all permanent cases and existing affected consumer/store/WS controls.
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec vitest run hooks/domains/review/use-finding-actions.test.tsx components/diff/review-finding-card.test.tsx components/review/review-findings-overview.test.tsx lib/state/slices/review/review-slice.test.ts lib/ws/handlers/review.test.ts --maxWorkers=1 --no-file-parallelism)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 exec eslint --max-warnings 0 hooks/domains/review/use-finding-actions.ts hooks/domains/review/use-finding-actions.test.tsx hooks/domains/review/use-finding-actions.test-utils.tsx lib/review/finding-action-publication.ts)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run typecheck)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:check)
(cd apps/web && NODE_OPTIONS=--max-old-space-size=4096 corepack pnpm@9.15.9 run i18n:ratchet)

python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
git status --short
```

The fixture helper is included in the affected eslint command. The main Vitest
suite imports it, so its behavior is covered by the required test run.
No empty-selection success is allowed.
The package-level typecheck is required because the shared hook/helper touches
app-store types; do not replace it with a synthetic standalone typecheck.

Run local PR-documentation coverage with `.github/scripts/pr-docs.cjs`'s
`validateCoverage({changedFiles, fileContents})`, supplying actual changed files
and these linked work order/plan/requirement/design contents from the workspace.
Assert `ok`, `status: covered`, and `errors: []`. This pure read preflight does
not publish a GitHub status. Design validation may supply the proposed hook and
helper paths to prove references before implementation; label that projected
input, not an actual runtime diff. After implementation use the actual diff.

No mobile browser command is scheduled: the pure-state exception and unchanged
composition rationale are recorded in [mobile parity](plan.md#e2e-tests-and-mobile-parity).
No visual or backend ordering claim follows from this component evidence.

## Files likely touched

- `apps/web/hooks/domains/review/use-finding-actions.ts`
- `apps/web/lib/review/finding-action-publication.ts` (new)
- `apps/web/hooks/domains/review/use-finding-actions.test.tsx` (new)
- `apps/web/hooks/domains/review/use-finding-actions.test-utils.tsx` (only if needed)
- `docs/specs/agents/requirements/native-code-review.md`
- `docs/specs/agents/system-design/native-code-review-action-publication.md`
- This work order and `plan.md` for implementation receipts.

Read-only context: real providers/cards/overview, `review-api.ts`, review slice,
`registerReviewHandlers`, and their existing affected tests. They are controls,
not additional production ownership targets.

## Dependencies

None within this one-work-order package. ROOT concrete design review, explicit
implementation INTERRUPT, and one global-heavy grant are external gates.

## Risks

Keep local initiation order distinct from server execution order. Row-reference
ownership can be taken by an independent event; it does not establish a global
reconciliation policy. Failed latest actions must use acknowledged rows rather
than optimistic input props. Cleanup of a completed old group must not remove
a new group. Respect managed worktree/dependency ownership at all checkpoints.

## Parallelism

`sequential`. No delegates, recursive tasks, new sessions/tabs, or model switches.

## Inputs

- [Native review requirement](../../specs/agents/requirements/native-code-review.md), `.4`, `.9` through `.13`.
- [Action publication design](../../specs/agents/system-design/native-code-review-action-publication.md).
- ROOT's immutable proof metadata in [evidence](plan.md#evidence-and-assumptions).
- Existing real-provider test patterns, `review-finding-card.test.tsx`, and
  `review-slice.test.ts`; `plan-comment-loading.ts` demonstrates store-scoped
  WeakMap identity, but its lifetime/loading service is outside this correction.
- Current task's versioned Kandev plan for identity, resource, and delivery gates.

## Results

Implemented `beginFindingAction` and its shared hook integration. The real
provider/rendered suite has 20 cases covering the plan matrix, including the
faithful synchronous subscriber replacement required by ROOT review. The
expected setter object is captured before notification; no readback is adopted.
No existing store/WS/API/backend/component writer was changed.

| Verification command from the block above | Actual result |
| --- | --- |
| Conditional pinned frozen install from `apps/` | Exit 0; pnpm 9.15.9, 935 reused packages, no download. Dependencies were absent. |
| RED focused Vitest before production changes | Actual exit 1; exact dismissed-expected/open-actual regression and two passing current Resolve controls. Initial fixture-only setup errors were corrected before this qualified RED; they are not counted as proof. |
| Expanded RED matrix before production changes | Actual exit 1; 12 behavioral failures, eight current/independent controls passed, 20 tests collected. |
| GREEN affected five-suite Vitest command | Exit 0; five suites, 54 tests passed. |
| Focused fixture Vitest after lint-only literal corrections | Exit 0; all 20 tests passed. No production change after affected GREEN. |
| Scoped four-file ESLint | Exit 0, zero warnings. Initial duplicate fixture literals were consolidated; an intervening wrong-directory invocation was not a lint verdict. |
| Package `typecheck` | Exit 0, including normal pretypecheck generators; no tracked generated-file change. |
| `i18n:check` | Exit 0; existing orphan-key warnings only, no new copy/catalog changes. |
| `i18n:ratchet` | Exit 0; normal staged new-file enforcement remains part of active commit hooks. |
| Catalog validation and all-spec lint | Exit 0; 363 decisions/1438 specifications, all passed. |
| Actual workspace `validateCoverage` | Exit 0, `covered`, `errors: []`, exactly one work order; includes actual new untracked implementation/test files. |
| `git diff --check`, status inspection | Exit 0; only the eight owned source/test/spec/plan files changed. |

All original local-heavy command handles were joined and their process groups
were freshly absent at termination; original JSON/log/native receipts are in
the current versioned task plan and its owned execution directory. This work
order records local implementation completion, not hosted CI/review or merge
completion. Active commit hooks, exact head alignment, heavy RETURN, the single
hosted collector, and separate ROOT merge authorization remain delivery gates.
