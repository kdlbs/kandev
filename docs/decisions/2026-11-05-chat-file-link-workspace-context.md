# ADR-2026-11-05-chat-file-link-workspace-context: Separate chat and Files roots

**Status:** accepted
**Date:** 2026-11-05
**Area:** backend, frontend, protocol

## Context

Legacy live attachment preserves the native agent working directory and promotes the Files root.
[The existing decision](2026-07-27-legacy-add-branch-live-rescan.md) requires both outcomes.
A repository-relative chat link therefore needs a base distinct from the current Files root.
The primary checkout also cannot establish the base for a session that starts at a parent workspace.

The raw session workspace column is mutable.
An ordinary session update can replace its original value with the current environment root.
The committed harness generation already records the native-session workspace.

## Decision

Project the confirmed native workspace separately as `agent_workspace_path` in session-detail data.
Use the live execution for ready events and the current incarnation's committed harness generation for durable reads.
Retain `workspace_path` as Files authority and `worktree_path` as primary checkout identity.
No database migration is necessary.

Explicit source-qualified links retain their source.
Other relative links use the confirmed native workspace within the current Files boundary.
Unknown or ambiguous expanded-workspace context cannot authorize a guessed repository.
Absolute aliases keep their existing identity and containment rules.

## Consequences

Relative links survive root promotion and reload without changing the running agent.
Session payloads gain one additive nullable field.
Partial merges must distinguish omission from authoritative null.
Source qualification must use complete workspace-relative prefixes, including nested placements.

The field describes the recorded native working directory.
It does not observe arbitrary shell directory changes or add historical message-location storage.
The [Tasks design](../specs/tasks/system-design/chat-file-link-context.md) owns the implementation contract.

## Alternatives considered

1. Always use the current Files root. This loses the unchanged checkout after promotion.
2. Always use the primary checkout. This changes links from agents that start at the parent root.
3. Preserve the raw session workspace column as the base. Existing session writes do not preserve that meaning.
4. Search attached repositories for an existing filename. Duplicate filenames can open the wrong repository.
5. Add per-message working-directory records. This repair needs no new message storage or backfill.
