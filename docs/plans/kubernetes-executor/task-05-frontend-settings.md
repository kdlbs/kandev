---
id: "05-frontend-settings"
title: "Kubernetes settings experience"
status: completed
wave: 3
depends_on: ["03-service-api"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 05: Kubernetes settings experience

- **Acceptance:** admins can create/test/save Kubernetes executors and edit raw
  Pod-template/storage profiles; members see a read-only configuration boundary.
- **Acceptance:** pure parse/serialize/dirty-state/API logic is tested and all
  executor capability/filter/presentation switches recognize `k8s`.
- **Acceptance:** desktop and phone preserve capability parity, visible touch
  actions, one document scroll owner, 44 px primary targets, and no document
  horizontal overflow; all copy is translated in six catalogs.
- **Verification:** `cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- --run && pnpm --filter @kandev/web lint && cd web && pnpm run typecheck && pnpm run i18n:check && pnpm run i18n:ratchet`
- **Files likely touched:** `apps/web/app/settings/executors/**`, focused
  `components/settings/kubernetes-*.tsx`, config serializers/baselines,
  settings API/types, executor capability/presentation files, and six locale catalogs.
- **Dependencies:** Task 03 API contract.
- **Parallelism:** sequential UI contract owner; E2E and docs consume it.
- **Inputs:** spec What/API/Permissions/mobile scenario; mobile UI language;
  SSH create/connection/profile patterns.

## Results

Implemented the Kubernetes settings experience against the final Task 03 HTTP
contract. Administrators can create, edit, delete, and test Kubernetes
executors and profiles; members retain profile use and sanitized active-session
status while every Kubernetes mutation/test surface is visibly read-only or
disabled with an administrator explanation. The shared settings save
coordinator owns executor/profile saves and components reach the backend only
through domain hooks and API clients.

Executor/profile creation is coordinated and compensating: the hook returns
both created identities, the page routes to the canonical executor connection
URL, and a failed profile create deletes the just-created executor. Rejected or
failed rollback paths retain both causes and surface localized, actionable
copy. Member visits to the direct create route remain readable but do not
register dirty state or trap navigation.

The responsive cards cover kubeconfig and in-cluster authentication, a fixed
namespace, request timeout, platform, main container, strict raw PodTemplate
YAML, conditional managed-PVC/emptyDir/existing-claim fields, admission
warnings, causal test steps, and 90-second active-session refresh. Phone and
desktop share state and handlers; phone actions use 44 px targets, YAML/table
content contains its own horizontal overflow, and page shells clip document
overflow without adding a second vertical scroll owner. Executor routes,
breadcrumbs, icons, labels, cleanup copy, task controls, remote-workspace
capabilities, agent credential filtering, and supporting type switches now
recognize `k8s`.

The integration routing audit also closed every legacy entry path. Settings
tree executor/profile rows and `ExecutorProfilesCard` now choose the dedicated
Kubernetes connection route or Kubernetes-aware flat profile editor while SSH
and other executors retain their scoped routes. Direct or bookmarked singular
Kubernetes routes replace themselves before mounting legacy editable controls,
including for members. The unused parallel executor/settings declarations in
`lib/settings/types.ts` were removed after `rg` confirmed that only `Theme`,
`KeyValue`, and `AgentProfile` have consumers; canonical HTTP executor types
remain authoritative.

Kubernetes profile editing participates in the existing remote-executor flow,
including credentials, configuration bundles, and Git identity. Workload and
storage parsing and serialization preserve unrelated profile keys. Saves first
refresh the persisted session inventory and require explicit confirmation when
sessions are active: current connection configuration applies on reconnect,
while workload edits apply to new sessions and the saved workload snapshot
continues to govern existing/replacement Pods. The ready baseline image is
`ghcr.io/kdlbs/kandev:latest`, with localized guidance to pin a tag or digest
for production and to retain the required bootstrap and agent dependencies.

Localization was authored in `en`, `pt-pt`, and `zh-cn`; `zh-hk` and `zh-tw`
were generated with `pnpm run i18n:zh-hant`, and the pseudo catalog was
regenerated with `pnpm run i18n:pseudo`. The final generators reported 17,492
real web messages with zero residual Simplified Chinese warnings and 8,746
pseudo messages.

Verification completed on 2026-08-24:

- `cd apps && pnpm install --frozen-lockfile` passed; the lockfile and install
  were already up to date (pnpm 9.15.9).
- The final strict-TDD command was `cd apps/web && pnpm exec vitest run
  components/settings/kubernetes-config.test.ts
  components/settings/kubernetes-validation.test.ts
  components/settings/kubernetes-create-error.test.ts
  components/settings/kubernetes-save-confirmation.test.ts
  components/settings/kubernetes-settings-cards.test.tsx
  components/settings/profile-edit/serialize-executor-config.test.ts
  components/settings/profile-edit/profile-edit-page-chrome.test.tsx
  hooks/domains/settings/use-kubernetes-settings.test.tsx
  lib/api/domains/kubernetes-api.test.ts src/settings-routes.test.ts`; it passed
  10 files and 86 tests in 35.47 seconds.
- The integration-routing RED run covered the settings tree, profile card, and
  singular route delegation: 5 expected failures and 64 passes. A second RED
  isolated direct/bookmarked routing: the two Kubernetes redirects failed while
  both SSH preservation cases passed. After GREEN and stale-type cleanup,
  `cd apps/web && pnpm exec vitest run
  components/app-sidebar/sections/settings/settings-menu-branches.test.ts
  components/settings/executor-profiles-card.test.tsx
  components/settings/legacy-executor-settings-route.test.tsx
  src/settings-routes.test.ts
  hooks/domains/settings/use-kubernetes-settings.test.tsx` passed 5 files and
  82 tests in 75.88 seconds.
- The completed full assertion run was `cd apps && timeout --signal=TERM
  --kill-after=10s 60m env VITEST_MAX_WORKERS=3 pnpm --filter @kandev/web test
  -- --run --pool=forks --testTimeout=30000 --hookTimeout=30000
  --dangerouslyIgnoreUnhandledErrors`; Vitest 4.1.10 exited 0 after 1,543 files,
  13,053 passing tests, 4 skipped tests, and 1,656.98 seconds. It still reported
  three unrelated Monaco lazy-import `EnvironmentTeardownError` diagnostics;
  the identical non-suppressing fork run passed every assertion but exited 1
  solely for three such diagnostics. The thread-pool aggregate passed every
  assertion too but amplified this existing teardown problem to 135 diagnostics.
- Earlier bounded aggregate attempts were stopped rather than left orphaned:
  the default single-worker run exited 143 at the integration reviewer's
  request; 3-worker thread runs reached their 7, 12, and 30 minute guards with
  no Task 05 failure; a 4-worker 20-minute probe hit two unrelated 5-second
  load timeouts. Those two files then passed alone: 2 files and 31 tests in
  58.15 seconds.
- `cd apps && pnpm --filter @kandev/web lint` passed with zero warnings.
- `cd apps/web && pnpm run typecheck` passed (`tsc --noEmit`).
- `cd apps/web && pnpm run i18n:check` passed: 7,228 referenced keys, 8,746
  English entries, 16 existing orphan warnings, pseudo in sync, and all four
  real translated catalogs complete.
- `cd apps/web && pnpm run i18n:ratchet` passed: 0 added and 23 modified source
  files clean; the 639-entry guard allowlist remained intact.
- `cd apps/web && pnpm run build:e2e` passed under Vite 8.0.16 with only the
  repository's existing chunk-size/dynamic-import warnings (14,796 modules,
  9.03 seconds).
- Focused Playwright desktop and Pixel 5 smoke coverage passed: 2 tests in
  10.6 seconds (`chromium` and `mobile-chrome`). E2E files were not edited.
- `git diff --check` passed.
- Per the integration reviewer's instruction, no additional full aggregate was
  started after the routing/type cleanup; the completed aggregate result and
  Monaco teardown caveat above remain the terminal full-suite result.

No commit, push, or pull request was created.
