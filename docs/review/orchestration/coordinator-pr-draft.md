# feat(orchestration): keep workspace tasks beside coordinator chat

The existing orchestration conversation leaves task progress on separate board
pages. This change adds a workspace Coordinator view with canonical task groups
beside the selected coordinator's persistent chat, following the central-task
view proposed in issue #3752. Mobile uses Tasks/Chat tabs with separate scrolling
and retained drafts.

The view reads ordinary workspace tasks through a scoped, paginated query. It
excludes hidden/Office workflows, archived/ephemeral tasks and configuration
conversations before counting. Native status-summary events update the rows;
reconnects refresh the loaded window. Search, workflow/repository filters and
selected-coordinator filtering preserve the conversation. Unknown status remains
unknown; review or a merged PR does not imply completion. Counts state loaded
coverage and known PR/diff coverage.

Profile failures and invalid selections offer configuration. Observation starts
no execution. Workspace/owner changes clear previous reads; each conversation's
draft stays separate in browser memory. The existing orchestration flag gates
the route and navigation. The assistant feature remains a separate delivery.

Validation: scoped task handler/service/repository and native ownership tests;
frontend grouping, paging, request races, drafts, owner isolation and routing;
full typecheck and localization checks; new and existing orchestration browser
flows together with no retries; Pixel 5 touch and first-row geometry assertions.
The [review media](media/coordinator-view/README.md) use only synthetic fixtures.
Public docs updated: `orchestration-personas.md` (how-to) and
`personal-assistant-api.md` (reference).

This is a private review draft on exact v0.94.0. The integration branch contains
additional assistant foundations and must not be submitted wholesale as this
focused PR. Contribution extraction, target-branch confirmation, PostgreSQL
qualification and the live-data rollout remain delivery work orders. No public
PR has been opened.
