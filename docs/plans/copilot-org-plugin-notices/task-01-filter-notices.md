---
id: "01-filter-notices"
title: "Filter routine Copilot plugin notices"
status: done
wave: 1
depends_on: []
plan: "plan.md"
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-006
  - REQ-AGENTS-ACP-SESSION-EVENTS-007
system_design:
  - ../../specs/agents/system-design/acp-session-events.md
acceptance_criteria:
  - AC-AGENTS-ACP-SESSION-EVENTS-006.1
  - AC-AGENTS-ACP-SESSION-EVENTS-007.1
  - AC-AGENTS-ACP-SESSION-EVENTS-007.2
  - AC-AGENTS-ACP-SESSION-EVENTS-007.3
---

# Filter routine Copilot plugin notices

Work in `apps/backend/internal/agentctl/server/adapter/transport/acp/`.
Read the backend and agentctl guides, including `ADAPTERS.md` and `ACP-PROTOCOL.md`.

1. Add `testdata/copilot-org-plugin-notices.jsonl` with the supplied ready frame and a representative updating frame.
   In `adapter_copilot_interruption_test.go`, add `TestCopilotOrgPluginNotices`.
   Feed decoded frames through `handleACPUpdate`. Assert zero emitted events for each notice.
   Repeat both with a message ID and assert the original text and ID reach the stream.
   Check a near match to the ready sentence, unrelated Info text, a plugin Warning, a plugin Error,
   and the same ready text from a standard ACP agent. Each must reach the stream.
2. Extend `TestCopilotInterruptedToolRecoveryNotice`. Insert the ready frame after the interrupted tool update and before the warning.
   Retain the existing assertions for event count, warning classification, and reply.
   Run the focused tests. Confirm the extra event and lost classification on the original adapter.
3. Implement the Copilot predicate in `dialect_copilot.go`. In `adapter_updates.go`, compute suppression before
   `observeCopilotInterruptionUpdate` and reuse the result at conversion. Preserve the load observer for other frames,
   including pending questions. Log each suppressed live Copilot notice at debug level.
   Use the existing Grok suppression hook as the precedent. Preserve Grok user-message handling.

## Verification

Run from the repository root. Use the focused test command before and after the fix.

```bash
pkg=internal/agentctl/server/adapter/transport/acp
go -C apps/backend test -trimpath "./$pkg" -run 'TestCopilot(OrgPluginNotices|InterruptedToolRecoveryNotice)$' -count=1
files=(apps/backend/"$pkg"/{dialect_copilot,adapter_updates,adapter_copilot_interruption_test}.go)
gofmt -w "${files[@]}"
bash scripts/lint-go-changed "${files[@]}"
go -C apps/backend test -trimpath "./$pkg" -count=1
```

## Results

The original adapter emitted both routine notices. The recovery sequence emitted four events instead of three. After the fix, both focused tests and the full ACP package passed. Changed-file lint reported zero issues.
