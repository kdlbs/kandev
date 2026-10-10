---
created: 2026-09-29
status: implemented
requirements:
  - REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001
system_design:
  - ../../specs/integrations/system-design/shared-prompt-writes.md
legacy_specs: []
---

# Implementation plan: Shared prompt MCP writes

## Overview

Deliver issue #3993 with per-prompt operator control. The user requested full-AFK
implementation, PR delivery, remediation, and merge in this session. Work remains
sequential in the primary session.

## Scope and approach

Create/update MCP tools, guarded persistence, generic-settings parity, HTTP
permission editing, immediate browser refresh, localized desktop/phone controls,
and public MCP documentation. Deletion tools and automatic workflow rewrites are
excluded. Existing transport authentication and instance-wide prompt ownership stay.

| Order | Work order | Status |
| --- | --- | --- |
| 1 | [Guarded prompt write contract](task-01-guarded-writes.md) | completed |
| 2 | [Operator control and live delivery](task-02-settings-and-delivery.md) | completed |

## Compatibility

Config and external MCP gain two tools. Task, Office, automation do not. Existing
HTTP saves omit the new field without resetting permission. Upgraded rows retain
content and timestamps with permission off. SQLite and PostgreSQL share guarded SQL.

## ASCII UI preview

UI-01: Settings > Prompts, editing a custom prompt (desktop and phone).

```text
@review-policy                         [Edit] [Delete]
[Prompt name                                      ]
[Prompt content editor                            ]
Allow agent edits                            [off]
Agents can change this prompt wherever it is used.
[Cancel]
                      [Discard] [Save changes]
```

The new permission row shares the existing page scroll owner and save bar. On
phones the label wraps and its switch row has a 44px touch target. No overlay or
navigation change. Built-in edit forms omit the permission control. Labels use
i18n. Geometry in the drawing is illustrative; order and shared save semantics
satisfy AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001.5 and .7.

## Verification and risks

Each work order records its exact checks. Risks: bypass via generic settings,
revocation racing an update, stale browser reads, and lost unsaved drafts. Targeted
tests cover these boundaries. PR CI and configured reviewers provide delivery gates.

## Implementation results

Both work orders are implemented and locally verified. The permission is enforced
at the database write boundary, including generic MCP settings updates, and the
browser refresh preserves open drafts. SQLite/PostgreSQL upgrade and replay,
backend race tests, web unit/type/i18n/lint checks, desktop/mobile E2E, and document
validation passed. External CI, automated review, and merge remain tracked in the
PR; local completion does not imply those delivery gates have passed.
