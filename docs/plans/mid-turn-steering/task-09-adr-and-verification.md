---
id: "09-adr-and-verification"
title: "Record the decision and verify the whole feature"
status: done
wave: 6
depends_on: ["01-negotiate-steer-capability", "02-runtime-toggle", "03-fold-handoff-fixtures", "04-adapter-steer-admission", "05-orchestrator-steer-admission", "06-session-steer-contract", "07-composer-steer-affordance", "08-mock-agent-and-e2e"]
plan: "plan.md"
spec: "../../specs/platform/mid-turn-steering.md"
---

# Task 09: Record the decision and verify the whole feature

- **Acceptance:** An ADR under `docs/decisions/` records the steer-admission
  trigger, the attribution contract for an early predecessor settlement, and the
  negotiated-capability gate replacing the agent-name whitelist. It states
  explicitly that the fold is undocumented and unadvertised, that the deciding
  component's version is unobservable, and that `deferred` is therefore a
  first-class success. `docs/decisions/INDEX.md` is updated, and the
  mid-turn-steering non-goal in ADR-0049 is marked as superseded for the
  steer-capable case.
- **Acceptance:** Every spec scenario maps to a test that exists and passes, or
  is explicitly listed as not covered with a reason. Any scenario left uncovered
  is reported, not quietly dropped.
- **Acceptance:** Public docs are updated if composer behavior or terminology
  changed for users; if not, say so and why.
- **Verification:** `cd apps/backend && go test -race ./internal/... && make -C apps/backend lint` then `cd apps/web && pnpm run typecheck && pnpm lint && pnpm test && pnpm run i18n:check && pnpm e2e`
- **Files likely touched:** `docs/decisions/` (new ADR),
  `docs/decisions/INDEX.md`,
  `docs/decisions/0049-fine-grained-foreground-idle-busy-signal.md` (supersession
  note), `docs/specs/platform/mid-turn-steering.md` (status → `building` or
  `shipped`), `docs/plans/mid-turn-steering/plan.md` (status), and
  `docs/public/**` only if user-facing behavior changed.
- **Dependencies:** All prior tasks.
- **Inputs:** Spec "Decision record" names what this ADR supersedes. `/record
  decision` owns the ADR format; `/docs-maintainer` decides the public-docs
  question. Root `CLAUDE.md` requires updating scoped `AGENTS.md` when
  conventions change — check `apps/backend/AGENTS.md` and
  `apps/web/AGENTS.md` for anything this feature invalidates.
- **Risks:** Do not let a green run stand in for scenario coverage: the spec's
  Scenarios section is the conformance surface, so verify each one maps to a real
  assertion rather than assuming the suite covers it.
- **Output contract:** Report the ADR path, the scenario→test coverage table with
  any gaps named, the public-docs decision, exact commands/results, and update
  only this task's status plus the plan's overall status.
