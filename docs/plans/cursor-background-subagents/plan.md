---
created: 2026-11-27
status: done
requirements:
  - REQ-AGENTS-SUBAGENT-LIFECYCLE-005
system_design:
  - ../../specs/agents/system-design/subagent-provider-lifecycle.md
legacy_specs: []
---

# Cursor background subagent completion

The interactive handshake on da5492db omits Cursor's subagent capability. With that handshake, a live child wrote its marker 25 seconds after the parent turn ended. The open connection received no subsequent frame in 90 seconds. With the capability enabled on the same CLI, the parent received a spawn at 7 seconds, a completed state at 39 seconds, and reported success before the prompt returned at 41 seconds. The runtime also needs to recognize these two provider-specific update shapes to populate Background Jobs.

## Work order

- [x] [Handle Cursor child lifecycle on the interactive connection](task-01-cursor-child-lifecycle.md)

Use the existing desktop and phone Background Work views. During the child run the job is active. After Cursor reports completion the job appears in History and the parent's report appears in the original chat turn.

## Verification status

The merged SDK hook preserves both vendor updates. The Kandev adapter regression passed for both launch orders. A live Cursor child produced one running job, one completed job with the same identity, and a parent report in the original turn. A disposable Kandev instance and Chromium showed one running child, the composer chip, and a completed History entry after reload.
