---
status: active
system: integrations
created: 2026-10-28
owners:
  - product
---

# Sentry short ID recognition requirements

## Overview

Integrations owns Sentry's provider identity syntax. The task system owns the
existing [external-reference action](../../tasks/requirements/link-existing-task-github-issue.md)
and title update. This requirement defines which Sentry issue references that
action can submit for provider validation.

## Requirements

### REQ-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001: Recognize Sentry issue references

**Intent:** Users can link an existing task to an issue identified by a Sentry
short ID, including IDs whose encoded suffix contains letters.

#### Acceptance criteria

- **AC-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001.1:** On desktop and phone, a user
  can submit a Sentry short ID such as `WEB-1A` through the existing task Link
  action. The selected workspace and Sentry instance validate the complete ID,
  and a successful result links the task using the provider's canonical short ID.
- **AC-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001.2:** Recognition accepts Sentry's
  base32 suffix alphabet `0123456789ABCDEFGHJKMNPQRSTVWXYZ` and the provider's
  `O`, `I`, and `L` decoder aliases, case-insensitively, while preserving the
  existing project-slug and numeric-short-ID behavior. Recognition and extraction
  must not silently turn a longer invalid suffix into a different valid ID.
- **AC-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001.3:** Existing numeric issue URLs
  continue to resolve through the selected instance, use the returned canonical
  short ID as the title prefix, and close on success. Invalid or unresolved
  references retain the existing failure behavior and do not update the task.
- **AC-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001.4:** On desktop and phone,
  relinking a task whose leading issue prefix is a supported Sentry base32
  short ID replaces that prefix with the newly resolved issue key instead of
  stacking both. The existing title and length-limit behavior applies after
  replacement, including when the new issue belongs to another supported
  title-link provider.

## Out of scope

Jira and Linear input parsing; unrelated title parsers; Sentry provider APIs,
backend routing, credentials, storage and task-link lifetime semantics.
