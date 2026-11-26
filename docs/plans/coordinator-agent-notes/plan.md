---
created: 2026-11-08
status: draft
requirements:
  - REQ-COORDINATOR-AGENT-NOTES-001
  - REQ-COORDINATOR-AGENT-NOTES-002
  - REQ-COORDINATOR-AGENT-NOTES-003
system_design:
  - ../../specs/coordinator/system-design/agent-notes.md
legacy_specs: []
---

# Coordinator agent-tier notes plan

Implement the [requirements](../../specs/coordinator/requirements/agent-notes.md) and [design](../../specs/coordinator/system-design/agent-notes.md) after the answer-proposals slice ([plan](../coordinator-answer-proposals/plan.md)) is available. The raisable-classes branch is the base, not a reimplementation. One vertical work order keeps storage, per-turn delivery, proposal references and settings coherent. Do not start Build until that dependency is integrated. Before the answer kind exists, AC-COORDINATOR-AGENT-NOTES-003.1 is verified on the existing proposal kinds; the flag requires the coordinator, phase 2, phase 3 and phase 3.1 flags.

| Order | Result | Dependency |
| --- | --- | --- |
| [01](task-01-agent-notes.md) | Manager-curated notes and validated per-turn use | answer-proposals slice |

## ASCII UI preview

UI-01: coordinator settings, Notes section, populated manager state (AC-001.6, AC-003.3):

```text
Notes                                      [Add note]
Project  Release target is November        Valid
  Expires Dec 8 | Used 3 times             [View]
  Referenced by: Proposal 42, Task 12
[Selected note]
  Text and citations
  Versions                                 [Edit] [Delete]
  Version 1                                [Revert]
```

Phone uses the same order in a scrolling list; View opens a full-screen detail/editor with Back and Save. Readers see no write buttons. Note states shown are live, stale, unavailable, not yet validated, expired, over the cap and (with the include-deleted toggle) deleted; the editor takes typed citation identifiers and shows the localized reason of a rejected save. Empty list says no notes; loading displays progress; failed read offers retry; editor failures keep the draft. Structural order and access are required; spacing and copy are illustrative. Verify with desktop and phone Playwright cases, Tab and Enter operation of list, editor, delete and revert, in English and one non-Latin-script locale, as the work order lists.

## Verification and risks

The settings surface and per-turn behavior require E2E coverage, including a second turn in the same conversation, stale citation, permissions and mobile controls. Run the targeted backend tests, web checks and Playwright command in the work order. Citation lookup failures omit affected notes; prompt delivery recovery and note/proposal transaction races are high-risk integration boundaries. No dream implementation or user-facing feature outside notes is included.
