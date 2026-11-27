---
created: 2026-10-03
status: implemented
requirements:
  - REQ-TASKS-PLAN-READ-001
  - REQ-TASKS-PLAN-READ-002
  - REQ-TASKS-PLAN-READ-003
system_design:
  - ../../specs/tasks/system-design/plan-partial-reads.md
legacy_specs: []
---

# Implementation Plan: Partial plan reads and fragment guidance

## Overview

Add bounded, version-aware plan reads, then guide agents to combine them with
the existing exact-edit and append tools. Implement the read contract first so
prompts never advertise an unavailable argument. Work remains sequential in
the primary session; this package does not authorize subagents.

The user requested partial plan updates to save tokens on large plans, then
explicitly added partial reads. Existing append and exact-edit behavior was
verified before this package; extending the write schema is unnecessary for
the requested outcome.

## Scope

### In scope

- Optional character offset/limit and expected version on the existing read.
- Coherent snapshot projection, authorization, range metadata, and failures.
- Agent guidance for bounded reads, exact edits, append, and version reuse.
- End-to-end MCP evidence, prompt tests, and public reference/how-to updates.

### Out of scope

- New write modes, batches, search, section parsing, or browser pagination.
- Storage migrations, streaming database reads, new flags, or token metrics.
- Changes to rendered desktop/mobile interaction or plan safety/recovery rules.

## Technical approach

### Bounded read slice

Extend `registerPlanTools` and `getTaskPlanHandler` in the MCP server; forward
presence-aware arguments through `mcp.get_task_plan` to the backend handler.
Introduce a focused read projection beside `GetPlanSnapshot` in the plan
service. Reuse its authorization and task-lock boundary, slice exact stored
bytes at Unicode code-point boundaries, and return a fixed metadata set plus
only the range. Preserve the default full read and existing no-plan result.
Extend `planws.GetError` to retain version conflicts and range error details.

Use the existing task/Office profile exposure rules. Configuration and other
excluded profiles gain no plan tool. The integration transport is the existing
MCP-to-WebSocket dispatcher, regardless of which agent consumes its text blocks.

### Agent adoption

Update backend system/planning prompts, applicable built-in workflow guidance,
and `buildDocumentContext` in the web hook. Explicitly distinguish fragments
from whole documents and recommend exact edit/append for focused changes.
Retain full reads for complete planning and preserve all system markers and
workflow boundaries. Publish the supported contract in existing public pages
when the implementation is ready.

## Tests

| Acceptance criteria | Evidence |
| --- | --- |
| `001.1` to `001.6` | New `plan_partial_read_test.go` in service, MCP handlers, and MCP server: defaults, Unicode/CRLF, bounds/types, EOF, absence, failures, and long-line/oversized content |
| `002.1`, `002.2`, `002.4` | Service authorization and pinned-version tests; backend error bridge tests; read-side absence/conflict and no-mutation assertions |
| `002.3` | `TestPlanPartialReadMCPJourney` through the real dispatcher: bounded read, exact fragment edit, untouched surrounding content, stale/ambiguous edits |
| `003.1` | MCP schema/text tests in new server read tests |
| `003.2`, `003.3` | Sysprompt and `buildDocumentContext` regression tests; existing scope and marker guards; public-doc validation |

## End-to-end evidence

The user-visible interface is MCP. Work order 01 owns the real
MCP-server-to-dispatcher-to-service-to-SQLite journey, including an intervening
browser/service write between pages. Browser pagination and rendered UI do not
change, so Playwright would not prove a new interface here.

Mobile-parity assessment: the frontend change only builds a shared agent prompt;
no layout, touch, navigation, scrolling, or viewport behavior changes. The
existing hook's unit tests cover both viewport consumers. No ASCII UI preview
or new mobile E2E is required for this package.

## Work orders

- [x] [Task 01: Bounded version-aware plan reads](task-01-bounded-reads.md)
- [x] [Task 02: Fragment guidance and public documentation](task-02-guidance-and-docs.md)

Both work orders passed. The requirements are active, the design is current,
and this plan is implemented. The delivered contract matches the design package.

## Verification results

Design preflight passed:

- `python3 scripts/list-docs.py validate`: validated the catalog.
- `python3 scripts/lint-spec-files.test.py`: 36 tests passed.
- `python3 scripts/lint-spec-files.py --all`: all specification files passed.
- `git diff --check`: passed. New files were confirmed with `git status --short`.
- Exported `.github/scripts/pr-docs.cjs` `validateCoverage`: both work orders'
  requirement/acceptance/design/plan references passed using a planned code path
  to exercise coverage. This is local design evidence, not a live PR result.

Node was absent from the shell PATH. The reference preflight used the installed
Node 24 binary at `/home/jcfs/.local/share/mise/installs/node/24.21.0/bin/node`.
Implementation requires Node 24 and pnpm on PATH for the web/docs commands.
Both work orders are complete. Their results record the red/green tests,
integration and race coverage, prompt checks, and public documentation checks.
The affected MCP server, handler, plan WebSocket bridge, and contract packages
also passed their complete test suites.

Existing behavior check before design:

```bash
(cd apps/backend && go test ./internal/mcp/server ./internal/mcp/handlers -run 'TestPlanSafeEditsMCPJourney|TestMCPPlanExactEdit|TestMCPUpdateTaskPlanAppendComposesFragment|TestPlanReadReturnsMetadataAndExactContentBlocks' -count=1)
```

Passed for both packages. This proves current partial writes and full reads,
not the proposed read ranges.

## Risks

- Offsets count code points; agents must use returned offsets rather than
  estimating byte or JavaScript string positions. Visual graphemes may split.
- After a write, offsets from the previous version are stale. Restart or select
  a new range deliberately; never refresh a token and replay stale data.
- Uniqueness within a page does not imply global uniqueness for exact edits.
- The backend still loads the row; response savings do not imply storage
  streaming or measured token reduction for every agent.
- Cached agent schemas need refreshed discovery/resume before using new fields.
