---
id: "04-review-cleanups-e2e-docs"
title: "Review cleanups, E2E, and docs"
status: done
wave: 3
depends_on:
  - "01-interim-settings-interlock"
  - "03-restart-and-launch-rollback"
plan: "plan.md"
spec: "../../specs/office/agents.md"
---

# Task 04: Review cleanups, E2E, and docs

## Acceptance

- Preview/save/launch prefix validation is identical; bad create requests make
  zero writes; the stale ownership comment and E2E typing casts are removed.
- Frontend async tests use controlled promises/`act`/`waitFor`, with no fixed
  sleep.
- The rebuilt mobile settings E2E passes, and public docs accurately describe
  ACP-only scope, quoting/argv preservation, fail-closed runtime behavior, and
  the replayable interim interlock versus the future operator boundary.

## Verification

```bash
cd apps/backend && go test ./internal/agent/settings/controller
cd apps && pnpm --filter @kandev/web test -- --run \
  'app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.test.tsx' \
  'lib/api/domains/agent-profile-normalize.test.ts'
cd apps/web && pnpm e2e:run tests/settings/mobile-agent-profile-config-selector.spec.ts
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
```

## Files likely touched

- `apps/backend/internal/agent/settings/controller/agent_config.go`
- `apps/backend/internal/agent/settings/controller/controller_test.go`
- `apps/backend/internal/agent/settings/controller/agent_crud.go`
- `apps/backend/internal/agent/settings/controller/profile_crud_test.go`
- `apps/backend/internal/agent/agents/agent.go`
- `apps/web/app/settings/agents/[agentId]/profiles/[profileId]/command-preview-card.test.tsx`
- `apps/web/e2e/helpers/api-client.ts`
- `apps/web/e2e/tests/settings/mobile-agent-profile-config-selector.spec.ts`
- `apps/web/lib/types/agent-profile.ts`
- `docs/public/agents-and-profiles.md`
- `docs/public/security.md`
- `docs/specs/office/agents.md`
- this task file

## Inputs

- Completed tasks 01 and 03.
- Review threads named in the parent task.
- `/mobile-parity`, `/e2e`, and `/docs-maintainer`.

## Output contract

Return a compact handoff capsule with intent/acceptance, base/head SHA, changed
files and entry points, risk tags, exact RED/GREEN/E2E/docs verification
commands/results, uncertainties, and this task status set to `done`. Do not edit
`plan.md`.
