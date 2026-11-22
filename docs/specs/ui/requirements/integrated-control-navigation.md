---
id: integrated-control-navigation
title: Integrated control navigation
status: draft
system: ui
owners: [kandev]
---

# Integrated control navigation

### REQ-UI-INTEGRATED-CONTROL-001: Native navigation and original conversations

- **AC-UI-INTEGRATED-CONTROL-001.1:** OFF preserves legacy navigation and performs no integrated reads; every shipped profile defaults OFF.
- **AC-UI-INTEGRATED-CONTROL-001.2:** ON adds the picker destination, five separate menus, and the same workspace/coordinator/current-conversation tree on desktop and phone.
- **AC-UI-INTEGRATED-CONTROL-001.3:** Selecting an existing conversation keeps integrated navigation, original IDs and drafts, with no prepare/open/resume/prompt or agent launch.
- **AC-UI-INTEGRATED-CONTROL-001.4:** Existing native permissions apply; unavailable and failed reads are distinct from empty data; scope limitations are not reported as completed capabilities.

The workspace picker offers an opt-in Integrated control destination. It owns
a dedicated navigation context, rather than representing a synthetic workspace.
Its sidebar keeps five destinations: Overview, Activity, Workspaces,
Provider / model, and History. Available workspaces contain their coordinators
and existing current conversations. Choosing a conversation renders the original
task/session centrally while keeping this navigation on desktop and phone.

Existing workspace permissions remain authoritative. Listing uses workspace
read access; the existing interactive coordinator conversation requires workspace
management access. Navigation never creates, prepares, resumes, copies, or moves
a task/session, sends a prompt, or starts an agent. Session drafts, selections,
questions, results, and histories retain their original identity.

The original task's GitHub/GitLab/Azure links, connection status, feedback,
and explicit VCS actions use its own workspace. Adding check feedback to chat
uses that conversation's session. Keeping Integrated control navigation must
neither redirect these operations to the selected regular workspace nor replace
that workspace's VCS cache.

This first slice exposes only existing workspace/coordinator/current-conversation
data. Full execution dashboards, provider/account/model usage, historical
conversation discovery, and multiple independently live conversations remain
parts of the same feature, not capabilities claimed by this slice. A coordinator's
current task, an earlier archived task, and runtime sessions of one task are
different objects. No new aggregate API, poller, ACL, or task ledger is introduced.

`features.integratedControl` / `KANDEV_FEATURES_INTEGRATED_CONTROL` is off in
every shipped profile. Disabling it restores the existing navigation without
deleting data or changing active work. Coordinator data also requires the
existing coordinator feature. AI cross-workspace control is a separate feature;
its completion does not block this authorized browser read surface.
