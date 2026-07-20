---
id: "03-frontend-validation-contract"
title: "Frontend validation contract"
status: done
wave: 3
depends_on: ["01-explicit-path-contract", "02-identity-bound-git-operations"]
plan: "plan.md"
spec: "../../specs/workspaces/local-repositories.md"
---

# Task 03: Frontend Validation Contract

## Acceptance

- The Add Local Repository dialog treats an existing Git repository as valid regardless of the
  deprecated `allowed` response value.
- Existing loading, success, error, keyboard, desktop, and mobile layout behavior is unchanged.
- A focused unit test prevents reintroducing discovery-root containment into manual validity.

## Verification

From `apps/web`:

```bash
rtk pnpm test -- app/settings/workspace/workspace-repositories-validation.test.ts
rtk pnpm run typecheck
```

From `apps/`:

```bash
rtk pnpm --filter @kandev/web lint
```

## Files Likely Touched

- `apps/web/app/settings/workspace/workspace-repositories-client.tsx`
- `apps/web/app/settings/workspace/workspace-repositories-validation.ts`
- `apps/web/app/settings/workspace/workspace-repositories-validation.test.ts`
- `apps/web/lib/types/http.ts`

## Dependencies

- Tasks 01 and 02 establish the backend response and identity behavior.

## Inputs

- Spec: manual validation API and user scenarios.
- Mobile-parity rule: this is state/contract normalization inside an existing responsive dialog; no
  layout or touch control changes are planned. Task 04 supplies real desktop and mobile flow proof.

## Output Contract

Report the contract change, tests, files, typecheck/lint results, visual behavior risks, blockers, and
update this task plus `plan.md` to done.
