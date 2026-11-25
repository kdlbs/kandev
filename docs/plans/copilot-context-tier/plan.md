---
created: 2026-11-23
status: completed
requirements:
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-001
system_design:
  - ../../specs/agents/system-design/copilot-context-tier.md
legacy_specs: []
---

# Implementation plan: Copilot long context profile preset

Use the existing Copilot CLI flag catalogue to expose an opt-in launch
preference. Remove the proposed context control and process replacement from
the prompt path.

| Work order | Outcome |
| --- | --- |
| [Task 01](task-01-copilot-profile-long-context.md) | Save and preview the Copilot flag on desktop and phone |

## ASCII UI preview

UI-01 is the existing Copilot profile page, scrolled to the CLI Flags section.
The same settings row appears in the phone's vertical settings flow.
The existing Save changes action saves the switch.

```text
CLI Flags
+-----------------------------------------------+
| Long context                             [off]|
| Request a larger window on supported models.  |
| --context long_context                        |
+-----------------------------------------------+
```

The row and command preview cover AC-AGENTS-COPILOT-CONTEXT-TIER-001.1 and
AC-AGENTS-COPILOT-CONTEXT-TIER-001.2. The drawing abbreviates the description.
