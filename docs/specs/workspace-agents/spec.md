---
status: implemented
---
# Workspace agent connections

Agents are configured from Workspace settings > Agents. The flow selects a role,
name, execution profile and executor, then links responsibilities, permissions,
routines and conversation. Each workspace explicitly selects at most one primary
chief, persisted server-side. Selecting a chief does not enable autonomous routines.
The chief remains accessible outside Office, with workspace-explicit links and a
return path from conversation to agent settings. Existing workspaces can enable
Office task workflows without creating a new workspace or a default CEO.

Execution profile overrides must be scoped and validated, must survive reload and
must not silently fall back to another account. Workspace changes must not display
or mutate stale agents. Feature-disabled installations retain existing navigation.
Readiness describes configuration, not proof of remote subscription identity.

Clarification: Kanban onboarding precedes Office setup. Personas attach to existing
execution profiles. Per-persona delegation context is editable and enters a bounded
chief directory; the chief can retrieve more workers as needed. Setup does not
create another workspace, default CEO or scheduled routine. Board/Office switching
retains workspace identity.
