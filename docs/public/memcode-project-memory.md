---
title: "MemCode Project Memory Recipe"
description: "An experimental community plugin recipe for approved decisions across task sessions."
status: experimental
---

# MemCode project memory recipe

The experimental community
[kandev-plugin-memcode](https://github.com/vivekgupta-memcode/kandev-plugin-memcode)
implements the plugin approach discussed in
[issue 4059](https://github.com/kdlbs/kandev/issues/4059).
It is source for review, not a released or marketplace-listed integration.

## How it works with ACP sessions

The plugin implements the SDK's `AgentToolPlugin` interface and declares a
read-only `recall_decisions` tool for task surfaces. Kandev exposes this through
its existing task MCP server and invokes the managed plugin with verified
workspace/task/session context. ACP agents use the existing MCP configuration;
the plugin does not run a separate MCP endpoint.

A toolbar popover lets the signed-in human type an exact fact and approve its
save through the authenticated `decisions.save` task action. The model has no
write tool. Prompts, conversation transcripts and worktree files are never
automatically ingested. Search queries and explicitly submitted facts are sent
to the external MemCode service over HTTPS.

## Authority and scope

The operator provisions an authorized MemCode space and credential, saves the
key in a secret config field, and grants exact workspace/task bindings to a
user, repository and MemCode actor/space. Bindings are explicit operator policy,
not model input. The plugin rejects missing or duplicate task bindings and
checks the human actor on saves. Recall drops records without exact space,
user and repository provenance.

The MCP tool context does not contain a human actor identity. This first recipe
therefore uses operator-approved task grants rather than inferring a user from
ACP. Grant the same user/repository/space to another task explicitly to share
approved project decisions across task sessions. Returned facts remain untrusted
reference data and must not be executed as instructions.

## Review and verification

The source repository contains configuration examples, tests, and packaging
commands. It pins the Kandev SDK checkout used by CI and local packaging to
`8bc94934e9a1a45a341d00913d7aa3df7864e96f`.

The backend tests cover task authorization, isolation, explicit saves, stable
idempotency, cancellation, and generic service errors with fake external
boundaries. The host-platform archive passes `plugin-pack` validation and
checksum inspection. This is not proof of installation or ACP compatibility.

Disposable-host installation, toolbar/ACP smoke testing, release-host version
compatibility, localization, and live MemCode verification remain pending.
Do not publish the release or registry entry until those checks pass. Ingestion
receipts are asynchronous, so a queued job is not proof a fact is searchable.
Retention and deletion belong to the authorized MemCode application flow.

See [Authoring a Plugin](plugins-authoring.md) for the host contract and required
install/smoke-test workflow.
