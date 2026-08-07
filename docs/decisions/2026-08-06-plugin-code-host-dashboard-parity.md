# ADR-2026-08-06-plugin-code-host-dashboard-parity: Code-Host Plugins Reuse Native Dashboard Primitives

**Status:** accepted
**Date:** 2026-08-06
**Area:** frontend, protocol

## Context

GitHub and GitLab dashboards share one interaction model, but their list and toolbar
components remained provider-coupled. The first Bitbucket plugin therefore reproduced
their appearance with plugin CSS while inventing a second dashboard review action and
an intermediate launch dialog. Visual similarity did not preserve the native task and
review workflow.

## Decision

Kandev owns provider-neutral code-host dashboard primitives under
`apps/web/components/integrations/` and exposes them through `host.ui`. GitHub, GitLab,
and compatible plugins render their result list, row, toolbar, scope controls, task
preset menu, linked-task indicator, and semantic change-request icons through those
same host components. Plugins select icon semantics such as open, merged, or closed;
they do not copy SVG paths from first-party providers.

Task presets use the same provider-neutral semantic icon keys as first-party presets.
The host resolves those keys to its Tabler components, so **Review**, **Address
feedback**, and **Fix CI** render the exact eye, message, and tool glyphs everywhere.
Plugins do not pass React icon components or rely on a generic fallback when a native
semantic equivalent exists.

Kandev also owns common task change-request linking anatomy. Code-host plugins invoke
the host task-link dialog contract with provider copy and a submit callback; they do not
rebuild the input, validation, loading state, footer, toast, close behavior, or responsive
surface inside an arbitrary plugin modal. A Link-submenu child names the target only
(for example, **Bitbucket Pull Request**) because the parent already supplies the verb.
The singleton plugin modal host mounts inside `AppShell`'s theme, tooltip, and toast
provider tree so host-owned workflows retain the same runtime contexts as first-party
dialogs.

Runtime React components remain in the Kandev host repository and are delivered by
the versioned `host.ui` contract. They do not move to a separate shared frontend
repository: host-owned React, Radix contexts, theme behavior, and release compatibility
must remain atomic. A future published SDK package may provide TypeScript contracts
and pure build-time helpers once multiple plugin repositories need them, but it must
not duplicate runtime UI implementations.

A code-host dashboard row is an inert result container. Its title opens the provider,
its metadata describes the change request, and its sole creation control is the compact
**Task** preset dropdown. Selecting a preset opens the host's real
`TaskCreateDialog` directly; provider linking runs after successful task creation.
Dashboard rows do not add a **Review** action or a provider-owned intermediate launch
modal. Review begins from linked task context through the existing review-provider
registry on desktop and mobile.

When the selected repository already exists in the workspace, the dialog keeps its
normal REST create transport. For a first-use plugin repository, the plugin may inject
the dialog's provider-neutral `createTask` transport and forward only native task
fields through an authenticated plugin action. The plugin server resolves the pull
request and trusted repository descriptor from its connection before calling Host
`Tasks.Create`; browser input never grants repository-provider authority. A per-dialog
launch identifier makes transport retries idempotent without limiting one pull request
to one task.

Kandev also owns task-scoped change-request status presentation. A review-provider
summary may include normalized status/check data; the host review registry preserves it,
refreshes it through the existing provider lifecycle, and automatically mounts the same
shared control in both first-party locations: the task topbar and the status row above
the chat composer (including passthrough sessions). The host owns the exact trigger,
delayed desktop hover popover, native mobile drawer, accessibility, touch geometry, and
responsive behavior. Providers do not register a visual status slot. They supply data,
refresh it around every 90 seconds, and retain provider API ownership.

The shared status body is the provider-neutral presentation extracted from GitHub's CI
popover. Its common header, grouped checks, review/comment summaries, links, loading,
empty, and error states render identically for built-ins and plugins. Provider-only
controls such as automation or merge actions are optional callbacks/capabilities inside
that anatomy, not alternate markup. Opening Review routes through the registered review
provider on desktop or the current mobile session.

Kandev likewise owns the common change-request detail presentation exposed through
`host.ui.ChangeRequestDetail`. GitHub and compatible review providers map their native
payloads into one normalized detail model and pass supported action callbacks. The host
owns header, branch/state metadata, description, reviews, checks, threaded comments,
action placement, loading/error states, scroll ownership, and responsive behavior.
Providers retain queries, authorization, mutation semantics, capability detection, and
normalization. A plugin may still own a review-panel adapter, but not a parallel visual
design for those common surfaces.

Every successful PR-from-task creation and watch-created task records the same durable
task-to-change-request association exposed by manual linking. Watch-owned links remain
authoritative in watch state and are unioned with manual link state when serving task
review/status and workspace association queries; implementations do not copy them into
a second store merely for presentation.

Task-scoped refresh also performs bounded branch-based discovery, equivalent to
GitHub's external-PR detection. It derives repository and checkout branch only from the
host-verified task, exact-matches an open provider change request's source branch, and
stores the result idempotently. Browser-supplied repository or branch values never
authorize automatic linking, and discovery failure does not hide an existing link.

The native task dialog's branch picker follows the same provider-neutral trust boundary.
For a persisted plugin-owned repository, the host resolves the active manifest owner and
invokes its declared workspace-scoped `repositories.branches` action with a descriptor
derived only from the stored repository row. The action returns normalized branch names;
the browser cannot substitute provider identity, host, namespace, clone URL, or repository
ID. GitHub remains on its first-party service path, while future code-host plugins reuse
this action instead of adding another host-native provider branch.

Provider adapters continue to own API queries, normalization, authorization, task-link
storage, provider data, and capability handling. Shared host components own presentation
and interaction geometry; they accept normalized data and callbacks and contain no
GitHub, GitLab, or Bitbucket API logic.

Pipeline/build status is read from the pull request's source (head) commit. The
destination commit describes the merge target and cannot represent the CI result for
the proposed change.

## Consequences

Code-host plugins require a host release containing these additive `host.ui` exports.
Dashboard fixes now apply uniformly to first-party and plugin providers, including
themes, touch targets, dropdown behavior, loading/error states, and row density.
Provider adapters remain independently releasable, while common review/status visuals
enter the versioned host contract. New semantic icon or presentation needs require an
additive host contract release rather than a plugin-local visual approximation. Plugin
repositories avoid a second runtime UI dependency and its version-skew/release-
coordination cost. Task-link, status, and detail fixes land once in the host and remain
consistent for first-party and plugin providers, without moving provider polling,
credentials, or API payloads into Kandev.

## Alternatives Considered

- Keep copying GitHub/GitLab markup and CSS into each plugin. Rejected because the
  first live implementation already drifted in behavior despite similar styling.
- Export GitHub's `PRList` directly. Rejected because its status hooks, payloads, and
  task associations are GitHub-specific rather than a reusable plugin contract.
- Permit each provider to choose its own dashboard actions. Rejected for first-party-
  parity code hosts because it makes task creation and review placement inconsistent.
- Put shared React components in a dedicated repository/package. Rejected because it
  would split host UI contexts and versioning; only pure types/helpers may be published
  later if repeated plugin-authoring demand justifies them.
