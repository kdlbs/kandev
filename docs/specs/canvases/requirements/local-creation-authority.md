---
id: canvases-local-creation-authority
title: Owner-authorized canvas creation
status: draft
system: canvases
owners:
  - canvases
created: 2026-09-10
last_updated: 2026-09-10
---

# Owner-authorized canvas creation

## Overview

A user who asks their task agent to create a canvas authorizes its first
publication within that task. They do not need a second permission dialog to
open the result. External packages retain explicit permission review.

This contract governs local creation authority. The plugin runtime continues
to enforce declarations, persisted grants, and current resource access.

## Terminology

- **Owner-authorized creation:** Creation through a task execution that Kandev
  binds to the current workspace owner, task, and creating session.
- **Initial publication:** The first persisted release of a newly authorized
  draft. A rejected package that creates no release does not consume it.

## Requirements

### REQ-CANVASES-LOCAL-CREATION-001: Initial publication authority

**Intent:** The owner's requested canvas opens without repeated consent, while
imported code and later changes cannot claim that creation authority.

#### Acceptance criteria

- **AC-CANVASES-LOCAL-CREATION-001.1:** The first valid release of an
  owner-authorized task canvas shall activate without a separate permission
  approval. Its grants shall contain only its declared, supported permissions.
- **AC-CANVASES-LOCAL-CREATION-001.2:** Initial grants shall remain within the
  task scope and current owner access. They can cover supported data reads,
  writes, events, instance state, and exact HTTPS external origins. They shall
  not enable remote scripts, managed backend execution, or workspace access.
- **AC-CANVASES-LOCAL-CREATION-001.3:** Agent fields, package metadata, display
  titles, and ownership of an imported package shall not establish local
  creation authority. Missing or mismatched trusted identity shall deny it.
- **AC-CANVASES-LOCAL-CREATION-001.4:** Publication shall either persist the
  initial release, grants, and consumed creation authority together, or leave
  all three unchanged. Concurrent publication shall consume that authority at
  most once.
- **AC-CANVASES-LOCAL-CREATION-001.5:** A later permission increase, external
  import, or workspace promotion shall require user review. Republishing,
  rollback, restart, archive, or restore shall not recreate consumed authority
  or restore a revoked grant.
- **AC-CANVASES-LOCAL-CREATION-001.6:** Upgrading Kandev shall preserve existing
  active and pending releases and grants. Existing drafts without recorded
  creation authority shall retain manual review.
- **AC-CANVASES-LOCAL-CREATION-001.7:** The create result shall explain the
  initial permission ceiling. Publication shall report whether the release
  activated or requires review, without asking the agent to approve grants.

## Compatibility and exclusions

This changes the first-publication exception to the review rule in
[canvas release governance](agent-authored-web-apps.md). The exception applies
only to new recorded owner-authorized creations. This package does not create
a marketplace, import UI, collaborator role, or ongoing blanket approval.

## Implementation plans

- [Canvas runtime and permission fixes](../../../plans/canvas-runtime-permission-fixes/plan.md)
