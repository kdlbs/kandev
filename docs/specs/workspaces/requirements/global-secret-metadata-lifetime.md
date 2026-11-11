---
status: active
system: workspaces
created: 2026-08-03
owners:
  - tbd
---

# Global Secret Metadata Lifetime Requirements

## Overview

An older initial Global secrets read can overlap accepted metadata changes. Successful
reads and qualified server failures preserve the accepted Global inventory. The
workspace-scope catalogue contract stays with the
[repository-secrets requirement](repository-secrets.md).

## Requirements

### REQ-WORKSPACES-REPOSITORY-SECRETS-002: Accepted Global metadata during initial loading

An older initial Global read can overlap accepted metadata changes. Successful reads and
qualified server failures preserve them. Workspace independence remains in
AC-WORKSPACES-REPOSITORY-SECRETS-001.14.

#### Acceptance criteria

- **AC-WORKSPACES-REPOSITORY-SECRETS-002.1:** When a Global secret creation is acknowledged while an older initial metadata read is pending, successful completion of that read, including an empty result, shall preserve the accepted secret and its rendered Settings row on desktop and phone.
- **AC-WORKSPACES-REPOSITORY-SECRETS-002.2:** When the current Global metadata list changes during an older initial read through acknowledged creation, update, removal, or accepted metadata events, successful completion of that read shall preserve the entire current list's membership, order, and metadata field values. Removed rows shall remain absent, updated fields shall not revert, and repeated creation acknowledgments shall not duplicate a row. This applies to every current row, including a mixed list with several changes.
- **AC-WORKSPACES-REPOSITORY-SECRETS-002.3:** When no metadata change intervenes, an initial successful read shall publish its complete result, including an empty list, and settle the shared loaded/loading state. Consumers shall share the existing initial-load gate and settled list; creating a secret from an already-loaded list shall retain the accepted rendered row without starting an initial read.
- **AC-WORKSPACES-REPOSITORY-SECRETS-002.4:** Global consumers shall retain Global and legacy unscoped metadata filtering, excluding Workspace rows. Completion of a superseded successful read shall settle shared readiness without restarting loading or replacing accepted metadata. Workspace list lifetimes and other initial-read failure settlement shall retain their established behavior.
- **AC-WORKSPACES-REPOSITORY-SECRETS-002.5:** When accepted creation, edit, deletion, or metadata events change the current Global inventory during an older initial read, HTTP 500–599 completion shall preserve the complete current inventory and rendered accepted rows on desktop and phone. Preservation is universal over membership, order, and all metadata fields, including untouched rows in mixed inventories and an empty inventory after deletion. Shared consumers shall settle loaded=true/loading=false with that inventory, without another initial read.
- **AC-WORKSPACES-REPOSITORY-SECRETS-002.6:** Initial HTTP 5xx without intervening accepted metadata changes, every 4xx (including 403), and nonqualified errors shall retain empty-list loaded=true/loading=false settlement. Challenged 401 retains its authentication notification; unchallenged 401 shall not acquire one. Intervening mutations shall not override authorization failure clearing.

#### Exclusions

Only current Global metadata publication is covered. Secret plaintext, reveal, authorization,
API, encryption, binding, transfer, running environments, retries, refresh, cross-workspace
caching, and navigation/lifecycle hardening are unchanged.
