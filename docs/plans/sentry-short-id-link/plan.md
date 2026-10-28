---
created: 2026-10-28
status: implemented
requirements:
  - REQ-INTEGRATIONS-SENTRY-SHORT-ID-LINK-001
system_design:
  - ../../specs/integrations/system-design/sentry-short-id-link.md
legacy_specs: []
---

# Implementation plan: Sentry short ID link

## Overview

A rendered dialog qualification submitted `WEB-1A` and received the existing
invalid-input notice before lookup; `WEB-123` and a numeric Sentry issue URL
succeeded as controls. API and availability boundaries were mocked, so that is
component evidence, not a browser or live-provider claim. The frontend patterns
in `sentry-issue-common.tsx` require decimal suffixes although Sentry emits
base32. Fix recognition in one sequential work order after independent RED tests.

## Scope

Accept supported base32 suffixes and aliases consistently in both exported
patterns and extraction; preserve numeric links, canonical title, instance and
workspace scope, dialog close and failures. Exercise desktop and native-phone
entry paths with isolated mock-backed app/backend flows. Numeric issue URLs are
recognized before short IDs so a hyphenated organization slug cannot shadow the
numeric ID. Do not change Jira,
Linear, generic parsers, provider APIs, backend routing, state/query ownership,
copy, global mock seed data, or dialog composition.

## Technical approach

Follow the [provider design](../../specs/integrations/system-design/sentry-short-id-link.md)
and existing [task action lifetime design](../../specs/tasks/system-design/task-actions-menu.md).
Use a newly authored independent component regression and focused recognition
cases. Seed the mock through its instance-scoped controller, not an intercepted
success response. The published Sentry example already permits numeric IDs and
needs no copy change. Public docs assessment: `docs/public/integrations.md`
describes task issue selection but does not restrict ID syntax; no public prose
change is required for this correction.

Desktop follows sidebar task actions → Link → Sentry issue → input → Save.
Phone uses the existing task picker drawer and tap-accessible nested menu, then
the same dialog. No new layout or labels are proposed. The work order requires
rendered outcome and persisted title on both paths, plus a numeric control.

## Work order

- [x] [Task 01: Recognize Sentry short IDs](task-01-recognize-sentry-short-ids.md) (wave 1; no dependencies).

## Risks and exclusions

Boundary matching must not return an earlier valid prefix of an invalid token or
mistake slug separators for the suffix. Provider alias acceptance is not local
canonicalization. Managed mock tests do not prove live Sentry or physical-device
behavior. This package is design-only until the separate implementation release.

## Local verification results

The independent component suite first reproduced the base32 rejection with two
numeric controls passing. A further regression exposed numeric URL slug
shadowing, and `WEB-1A-U` exposed a truncated identity; both were fixed after
observed failing tests. The final focused component suite passed 9 tests.

The final managed host runs passed two desktop and two phone cases, one base32
and one numeric control in each project, with a real scoped issue GET, mock
provider response, rendered title and persisted task title. The phone run used
a fresh build. Scoped ESLint passed with zero warnings. Public docs need no
change because the existing integration page does not restrict short-ID syntax.
These tests do not claim live Sentry or physical-device coverage.
