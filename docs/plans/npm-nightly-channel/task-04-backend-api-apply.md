---
id: "04-backend-api-apply"
title: "Backend API and apply semantics"
status: completed
wave: 3
depends_on: ["03-backend-channel-foundation"]
plan: "plan.md"
spec: "../../specs/release/requirements/npm-nightly-channel.md"
---

# Task 04: Backend API and apply semantics

- **Acceptance:** Updates responses expose effective channel capability, and admin PATCH persists a
  valid supported selection while returning `400`/`409` for invalid/unsupported requests.
- **Acceptance:** `Get`, `Check`, poller, notifications, and manual commands use the selected
  channel; apply binds to the submitted cached target, returns 409 if stale, and installs the exact
  immutable version; downgrade-like channel returns are not sent as normal upgrade notifications.
- **Acceptance:** verified npm/npx user services can use Nightly; all other install kinds remain
  Stable.
- **Acceptance:** update-status read failures are logged with their internal detail while the API
  returns a generic error body.
- **Verification:** `cd apps/backend && go test -v ./internal/system/...`
- **Verification:** `cd apps && pnpm --filter kandev exec vitest run src/service/self_update.test.ts`
- **Files likely touched:** `apps/backend/internal/system/system.go`, updates `handler.go`,
  `install_state.go`, `apply.go`, service/poller tests, and `apps/cli/src/service/self_update.test.ts`.
- **Dependencies:** Task 03.
- **Parallelism:** sequential.
- **Inputs:** spec API, permissions, supported-install, and exact-apply scenarios.
- **Risks:** persisted preference, effective capability, apply preflight, and notification policy
  must agree from one install snapshot.

## Verification results

- `cd apps/backend && go test -v ./internal/system/...` — passed; environment-specific tests for
  `xdg-open` and PostgreSQL skipped with their documented prerequisites absent.
- `cd apps && pnpm --filter kandev exec vitest run src/service/self_update.test.ts` — passed,
  11 tests.
