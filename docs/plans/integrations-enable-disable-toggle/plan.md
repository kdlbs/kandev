---
spec: docs/specs/integrations/requirements/enable-disable-toggle.md
created: 2026-08-06
status: complete
---

# Implementation Plan: Integration Enable/Disable Toggle & Nav Visibility

## Overview

Three new `localStorage`-backed hooks (`useAzureDevOpsEnabled`, `useGitHubEnabled`,
`useGitLabEnabled`) bring Azure DevOps, GitHub and GitLab up to parity with
Jira/Linear/Sentry/Slack's existing enable/disable toggle (Task 01). The
existing `<DraftedIntegrationEnabledControl>` is wired onto those three
integrations' own settings pages (Task 02) and onto every integration's row
on the `/settings/integrations` index page, which requires a small DOM
restructure of the index-page cards so the slider is never nested inside the
card's navigating `<a>` (Task 03). A new `useHideDisabledIntegrationsInNav`
hook backs a new index-page toggle row, default off (Task 04). Finally
`useNavAvailability` is changed to gate left-panel nav visibility on
"configured" independent of "enabled" by default, only folding in "enabled"
when the new toggle is on (Task 05). E2E coverage proves the index-page
sliders and the nav-hiding behavior end to end (Task 06).

Order follows the dependency chain: the enabled hooks (01) are needed before
anything can display or persist a slider (02, 03, 04); the nav-availability
rewire (05) depends on the enabled hooks from 01 (and 04's hide-disabled hook)
existing; E2E (06) exercises 02–05 together.

---

## Frontend

### Task 01 — Enabled hooks for Azure DevOps, GitHub, GitLab

New files, each a one-line wrapper over the existing generic
`useIntegrationEnabled` (`apps/web/hooks/domains/integrations/use-integration-enabled.ts`),
mirroring `apps/web/hooks/domains/jira/use-jira-enabled.ts`:

- `apps/web/hooks/domains/azure-devops/use-azure-devops-enabled.ts`
  - `STORAGE_KEY = "kandev:azure-devops:enabled:v1"`
  - `LEGACY_KEY_PREFIX = "kandev:azure-devops:enabled:"` (no real legacy data;
    prefix kept only for signature symmetry with `useIntegrationEnabled`,
    which requires it — the migration scan will simply find nothing).
  - `SYNC_EVENT = "kandev:azure-devops:enabled-changed"`
  - exports `useAzureDevOpsEnabled()`
- `apps/web/hooks/domains/github/use-github-enabled.ts` — same shape,
  `kandev:github:enabled:v1` / `kandev:github:enabled:` /
  `kandev:github:enabled-changed`, exports `useGitHubEnabled()`
- `apps/web/hooks/domains/gitlab/use-gitlab-enabled.ts` — same shape,
  `kandev:gitlab:enabled:v1` / `kandev:gitlab:enabled:` /
  `kandev:gitlab:enabled-changed`, exports `useGitLabEnabled()`

### Task 02 — Own-page slider for Azure DevOps, GitHub, GitLab

New wrapper components, one per integration, mirroring
`apps/web/components/jira/jira-enabled-control.tsx`:

- `apps/web/components/azure-devops/azure-devops-enabled-control.tsx` —
  `AzureDevOpsEnabledControl()` renders
  `<DraftedIntegrationEnabledControl id="azure-devops" enabled={enabled} persist={setEnabled} />`
  using `useAzureDevOpsEnabled()`.
- `apps/web/components/github/github-enabled-control.tsx` — same shape for
  GitHub, `id="github"`, using `useGitHubEnabled()`.
- `apps/web/components/gitlab/gitlab-enabled-control.tsx` — same shape for
  GitLab, `id="gitlab"`, using `useGitLabEnabled()`.

Wire each into its integration's top connection `<SettingsSection>` via the
existing `action` prop (`apps/web/components/settings/settings-section.tsx`
already supports `action?: ReactNode`, used today by
`apps/web/components/jira/jira-settings.tsx:576`):

- `apps/web/components/azure-devops/azure-devops-settings.tsx` —
  `AzureDevOpsConnectionSection`'s `<SettingsSection>` (around line 489) gets
  `action={<AzureDevOpsEnabledControl />}`.
- `apps/web/components/github/github-settings.tsx` —
  `GitHubConnectionSection`'s `<SettingsSection>` (around line 263) gets
  `action={<GitHubEnabledControl />}`.
- `apps/web/components/gitlab/gitlab-settings.tsx` —
  `GitLabConnectionSection`/`GitLabConnectionCard`'s top `<SettingsSection>`
  (around line 487–526; confirm exact section in-task, since the file was only
  partially read during planning) gets
  `action={<GitLabEnabledControl />}`.

### Task 03 — Per-row slider on the integrations index page

File: `apps/web/app/settings/integrations/page.tsx`.

Current structure wraps the whole `<Card>` in a `<Link>` so the entire card
navigates on click (`integrationCards` in
`apps/web/e2e/tests/integrations/integrations-index-layout-helpers.ts` locates
cards via `a[href*="/integrations/"] > [data-slot="card"]`). Nesting an
interactive `<Switch>` inside that anchor is invalid DOM (interactive-in-
interactive) and fragile for click-through. Restructure so the `<Card>` is
the outer element (not itself inside an `<a>`), and only the label/description
region is an anchor, with the slider as a sibling in a header row — never
inside any anchor's subtree:

```tsx
<Card key={slug} data-testid={`integration-card-${slug}`} className="h-full w-full transition-colors hover:border-primary/40">
  <CardContent className="flex h-full flex-col gap-2">
    <div className="flex items-center justify-between gap-2">
      <Link href={href} className="flex items-center gap-2 text-base font-semibold hover:underline">
        <Icon className="h-5 w-5" />
        {label}
      </Link>
      <IntegrationEnabledRowControl slug={slug} />
    </div>
    <Link href={href} className="text-sm text-muted-foreground">
      {t(descriptionKey, { trigger: "!kandev" })}
    </Link>
  </CardContent>
</Card>
```

`IntegrationEnabledRowControl` is a small local switch component (in the same
file, or a new `apps/web/components/integrations/integration-enabled-row-control.tsx`
if reused elsewhere) that maps `slug` to the right `useXEnabled()` hook via a
lookup table (`azure-devops` → `useAzureDevOpsEnabled`, `github` →
`useGitHubEnabled`, `gitlab` → `useGitLabEnabled`, `jira` → `useJiraEnabled`,
`linear` → `useLinearEnabled`, `sentry` → `useSentryEnabled`, `slack` →
`useSlackEnabled`) and renders
`<DraftedIntegrationEnabledControl id={slug} enabled={enabled} persist={setEnabled} />`.
Hooks cannot be selected dynamically at runtime (rules of hooks), so
`IntegrationEnabledRowControl` must be one small component per slug internally
(a `switch (slug)` returning seven `<DraftedIntegrationEnabledControl>` calls,
each backed by its own statically-imported `useXEnabled` hook) — not a single
component that looks up a hook function from a map and calls it conditionally.

Update `apps/web/e2e/tests/integrations/integrations-index-layout-helpers.ts`:
`integrationCards()` currently derives cards from
`a[href*="/integrations/"]` and asserts `:scope > [data-slot="card"]`. Change
it to locate cards directly, e.g.
`page.getByTestId("settings-scroll-container").locator('[data-testid^="integration-card-"]')`,
and update `integrationCardIconTopInsets` if the icon is no longer the first
child of `CardContent` in a way that changes its measured offset (it still is
— the header row is the first child of `CardContent`, and the icon is the
first child of that row's `<Link>`, so `card.locator("svg").first()` still
resolves to the same icon and the existing `MAX_ICON_TOP_INSET_PX` assertion
should still hold; verify in-task and adjust the constant only if the new
layout genuinely shifts it).

### Task 04 — "Hide disabled integrations from left panel navigation" setting

New hook: `apps/web/hooks/domains/integrations/use-hide-disabled-integrations-in-nav.ts`

```ts
"use client";
// Same useSyncExternalStore + storage-event + custom-event shape as
// use-integration-enabled.ts, but standalone (not per-integration, no legacy
// migration) and defaults to false instead of true.
const STORAGE_KEY = "kandev:integrations:hideDisabledInNav:v1";
const SYNC_EVENT = "kandev:integrations:hide-disabled-in-nav-changed";
export function useHideDisabledIntegrationsInNav(): {
  hideDisabled: boolean;
  setHideDisabled: (next: boolean) => void;
};
```

Add a new row to `apps/web/app/settings/integrations/page.tsx`, below the
integration cards grid, using the existing `useDraftedIntegrationEnabled` +
a plain `<Switch>` row (mirroring
`apps/web/components/settings/archive-confirmation-settings.tsx`'s row
layout, but inline in this page rather than a separate `SettingsCard`, since
the index page is not yet composed of `SettingsCard`s):

```tsx
function HideDisabledIntegrationsSetting() {
  const { t } = useTranslation();
  const { hideDisabled, setHideDisabled } = useHideDisabledIntegrationsInNav();
  const draft = useDraftedIntegrationEnabled({
    id: "integrations-hide-disabled-in-nav",
    enabled: hideDisabled,
    persist: setHideDisabled,
  });
  return (
    <div className="flex min-h-11 items-center justify-between gap-4 rounded-lg border p-4">
      <div className="min-w-0 space-y-0.5">
        <Label htmlFor="hide-disabled-integrations-in-nav">
          {t("settings:hideDisabledIntegrationsFromNav")}
        </Label>
        <p className="text-xs text-muted-foreground">
          {t("settings:hideDisabledIntegrationsFromNavDescription")}
        </p>
      </div>
      <Switch
        id="hide-disabled-integrations-in-nav"
        checked={draft.enabled}
        data-settings-dirty={draft.isDirty}
        onCheckedChange={draft.setEnabled}
        className="shrink-0 cursor-pointer"
      />
    </div>
  );
}
```

Add new locale keys to `apps/web/src/locales/en/settings.json`:
- `hideDisabledIntegrationsFromNav`: `"Hide disabled integrations from left panel navigation"`
- `hideDisabledIntegrationsFromNavDescription`: `"When on, a disabled integration is removed from the sidebar and mobile menu even if it's still configured."`

Run the project's i18n tooling for the pseudo-locale/ratchet as part of this
task's verification (see Task 04's own file for the exact command); do not
hand-author the pseudo-locale entry.

### Task 05 — Decouple left-panel nav visibility from "enabled"

File: `apps/web/hooks/use-nav-availability.ts`.

Replace the `useJiraAvailable`/`useLinearAvailable` imports with
`useJiraAuthed`/`useLinearAuthed` (already exported by
`hooks/domains/jira/use-jira-availability.ts` and
`hooks/domains/linear/use-linear-availability.ts` respectively — these are
the existing "configured and healthy" signal, independent of the enabled
toggle). Add the three new `useXEnabled` hooks from Task 01 plus the existing
`useJiraEnabled`/`useLinearEnabled`, plus `useHideDisabledIntegrationsInNav`
from Task 04. Compute each of the five nav-gated keys as:

```ts
const configured = { /* azure-devops, github, gitlab, jira, linear — unchanged signals */ };
const enabled = { /* one useXEnabled().enabled per key */ };
const { hideDisabled } = useHideDisabledIntegrationsInNav();
const visible = (key: AvailabilityKey) => configured[key] && (!hideDisabled || enabled[key]);
return {
  "azure-devops": visible("azure-devops"),
  github: visible("github"),
  gitlab: visible("gitlab"),
  jira: visible("jira"),
  linear: visible("linear"),
};
```

`azure-devops`'s `configured` signal stays `useAzureDevOpsAvailable(scopedWorkspaceId)`
(unchanged import/name — it is already the pure "authed" signal today, since
Azure DevOps had no enabled toggle before this feature). `github`'s
`configured` signal stays `getGitHubIntegrationStatus(status, loading).ready`
(unchanged). `gitlab`'s stays `useGitLabAvailable()` (unchanged). Do not
rename or change these three existing hooks/consumers — they are reused
unmodified by other surfaces (e.g. `apps/web/components/gitlab/mr-topbar-button.tsx`)
that must keep gating purely on configured/health, never on the new
navigation-only "enabled" concept (see spec's Out of scope).

Update existing tests that mock the hooks this file imports:
- `apps/web/hooks/use-nav-availability.test.ts` — currently mocks
  `useJiraAvailable`/`useLinearAvailable`; change mocks to
  `useJiraAuthed`/`useLinearAuthed`, add mocks for the three new `useXEnabled`
  hooks, `useJiraEnabled`, `useLinearEnabled`, and
  `useHideDisabledIntegrationsInNav`. Add new test cases per the Tests
  section below.
- `apps/web/components/integrations/integrations-menu.test.ts` — currently
  mocks `useGitLabAvailable`/`useJiraAvailable`/`useLinearAvailable` at the
  hook level consumed transitively through `useNavAvailability`; update its
  mocks the same way (switch Jira/Linear mocks to the `Authed` hooks; add the
  enabled/hide-disabled mocks it now transitively needs, defaulting
  `hideDisabled` to `false` so existing assertions keep passing unchanged).

---

## Tests

- **What:** `useAzureDevOpsEnabled`/`useGitHubEnabled`/`useGitLabEnabled`
  default to `true`, persist writes, and broadcast their sync event.
  **File:** one `*.test.ts` per hook beside it (e.g.
  `apps/web/hooks/domains/azure-devops/use-azure-devops-enabled.test.ts`).
  **How:** unit test with `renderHook`, mirroring
  `apps/web/hooks/domains/jira/use-jira-enabled.ts`'s existing test if one
  exists (check first; if the generic `useIntegrationEnabled` already has
  full coverage in `use-integration-enabled.test.ts`, a thin smoke test per
  wrapper — default true, `setEnabled` round-trips — is sufficient; do not
  duplicate the generic hook's exhaustive cases).
- **What:** `useHideDisabledIntegrationsInNav` defaults to `false`, persists,
  broadcasts a same-tab sync event, and degrades to `false` when
  `localStorage` throws. **File:**
  `apps/web/hooks/domains/integrations/use-hide-disabled-integrations-in-nav.test.ts`.
  **How:** unit test with `renderHook`, mirroring
  `use-integration-enabled.ts`'s own test suite structure but asserting the
  opposite default.
- **What:** the index page renders a slider per integration row that
  reflects and updates that integration's stored enabled state, and clicking
  the slider does not navigate. **File:**
  `apps/web/app/settings/integrations/page.test.tsx` (new).
  **How:** component test with Testing Library; mock each `useXEnabled` hook,
  render the page, assert seven switches present with correct initial
  `checked`, click one and assert its `persist`/`setEnabled` was called and
  `window.location`/router navigation was not triggered.
- **What:** the new "hide disabled" row renders default-off, and toggling it
  calls `setHideDisabled` only after the settings save action fires (drafted,
  not immediate) — consistent with `useDraftedIntegrationEnabled`'s existing
  contract. **File:** same `page.test.tsx` as above.
  **How:** component test, mirroring
  `apps/web/components/integrations/use-drafted-integration-enabled.test.tsx`'s
  assertions.
- **What:** `useNavAvailability` returns `true` for a configured-but-disabled
  integration when `hideDisabled` is `false` (default), and `false` when
  `hideDisabled` is `true`; an unconfigured integration stays `false`
  regardless of `hideDisabled`/`enabled`. **File:**
  `apps/web/hooks/use-nav-availability.test.ts` (extend existing describe
  block). **How:** unit test with `renderHook`, table-driven over the five
  nav-gated keys crossed with `{configured, enabled, hideDisabled}` per the
  spec's Scenarios section.
- **What:** `AzureDevOpsEnabledControl`/`GitHubEnabledControl`/`GitLabEnabledControl`
  render `<DraftedIntegrationEnabledControl>` wired to the right hook.
  **File:** one `*.test.tsx` per control, or omit if
  `drafted-integration-enabled-control.test.tsx`-style coverage of the
  generic component already makes a thin wrapper test low-value — decide
  in-task; if omitted, cover the wiring through the E2E test in Task 06
  instead (own-page slider scenario) and state that explicitly in the task's
  Results.

---

## E2E Tests

> Files land under `apps/web/e2e/tests/integrations/`.

- **Scenario:** GIVEN the integrations index page, WHEN it renders, THEN
  every one of the seven rows shows a slider, and toggling GitHub's slider
  flips its own settings-page slider to the same state (cross-page sync,
  same mechanism proven for Jira today) — restated from the spec's "index
  page ... every row ... slider" and "keeps both locations in sync"
  scenarios. **File:**
  `apps/web/e2e/tests/integrations/integrations-index-enabled-toggle.spec.ts`
  (new). **What to verify:** switch role + accessible name per row; toggling
  does not navigate (`expect(page).toHaveURL(...)` unchanged); the GitHub
  settings page slider reflects the new state after navigating there.
- **Scenario:** GIVEN GitHub is configured/healthy and disabled, and "hide
  disabled" is off (default), WHEN the sidebar renders, THEN GitHub's nav
  entry is visible; WHEN the user turns "hide disabled" on, THEN GitHub's
  nav entry disappears without a reload; WHEN the user re-enables GitHub,
  THEN it reappears. **File:**
  `apps/web/e2e/tests/integrations/hide-disabled-integrations-nav.spec.ts`
  (new). **What to verify:** `apiClient.mockGitHubSetUser(...)` (existing
  fixture) to make GitHub configured/healthy; sidebar `getByRole("link",
  {name: "GitHub"})` visible/hidden assertions before/after each toggle,
  matching the visibility-check style already used in
  `apps/web/e2e/tests/integrations/mobile-integrations-nav.spec.ts`.
- Update `apps/web/e2e/tests/integrations/integrations-index-layout.spec.ts`
  only if Task 03's DOM restructure changes the measured layout invariants;
  otherwise it should keep passing unmodified against the updated helper
  from Task 03.

---

## Verification Results

All six tasks completed; full detail lives in each task file's `## Results`
section. Summary:

- Unit/component tests: 80 passed across 11 vitest files (task-01 hooks,
  task-02/04 settings-page component tests, task-03/04 index-page component
  tests, task-05 nav-availability + integrations-menu tests, plus the
  existing azure-devops/gitlab/linear/sentry/slack settings suites, all
  re-run green after the new `action` controls were wired in).
- `cd apps/web && pnpm run typecheck` — clean.
- `cd apps/web && pnpm run i18n:check && pnpm run i18n:ratchet` — clean (no
  new orphans, new-code ratchet clean, guard allowlist intact).
- E2E (built locally: `make build-backend`, `make build-web`,
  `make -C apps/backend e2e-plugin-package`):
  `integrations-index-enabled-toggle.spec.ts`,
  `hide-disabled-integrations-nav.spec.ts`, and the pre-existing
  `integrations-index-layout.spec.ts` — `3 passed`.
- Scoped `eslint --max-warnings 0` over every changed `.ts`/`.tsx` file —
  clean (one `sonarjs/no-duplicate-string` warning in the new page test was
  fixed by extracting `ariaChecked()`/`ARIA_CHECKED_TRUE`/`ARIA_CHECKED_FALSE`
  helpers).
- `make fmt` was run repo-wide; the only out-of-scope reformat it produced
  (pre-existing gofmt drift in `apps/backend/internal/agent/runtime/lifecycle/executor_backend.go`,
  unrelated to this feature) was reverted before committing.

---

## Implementation Waves And Parallel Candidates

```text
Wave 1:
- [x] [task-01-enabled-hooks](task-01-enabled-hooks.md)

Wave 2 (parallel candidates — user authorization required):
- [x] [task-02-own-page-sliders](task-02-own-page-sliders.md)
- [x] [task-03-index-page-sliders](task-03-index-page-sliders.md)

Wave 3:
- [x] [task-04-hide-disabled-setting](task-04-hide-disabled-setting.md)

Wave 4:
- [x] [task-05-nav-availability](task-05-nav-availability.md)

Wave 5:
- [x] [task-06-e2e](task-06-e2e.md)
```

Task 02 and 03 touch disjoint files (different integration components vs.
the index page) and are the only pair labeled parallel-safe. Task 04 also
edits `apps/web/app/settings/integrations/page.tsx` (Task 03's file), so it
runs after 03, not alongside it, even though both ultimately land in the
same file. Task 05 depends on the hooks from 01 and the hide-disabled hook
from 04. Task 06 exercises 02–05 together.

---

## Open Questions
(none)
