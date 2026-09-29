---
status: current
system: executors
requirements:
  - REQ-EXECUTORS-ONBOARDING-001
---

# First-run executor discovery system design

## Purpose and boundaries

The first-run executor step is a static explanation of product executor choices. The executor system owns the facts shown on this step. The existing `OnboardingDialog` owns tour navigation and browser-local completion. Task creation continues to own profile selection and defaults.

The six supported types are `local`, `worktree`, `local_docker`, `sprites`, `ssh`, and `k8s`. `remote_docker` remains absent because its runtime cannot create or stop an environment. `mock_remote` is test-only. The Settings executor hub provides the current product catalog against which the tour must stay aligned.

## Requirement mapping

| Requirement                    | Design sections                                                                                                                                          |
| ------------------------------ | -------------------------------------------------------------------------------------------------------------------------------------------------------- |
| `REQ-EXECUTORS-ONBOARDING-001` | [Content model](#content-model), [Dialog composition](#dialog-composition), [Navigation and state](#navigation-and-state), [Verification](#verification) |

## Content model

The executor step uses a fixed, ordered presentation model. Worktree leads because it is the recommended starting point for an existing repository. Each card has a stable type ID, icon, localized label, localized description, and localized prerequisite or recommendation label. The IDs remain untranslated; copy resolves at render time through `useTranslation()`.

The text distinguishes product support from live readiness. Local and Worktree are built in; a new database also has a Local Docker executor row, but a task still needs a usable Docker daemon. Sprites starts disabled. SSH and Kubernetes need user or administrator setup. No static label reports a connection as healthy. For Worktree, say "recommended for an existing repository" rather than "always the default": a workspace default or source type can choose another executor.

The Worktree card names its Git checkout boundary and shared host account. The Local card says that work runs in the selected folder on the Kandev host, with access through that host account. Docker copy names a container and the need to review mounts and access. The remote cards identify the external host, provider, or cluster. One short note explains that an executor chooses where work runs, while an executor profile stores reusable settings. A second note points to **Settings > Executors** and explains that the user chooses a profile when starting a task. The existing public [executor guide](../../../public/executors.md) supplies detailed setup and security guidance through a labeled link.

Use the existing localization catalogs in `apps/web/src/locales/`. Add every new user-facing message in English, Portuguese, Simplified Chinese, Traditional Chinese, and Japanese; generate both Traditional Chinese catalogs and the pseudo-locale with the repository commands. Keep brand and protocol names as defined by the existing executor label helpers. Do not use an English fallback message in product markup.

## Dialog composition

`apps/web/components/onboarding-dialog.tsx` retains four steps and owns the executor step. The current bordered rows become compact informational cards in two columns inside the existing dialog width. The card body has no selection state or card-level click handler; only the guide link is interactive. The [UI availability design](../../ui/system-design/first-run-dialog-availability.md) keeps the entire dialog off phones.

Apply the existing dialog-content containment pattern locally: cap the dialog at the viewport, make its body the single scroll owner, and keep title, step progress, Back, Next, and Skip outside that scroll region. The body has `min-height: 0` and auto overflow. This retains the current centered dialog primitive and its focus management on supported viewports.

Long translations wrap inside cards without clipping or document-level horizontal overflow.

## Navigation and state

No executor API, store field, or readiness probe is added. Opening and reading the step does not mutate an executor or task. The guide link opens `https://kandev.ai/docs/executors` in a separate browser tab without changing wizard state. The Settings path is instructional copy, not a second configuration flow inside the tour.

The existing `OnboardingFooter`, `handleNext`, `handleBack`, `handleSkip`, and `handleGetStarted` remain the only wizard transition path. On reopening, preserve dirty form data only when the refreshed profile has the same ID; use fresh settings when a profile has been replaced or removed. Save requests are single-flight, and footer actions stay disabled while a save is pending. Handled backend-reload errors keep their existing path. Other save errors show a localized toast and leave the current step open for retry. The completion marker stays browser-local through `PageClient`.

## Verification

- Component coverage checks the six visible choices, their information-only semantics, Worktree and Docker guidance, prerequisite text, guide link, and unchanged wizard navigation.
- A catalog regression check compares the tour's supported set with the Settings hub's product choices. It excludes the legacy `remote_docker` route and test-only types.
- Chromium Playwright coverage opens the first-run dialog and checks the two-column executor view, visible guidance, and progression.
- The UI-owned mobile Playwright coverage confirms that the entire first-run dialog stays hidden on phones and does not consume its completion marker.
- Run the locale completeness and pseudo-locale gates. Check the rendered card layout with longer translated copy on a supported viewport.

## Related decisions

- [Task executor defaults](../../../decisions/2026-08-01-repository-task-executor-defaults.md) explain why the Worktree guidance is a recommendation, not a universal default.

No new architectural decision is needed. This change alters a static tour presentation within existing executor and dialog boundaries.
