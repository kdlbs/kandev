# ADR-2026-07-31-plugin-repository-provider-extensions: Plugin Repository Provider Extensions

**Status:** accepted
**Date:** 2026-07-31
**Area:** frontend, backend, protocol

## Context

Kandev task creation, Link menus, and review panels contain provider-specific GitHub/
GitLab assumptions. A connector plugin must contribute repository discovery, pull-
request actions, and native review UI without adding a Bitbucket branch or leaving
mobile as a compressed desktop-only escape hatch.

## Decision

The frontend plugin registry gains revocable, manifest-owned registrations for:

- `registerRepositoryProvider(...)` with repository listing, URL matching, branch
  listing, and credential-free `inspectURL` descriptors whose clone URL is HTTPS;
- `registerTaskAction(...)` for children of the existing **Link** submenu, with
  visibility and an async handler receiving read-only current
  workspace/task/repository context;
- `registerReviewProvider(...)` with normalized task-item summaries, external-store
  `getSnapshot`/`subscribe`/cancellable `refresh`, and a plugin-owned `ReviewPanel`.

Providers have unique active ownership, are removed on plugin disable/unload, and
abort their in-flight work. Activation is transactional: a failed or timed-out
`initialize` revokes partial registrations and fences late callbacks from restoring
them. The host adapts GitHub, GitLab, and Azure implementations to the same contracts.
It accepts a complete plugin-inspected provider descriptor, including an exact
credential-free HTTPS clone URL, rather than parsing plugin provider URLs.

Task link actions render in existing desktop and visible mobile menus. A generic
top-level `action` placement is deliberately deferred until Kandev has a host-owned
workflow contract for command eligibility, result handling, and review navigation;
accepting registrations with no consumer would be a false public API. Review panels use
provider-neutral IDs and params `{ providerId, reviewKey }`; legacy GitHub/GitLab
layout names remain aliases. The host's native desktop and mobile review surfaces own
navigation and placement, while the plugin owns provider data and rendering.
Task-link surfaces remain host-owned through `openTaskLinkDialog`; `openModal` remains
available for plugin-owned workflows but is not a substitute for native task actions.
Native review surfaces expose provider-neutral selection when
multiple built-in or plugin reviews exist instead of silently choosing the first.
Selections are revalidated by provider and review key after unload/reload, so a stale
same-ID provider instance cannot keep an obsolete item selected.

## Consequences

Repository providers and review actions become extensible without host provider
knowledge. Registry lifecycle and normalized item contracts are more deliberate, and
existing built-ins require adapter work. Plugin review components must tolerate host
selection, persistence, task switching, close, and mobile presentation changes.

## Alternatives Considered

- Hard-code Bitbucket alongside GitHub/GitLab: rejected because every new provider
  would enlarge host provider unions and duplicate routing logic.
- Give plugins only a standalone route: rejected because it omits native Link and
  review flows users already rely on.
- Register React hooks for reviews: rejected because plugin load/unload can violate
  hook ordering; an external-store source is lifecycle-safe.
