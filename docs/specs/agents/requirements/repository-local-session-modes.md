---
status: active
system: agents
created: 2026-11-02
owners:
  - kandev
---
# Repository-local session modes

## Overview

Agent profiles can name modes defined in a task repository. A host capability
probe runs outside that repository and cannot establish whether the mode exists
in the task executor. The agent system owns the profile selection and the
session startup contract.

### REQ-AGENTS-REPOSITORY-LOCAL-MODE-001: Preserve repository-local mode selections

- **AC-AGENTS-REPOSITORY-LOCAL-MODE-001.1:** A profile editor shall allow a user to enter, save, reload, inspect, and replace a mode ID that the host does not advertise, including when the host mode list is empty.
- **AC-AGENTS-REPOSITORY-LOCAL-MODE-001.2:** When a host probe receives a requested mode absent from a present mode advertisement, including an empty advertisement, it shall continue without applying that mode. When no mode advertisement exists, it shall attempt the requested mode and report an application error.
- **AC-AGENTS-REPOSITORY-LOCAL-MODE-001.3:** When a host probe receives a requested mode in the advertisement, it shall apply that mode and report an application error if the provider rejects it.
- **AC-AGENTS-REPOSITORY-LOCAL-MODE-001.4:** When an explicit mode is requested for a task session, the executor shall confirm that exact mode before the first prompt. A rejected, unconfirmed, or different effective mode shall fail the session start.
- **AC-AGENTS-REPOSITORY-LOCAL-MODE-001.5:** The mode picker shall clear its search when it closes, including after selection, so that reopening it shows the available choices.
- **AC-AGENTS-REPOSITORY-LOCAL-MODE-001.6:** The profile mode flow shall support keyboard and touch interaction on desktop and phone without horizontal document overflow.

## Boundaries

A host snapshot after a skipped mode describes the host default context. It does
not prove that a repository mode exists or describe all of its options. An empty
selection retains the provider default. Model fallback follows the separate
[model policy](no-silent-model-fallback.md); this requirement adds no mode
fallback policy or new persistence field.
