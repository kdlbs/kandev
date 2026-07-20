---
id: "16-onboarding-security-qa"
title: "Integrated onboarding security and QA"
status: done
wave: 11
depends_on: ["15-onboarding-e2e-docs"]
plan: "plan.md"
spec: "../../specs/integrations/github-authentication.md"
---

# Task 16: Integrated Onboarding Security and QA

## Inputs

- Completed Tasks 11-15 and every amended spec scenario.
- Security boundaries: deployment operator, callback state, secret bundle, environment precedence,
  SSRF avoidance, hot runtime generation, workspace installation binding, logs/errors/backups.

## Acceptance

1. A security review finds no secret in API responses, redirects, logs, process arguments, frontend
   state, E2E snapshots, or persisted metadata and confirms callback CSRF/replay/size bounds.
2. QA exercises rollback, restart rehydration, partial env, orphan-App recovery messaging, webhook
   unverified state, binding-safe deletion, PAT/CLI unaffected behavior, and desktop/mobile handoff.
3. Full format, tests, typecheck, lint, and focused E2E pass; a real GitHub staging checklist records
   the only external verification that mocks cannot prove.

## Files Likely Touched

- `docs/plans/github-authentication/plan.md` status only
- `docs/plans/github-authentication/task-16-onboarding-security-qa.md` status/results only
- Production or test files only when fixing a finding, followed by affected targeted verification

## Verification

```bash
rtk make -C apps/backend fmt
rtk make -C apps/backend test
rtk make -C apps/backend lint
cd apps/web && rtk pnpm run typecheck
cd apps && rtk pnpm --filter @kandev/web test
cd apps && rtk pnpm --filter @kandev/web lint
cd apps/web && rtk pnpm e2e:run tests/settings/github-app-registration.spec.ts -- --project=chromium
cd apps/web && rtk pnpm e2e:run --no-build tests/settings/mobile-github-app-registration.spec.ts -- --project=mobile-chrome
```

## Dependencies

Task 15. This task starts after the E2E/docs task and owns no planned production files.

## Output Contract

Report findings ordered by severity, remediation, commands/results, files touched, staging checklist,
blockers, and residual trust-model/GHES/rotation risk. Mark this task `done` and set plan status to
`done` only when all acceptance criteria pass.

## Results

The security review found four blocking issues, all resolved with regression coverage:

1. Callback redirects now set `Cache-Control: no-store` and `Referrer-Policy: no-referrer`, and the
   stable redirect never contains the manifest code or state.
2. Managed removal deletes registration flows before metadata and keeps the active runtime intact
   if metadata deletion fails; retries remain safe.
3. Callback state, manifest code, and provider error fields have strict pre-storage and pre-network
   size bounds in addition to state expiry and single-use enforcement.
4. E2E reset removes App-backed workspace bindings before managed registration state, then clears
   the active runtime and outstanding flows.

An independent remediation review found no remaining blocker. Focused regression suites passed 14
backend cases and eight frontend hook/layout cases. The final repository verification passed format,
metadata generation, typecheck, the complete backend suite, 719 web test files (5,614 passed and
four skipped), 30 CLI test files (280 passed), script tests including 58 public-doc validations,
full lint, and `git diff --check`.

The final Playwright run passed four Chromium desktop scenarios and three Pixel 5 scenarios. The
captured manifest handoff and workspace-installation layouts were inspected for containment,
readability, overlap, touch-target size, internal scrolling, and horizontal overflow.

### Real GitHub.com Staging Checklist

These checks require a deployed public callback URL and GitHub-owned credentials and are not
performed by the deterministic mock suite:

- [ ] Create a personal-owned App from the manifest and verify callback hot activation without a
  restart.
- [ ] Create an organization-owned App from the manifest and verify owner selection and callback.
- [ ] Install the App from workspace settings and verify repository selection and binding.
- [ ] Deliver a signed webhook and verify that webhook health becomes verified.
- [ ] Cancel a registration and replay a completed callback; verify actionable, non-secret errors.
- [ ] Disconnect all workspace installations, remove the managed registration, and separately
  delete the generated App on GitHub.

Residual boundaries are the approved trusted-single-user `default-user` operator model,
GitHub.com-only manifest support with GHES deferred, replace-not-edit managed credential rotation,
and the real-provider staging checks above. There are no implementation or verification blockers.
