---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001
created: 2026-10-27
owners:
  - Kandev
---

# Review thread context System Design

## Purpose and boundaries

Integrations owns the normalized change-request feedback outcome. The host's
`ChangeRequestDetail` renders it for GitHub and compatible plugin providers;
[plugin detail contract](../../plugins/system-design/plugins-04.md) and
[Host API](../../../plans/plugins/PLUGIN-API.md) continue to own exposure of
that component, callbacks, and provider capabilities. The adjacent
[link-copy design](pr-link-copy-actions.md) owns comment permalink actions.

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-REVIEW-THREAD-CONTEXT-001` | [Projection and action](#projection-and-action), [Presentation and mobile](#presentation-and-mobile), [Verification](#verification) |

## Projection and action

`ChangeRequestComments` in `apps/web/components/integrations/change-request-detail-comments.tsx`
currently filters `detail.comments` by `author.isBot` before grouping each list
with `commentThreads`. A cross-group reply is consequently rendered as a root of
its own fragment, and `buildThreadContext` sees only that fragment. A two-comment
mixed thread can even have no whole-thread button. GitHub's
`mapGitHubComments` in `apps/web/components/github/pr-detail-panel.tsx` already
maps `in_reply_to` and `author_is_bot` to `parentId` and `author.isBot`.

Keep the existing filtered projections for rendering, bot disclosure, counts,
individual actions and reply forms. Independently derive the same one-level
thread grouping from all supplied comments and associate each displayed fragment
root with its complete supplied thread, including when the displayed root is a
reply in that thread. The existing unique comment IDs and direct-parent contract
are assumed; do not add recursive nesting or provider fetching. An absent parent
makes the comment a root, just as the existing grouping does. Preserve its direct
replies in supplied order. The provider supplies identity and order; do not sort
or deduplicate content by body or author.

Show a fragment's whole-thread action when its associated complete thread has
at least one reply and `onAddContext` exists. Activation passes the complete
thread to the existing formatter and invokes `onAddContext("comment", content)`
once. Multiple visible fragments can expose the same complete thread; each
activation sends it once. Existing formatting uses root followed by direct replies,
author, optional path and line, body, and the detail PR URL; it does not use
per-comment permalinks. Preserve those bytes and callback signatures. Do not
change the visible fragment's root identity for provider reply actions.

## Presentation and mobile

Keep the existing Comments section, human rows, bot toggle and bot rows in their
current places. Human replies whose bot parent is hidden remain visible; the
whole-thread action on that visible fragment nevertheless includes the supplied
bot root and siblings. Conversely, a human root's action includes hidden bot
replies. This changes action availability/content, not display grouping.

The existing desktop PR detail and `SessionMobileLayout` → `MobileReviewPanel`
use the same detail and context callback; the adjacent link-copy design is the
nearest shipped mobile exemplar for its entry point and single
`change-request-detail-scroll` owner. The existing inline action is a direct
one-step operation, so no drawer or extra navigation is needed. Keep keyboard
access and a localized accessible name. Scope a 44-by-44 active target to the
whole-thread phone/coarse-pointer action while keeping compact desktop sizing;
do not rely on the existing 24-pixel shared button to satisfy touch geometry.
Keep existing scroll ownership, safe-area handling and zero page-level horizontal
overflow. No new copy is planned; reuse the existing localized action label.

## Failure, security and persistence

No callback means no context action. Missing parents follow the orphan rule;
no unseen comment is fabricated. The action only forwards already supplied
feedback to the existing context callback; no new network, permission, storage,
telemetry, or provider contract is introduced.

## Verification

Rendered shared-component tests exercise real actions and callback content for
mixed human/bot roots and replies, the two-comment case, homogeneous controls,
missing parents, visibility and callback identity. Focused desktop and phone
Playwright scenarios use the existing GitHub mock feedback seed and Review
entry points to prove complete context reaches the user-facing pending context;
the phone scenario checks the actual target and no document overflow.
