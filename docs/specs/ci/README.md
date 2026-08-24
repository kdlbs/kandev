---
status: active
system: ci
specification_version: 1
migration: in_progress
owners:
  - kandev
---

# CI automation system

## Purpose

The CI automation system defines base-controlled GitHub Actions behavior for
contributor pull requests. It owns the trust gates for AI review, preview
deployment, and pull request walkthrough generation.

## Ownership

- Maintainer approval labels and their persistence rules.
- Direct contributor allowlist gates and their fail-closed behavior.
- Workflow event filters and job-level authorization expressions.
- Permissions and credential boundaries for review, preview, walkthrough, and
  publication jobs.
- Workflow contract tests for these boundaries.

## Exclusions

- The review provider prompts and provider-specific agent behavior.
- The preview deployment command and its infrastructure.
- The walkthrough skill, renderer, and artifact format.
- Kandev application state or user-interface behavior.

## Specification map

### Requirements

- [Unified contributor PR automation](requirements/unified-contributor-pr-automation.md)

### System design

- [Unified contributor PR automation](system-design/unified-contributor-pr-automation.md)

## Migration

The new shared trust contract replaces the label portions of the legacy
[Claude fork review allowlist specification](../claude-fork-review-allowlist/spec.md)
and the legacy
[PR walkthrough specification](../pr-walkthrough/spec.md). Those documents
remain useful for provider and walkthrough detail until their content is
migrated fully.
