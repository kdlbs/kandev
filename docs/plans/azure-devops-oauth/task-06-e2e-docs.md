---
id: "06-e2e-docs"
title: "End-to-end proof and public docs"
status: done
wave: 5
depends_on:
  - "05-settings-ui"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.4
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.5
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.6
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.7
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.9
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-003.1
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
---

# Task 06: End-to-end proof and public docs

## Summary

Prove the user flows on desktop and phone against the mock authority, update
public documentation, and promote the specs once everything matches.

## In scope

- `e2e/tests/integrations/azure-devops-oauth.spec.ts` (`chromium`): switch PAT
  to Microsoft account, popup sign-in, identity shown, work items browse;
  paste-URL completion; cancelled sign-in leaves the PAT connection working.
- `e2e/tests/integrations/mobile-azure-devops-oauth.spec.ts` (`mobile-chrome`):
  method select, sign-in, paste dialog without horizontal overflow.
- `docs/public/integrations.md`: Microsoft account option, admin consent note,
  operator keys, remote-host paste fallback (use `/docs-maintainer`).
- Promote requirement to `active`, design to `current`, ADR to `accepted`, plan
  to `implemented` only after all tasks are done and matching.

## Out of scope

- Real Microsoft sign-in in CI.

## Acceptance

- Both specs pass on one shard; existing `azure-devops.spec.ts` and
  `mobile-azure-devops.spec.ts` still pass.
- Public docs describe both methods and the operator override.

## Verification

```bash
cd apps/web && pnpm e2e:run --host --shards 1 --project chromium tests/integrations/azure-devops-oauth.spec.ts tests/integrations/azure-devops.spec.ts
cd apps/web && pnpm e2e:run --host --shards 1 --project mobile-chrome tests/integrations/mobile-azure-devops-oauth.spec.ts tests/integrations/mobile-azure-devops.spec.ts
python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Files likely touched

- `apps/web/e2e/tests/integrations/{azure-devops-oauth.spec.ts,mobile-azure-devops-oauth.spec.ts}`
- `apps/web/e2e/` Azure fixtures/helpers if needed
- `docs/public/integrations.md`
- Spec, design, ADR, and plan status fields

## Dependencies

Task 05.

## Risks

- Popup handling in Playwright needs `context.waitForEvent("page")`; the mock
  authorize route must redirect without user interaction.

## Parallelism

`sequential`

## Results

- `e2e/tests/integrations/azure-devops-oauth.spec.ts` (`chromium`): PAT to
  Microsoft account through the popup, a declined sign-in keeping the PAT
  connection, and paste-address completion. With the existing
  `azure-devops.spec.ts`: 4 passed.
- `e2e/tests/integrations/mobile-azure-devops-oauth.spec.ts` (`mobile-chrome`):
  touch-sized sign-in and paste actions, no horizontal overflow, paste
  completion. With the existing `mobile-azure-devops.spec.ts`: 3 passed.
- `e2e/helpers/api-client.ts`: `deleteAzureDevOpsConfig`, `getAzureDevOpsConfig`.
- `docs/public/integrations.md` and `docs/public/configuration.md` describe the
  Microsoft account method, admin consent, the paste fallback, copy behavior,
  and the two operator keys.
- ESLint and `e2e:sleep-ratchet` on the E2E files: clean.
- Spec promotion is deliberately not done: Task 07 (published client ID) and
  the manual real-organization check are still open, so the requirement stays
  `draft`, the design `draft`, the ADR `proposed`, and the plan `building`.
