# ADR-2026-09-05-agent-owned-credential-file-conflicts: Agent-owned credential file conflicts

**Status:** accepted
**Date:** 2026-09-05
**Area:** backend

## Context

Remote credential methods identify source files and target directories. The shared transfer layer treats each file as an opaque value and replaces its target.

This behavior is correct for single-session token files. It is destructive for OpenCode `auth.json`, which is a provider map.

An SSH target can contain provider logins that do not exist on the Kandev host. A task launch currently removes those target-only entries.

The transfer layer cannot safely infer a file format from its path. Different agents can also change their credential formats independently.

## Decision

The agent remote-auth descriptor owns the existing-file policy. The default policy replaces the target to keep current behavior for opaque credential files.

OpenCode declares a top-level JSON-object merge for `auth.json`. The shared transfer layer applies this policy before it writes the target.

The merge preserves target-only providers and adds source-only providers. A source entry replaces the target entry for the same provider.

The transfer fails closed when an existing target is unreadable or is not a valid JSON object. The transfer does not replace that target.

## Consequences

- SSH task launches preserve OpenCode logins that exist only on the target.
- A selected host login remains authoritative when both hosts contain the same provider.
- Other agents keep their current replacement behavior.
- Persistent file transports need a target-read capability for merge policies.
- Isolated transports validate and write the source because no user target exists.
- New structured credential files can declare a policy without path checks in executor code.
- A concurrent agent write can still race with a read-merge-write transfer. This change does not add a remote file-lock protocol.

## Alternatives Considered

### Refuse every existing credential file

This policy prevents rotation and makes the selected copy method ineffective on persistent executors.

### Back up every replaced file

This policy retains old secrets and creates an unbounded set of credential copies on the remote host.

### Merge every JSON credential file

JSON structure does not imply merge semantics. A generic merge can create an invalid credential document for another agent.

### Add OpenCode path logic to the SSH uploader

This policy couples the transport to one agent and leaves the same transfer rule inconsistent on other executors.
