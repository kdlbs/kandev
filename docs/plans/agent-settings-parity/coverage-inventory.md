# Agent settings coverage inventory

This inventory is the independent denominator for the agent-settings parity
delivery. It is maintained from Settings UI inputs, HTTP/WS request contracts,
and domain owners. The runtime discovery catalog is not its source.

## Evidence source

The executable inventory is
`apps/web/lib/settings-discovery/coverage-inventory.ts`. Its test verifies all
required domains, owner records, and explicit exception recovery paths.

## Required domains

| Domain | Field-level evidence | Work order | Final status |
| --- | --- | --- | --- |
| `agent_profile` | identity, model, fallback, mode, config, flags, environment, command prefix | 01-02 | supported |
| `agent_profile_mcp` | complete MCP server document | 02 | supported |
| `user_settings` | task behavior, shortcuts, terminal, layouts, utility defaults | 05 | supported |
| `workflow` | name and description | 06 | supported |
| `workflow_step` | step settings, bindings, transitions, prompts | 06 | supported |
| `workspace` | name and default execution bindings | 07 | supported |
| `repository` | repository settings and branch policies | 07 | supported |
| `repository_set` | saved repository-set settings | 07 | supported |
| `repository_script` | repository custom scripts | 07 | supported |
| `executor` | executor configuration and status settings | 08 | supported |
| `executor_profile` | profile details and execution settings | 08 | supported |
| `environment` | environment variables and runtime settings | 08 | supported |
| `task` | permitted task configuration | 09 | supported |
| `prompt` | saved prompt metadata and content | 10 | supported |
| `utility_agent` | utility agent configuration and binding | 11 | supported |
| `editor` | editor definitions | 12 | supported |
| `notification_provider` | provider defaults and event subscriptions | 13 | supported |
| `issue_integrations` | noncredential issue integration settings | 14 | supported |
| `code_host_integrations` | noncredential code-host settings | 15 | supported |
| `automation` | automation definitions and triggers | 16 | supported |
| `runtime_flag` | permitted persisted overrides | 17 | supported |
| `storage_maintenance` | schedule and retention document | 18 | supported |

## Explicit exceptions

The executable inventory records these categories with reasons and recovery
destinations: device-local preferences, read-only computed state, interactive
credential enrollment, explicit lifecycle actions, deployment-owned startup
configuration, plugin-owned settings, and Office organization management.

Exceptions do not hide eligible fields. The executable inventory and delivery
test contain no pending eligible field.

## Delivery rule

Task 19 changed every eligible `pending` record to `supported` and retained
every exception reason. The final report lists supported fields and exceptions
without claiming a percentage.
