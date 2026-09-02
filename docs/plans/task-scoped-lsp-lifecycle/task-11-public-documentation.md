---
id: "11-public-documentation"
title: "Public LSP documentation"
status: completed
wave: 5
depends_on: ["10-e2e-conformance"]
plan: "plan.md"
spec: "../../specs/platform/requirements/lsp-file-intelligence.md"
---

# Task 11: Public LSP Documentation

## Acceptance

- Developer Tools explains task-scoped policy/control, aggregate and fallback surfaces, mobile value,
  discovery, persistence/recovery, Restart impact, progress honesty, supported executors, and trust
  boundaries without retaining browser/session/idle ownership claims.
- Feature Status and Configuration accurately describe the shipped boundary,
  `limits.lspMaxServers`, and `KANDEV_LSP_MAX_SERVERS`, including deprecated YAML/environment
  connection-name fallbacks.
- Scoped AGENTS guidance matches the implemented backend/task-host ownership, the spec/index becomes
  `shipped`, all plan/task results are synchronized, and public-doc/link/diff validation is clean.

## TDD sequence

1. Search all public/spec/decision/AGENTS content for obsolete session, browser connection,
   two-minute idle, active-file-only, phone-no-control, and max-connections claims.
2. Update the smallest owning sections: Developer Tools (how-to), Feature Status (reference),
   Configuration (reference), backend AGENTS, and agentctl AGENTS. Do not duplicate the ADR.
3. Mark the spec/index shipped only after Task 10 evidence proves conformance. Fill every task
   Results section and the plan Verification Results with exact commands/outcomes.
4. Run public-doc validators, link/search checks, and `git diff --check`; inspect the final diff for
   stale product terminology and accidental implementation-result claims.

## Verification

```bash
rg -n 'browser connection owns|session and language|two minutes|active supported file|KANDEV_LSP_MAX_CONNECTIONS|phone has no LSP control' docs/public docs/specs docs/decisions apps/backend/AGENTS.md apps/backend/internal/agentctl/AGENTS.md
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
git diff --check
```

Review each search match: historical ADR alternatives may intentionally mention the old model;
current product and guidance must not.

## Files likely touched

- `docs/public/developer-tools.md`
- `docs/public/feature-status.md`
- `docs/public/configuration.md`
- `docs/specs/platform/requirements/lsp-file-intelligence.md`
- `docs/specs/INDEX.md`
- `docs/plans/task-scoped-lsp-lifecycle/plan.md`
- `docs/plans/task-scoped-lsp-lifecycle/task-*.md`
- `apps/backend/AGENTS.md`
- `apps/backend/internal/agentctl/AGENTS.md`
- `apps/web/AGENTS.md` only if the landed frontend architecture makes its current guidance inaccurate

## Dependencies

Task 10 provides final observable behavior, exact verification results, and shipped evidence.

## Parallelism

Sequential. Documentation and status must describe the verified final implementation, not intent.

## Inputs

- Spec, ADR, completed task Results, current public LSP sections, and docs-maintainer skill.
- Implemented names/defaults/routes only; the public guide should not expose internal agentctl APIs.

## Output contract

Report public pages by primary Diátaxis type, AGENTS changes, stale-search disposition, validator
results, final spec/plan status, and `git diff --check`. Update task/plan status and actual files.

## Results

Completed 2026-08-05.

- Updated Developer Tools as the task-scoped LSP how-to: policy, Start/Stop/Restart, aggregate and
  fallback surfaces, phone/tablet composition, bounded discovery, reconnect/recovery, task cleanup,
  executor support, capacity, progress honesty, and binary trust boundaries.
- Updated Configuration and Feature Status as references for `limits.lspMaxServers`,
  `KANDEV_LSP_MAX_SERVERS`, their deprecated fallbacks, and the shipped task/language ownership
  model. Updated the WebSocket route reference and public coverage evidence.
- Updated backend and agentctl scoped AGENTS guidance for the dedicated task-host execution and
  instance-owned language manager. No web AGENTS change was needed; its store/UI conventions
  remain accurate.
- Stale-claim search leaves only intentional matches: the deprecated configuration entry, two
  behavioral spec assertions, and the ADR's historical context.
- `node --test scripts/validate-public-docs.test.mjs` passed 58/58 tests;
  `node scripts/validate-public-docs.mjs` validated 41 pages; `git diff --check` passed.
- Reconciled the feature spec and specs index to `shipped`; synchronized all task checkboxes,
  results, and the parent verification ledger.
