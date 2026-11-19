---
id: "06-e2e-and-docs"
title: "E2E coverage and public docs"
status: done
wave: 4
depends_on: ["03-promptsync-backend", "04-workflow-sync-reference-warnings", "05-prompt-sync-ui"]
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-PROMPT-SYNC-001
  - REQ-INTEGRATIONS-PROMPT-SYNC-003
  - REQ-INTEGRATIONS-PROMPT-SYNC-004
  - REQ-INTEGRATIONS-PROMPT-SYNC-006
acceptance_criteria:
  - AC-INTEGRATIONS-PROMPT-SYNC-001.3
  - AC-INTEGRATIONS-PROMPT-SYNC-003.1
  - AC-INTEGRATIONS-PROMPT-SYNC-003.2
  - AC-INTEGRATIONS-PROMPT-SYNC-003.6
  - AC-INTEGRATIONS-PROMPT-SYNC-004.3
  - AC-INTEGRATIONS-PROMPT-SYNC-004.5
  - AC-INTEGRATIONS-PROMPT-SYNC-006.1
system_design:
  - ../../specs/integrations/system-design/prompt-sync.md
---

# Task 06: E2E coverage and public docs

## Summary

Prove the user flow end to end on desktop and phone, document Prompt Sync,
and promote the specs once everything matches.

## In scope

- `apps/web/e2e/tests/settings/prompt-sync.spec.ts`, using the mock GitLab
  repository fixture from `workflow-sync-gitlab.spec.ts`. The flow: configure,
  then **Sync now**, and see the synced badge with no edit control. Change the
  file and sync again to see the updated preview without a reload. Remove a
  file that a workflow step references and see the kept-warning. Then remove
  sync and see the prompt become editable. Run in the mobile project too.
- Extend the Workflow Sync GitLab spec with one unresolved-reference warning
  assertion.
- `docs/public/prompt-sync.md` (layout, configuration, reconciliation rules,
  read-only behavior, troubleshooting), cross-linked from
  `docs/public/workflow-sync.md`. Follow `/docs-maintainer`.
- Update `CLAUDE.md`/`AGENTS.md` (the Observability counters) and the
  `apps/backend/AGENTS.md` package list (`promptsync`, `reposync`).
- Promote the requirement to `active`, the system design to `current`, and
  the plan to `implemented`, but only when everything matches.

## Out of scope

New behavior.

## Acceptance

- The E2E spec passes in the desktop and mobile projects.
- The docs and spec checks pass.

## Verification

```bash
cd apps/web && pnpm e2e:run --host -- tests/settings/prompt-sync.spec.ts tests/workflow/workflow-sync-gitlab.spec.ts
pnpm e2e:run --host --no-build --project mobile-chrome -- tests/settings/mobile-prompt-sync.spec.ts
cd ../.. && python3 scripts/list-docs.py validate && python3 scripts/lint-spec-files.py --all
```

## Likely files

- `apps/web/e2e/tests/settings/prompt-sync.spec.ts` (new), `apps/web/e2e/tests/workflow/workflow-sync-gitlab.spec.ts`, and `apps/web/e2e/helpers/api-client.ts`
- `docs/public/prompt-sync.md` (new), `docs/public/workflow-sync.md`, and the docs index
- `CLAUDE.md` and `apps/backend/AGENTS.md`

## Risks

- The mock GitLab fixture may need a nested-directory listing for
  `.kandev/prompts`. Extend the mock rather than bypassing the provider.

## Results

- `pnpm e2e:run --host -- tests/settings/prompt-sync.spec.ts`: 1 passed.
- `pnpm e2e:run --host --no-build --project mobile-chrome -- tests/settings/mobile-prompt-sync.spec.ts`: 1 passed.
- `pnpm e2e:run --host --no-build -- tests/workflow/workflow-sync-gitlab.spec.ts`: 6 passed.
- Related specs: mobile workflow and prompt settings (11 passed) and desktop
  prompt settings (10 passed).
- `python3 scripts/list-docs.py validate`, `python3 scripts/lint-spec-files.py --all`,
  and `node scripts/validate-public-docs.mjs`: pass.
- Requirement promoted to `active`, design to `current`.
- AC-INTEGRATIONS-PROMPT-SYNC-005.3 was clarified during implementation:
  GitLab credential failures keep Workflow Sync's bounded circuit retries; a
  missing credential workspace suspends for both providers.
