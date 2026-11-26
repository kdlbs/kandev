---
status: current
system: agents
requirements:
  - REQ-AGENTS-COPILOT-CONTEXT-TIER-001
---

# Copilot context tier system design

## Ownership and flow

The Copilot agent catalogue provides a disabled preset with flag name
`--context` and value `long_context`. The shared profile editor renders the
preset alongside other CLI flags on desktop and phone. Profile settings
persist the choice in the existing `cli_flags` list.

Command Preview and agent launch resolve enabled entries into command
arguments. The same profile flag applies to the ACP and CLI passthrough
launch paths. When the preset is off, Copilot resolves the context tier
from its own settings unless the user supplies a custom context flag.

Copilot owns model eligibility, session resume, and the effective context
limit. Kandev limits this control to profile launch settings. Prompt dispatch
uses the standard lifecycle, and capability discovery uses the provider's
advertised options.
