---
id: "02-frontend-runtime-disclosure"
title: "Frontend Kubernetes runtime disclosure"
status: complete
wave: 2
depends_on: ["01-exact-session-status"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 02: Frontend Kubernetes runtime disclosure

- **Acceptance:** every Kubernetes executor/runtime presentation uses the
  central Pod-shaped cube icon, including normal/error task status and both
  Kubernetes settings headers.
- **Acceptance:** desktop shows exact Pod state/details plus Refresh and
  Executor settings; it never falls back to `ready` plus an empty-resource
  message after the Kubernetes lookup completes, and it does not offer the
  unsupported generic Reset environment action.
- **Acceptance:** coarse-pointer task chrome exposes the same information and
  actions in an accessible Drawer with a visible 44 px trigger and no hover
  dependency.
- **Verification:**
  `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web exec vitest run lib/api/domains/kubernetes-api.test.ts lib/executor-icons.test.ts components/task/executor-environment-status.test.ts components/task/executor-environment-info.test.tsx components/task/executor-settings-button.test.tsx components/task/mobile/session-mobile-top-bar.test.tsx && pnpm --filter @kandev/web lint && cd web && pnpm run typecheck && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet && cd ../.. && ! rg -n 'IconCloudComputing' apps/web/lib/executor-icons.ts apps/web/app/settings/executors/k8s apps/web/app/settings/executors/new/'[type]'/kubernetes-create-page.tsx apps/web/components/settings/profile-edit/profile-connection-settings-action.tsx && git diff --check`
- **Files likely touched:**
  `apps/web/lib/api/domains/kubernetes-api.ts`, its test,
  `apps/web/lib/executor-icons.ts`, its test,
  `apps/web/hooks/domains/session/use-task-environment.ts`,
  `apps/web/components/task/executor-environment-status.ts`, its test,
  `apps/web/components/task/executor-environment-info.tsx` and a focused test,
  `apps/web/components/task/executor-settings-button.tsx`, its test, an optional
  extracted disclosure-content component,
  `apps/web/components/task/mobile/session-mobile-top-bar.tsx` and focused test,
  the two Kubernetes settings headers,
  `apps/web/components/settings/profile-edit/profile-connection-settings-action.tsx`,
  and affected six-locale `task.json`/`executors.json` catalogs.
- **Dependencies:** Task 01 exact filter contract.
- **Parallelism:** sequential.
- **Inputs:** spec What/API/scenarios; plan Frontend section;
  `TaskDependencyChip` coarse-pointer Drawer pattern; existing Kubernetes
  session normalizer and settings route helper.
- **Output contract:** report RED/GREEN evidence, accessibility/mobile checks,
  files changed, commands/counts, blockers/risks, and synchronize task/plan
  status.

## Results

- RED: focused API, status, disclosure, icon, and mobile-topbar tests first
  failed because the exact Kubernetes client and disclosure state did not
  exist, Kubernetes inherited the persisted `ready` state, and mobile exposed
  only the old tooltip-only remote indicator.
- GREEN: the focused Vitest command passed 7 files and 36 tests. Full web lint,
  web typecheck, Traditional Chinese and pseudo-locale generation,
  `i18n:check`, and `i18n:ratchet` all passed; the ratchet reported 0 added and
  30 modified violations. The focused icon search found no remaining
  `IconCloudComputing` Kubernetes presentation.
- Added exact task/session status loading with stale-response protection,
  Kubernetes-specific status resolution and Pod fields, Refresh and canonical
  settings actions, a shared desktop/touch disclosure, a named 44 px mobile
  Drawer trigger, and central cube/cube-off icons. Kubernetes intentionally
  omits the generic Reset environment action.
