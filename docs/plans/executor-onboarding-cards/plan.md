---
created: 2026-09-25
status: done
requirements:
  - REQ-EXECUTORS-ONBOARDING-001
  - REQ-UI-FIRST-RUN-DIALOG-001
system_design:
  - ../../specs/executors/system-design/first-run-discovery.md
  - ../../specs/ui/system-design/first-run-dialog-availability.md
legacy_specs: []
---

# Implementation plan: Desktop first-run tour and executor cards

## Overview

Keep the entire first-run tour off phones without consuming its completion marker. On larger screens, replace the four-item executor list with six compact information cards. Show the setup each type needs and recommend Worktree for an existing repository. Two work orders separate the dialog availability rule from executor content.

The [UI requirement](../../specs/ui/requirements/first-run-dialog-availability.md) and [design](../../specs/ui/system-design/first-run-dialog-availability.md) define when the tour appears. The [executor requirement](../../specs/executors/requirements/first-run-discovery.md) and [design](../../specs/executors/system-design/first-run-discovery.md) define the content inside its executor step.

## Scope

### In scope

- Local, Worktree, Local Docker, Sprites.dev, SSH, and Kubernetes cards.
- Static prerequisite labels that do not claim live readiness.
- Accurate Worktree, Local, Docker, executor, and profile explanations.
- A public executor-guide link and Settings > Executors direction.
- Suppress the entire tour below 768 CSS pixels without marking it complete, including resize transitions.
- Desktop composition, localization, accessibility, and regression coverage.
- Update the first-run paragraph in `docs/public/use-kandev.md` for the new tour content.

### Out of scope

- Remote Docker, `mock_remote`, runtime changes, profile creation, or connection probes.
- Changes to task executor defaults, task creation, other tour-step content, or Office onboarding.
- Additional onboarding screens for other features.
- Cross-install or cross-browser persistence changes to the existing first-run tour.

## Technical approach

Gate the `OnboardingDialog` in `apps/web/app/page-client.tsx` with the existing responsive hook's `isMobile` value. Keep it mounted while the tour is unfinished and pass `open=false` below 768 CSS pixels, so viewport changes do not reset its step or dirty profile edits. When it reopens, refresh live agent data while retaining dirty in-memory settings. A phone visit does not write the completion marker. Preserve the current completion and profile-save handlers for actual desktop actions.

Update `RUNTIMES` and `StepEnvironments` in `apps/web/components/onboarding-dialog.tsx`. Keep the catalog static and explicit, with stable executor IDs and localized keys. Show Worktree first in two columns on supported viewports. Do not add card selection state or card-level navigation. The guide link opens `https://kandev.ai/docs/executors` in a separate tab.

Use the current Settings hub's six offered types as the product reference. Add a focused test that compares the tour's visible types with the operational Settings choices and guards against the legacy Remote Docker type. Keep the existing `OnboardingFooter` transition handlers and `PageClient` completion marker. Apply the local dialog-content containment pattern to the executor view. Do not change the shared Dialog primitive or add an executor API call.

Add new messages to `apps/web/src/locales/en/common.json`, `pt-pt`, `zh-cn`, and `ja`. Generate the `zh-tw` and `zh-hk` values with `pnpm run i18n:zh-hant`, and generate the pseudo-locale with `pnpm run i18n:pseudo`. Run the repository's i18n gates. Remove or reuse old keys according to the catalog checks; do not leave untranslated visible copy. Review the pseudo-locale at a supported viewport.

The public [Get Started](../../public/use-kandev.md) guide describes the current first-run dialog. Update its tour paragraph when the UI changes. The [Executors](../../public/executors.md) guide already owns detailed prerequisites and trust limits; link to it rather than copying that material into the tour.

## ASCII UI preview

### UI-01: Executors step, desktop

Entry: first-run tour, step two. The cards are informational. The body may scroll; the header, progress, and footer stay visible.

```text
+--------------------------------------------------------------------------+
|                                Executors                                 |
| Start with Worktree for most tasks on an existing Git repository.       |
|                                                                          |
|  +-------------------------------+  +-------------------------------+   |
|  | Worktree      RECOMMENDED      |  | Local         BUILT IN        |   |
|  | Separate Git checkout. Shares |  | Runs in its selected folder   |   |
|  | the host account.             |  | on the Kandev host with that  |   |
|  |                               |  | host account's access.        |   |
|  +-------------------------------+  +-------------------------------+   |
|  +-------------------------------+  +-------------------------------+   |
|  | Docker        DAEMON NEEDED   |  | SSH           HOST SETUP      |   |
|  | Run in a local container.     |  | Run on a trusted remote       |   |
|  | Review mounts and access.     |  | machine over SSH.             |   |
|  +-------------------------------+  +-------------------------------+   |
|  +-------------------------------+  +-------------------------------+   |
|  | Sprites       PROVIDER SETUP  |  | Kubernetes    ADMIN SETUP     |   |
|  | Run in a remote sandbox.      |  | Run in a cluster Pod.         |   |
|  | Provider account required.    |  | Cluster access required.      |   |
|  +-------------------------------+  +-------------------------------+   |
|                                                                          |
| Executors choose where work runs. Profiles store reusable settings.     |
| Choose a profile when starting a task. Configure it in Settings >       |
| Executors.                                      [View executor guide]     |
|                                                                          |
| [Skip]                           o  O  o  o                 [Back] [Next] |
+--------------------------------------------------------------------------+
```

### UI-02: Phone visit

Entry: a user opens Kandev on a phone. The normal page appears without any first-run dialog. This visit leaves the completion marker unset, so an unfinished tour can appear later on desktop or tablet.

```text
+--------------------------------+
| Kandev                    [menu] |
|                                |
| Your workspace                 |
|                                |
|          Normal phone UI         |
|                                |
|          No tour overlay         |
|                                |
+--------------------------------+
```

UI-01 covers the executor card criteria. UI-02 illustrates `AC-UI-FIRST-RUN-DIALOG-001.1` and `.2`. The phone page details are illustrative; the test verifies tour absence and normal page access rather than these exact labels.

## Tests

| Criteria        | Evidence                                                                                                                                                                 |
| --------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| `.1`            | Component test renders exactly the six supported cards and excludes Remote Docker and mock types. Catalog test compares their IDs with the Settings hub's offered types. |
| `.2` to `.5`    | Component assertions cover Worktree/Local boundaries, Docker wording, prerequisite labels, profile explanation, Settings direction, and guide URL.                       |
| `.7` and `.8`   | Component test checks informational semantics, locale switching, and Back/Next/Skip without executor mutations or changed agent-profile save behavior.                   |
| UI `.1` to `.5` | `PageClient` component and browser tests cover phone suppression, marker preservation, resize, completed state, and unchanged desktop actions.                           |

Use `apps/web/components/onboarding-dialog.test.tsx` for the focused component cases. Keep the existing stale-save cases.

## E2E tests

- `apps/web/e2e/tests/office/onboarding-executors.spec.ts` in the `chromium` project: open the first-run dialog, advance to Executors, check six cards and two-column geometry, inspect the guide link, and continue to Workflows (`.1` to `.5`, `.6`, `.8`).
- Replace the expectations in `apps/web/e2e/tests/office/mobile-onboarding-dialog.spec.ts` and `mobile-onboarding-dialog-rich.spec.ts` in `mobile-chrome`: the normal page is usable, the entire dialog stays absent, and the completion marker remains unset. Resize to 768 CSS pixels or wider and confirm the unfinished tour opens (UI `.1` to `.4`).

The runner builds the production Vite bundle before these checks.

## Work orders

- [x] [Task 01: Keep the first-run dialog off phones](task-01-phone-availability.md) (done)
- [x] [Task 02: Present supported executors in onboarding](task-02-executor-cards.md) (done; after Task 01)

## Verification results

Implementation and PR-fixup checks passed: 32 focused component tests, the TypeScript check, the pseudo-locale production build, all localization gates, targeted ESLint and Prettier checks, one desktop executor E2E test, two mobile onboarding E2E tests, 62 public-doc tests, public-doc and specification validation, specification lint, and `git diff --check`. Regression coverage confirms a dirty profile survives desktop-to-phone-to-desktop resizing and resource refetch while the profile remains the same, replaced or removed profiles cannot be saved with stale IDs, duplicate save actions are suppressed, and unexpected save errors keep the user on the current step with a localized message.

## Risks

- A static catalog can drift from Settings. A parity test must fail when an operational type is added or removed.
- Long translations can make cards or the footer exceed a supported viewport. Browser geometry and pseudo-locale checks must cover the scroll boundary.
- Suppressing the dialog on phones must not write the completion marker or save an unfinished agent profile. The resize tests must cover both directions.
- The tour must not report a seeded Docker row or disabled Sprites row as a working connection.
- The Worktree recommendation must not override an explicit workspace default or the Local choice for a new repository.
