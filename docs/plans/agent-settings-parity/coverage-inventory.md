# Agent settings coverage inventory

This inventory is the independent denominator for the agent-settings parity
delivery. It is maintained from Settings UI inputs, HTTP/WS request contracts,
and domain owners. The runtime discovery catalog is not its source.

## Evidence source

The executable inventory is
`apps/web/lib/settings-discovery/coverage-inventory.ts`. Its tests verify all
required domains, concrete catalog resource mappings, owner records, and
explicit exception recovery paths.

## Required domains

| Domain                   | Field-level evidence                                                | Concrete resource types                                                                                                                                                                                                                             | Work order | Final status |
| ------------------------ | ------------------------------------------------------------------- | --------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------- | ---------- | ------------ |
| `agents`                 | editable agent definitions                                          | `agent`                                                                                                                                                                                                                                             | 01-02      | supported    |
| `profiles`               | profile fields and MCP document                                     | `agent_profile`, `agent_profile_mcp`                                                                                                                                                                                                                | 01-02      | supported    |
| `user_preferences`       | task behavior, shortcuts, terminal, layouts, utility defaults       | `user_settings`                                                                                                                                                                                                                                     | 05         | supported    |
| `workflows`              | workflow and step settings                                          | `workflow`, `workflow_step`                                                                                                                                                                                                                         | 06         | supported    |
| `workspaces`             | workspace, repository, set, and script settings                     | `workspace`, `repository`, `repository_set`, `repository_script`                                                                                                                                                                                    | 07         | supported    |
| `execution`              | executor, profile, and environment settings                         | `executor`, `executor_profile`, `environment`                                                                                                                                                                                                       | 08         | supported    |
| `tasks`                  | permitted task configuration                                        | `task`                                                                                                                                                                                                                                              | 09         | supported    |
| `prompts`                | saved prompt metadata and content                                   | `prompt`                                                                                                                                                                                                                                            | 10         | supported    |
| `utilities`              | utility agent configuration and binding                             | `utility_agent`                                                                                                                                                                                                                                     | 11         | supported    |
| `editors`                | editor definitions                                                  | `editor`                                                                                                                                                                                                                                            | 12         | supported    |
| `notifications`          | provider defaults and event subscriptions                           | `notification_provider`                                                                                                                                                                                                                             | 13         | supported    |
| `issue_integrations`     | noncredential Jira, Linear, and Sentry settings and watches         | `issue_integration`, `jira_settings`, `jira_issue_watch`, `linear_settings`, `linear_issue_watch`, `sentry_instance`, `sentry_issue_watch`                                                                                                          | 14         | supported    |
| `code_host_integrations` | noncredential GitHub, GitLab, and Azure DevOps settings and watches | `code_host_integration`, `github_settings`, `github_review_watch`, `github_issue_watch`, `gitlab_settings`, `gitlab_review_watch`, `gitlab_issue_watch`, `azure_devops_settings`, `azure_devops_work_item_watch`, `azure_devops_pull_request_watch` | 15         | supported    |
| `automation`             | automation definitions and triggers                                 | `automation`, `automation_trigger`                                                                                                                                                                                                                  | 16         | supported    |
| `runtime`                | permitted persisted overrides                                       | `runtime_flag`                                                                                                                                                                                                                                      | 17         | supported    |
| `storage`                | schedule and retention document                                     | `storage_maintenance`                                                                                                                                                                                                                               | 18         | supported    |

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
