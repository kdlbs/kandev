# ADR-2026-08-31-deterministic-noninteractive-git-transport: Keep Internal Git Transport Deterministic and Non-interactive

**Status:** accepted
**Date:** 2026-08-31
**Area:** backend, agentctl, protocol, security, operations

## Context

Kandev materializes repository-qualified comparison targets with Git. The target uses a canonical HTTPS URL. This work can occur during instance creation or a Git-status UI request.

The workspace tracker starts Git with the ambient agentctl environment. It does not use the instance environment that contains managed Git credentials. It also permits terminal prompts.

If HTTPS credentials are absent, Git can request a user name and password from the launcher terminal. The Git command then blocks instance readiness and the UI.

One possible repair retries the target through SSH after an HTTPS error. This retry can use a different identity, credential source, and host-trust policy. It can also start an SSH host-key or key-passphrase prompt.

## Decision

Kandev-owned Git network commands are non-interactive. They cannot read credentials, confirmation, or host-trust answers from the launcher terminal.

Each workspace tracker uses a detached copy of the instance's effective Git environment. The environment preserves managed credential-helper entries and direct OpenSSH command options. Kandev adds the established prompt controls and puts `BatchMode=yes` before inherited batch-mode options. Unsupported shell prefixes and wrappers use the safe non-interactive default. The existing command deadline remains in force.

Kandev selects one transport before it starts the command. An authentication or transport error does not cause an automatic retry through SSH, HTTPS, or another transport.

Repository-qualified comparison targets keep their canonical HTTPS URLs. A missing credential makes comparison data unavailable. It does not change `origin`, the checkout upstream, or push routing.

Comparison-target network work does not delay instance readiness or Git-status UI requests. The target remains pending and fail-closed until background materialization succeeds. The process-manager lifetime controls cancellation.

## Consequences

- A Git authentication error cannot capture the launcher terminal.
- Instance creation and Git-status UI requests do not wait for comparison-target network work.
- Managed HTTPS credentials can reach comparison-target commands through the instance environment.
- Explicit SSH configuration remains available for commands that already select SSH, with terminal prompting disabled.
- A user must select or configure another transport explicitly. Kandev does not guess after an error.
- Private comparison targets remain unavailable when the selected credential scope cannot read them.

## Alternatives considered

### Retry SSH after an HTTPS error

Rejected. The retry can change identity and host trust after the user selected a different transport. It can also create a second interactive prompt.

### Use only the host environment

Rejected. Remote executors and managed credentials use the instance environment. The host environment can contain unrelated credentials or no credentials.

### Keep materialization synchronous with prompts disabled

Rejected. Prompt suppression removes one unbounded wait. Network and credential-helper deadlines can still delay startup and UI requests.

## Related

- [ADR-2026-08-19-repository-qualified-comparison-targets](2026-08-19-repository-qualified-comparison-targets.md)
- [Workspace Git Status requirements](../specs/platform/requirements/workspace-git-status.md)
- [Workspace Git Status system design](../specs/platform/system-design/workspace-git-status.md)
