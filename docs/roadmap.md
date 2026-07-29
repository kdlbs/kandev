# Roadmap

High-level direction for the project. This is not a commitment - priorities shift as we learn more.

## Now

- Stability and bug fixes across all agent integrations - current primary focus
- Improved mobile UI - polish the existing mobile view for orchestrating and reviewing from your phone
- Plugin system - installable extensions and plugin APIs, with initial plugins for [Kandy](https://github.com/kdlbs/kandev-plugin-kandy), [provider usage](https://github.com/kdlbs/kandev-plugin-provider-usage), and [session cost](https://github.com/kdlbs/kandev-plugin-session-cost)
- Authentication - secure access controls for Kandev deployments
- Internationalization (i18n) - localized Kandev UI and user experience
- Office mode - new scheduler, routines, agents with skills, cost tracking
- Remote SSH runtime - run agents on remote servers over SSH
- GitLab integration - import issues, manage repos, trigger pipelines

## Later

- Kubernetes operator - auto-scaling agent workloads in a cluster
- Analytics dashboard - agent performance, cost tracking, success rates

## Done

- Multi-repo task support - a single task can span multiple repositories
- Coordinator mode - have an agent coordinate sub-tasks executed by other agents
- Issue tracker integration - import issues from GitHub, Linear, and Jira as tasks
