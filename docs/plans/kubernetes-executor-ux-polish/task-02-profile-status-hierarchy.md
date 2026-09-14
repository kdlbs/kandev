---
id: "02-profile-status-hierarchy"
title: "Profile status hierarchy"
status: complete
wave: 2
depends_on: ["01-task-disclosure"]
plan: "plan.md"
spec: "../../specs/kubernetes-executor/spec.md"
---

# Task 02: Profile status hierarchy

## Acceptance

- Every Kubernetes profile page leads with a cluster-status section containing
  persisted executor context, the current-draft connection/admission test,
  executor-wide active sessions, and an Edit/View cluster connection action.
- Administrators can test and edit the destination; members see sanitized
  sessions and the read-only destination while the test remains visibly
  disabled with its existing explanation.
- The raw PodTemplate textarea grows and shrinks after hydration, typing, and
  discard/reset, owns no vertical scrollbar, and contains long unwrapped lines
  without document overflow.

## Files likely touched

- `apps/web/app/settings/executors/[profileId]/page.tsx`
- `apps/web/components/settings/kubernetes-profile-cluster-section.tsx` (new)
- `apps/web/components/settings/kubernetes-profile-cluster-section.test.tsx` (new)
- `apps/web/components/settings/kubernetes-profile-sections.tsx`
- `apps/web/components/settings/kubernetes-workload-card.tsx`
- `apps/web/components/settings/kubernetes-workload-card.test.tsx` (new)
- `apps/web/components/settings/profile-edit/profile-connection-settings-action.tsx`
- `apps/web/components/settings/profile-edit/profile-edit-page-chrome.test.tsx`
- `apps/web/src/locales/en/executors.json`
- `apps/web/src/locales/pt-pt/executors.json`
- `apps/web/src/locales/zh-cn/executors.json`
- `apps/web/src/locales/zh-hk/executors.json`
- `apps/web/src/locales/zh-tw/executors.json`
- `apps/web/src/locales/pseudo/executors.json`

## Dependencies

Task 01. Its finalized action hierarchy and mobile language are the visual
baseline for the settings section.

## Parallelism

`sequential`. The page composition, diagnostic ownership, session hook, action
label, textarea behavior, and locale catalogs form one user-visible route.

## Inputs

- Spec `What`: first-class profile cluster status and content-sized YAML.
- Spec scenarios: admin profile entry, member boundary, current draft testing,
  active sessions, and short/long YAML.
- Existing `KubernetesDiagnosticsCard`, `KubernetesSessionsCard`,
  `useKubernetesDiagnostics`, and `useKubernetesSessions`.
- Settings save coordinator boundary in ADR 0046; connection edits remain on
  the executor route, and the profile contributor remains the only profile
  persistence owner.

## TDD sequence

1. Add a profile-cluster-section test proving diagnostics/sessions are absent
   from the direct profile surface and the action is only a header escape;
   record the expected RED failure.
2. Add the leading composition, move diagnostic ownership out of
   `KubernetesProfileSections`, and verify current draft payloads plus
   admin/member action labels.
3. Add a controlled textarea regression with mocked `scrollHeight`; prove the
   current fixed-floor/CSS-only implementation has no inline grow/shrink
   behavior, then add the measured fallback and overflow contract.
4. Generate locale catalogs, run the exact checks below, and synchronize task
   and plan results.

## Verification

```bash
cd apps && pnpm install --frozen-lockfile && pnpm --filter @kandev/web test -- components/settings/kubernetes-profile-cluster-section.test.tsx components/settings/kubernetes-workload-card.test.tsx components/settings/profile-edit/profile-edit-page-chrome.test.tsx components/settings/kubernetes-settings-cards.test.tsx && pnpm --filter @kandev/web lint -- 'app/settings/executors/[profileId]/page.tsx' components/settings/kubernetes-profile-cluster-section.tsx components/settings/kubernetes-profile-cluster-section.test.tsx components/settings/kubernetes-profile-sections.tsx components/settings/kubernetes-workload-card.tsx components/settings/kubernetes-workload-card.test.tsx components/settings/profile-edit/profile-connection-settings-action.tsx components/settings/profile-edit/profile-edit-page-chrome.test.tsx && cd web && pnpm run i18n:zh-hant && pnpm run i18n:pseudo && pnpm run i18n:check && pnpm run i18n:ratchet && pnpm run typecheck && git diff --check
```

## Output contract

Report the RED hierarchy/autosize evidence, admin/member behavior, exact
diagnostic payload and session scope, files changed, locale generation, exact
commands and counts, blockers, risks, and synchronized task/plan status.
External side effects: none.

## Results

- RED: the profile cluster-section suite could not resolve the absent component;
  the PodTemplate sizing test observed an empty inline height instead of the
  measured `96px` and no horizontal-only overflow contract.
- GREEN: Kubernetes profiles now render a leading Cluster status composition
  before the editing fieldset. It owns persisted executor context,
  current-draft diagnostics, executor-wide session polling, and the
  permission-appropriate Edit/View cluster connection action. The old header
  escape and duplicate profile-section diagnostics were removed.
- The controlled PodTemplate textarea resets and measures `scrollHeight` on
  every committed YAML value, grows from a compact minimum, shrinks on profile
  replacement/discard, disables vertical resize/scroll, and retains contained
  unwrapped horizontal scrolling.
- `pnpm --filter @kandev/web test -- components/settings/kubernetes-profile-cluster-section.test.tsx components/settings/kubernetes-workload-card.test.tsx components/settings/profile-edit/profile-edit-page-chrome.test.tsx components/settings/kubernetes-settings-cards.test.tsx`: 4 files, 15 tests passed.
- Focused ESLint, `pnpm --filter @kandev/web typecheck`, `i18n:check`,
  `i18n:ratchet`, and `git diff --check` passed. Traditional Chinese and pseudo
  catalogs were regenerated; all six catalogs contain the new copy.
- Blockers: none. External side effects: none.
