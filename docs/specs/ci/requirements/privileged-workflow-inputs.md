---
status: active
system: ci
created: 2026-10-25
owners:
  - kandev
---

# Privileged workflow input requirements

## Overview

Maintainers need automated reviews and publications without allowing pull
request content to control privileged execution. CI owns input provenance and
credential boundaries. The release system retains version and publication
semantics. The existing [contributor automation requirements](unified-contributor-pr-automation.md)
retain event eligibility and approval-label behavior.

## Requirements

### REQ-CI-ACTIONS-SECURITY-001: Trusted automatic review execution

**Intent:** Review the requested change without executing its agent configuration
or adopting its instructions as the review policy.

#### Acceptance criteria

- **AC-CI-ACTIONS-SECURITY-001.1:** Automatic Claude fork review and both automatic
  OpenCode review paths shall use trusted instructions, configuration, and tools.
  Pull request files shall remain review data, including files that define agent
  hooks, plugins, skills, or tools. Prepared review and walkthrough files shall
  not become agent instructions through automatic filename discovery.
- **AC-CI-ACTIONS-SECURITY-001.2:** Automatic review shall bind its diff and complete
  file reads to the event head. If the fetched head differs, preparation shall
  fail before model execution. Missing, unsafe, binary, or oversized files shall
  produce a bounded diagnostic or omission, without executing their contents.
- **AC-CI-ACTIONS-SECURITY-001.3:** Review eligibility, durable contributor approval,
  model selection, inline findings, and manual rerun behavior shall remain
  available through their existing entry points. Same-repository OpenCode evidence
  shall retain its dedicated producer identity. Fork evidence shall remain advisory.

### REQ-CI-ACTIONS-SECURITY-002: Limited publication inputs and credentials

**Intent:** Limit executable inputs and credential lifetime while preserving
successful release, recovery, image, and documentation publication behavior.

#### Acceptance criteria

- **AC-CI-ACTIONS-SECURITY-002.1:** Release metadata and weekly image dates shall be
  consumed as data. Embedded shell syntax shall not execute when these values
  reach publication commands.
- **AC-CI-ACTIONS-SECURITY-002.2:** The affected publication and image checkouts
  shall not retain checkout credentials. Release branch and signed-tag writes
  shall use the ordinary repository token only for those operations. The
  administrator token shall remain confined to the existing exact-head merge.
- **AC-CI-ACTIONS-SECURITY-002.3:** Documentation preview publication shall not
  restore or save a dependency cache. It shall retain frozen dependency installation,
  the trusted landing publisher, and the separate preview-link writer.
- **AC-CI-ACTIONS-SECURITY-002.4:** Required checks, workflow events, release mode
  gates, signing, artifact verification, publication order, and backfill source
  selection shall retain their existing behavior. Diagnostics shall distinguish
  workflow identity from checkout identity without exposing credentials.

## Exclusions

This change does not authorize publishing, merging, external configuration
changes, credential rotation, or a provider/model change. It does not replace
intentional triggers solely to satisfy a scanner. The same-repository Claude
`pull_request` path retains its existing contributor trust boundary.

## Implementation plans

- [GitHub Actions security findings](../../../plans/github-actions-security/plan.md)
