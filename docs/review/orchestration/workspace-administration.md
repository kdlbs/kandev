# Workspace and workflow administration

The Orchestrator previously had task controls and a read-only resource directory,
but no way to register a repository or change workspace/workflow configuration.
This repair adds `manage_workspace` to both workspace and private conversation
brokers. It uses existing Kandev runtime credentials and native services.

## Scope

- Update assigned-workspace name, description, executor/environment defaults and
  task/configuration agent defaults.
- Create, update, delete and reorder delivery workflows. Templates are discoverable
  in `workspace`; workflow deletion uses native task archival.
- Create, update, delete and reorder workflow columns, including prompts, event
  rules, start-column selection, execution profiles, queue/WIP settings and
  progression settings. Publish changes and demoted start columns to live views.
- Register existing local Git checkouts or remote repositories; update native
  repository settings and remove registrations through native cleanup checks.
- Expose current workspace settings and workflow templates in the directory.

## Boundaries and verification

The signed session fixes the workspace. Current run/intent checks remain active;
private conversations require execute mode. Payload IDs cannot override the
workspace or target another workspace's workflow, step, task or execution profile.
Hidden/system and source-managed workflows remain protected. Linked-workspace
task grants do not authorize configuration. Workspace ownership, organization
access and global installation configuration remain human administration.

Regression tests exercise native persistence, lifecycle events, start-column
demotion, ordering, invalid WIP/cycle references, foreign task/step/review-profile
references, expired runs, private mode restrictions and broker discovery.
The immutable candidate is `0.94.0-orchestration.20260921.sha8d9bb81c64d0`.
Its [two-turn Claude-backed trial](workspace-administration-provider-receipt.json)
verified saved workspace settings, workflow and column creation/edits, repository
registration and subsequent cleanup. Repository removal is verified using native
visibility: the registration becomes a tombstone rather than disappearing from
storage. The trial used the Orchestrator's existing native login after renewal;
the broker requires no additional API key.

The [portable trial fixture](fixtures/workspace-administration-provider-trial.py)
accepts the bundle's `bin/kandev` path and the selected native Claude configuration
directory. It creates a disposable Kandev database and Git checkout, sends only
generic prompts and checks persisted state. Native provider authentication remains
in the selected profile; credential contents are never copied or printed. Raw
provider logs are local diagnostic material and must not be published.

Go race tests cover backend composition, Orchestration and agentctl; repository
lint/hooks and public documentation validation passed. Deployment qualification
is recorded in [the live receipt](workspace-administration-live-receipt.json).
No user prompts, credential values or production row contents are in this packet.
