---
status: draft
system: integrations
requirements:
  - REQ-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001
created: 2026-08-31
owners:
  - Kandev
---

# Pull request link copy actions System Design

## Purpose and boundaries

The integration system owns the provider URL data that crosses the GitHub
feedback boundary and the vertical behavior that exposes those URLs from a
linked change-request detail surface. The UI system owns reusable layout and
responsive interaction primitives, while the integration design decides which
normalized provider values are safe to expose. This design does not add a
provider-specific detail panel or a second mobile review surface.

## Requirement mapping

| Requirement | Design sections |
| --- | --- |
| `REQ-INTEGRATIONS-PR-LINK-COPY-ACTIONS-001` | [Data and contracts](#data-and-contracts), [Control flow](#control-flow), [Presentation and responsive behavior](#presentation-and-responsive-behavior), [Failure and recovery](#failure-and-recovery), [Tests](#tests) |

## Components and responsibilities

### GitHub provider and feedback boundary

- `apps/backend/internal/github/gh_client.go` and `pat_client.go` decode the
  existing review-comment and issue-comment API responses.
- `apps/backend/internal/github/client_helpers.go` converts both raw comment
  shapes into the shared `PRComment` model.
- `apps/backend/internal/github/models.go` carries the canonical comment URL in
  the feedback payload returned by the existing PR feedback endpoint.
- `apps/backend/internal/github/mock_client.go` and `mock_controller.go` keep
  the field available to deterministic browser fixtures.

### Shared change-request presentation

- `apps/web/lib/types/github.ts` preserves the GitHub comment URL in the
  frontend feedback type.
- `apps/web/components/github/pr-detail-panel.tsx` maps the GitHub URL into the
  provider-neutral `ChangeRequestDetailModel`.
- `apps/web/components/integrations/change-request-detail-header.tsx` renders
  the header copy action from the normalized change-request URL.
- `apps/web/components/integrations/change-request-detail-feedback.tsx` and
  `change-request-detail-comments.tsx` render the per-comment action for rows
  that have a URL. The same components serve desktop, tablet, and phone Review
  surfaces.
- `apps/web/lib/utils/copy-to-clipboard.ts` remains the browser clipboard
  boundary. The detail surface does not call the Clipboard API directly.

## Data and contracts

The existing GitHub PR feedback response remains the transport boundary. No
new endpoint, store slice, database column, WebSocket event, or recurring
request is needed.

The normalized values are:

| Boundary | Field | Rule |
| --- | --- | --- |
| GitHub raw review comment | `html_url` | Preserve the provider value. |
| GitHub raw conversation comment | `html_url` | Preserve the provider value. |
| Backend `PRComment` | `html_url` | Serialize the preserved canonical URL. |
| Frontend `PRComment` | `html_url` | Type the existing payload field. |
| `ChangeRequestDetailComment` | `url?: string` | Map a non-blank provider URL; omit it when absent. |
| `ChangeRequestDetailModel` | `url: string` | Use the live PR `html_url` when available, then the persisted task PR URL. |

GitHub review-comment URLs already contain the provider's
`#discussion_r<id>` anchor, while conversation-comment URLs contain the
`#issuecomment-<id>` anchor. The host copies the returned string exactly. It
does not reconstruct provider URLs from owner, repository, number, or comment
type, because the provider remains authoritative for permalink format and
future URL changes.

The persisted `github_task_prs.pr_url` continues to supply the existing PR URL
fallback. Comment URLs remain transient feedback data and are not persisted.

## Control flow

1. The GitHub client fetches review comments and conversation comments through
   the existing feedback read path.
2. Each raw comment converter preserves its `html_url` in `PRComment`; the
   response carries the field through the existing controller response.
3. `PRDetailContent` maps the live PR URL to the detail model and maps each
   non-blank comment `html_url` to `ChangeRequestDetailComment.url`.
4. The shared header renders one copy action when the detail URL is non-blank.
   Each comment or reply row renders its own action when its URL is non-blank.
5. The action calls `copyToClipboard` with the exact normalized URL. On success
   it switches to a transient copied state; on failure it keeps the normal
   action state and does not show a success confirmation.

Copying is client-only. It does not refresh feedback, mutate GitHub, update the
task store, or alter the selected Review item.

## Presentation and responsive behavior

The header copy action sits with the existing state and number identity row so
it remains close to the title without competing with provider mutation
actions. It is icon-only, has a localized accessible name, and uses the
existing compact desktop sizing with a minimum 44 by 44 pixel phone target.

Comment rows use the existing feedback-row action area. On fine pointers the
copy action can use the established hover-reveal treatment, with focus-within
keeping it discoverable to keyboard users. On coarse pointers it remains
visible without hover. Tooltip text and the copied state are localized.

The phone path remains `SessionMobileLayout` to `MobileReviewPanel` to the
shared change-request detail. The header stays above
`change-request-detail-scroll`, which remains the single vertical scroll owner;
safe-area padding and `h-dvh` layout are unchanged. No provider-specific phone
branch is introduced.

## Failure and recovery

- An absent or blank provider URL suppresses only the corresponding copy action
  and leaves all existing detail content rendered.
- A clipboard rejection leaves the action enabled for retry and does not show a
  false copied state.
- Clipboard fallback behavior remains owned by `copyToClipboard`, including
  browsers without the modern Clipboard API.
- Closed and merged PRs use the same URL fields and copy behavior as open PRs;
  no lifecycle refresh or mutation permission is required.

## Persistence

No persistence changes or migrations are required. The PR URL continues to use
the existing task association field, and comment URLs are carried only in the
live feedback response.

## Security

The provider supplies the URL and remains the authority for its external
destination. Kandev does not construct a destination from untrusted path
fragments or send the URL to a new server endpoint. The action copies plain
text only and does not grant or change provider permissions.

## Observability

No new backend metrics, logs, or polling are required. The feature is a local
clipboard interaction. Existing feedback request errors remain visible through
the existing detail loading and error states.

## Tests

- Backend converter and transport tests cover `html_url` preservation for both
  GitHub review comments and conversation comments through the gh and PAT
  clients.
- Shared detail component tests cover the header action, per-row actions,
  exact clipboard values, transient success state, missing URLs, and terminal
  PR states.
- GitHub panel tests cover mapping the feedback URL into the normalized detail
  model without changing the existing PR URL fallback.
- Desktop Playwright coverage seeds one review comment and one conversation
  comment, copies the PR URL and both comment URLs, and checks the visible
  confirmation.
- Mobile Playwright coverage enters through the existing Review destination,
  copies a comment URL without hover, and checks the 44-pixel target and zero
  document-level horizontal overflow.

## Related systems

- [UI system](../../ui/README.md): owns the reusable detail presentation and
  responsive interaction primitives.
- [GitHub task pull request synchronization](github-task-pr-sync-coordination.md):
  continues to own task-association synchronization and freshness, which this
  read-only action does not change.
