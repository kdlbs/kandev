---
created: 2026-11-01
status: in_progress
requirements:
  - REQ-AGENTS-ACP-SESSION-EVENTS-001
  - REQ-AGENTS-ACP-SESSION-EVENTS-002
  - REQ-AGENTS-ACP-SESSION-EVENTS-003
  - REQ-AGENTS-ACP-SESSION-EVENTS-004
  - REQ-AGENTS-ACP-SESSION-EVENTS-005
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-001
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-002
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-003
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-004
system_design:
  - ../../specs/agents/system-design/acp-session-events.md
  - ../../specs/agents/system-design/subagent-provider-lifecycle.md
legacy_specs: []
---

# Implementation Plan: ACP Schema 1.28 Adoption

## Overview

Deliver the ACP improvements on PR #4983 (`feature/compare-fork-with-up-f9i`). The order is: repin
the merged SDK, add debug tooling that later checks use, ship compaction and notices with their
capability advertisement, then improve subagents per provider. Control labels come first in the
subagent work because the Codex, Copilot, and Claude work orders populate them.

## Scope

### In scope

- SDK repin to fork `main`.
- Compaction entries and notice rows, with capability advertisement.
- Codex child end state and results.
- A Copilot dialect with safe nesting.
- Claude background settlement, final report, and resume marking.
- Control operation labels.
- `acpdbg --client-capabilities` and model selection through config options.

### Out of scope

- Native child sessions, session messages, system prompts, and MCP over ACP.
- Separating Copilot child replies from assistant text.
- A release toggle; see the system design.

## Technical approach

### Provider compatibility matrix

| Provider | Transport | Identity or capability shape | Intended behavior | Evidence | Unsupported fallback |
| --- | --- | --- | --- | --- | --- |
| Claude 0.81.2 | ACP | `session.compaction`, `session.notices`, `toolResponse.agentId` | Entry with summary and tokens; notices; settlement; report; resume | Live captures | Generic rendering; no report |
| Codex 2.2.2 | ACP | `session.compaction`, `session.notices`, `agentsStates` | Status-only entry; card outcomes | Live captures | Card stays last state until turn ends |
| Copilot 1.0.88 | ACP | `agent_type`, `_meta["github.com/copilot"].agentId` | Cards; safe nesting | Live captures | Top-level tool rows |
| OpenCode 1.18.32 | ACP | None observed | Unchanged | Live captures | Existing text output |
| Other ACP agents | ACP | Standard fields if sent | Standard entries and notices | Mock agent only | Existing frames |

### Boundaries

- **SDK:** `apps/backend/go.mod`, `apps/backend/go.sum`.
- **Compaction and notices:** `streams.ToolKindContextCompaction`, `EventTypeSessionNotice`,
  `adapter_session_events.go`, `clientCapabilitiesForAgent`, `handleACPUpdate`,
  `handleSessionNoticeEvent`, and `ContextCompactionMessage`.
- **Codex:** `codexControlCardUpdates` in `adapter_tools.go` or a sibling file.
- **Copilot:** `dialect_copilot.go`, `adapter_copilot_subagents.go`.
- **Claude:** `adapter_claude_subagents.go`, `adapter_prompt.go`, `ResumeCount`.
- **Labels:** `GenericPayload.SubagentControl`, `tool-call-message.tsx`.

## ASCII UI preview

Spacing is illustrative. Icon position, row grouping, and label text are the structural
requirements. Copy is localized.

UI-01: Compaction entry, in progress and completed, shared by desktop and phone (AC 002.1 to 002.5,
002.10). The phone uses the same stacked row, with metadata wrapping below the title and a full-width
summary. The whole row is the tap target.

```text
[~] Compacting context...

[v] Context compacted                     31.3k -> 2.4k tokens   manual   26.8s  >
    expanded:
    +-------------------------------------------------------------------------+
    | Summary (markdown, bounded preview)                                     |
    +-------------------------------------------------------------------------+

[!] Context compaction failed                                                  >
    expanded: reported error text
```

UI-02: Notice rows, using the existing status row with expandable details (AC 003.1, 003.2).

```text
(i) Info title
/!\ Rate limit approaching                                                     >
    expanded: Retry later
(x) Error title
```

UI-03: Subagent card identity and result; the shared card shows the reported name and type while
expanded content retains description, prompt, result, and child calls on desktop and phone (AC
001.1, 001.2, 002.1, 002.3, 003.3, 003.4).

```text
[>] explore  reader  Read a.txt   background  resumed x1  sonnet
    expanded:
    Description: Read a.txt contents
    Prompt: Read a.txt and report its contents.
    Result: The command output was: apple
    +- Read b.txt
```

UI-04: Control operation rows; existing tool row with a label (AC 004.1, 004.3).

```text
[v] Waited for subagents
[v] Messaged subagent
[v] Closed subagent
```

## Tests

| ACs | Evidence |
| --- | --- |
| 001.1, 001.2, 001.3 (`ACP-SESSION-EVENTS`) | `go.mod` check; `adapter_prompt_failure_test.go`; `acp_probe_capabilities_test.go` |
| 002.1 to 002.6, 002.8, 002.9 | `adapter_session_events_test.go` |
| 002.7, 002.10 | `context-compaction-message.test.tsx`; E2E below |
| 003.1 to 003.5 | `adapter_session_events_test.go`; `event_handlers_session_notice_test.go`; `status-message.test.tsx` |
| 004.1 to 004.3 | `adapter_client_capabilities_test.go`; `acp_executor_test.go` |
| 005.1 to 005.3 | `apps/backend/cmd/acpdbg/main_test.go`; `internal/agent/acpdbg/ops_test.go` |
| 001.1 to 001.6 (`SUBAGENT-LIFECYCLE`) | `codex_v2_subagent_outcomes_test.go`, using captured frames |
| 002.1 to 002.6 | `dialect_copilot_test.go`, using captured parallel and background frames |
| 003.1 to 003.6 | `claude_subagent_settlement_test.go` |
| 004.1 to 004.3 | `subagent_control_test.go`; `tool-call-message.test.tsx` |

Wire fixtures live under `apps/backend/internal/agentctl/server/adapter/transport/acp/testdata/`.
They contain only sanitized `session/update` frames. Remove `_auth` frames, account identity, and
host paths before committing.

## E2E tests

| Flow | ACs | File | Project |
| --- | --- | --- | --- |
| Mock agent compaction entry and summary | 002.1 to 002.5 | `apps/web/e2e/tests/chat/context-compaction.spec.ts` | `chromium` |
| Same flow on phone | 002.10 | `apps/web/e2e/tests/chat/mobile-context-compaction.spec.ts` | `mobile-chrome` |
| Mock agent notice row | 003.1, 003.2 | `apps/web/e2e/tests/chat/session-notice.spec.ts` | `chromium` |
| Mock agent resumed subagent and control label | `SUBAGENT-LIFECYCLE` 003.4, 004.1 | `apps/web/e2e/tests/chat/subagent.spec.ts` | `chromium` |
| Same flow on phone | `SUBAGENT-LIFECYCLE` 004.3 | `apps/web/e2e/tests/chat/mobile-subagent.spec.ts` | `mobile-chrome` |

## Work orders

- [x] [Task 01: Repin the ACP SDK to fork main](task-01-repin-sdk.md)
- [x] [Task 02: Extend acpdbg capabilities and model selection](task-02-acpdbg-capabilities.md)
- [x] [Task 03: Show compaction entries](task-03-compaction-entries.md)
- [ ] [Task 04: Show provider notices](task-04-provider-notices.md) (blocked: notice row and durable inbox cursor are not committed atomically)
- [x] [Task 05: Label subagent control operations](task-05-control-labels.md)
- [x] [Task 06: Apply Codex child outcomes](task-06-codex-outcomes.md)
- [x] [Task 07: Add the Copilot subagent dialect](task-07-copilot-dialect.md)
- [x] [Task 08: Settle and resume Claude subagents](task-08-claude-lifecycle.md)

## Verification results

Task 01 complete. `go build -trimpath -tags fts5 ./...` and
`go test -trimpath -tags fts5 -p=2 ./...` passed against SDK revision
`70cc752b95c6`. The focused SDK malformed-option test, Kandev unsupported-model
test, and remote prompt-error-origin test also passed.

Task 02 complete. The scoped acpdbg tests and Go lint passed. The CLI build
passed, and the live Claude ACP probe recorded both requested session
capabilities in its initialize frame.

Task 03 complete. The full ACP/stream race suite and targeted mock-agent and
utility tests passed. The backend build, web typecheck, compaction and grouping
unit tests, locale check, and full web lint passed. Desktop and phone E2E passed;
the phone flow checks a touch-sized control, separate metadata layout, and no
horizontal overflow. Changed-file Go lint and docs/spec validation passed.

Task 05 complete. ACP/stream race tests, mock-agent tests, focused control-row
unit tests, i18n check and ratchet, web typecheck and lint, changed-file Go lint,
and docs/spec validation passed. Desktop and phone subagent E2E each passed with
three tests.

Task 06 complete. Captured parallel and interrupted Codex outcome tests passed
in the ACP/stream race suite, along with changed-file Go lint and docs/spec
validation. Outcomes preserve sibling isolation and do not attest background
work.

Task 07 complete. Captured Copilot foreground, parallel, and background frame
tests passed in the full ACP/stream race suite. Changed-file Go lint and docs/spec
validation passed. Regression tests cover unresolved background identity and
self-parent exclusion. Copilot-only UI rendering and generic fallback tests passed;
web lint and desktop and phone subagent E2E passed.

Task 08 complete. The full ACP/stream race suite, orchestrator and mock-agent
tests, changed-file Go lint, web lint and typecheck, focused subagent unit tests,
locale checks, desktop and phone subagent E2E, and docs/spec validation passed. The
backend Go build and production Vite build also passed. Regression tests cover
background count preservation after completion, inactive-turn activity, Claude
foreground resume from sanitized frames, result preservation, and parent links.
Task 04 remains blocked pending atomic notice and inbox-cursor persistence.

## Risks

- **PR scope:** PR #4983 changes from a dependency chore to a feature set. Its title and body need
  updating at push, and its branch was rebased, so the push needs `--force-with-lease`.
- **Notice duplicates:** exactly-once depends on the durable delivery cursor. Task 04 tests this and
  stops if it is not guaranteed.
- **Claude output file:** the format is private and can change; the feature degrades to no result.
- **Liveness:** Claude presentation updates pass through a payload that can carry background-work
  identity. Task 08 must prove counts do not change.
- **Claude turn settlement:** the end-of-prompt signal is based on three captures and provider source.
  A new provider version may settle differently, which would mark a card completed early.
- **Copilot background parsing:** the agent ID comes from prose. A wording change removes nesting
  for background children but breaks nothing else.
- **Capture retention:** evidence is in `/tmp/acp-pr7/`, which is temporary. Recapture with
  `acpdbg --client-capabilities` from Task 02 if it is gone.
