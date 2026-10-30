---
id: "05-settings-ui"
title: "Settings UI"
status: done
wave: 4
depends_on:
  - "03-token-renewal"
  - "04-method-transitions"
plan: "plan.md"
requirements:
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001
  - REQ-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002
acceptance_criteria:
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.1
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.2
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.3
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.5
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.6
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.7
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-001.9
  - AC-INTEGRATIONS-AZURE-DEVOPS-OAUTH-002.5
system_design:
  - ../../specs/integrations/system-design/azure-devops-oauth.md
---

# Task 05: Settings UI

## Summary

Expose the method choice, Microsoft sign-in, paste fallback, signed-in identity,
and re-sign-in state in Azure DevOps settings on desktop and phone.

## In scope

- Types and API client functions for availability, start, and complete.
- `use-azure-devops-oauth-connect.ts` (popup, origin/source/state checks,
  closed-popup and blocked-popup paste fallback).
- `azure-devops-oauth-fields.tsx` and method `Select` in
  `azure-devops-settings.tsx`; form `authMethod` from config.
- Seven-locale copy in `apps/web/src/locales/*/azuredevops.json`; "Microsoft"
  verbatim as a brand noun; no em dash.

## Out of scope

- Backend changes.

## ASCII UI preview

See [plan UI-01 to UI-05](plan.md#ascii-ui-preview). Excerpt:

```text
| Authentication method                                              |
| [Microsoft account                                            v]  |
| +----------------------------------------------------------------+ |
| | (check) Signed in as Ana Silva            [Sign in again]      | |
| +----------------------------------------------------------------+ |
```

## Acceptance

- Component tests cover default PAT, stored OAuth, unavailable OAuth, sign-in
  success refresh, cancellation, paste completion, and re-sign-in rendering.
- Hook tests reject messages from another origin, another window, or another
  state.
- Rendered desktop and phone layouts match UI-01 to UI-05 structure.

## Verification

```bash
cd apps && pnpm --filter @kandev/web test -- components/azure-devops/azure-devops-settings.test.tsx components/azure-devops/use-azure-devops-oauth-connect.test.ts lib/api/domains/azure-devops-api.test.ts
cd apps/web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet
cd apps && pnpm --filter @kandev/web lint
```

## Files likely touched

- `apps/web/lib/types/azure-devops.ts`
- `apps/web/lib/api/domains/azure-devops-api.ts` and test
- `apps/web/components/azure-devops/{azure-devops-settings.tsx,azure-devops-oauth-fields.tsx,use-azure-devops-oauth-connect.ts}` and tests
- `apps/web/src/locales/{en,pt-pt,zh-cn,zh-hk,zh-tw,ja,ko,pseudo}/azuredevops.json`

## Dependencies

Tasks 03 and 04.

## Risks

- `azure-devops-settings.tsx` is near the TS function-size limits; keep the
  Microsoft block in its own component.

## Parallelism

`sequential`

## Results

- `lib/types/azure-devops.ts`: `AzureDevOpsAuthMethod = "pat" | "oauth"`, OAuth
  response fields and start/complete types; `lib/api/domains/azure-devops-oauth-api.ts`
  (new file, because `azure-devops-api.ts` is already over 600 lines).
- `azure-devops-settings-form.ts`: pure form helpers moved out of the component
  (method-aware `requestFromForm`, `savedPATMatches`, `signedInWithMicrosoft`,
  `canSubmitForm`, `withFieldUpdate`).
- `use-azure-devops-oauth-connect.ts` and `azure-devops-oauth-fields.tsx`: method
  `Select` (Microsoft option disabled with a hint when unavailable), Microsoft
  block (identity, re-sign-in warning, sign-in button), and the paste fallback.
  The paste fallback is an inline panel inside the Microsoft block (same pattern
  as Jira) rather than a modal; structure and copy match UI-04.
- 19 new `azuredevops` keys in seven locales (zh-tw/zh-hk via
  `convert-zh-cn-to-zh-hant.mjs`), pseudo regenerated for this namespace only.
- Follow-up: the hook also completes when the popup returns to any Kandev page
  on the same origin (redirect URL without the callback path); covered by
  `azure-devops-oauth-fields.test.tsx`.
- Vitest: `components/azure-devops/` and `lib/api/domains/azure-devops*`: 80 passed.
- `tsc --noEmit`: pass (needs `NODE_OPTIONS=--max-old-space-size=12288` here; the
  default 4 GB heap runs out of memory in this environment before and after this change).
- ESLint on changed files: 0 problems. `i18n:check` and `i18n:ratchet`: pass.
