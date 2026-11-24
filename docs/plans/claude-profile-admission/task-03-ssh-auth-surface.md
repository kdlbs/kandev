---
id: "03-ssh-auth-surface"
title: "Prove SSH credential separation and settings parity"
status: in_progress
wave: 3
depends_on:
  - "02-guarded-workflow-admission"
plan: "plan.md"
requirements:
  - REQ-AGENTS-REMOTE-AUTH-ADMISSION-001
  - REQ-TASKS-CHANGE-WORKFLOW-002
acceptance_criteria:
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.1
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.4
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.6
  - AC-AGENTS-REMOTE-AUTH-ADMISSION-001.7
  - AC-TASKS-CHANGE-WORKFLOW-002.3
  - AC-TASKS-CHANGE-WORKFLOW-002.4
system_design:
  - ../../specs/agents/system-design/remote-auth-admission.md
  - ../../specs/tasks/system-design/change-workflow.md
---

# Task 03: SSH, settings parity, and operator documentation

## Summary

Prove the new admission path through a disposable SSH executor and the real credential catalog.
Verify the added settings choice on desktop and phone, then document the supported authentication routes.

## In scope

- Cover `resolveAuthSecrets`, `sshRemoteAgentEnv`, and the actual SSH child configuration boundary.
  Test both agent-profile API-key bindings and explicit executor API-key method bindings.
  Use value-free sentinels for key presence. Assert no API-key-only path creates `.credentials.json` or invokes the OAuth setup script.
- Preserve the existing OAuth script and selected file routes. Do not invent a Claude file-auth method.
- Capture diagnostics for inaccessible selected secrets using distinct synthetic ID/value sentinels.
  Sanitize raw store errors at any touched SSH resolver boundary where they disclose those sentinels.
- Add a real-SSH fixture test for preview/apply, defaults/overrides, independent Review/QA sessions, and deletion between preview and apply.
  Use the existing disposable sshd fixture from `ssh-test-base.ts`.
- Keep the actual Claude `RemoteAuth` catalog in the admission path while replacing inference with a credential-free ACP stub.
  Inspect `agent/registry/provider.go` and `agents/mock.go` before selecting the smallest E2E-only fixture seam.
  The ordinary methods-less mock alias cannot establish this acceptance.
  A fixture extension must be enabled only through existing E2E mode and must have an outside-E2E rejection test.
- Exercise separate API-key/OAuth secret choices in executor settings, including independent save/reload and deselection.
  Use existing localized env-method labels. Do not add raw English catalog hints.
  If implementation needs new copy, add all seven shipped locale values and generate Traditional Chinese with `i18n:zh-hant`.
- Update public auth configuration and troubleshooting in `agents-and-profiles.md` and `executors.md`.
  Explain current actor scope, Global bindings, no duplicate executor binding requirement, and API-key/OAuth separation.
  Document the manual full-ID model workaround as a workaround, without claiming alias equivalence.

## Out of scope

Real Anthropic credentials or inference, a live SSH host, production data, secret sharing UI, new navigation, model-policy changes, or a general compatibility UI rewrite.
The shared browser compatibility helper omits SSH from its credential-gated executor set today.
Do not broaden that unrelated task-create gate or treat browser ID presence as secret authorization.

## Acceptance

1. Disposable SSH evidence confirms the admitted API key reaches the child through each supported configured route without OAuth file provisioning.
   Guarded workflow changes preserve intended defaults/replacements and independent Review/QA sessions.
2. UI-01 exposes both methods on desktop and phone. Save/reload preserves their independent bindings, localized labels, keyboard access, focus return, and contained touch controls.
3. Public docs explain the actual routes and limits. The final record distinguishes mock ACP/real SSH proof from real-provider authentication.

## ASCII UI preview

UI-01 uses the same vertical inline region on desktop and phone:

```text
Claude
  [ ] Provide secret for CLAUDE_CODE_OAUTH_TOKEN
      [Stored secret selector]
  [ ] Provide secret for ANTHROPIC_API_KEY
      [Stored secret selector]
```

See the [combined before/after preview](plan.md#ascii-ui-preview).
The env names and independent bindings are structural requirements for AC-AGENTS-REMOTE-AUTH-ADMISSION-001.6.
Exact English labels and spacing are illustrative. Existing translation keys own the rendered copy.
The profile page owns scrolling. Phone actions need actual 44px hit targets and no document overflow.
Desktop keeps compact controls. Existing picker dismissal and focus return remain intact.
Use the current remote-auth accordion and mobile SSH profile navigation as exemplars.

## TDD and verification

Add `TestClaudeAPIKeySSHProvisioning` before adjusting any runtime mapping found to be incomplete.
Add `TestE2EClaudeAuthCatalogFixture` if the E2E registry needs a catalog-preserving test adapter.
Keep that fixture unavailable outside E2E.
Use `tests/ssh/claude-profile-admission.spec.ts` with project `containers`.
Its real SSH assertions must retain the real Claude catalog and use synthetic secrets only.

Add `tests/settings/claude-remote-auth.spec.ts` and `mobile-claude-remote-auth.spec.ts` for UI-01.
Measure touch hitboxes and viewport containment. Include a narrow fine-pointer viewport and a long translated label.
Run desktop and phone commands separately. Never pass all-worker overrides or overlap the suites.

Run each command from the repository root:

```bash
(cd apps/backend && go test -trimpath -race ./internal/agent/runtime/lifecycle -run '^(TestClaudeAPIKeySSHProvisioning.*|TestSSHRemoteAgentEnv.*|TestRemoteExecutorsResolveExplicitGitHubTokenSecret)$' -count=1)
(cd apps/backend && go test -trimpath -race ./internal/agent/registry ./internal/backendapp -run '^TestE2EClaudeAuthCatalogFixture.*$' -count=1)
(cd apps && pnpm install --frozen-lockfile)
(cd apps/web && pnpm exec vitest run components/settings/profile-edit/remote-credentials-card.test.tsx)
(cd apps/web && KANDEV_E2E_CONTAINERS=1 pnpm e2e:run --project containers tests/ssh/claude-profile-admission.spec.ts)
(cd apps/web && pnpm e2e:run --project chromium tests/settings/claude-remote-auth.spec.ts tests/task/change-workflow.spec.ts)
(cd apps/web && pnpm e2e:run --project mobile-chrome tests/settings/mobile-claude-remote-auth.spec.ts tests/task/mobile-change-workflow.spec.ts)
(cd apps/web && pnpm run i18n:check)
node --test scripts/validate-public-docs.test.mjs
node scripts/validate-public-docs.mjs
python3 scripts/list-docs.py validate
python3 scripts/lint-spec-files.py --all
git diff --check
```

If no registry fixture extension is needed, replace its command with the exact test that proves catalog retention.
Do not record a zero-test match as a pass.
Managed E2E commands rebuild the Vite/backend fixtures. Record the actual discovered tests and results.
If Docker is unavailable, report that blocker and retain this work order as incomplete until the required SSH evidence exists.
Run changed-Go lint for any runtime/fixture Go changes. Run web typecheck and changed-file lint if TS production changes become necessary.

## Files likely touched

- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_credentials_test.go` or a focused new companion
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_credentials.go` only for a proven mapping or diagnostic-redaction gap
- `apps/backend/internal/agent/runtime/lifecycle/executor_ssh_operations.go` only if a child-env regression proves a missing mapping
- `apps/backend/internal/agent/registry/provider.go` and a new fixture-specific test, only for the E2E-only catalog adapter
- `apps/backend/internal/agent/agents/mock.go` only if the bounded test adapter belongs there
- `apps/web/e2e/tests/ssh/claude-profile-admission.spec.ts` (new)
- `apps/web/e2e/tests/settings/claude-remote-auth.spec.ts` (new)
- `apps/web/e2e/tests/settings/mobile-claude-remote-auth.spec.ts` (new)
- `apps/web/components/settings/profile-edit/remote-credentials-card.test.tsx`
- `apps/web/components/settings/profile-edit/remote-auth-section.tsx` only if the existing generic renderer fails the new catalog case
- `docs/public/agents-and-profiles.md`, `docs/public/executors.md`

## Dependencies

Task 02. No model alias decision or unmerged credential-sharing PR is required.
Re-read scoped backend, agentctl, and web instructions before touching their fixtures.

## Risks

Mock replacement can erase the provider's authentication methods and create false evidence.
OAuth setup currently has best-effort/script conditions. Do not refactor those conditions without a failing regression.
Public documentation must not imply that configuration admission proves provider-side credential validity.

## Parallelism

`sequential`

## Inputs

- [Auth design: provisioning and settings](../../specs/agents/system-design/remote-auth-admission.md)
- Existing SSH `repository-secrets.spec.ts`, settings mobile SSH link test, and task change-workflow desktop/mobile tests
- `apps/web/e2e/fixtures/ssh-test-base.ts`, `apps/web/e2e/playwright.config.ts`
- `apps/web/components/settings/profile-edit/remote-auth-section.tsx`
- `/mobile-parity`, `/e2e`, `/docs-maintainer`, and `docs/i18n.md`

## Results

In progress.

Completed:

- `TestClaudeAPIKeySSHProvisioning` proves an explicitly selected Claude API key resolves into `ANTHROPIC_API_KEY`, does not create `CLAUDE_CODE_OAUTH_TOKEN`, and that the API-key catalog method carries no OAuth setup script. `TestSSHRemoteAgentEnv` already proves request-only credential forwarding and no control-plane fallback.
- The generic remote-credentials renderer already supports multiple env methods. A desktop/unit regression in `remote-credentials-card.test.tsx` renders the independent Claude OAuth and API-key choices and confirms selecting both keeps distinct method IDs.
- `docs/public/executors.md` now documents the two Claude routes, the no-duplicate executor binding rule, the admission boundary, and the provider-validity limit.
- `docs/public/agents-and-profiles.md` documents profile API-key admission and the exact-model full-ID guidance without claiming alias equivalence.

Verification:

```text
go test -trimpath -race ./internal/agent/runtime/lifecycle -run '^(TestClaudeAPIKeySSHProvisioning|TestSSHRemoteAgentEnv|TestRemoteExecutorsResolveExplicitGitHubTokenSecret)$' -count=1
ok

cd apps/web && pnpm exec vitest run components/settings/profile-edit/remote-credentials-card.test.tsx
12 passed

cd apps/web && pnpm exec oxlint -c oxlint.config.mjs --deny-warnings components/settings/profile-edit/remote-credentials-card.test.tsx
passed
```

Pending:

- `apps/web/e2e/tests/ssh/claude-profile-admission.spec.ts` has not been added or run.
- `apps/web/e2e/tests/settings/claude-remote-auth.spec.ts` and its mobile companion have not been added or run.
- No live provider authentication or production deployment was exercised.
