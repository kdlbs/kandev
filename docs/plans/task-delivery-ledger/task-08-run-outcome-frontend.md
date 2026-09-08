---
id: "08-run-outcome-frontend"
title: "Run activity chart: skipped and unclassified buckets"
status: pending
wave: 4
depends_on: ["07-run-outcome-backend"]
plan: "plan.md"
spec: "../../specs/task-delivery-ledger/spec.md"
---

# Task 08: Run activity chart: skipped and unclassified buckets

The only user-visible part of this card. The ledger adds no frontend surface; the
Office run outcome changes a response shape two existing charts read.

## Types

`apps/web/lib/state/slices/office/types.ts` — the run-activity day type (around
`:308`, the one carrying `succeeded`) gains `skipped: number` and
`unclassified: number`, matching the backend `AgentRunActivityDay` JSON tags from
task 07.

## Run activity chart

`apps/web/app/office/agents/[id]/dashboard/components/run-activity-chart.tsx` —
the stacked bar gains two segments between `succeeded` and `failed`: `skipped`
and `unclassified`. Extend the legend to match. `total` comes from the backend
and already sums all five buckets after task 07, so the existing proportional
math needs no change.

Pick colours that read as distinct from the existing emerald (succeeded), red
(failed) and muted (other) in both light and dark themes.

## Success rate chart

`apps/web/app/office/agents/[id]/dashboard/components/success-rate-chart.tsx` —
no shape change: it reads `succeeded` and `total`, both of which still exist. No
code change is expected. Verify the rendered percentage against the new `total`
and stop there.

The displayed success rate will fall for agents whose runs were being blocked.
That is the correction landing, not a bug to compensate for in the UI.

## i18n

Two new keys in the `office` namespace for the new legend labels, following the
existing `t("office:succeeded")` at `run-activity-chart.tsx:52`. Non-negotiable
rules from `apps/web/AGENTS.md` and `docs/i18n.md`:

- Both labels go through `t()`; no hardcoded literal. The ratchet
  (`pnpm run i18n:ratchet`) fails on a hardcoded string in a line you changed.
- Neither is compared with `===`, and neither `t()` call is at module scope.
- Add the English and pseudo catalog entries in the same change.
- Plain punctuation only. No U+2014 em dash in any catalog value.

A clean lint is not proof the file is done: the rule skips literals assigned to
SCREAMING_CASE identifiers, so review any config-table constants in these two
components by eye.

## Mobile

These are existing charts inside an existing dashboard route; the change adds
segments to a bar and entries to a legend. Confirm the wider legend still wraps
rather than overflowing at mobile width, and that no new interaction is
introduced that would need a native mobile pattern.

- **Acceptance:**
  1. The run-activity bar renders five segments and the legend lists all five
     buckets, both labels resolved through `t()`.
  2. `pnpm run i18n:check` and `pnpm run i18n:ratchet` pass.
  3. The success-rate chart renders correctly against the new `total` with no
     code change.
  4. The legend wraps rather than overflowing at mobile width.

- **Verification:**
  `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web lint && pnpm --filter @kandev/web test -- app/office/agents && cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet`

- **Files likely touched:**
  - `apps/web/lib/state/slices/office/types.ts`
  - `apps/web/app/office/agents/[id]/dashboard/components/run-activity-chart.tsx`
  - `apps/web/app/office/agents/[id]/dashboard/components/run-activity-chart.test.tsx`
  - `apps/web/public/locales/en/office.json` (or the equivalent catalog path)
  - the pseudo catalog for the same namespace

- **Dependencies:** Task 07 (the response shape must exist first).

- **Parallelism:** parallel-safe with task 05 — disjoint trees (`apps/web` vs
  `apps/backend/internal/delivery`).

- **Inputs:** Spec **Office run outcome** and its scenarios; plan **Frontend**;
  `apps/web/AGENTS.md`; `docs/i18n.md`; the existing chart components as the
  pattern.

- **Output contract:** summary, files changed, tests run with counts, i18n check
  results, blockers, risks, and task/plan status update in the same conversation.

## Results

Pending. Before marking this task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence. Record security/trust and external side-effect boundaries when
applicable, or explicitly state `None`.
