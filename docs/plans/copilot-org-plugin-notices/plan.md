---
created: 2026-11-27
status: done
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-006
  - REQ-AGENTS-ACP-SESSION-EVENTS-007
system_design:
  - ../../specs/agents/system-design/acp-session-events.md
legacy_specs: []
---

# Hide routine Copilot plugin notices

## Diagnosis

The supplied Copilot CLI 1.0.116 trace shows an untagged ready notice followed by
a reply with a message ID. Copilot's dialect has no suppression hook, so live
text reaches chat. The recovery observer also closes its warning window on the
first assistant chunk. A routine notice can therefore consume that window.

## Work order

- [x] [Filter notices and preserve recovery classification](task-01-filter-notices.md)

Use one adapter-level regression for live delivery and extend the existing
recovery sequence. The update prefix comes from a screenshot. Its fixture is
synthetic. If the provider changes either wording, the new text stays visible.
Replay already suppresses history. UI, persistence, and message types are outside scope.

## Verification results

Both regressions reproduced before the fix. The focused tests and full ACP package passed after the fix. Changed-file Go lint reported zero issues.
