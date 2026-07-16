---
title: "Operations"
description: "Operate, back up, update, monitor, and recover a local or self-hosted Kandev installation."
---

# Operations

Kandev can run as a local developer application or an always-on service. In both cases the backend owns durable task state, worktree metadata, credentials, logs, and the static web application. Treat its data directory as stateful application storage.

## Start with system status

Open **Settings > System > Status** to inspect health issues, build/version information, disk usage, and UI state. Use this page before debugging an individual task: a database, filesystem, process-limit, or disk problem can affect every executor.

The backend also exposes health checks for service managers and deployment probes. Use the endpoints documented by the selected deployment guide rather than checking only whether the web page returns HTML.

## Data and database

SQLite is the default database and fits local or single-instance service deployments. The database and managed workspaces live under the configured Kandev data directory, `~/.kandev` by default.

PostgreSQL is optional for deployments that configure it. Do not point multiple Kandev processes at one SQLite file or assume that switching database drivers migrates existing state automatically.

**Settings > System > Database** shows the active driver, size, and supported maintenance operations. Take a verified backup before reset, migration, upgrade, or destructive cleanup.

## Backups

For SQLite, **Settings > System > Backups** creates consistent snapshots with SQLite `VACUUM INTO` under `<data-dir>/backups/`. Kandev can also create safety snapshots around update flows.

A complete recovery plan also accounts for:

- repository branches and unpushed commits in managed worktrees;
- executor-specific remote state;
- service configuration and environment variables;
- secrets stored outside or alongside the database;
- provider-side pull requests, issues, and Gist shares.

Copy backups off the host and test restoration on a separate instance. A backup that has never been restored is not a verified recovery path.

## Logs

**Settings > System > Logs** shows recent backend output and downloadable rotated log files. The CLI flags `--verbose` and `--debug` increase detail; debug mode can include agent message dumps, so avoid it on sensitive workloads unless the storage and retention are acceptable.

For a service deployment, capture stdout/stderr in the service manager as well. Correlate task ID, session ID, execution ID, repository, and timestamp across Kandev, agentctl, executor, reverse-proxy, and provider logs.

## Disk and stale environments

Repositories, worktrees, task files, agent logs, package caches, container layers, and backups all consume disk. System status reports Kandev-managed usage, but remote hosts and Docker storage need their own monitoring.

Before removing a stale worktree or environment:

1. inspect uncommitted and untracked files;
2. confirm commits were pushed to the intended remote;
3. record any associated pull request;
4. stop active sessions using it;
5. remove it through Kandev when possible so database state stays consistent.

## Updates

**Settings > System > Updates** reports the current and latest release and shows changelog information. For persistent installations, update through the install channel:

```bash
brew upgrade kandev
npm install -g kandev@latest
```

If you run Kandev transiently with npx, invoke the latest release with `npx kandev@latest`; this does not update a persistent installation.

For a service or container, follow that deployment's replacement and restart process. Read release notes, create a backup, drain or stop active sessions, update the runtime, and check status before resuming automation.

## Runtime metrics

Kandev can show supported host and task-environment resource metrics. Use them to identify CPU, memory, load, temperature, and disk pressure. Configure alerting and hard limits in the host, container platform, or remote provider; the UI is observability, not a scheduler quota.

## Feature toggles and diagnostics

**Settings > System > Feature Toggles** controls experimental or diagnostic features that are compiled into the running release. A toggle can require a restart and can expose unfinished behavior. Record non-default toggles in incident reports and disable them when isolating a regression.

Office mode is feature-flagged and in progress. Do not use its internal presence as evidence that persistent autonomous teams are supported for production use.

**Settings > System > Licenses** lists shipped Go and npm dependency licenses. **About** shows build metadata useful in support reports.

## Remote deployment security

- Put the web/backend behind a private network or an authenticated ingress appropriate to your organization.
- Do not expose the unauthenticated external MCP endpoint to the internet.
- Run Kandev under a dedicated service user with only the repository and executor access it needs.
- Store provider and Git credentials in the deployment secret system and rotate them independently.
- Restrict Docker socket, SSH keys, forwarded ports, and prepare scripts as privileged interfaces.
- Back up state before changing versions, database drivers, or data-directory ownership.

See [Run as a service](run-as-a-service.md), [Docker](docker.md), [Remote environments](remote-cloud-environment.md), and [Configuration](configuration.md) for concrete deployment settings.

## Incident checklist

1. Disable the affected automation or stop new task starts.
2. Preserve logs and note version/build metadata.
3. Inspect database, disk, process, executor, and integration health.
4. Protect unpushed repository state before cleanup or rollback.
5. Restore or roll back only from a verified backup and compatible version.
6. Re-enable one workflow or executor at a time and monitor run history.
