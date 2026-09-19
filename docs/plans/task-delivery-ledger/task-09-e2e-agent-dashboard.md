---
id: "09-e2e-agent-dashboard"
title: "E2E: agent dashboard run-activity legend"
status: pending
wave: 6
depends_on: ["08-run-outcome-frontend"]
plan: "plan.md"
spec: "../../specs/task-delivery-ledger/spec.md"
---

# Task 09: E2E: agent dashboard run-activity legend

One assertion on the only user-visible change in this card.

**Scenario.** GIVEN an Office agent with runs on a day, WHEN the agent dashboard
is opened, THEN the Run Activity legend shows the succeeded, skipped,
unclassified, failed and other buckets and the chart renders without error.

Extend the existing `apps/web/e2e/tests/office/agent-dashboard.spec.ts` rather
than adding a new spec file — this is an additional assertion on a route that
already has coverage, not a new user flow.

The ledger gets no E2E: it has no user-visible surface, which the spec's **Out of
scope** fixes as a contract. Do not add board columns, badges, filters or
dashboard tiles for it here.

- **Acceptance:**
  1. The extended spec passes against a seeded Office agent.
  2. The legend assertion reads the translated labels the way the rest of the
     suite does, not hardcoded English that would break under the pseudo-locale.
  3. No new spec file is added and no unrelated spec is modified.

- **Verification:**
  `cd apps && pnpm install --frozen-lockfile && cd apps/web && pnpm e2e:raw -- e2e/tests/office/agent-dashboard.spec.ts`

- **Files likely touched:**
  - `apps/web/e2e/tests/office/agent-dashboard.spec.ts`
  - `apps/web/e2e/fixtures/` or `apps/web/e2e/helpers/` only if the existing
    seed does not produce runs with distinct outcomes

- **Dependencies:** Task 08.

- **Parallelism:** sequential — it verifies task 08's rendered output.

- **Inputs:** Spec **Office run outcome** scenarios; plan **E2E Tests**;
  `apps/web/e2e/README.md`; the existing `agent-dashboard.spec.ts`.

- **Output contract:** summary, files changed, exact E2E command and pass/fail
  counts, any fixture changes, blockers, risks, and task/plan status update in
  the same conversation.

## Results

Pending. Before marking this task done, replace this with every exact command
actually run and its outcome/count, generated artifact paths, and cleanup or
teardown evidence (including temporary capture-spec removal and
`git diff --check` when used). Record security/trust and external side-effect
boundaries when applicable, or explicitly state `None`.
