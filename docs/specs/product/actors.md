# Product actors

**Status:** Proposed baseline for product review.

An actor is a person, process, service, or extension that interacts with the
Kandev product boundary. Repository content, provider responses, URLs, agent
output, and command arguments are data crossing a trust boundary, not trusted
actors.

## Primary actors

### Developer or task owner

The developer defines the desired outcome, supplies repository context, chooses
the agent and executor boundary, and decides whether the result is acceptable.
The developer can steer sessions, inspect changes, run checks, and continue,
archive, commit, or publish the task.

### Reviewer or maintainer

The reviewer examines the agent result and its evidence. The reviewer may be
the task owner or another human with access to the workspace. Review includes
the diff, tests, repository status, generated files, and the intended external
change before merge or release.

### Operator

The operator installs and runs Kandev, configures the data directory, launch
mode, profiles, executors, integrations, and release channel, and responds to
health or storage problems. In a single-user installation, the operator and
developer are often the same person. Kandev does not currently provide a
general multi-user login and role system.

## Agent actors

### Coding agent

The coding agent interprets task context and proposes or applies repository
changes through the capabilities exposed by its profile and executor. It can
use task MCP, terminal, files, Git, and provider tools when those capabilities
are enabled. Agent output and actions remain subject to the selected trust
boundary and human review.

### Agent provider and CLI

The provider supplies the model, agent CLI, protocol behavior, credentials, and
provider-native capabilities. Kandev represents provider differences through
agent profiles and adapters. A provider is not the owner of Kandev task state,
workflow state, review state, or user permissions.

### Utility or coordination agent

Some Kandev surfaces use an agent for bounded support work such as summarizing
a session, preparing context, or answering a configured external question.
These agents operate under an explicit profile and task or integration scope.
They do not gain authority over the human's review decisions.

## Environment and service actors

### Executor environment

The executor decides where the agent process runs and how its workspace,
processes, ports, credentials, and network access are prepared. Local,
worktree, Docker, SSH, and other executor types provide different isolation
and operational trade-offs. An executor changes the environment boundary; it
does not automatically reduce the permissions granted by a profile.

### Repository and code host

The repository supplies source, history, branches, worktrees, and local Git
state. A code host such as GitHub or GitLab supplies remote branches, issues,
pull requests, reviews, and provider actions when the workspace integration is
configured. Kandev coordinates these systems but does not become their source
of truth.

### External service integration

An integration connects a workspace to a provider such as GitHub, GitLab,
Jira, Linear, Azure DevOps, or Sentry. It owns provider authentication,
provider identity, synchronization, and provider-specific action contracts.
Generic Kandev authentication and durable task ownership remain separate.

### Plugin

A plugin extends Kandev through a manifest, host API, contribution point,
integration, or task capability. Plugins may provide product-specific behavior,
but they do not own core task, workspace, authentication, or platform
contracts. Plugin permissions and external credentials remain explicit.

## Kandev surfaces

The browser UI, Tauri desktop shell, native CLI, HTTP API, WebSocket API, and
task MCP tools are product surfaces used by the actors above. They are not
independent sources of durable authority. The Go backend and its persistence
layer own product state; surfaces recover and render that state.

## Future or limited actors

Office coordinator agents, autonomous routines, scheduled heartbeats, and
participant or quorum flows are documented as an evolving product surface.
They are not treated as supported production actors until the Office boundary
is explicitly graduated in the feature-status contract.

## Open product questions

- Which human roles need distinct permissions when Kandev supports more than a
  single trusted operator?
- Which plugin and integration actions require explicit approval every time,
  and which can be delegated to a workflow?
- What evidence distinguishes a utility agent from an autonomous coordinator in
  user-facing language and telemetry?
