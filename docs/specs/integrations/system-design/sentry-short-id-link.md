---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001
---

# Sentry short ID recognition design

## Purpose and boundaries

Sentry emits base32 issue counters, not decimal-only counters. The integration
owns recognizing that provider-specific identity; the task system owns the
existing external-link action and dialog lifetime. [ADR 0030](../../../decisions/0030-workspace-scoped-integration-settings.md)
continues to govern workspace and instance scope. Sentry's
[group model](https://github.com/getsentry/sentry/blob/master/src/sentry/models/group.py)
and [number utilities](https://github.com/getsentry/sentry/blob/master/src/sentry/utils/numbers.py)
define the encoder alphabet and decoder aliases.

## Requirement mapping

`REQ-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001` maps to the recognition and flow
below, including failure and verification. Existing task action and request
lifetime rules remain in the [task link requirement](../../tasks/requirements/link-existing-task-github-issue.md)
and [task actions design](../../tasks/system-design/task-actions-menu.md).

## Components and contracts

`apps/web/components/sentry/sentry-issue-common.tsx` owns both exported Sentry
short-ID recognition patterns and `extractSentryShortId`. Align them on the
provider alphabet and `O`/`I`/`L` decoder aliases, preserving case normalization
and the existing project-slug grammar. A complete invalid suffix must not be
accepted as a valid prefix; test boundaries and ambiguous slug separators.
When an `/issues/` path is present, validate its complete issue segment before
considering any other text; an empty or invalid segment fails closed instead of
matching an organization slug or host. Do not decode or canonicalize aliases
locally: the provider resolves them.

`TaskExternalLinkDialog` recognizes its existing numeric issue URL first, then
uses the short-ID extractor. This prevents a hyphenated organization slug in a
numeric URL from being mistaken for a short ID. It calls `getSentryIssue` with
the captured workspace and selected instance.
A successful response supplies the canonical `shortId` for the existing task
title update and close. Preserve current errors, cancellation, query/store
ownership, routing, and request lifetime. The backend's existing Sentry REST
resolver forwards short IDs; no provider or backend production change is needed.

The task system's existing external-reference contract requires replacement of
an existing leading prefix. `buildLinkedIssueTitle` in
`apps/web/components/task/task-external-link-utils.ts` must also recognize a
complete Sentry base32 prefix followed by the existing colon delimiter before
composing the canonical new key. Keep its existing numeric Jira, Linear and
Sentry prefix behavior, non-prefix handling, and code-point title truncation.
This is the shared link action's title builder, not a change to unrelated title
parsers. The existing title representation has no provider provenance for a
prefix; recognition is therefore limited to the accepted anchored syntax.

## Desktop and phone flow

Desktop uses the existing task sidebar action and phone uses the task picker,
visible task actions, and touch Link submenu. Both enter the same existing
single-field dialog and share recognition, scope, and save logic. No composition,
copy, controls, touch geometry, scroll owner or navigation changes are planned.
The mobile picker and nested menu in `session-task-switcher-sheet.tsx` and
`e2e/tests/task/mobile-external-link-menu.spec.ts` are the shipped precedents.

## Failure, security, persistence and verification

Malformed input retains existing validation; provider failures leave the title
unchanged. No new persistence, credentials, events, metrics, or API is added.
Independently authored component and extraction regressions prove the actual
submit callback and matching boundaries; isolated desktop and phone managed
browser tests use the instance-scoped mock controller and real app/backend path,
observe the correlated issue response, and check the rendered and persisted
title. Relinking verification must exercise the shared title builder and actual
dialog callback as well as the desktop and phone task actions. They do not
establish live Sentry behavior or physical-device coverage.
