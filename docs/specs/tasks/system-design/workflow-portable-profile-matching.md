---
status: current
system: tasks
requirements:
  - REQ-TASKS-PORTABLE-PROFILE-001
---

# Portable workflow profile matching design

## Purpose and boundaries

The workflow portable format and sync reconciliation belong to tasks. Agent
settings supplies profile values and eligibility; the workflow service does
not modify profiles. This extends the existing import-selection contract rather
than replacing explicit browser bindings.

## Requirement mapping

| Acceptance criteria | Design sections |
| --- | --- |
| 001.1, 001.4, 001.7 | Descriptor and compatibility |
| 001.2, 001.3, 001.6 | Matching and synchronization |
| 001.5 | Import and review actions |

All identifiers have prefix `AC-TASKS-PORTABLE-PROFILE-`.

## Descriptor and compatibility

Add version 3 of the existing envelope. Version 3 inherits all version 2
completion and session-target rules. Each present profile descriptor carries
`agent_name`, `model`, `mode`, and a required `config_options` object, including
`{}` for no options. Export sanitized persisted ACP profile configuration
options, never the broader profile settings, environment, flags, secret IDs,
credentials, or provider authentication. Options are user-supplied values and
are included verbatim in shared files; document that exposure. Compare maps by
key and value independent of order, treating nil and empty local maps equally.
Reject malformed options, including null, non-string values, reserved
model/mode keys, and blank keys or values, rather than silently modifying the
authored identity. Model and mode retain their existing exact empty semantics.
When exporting version 3, emit every version 2 required completion field even
when false, so the emitted document passes its own validation.

Versions 1 and 2 omit the option constraint; absence differs from an explicit
empty version 3 map. Reject option-bearing descriptors in versions 1 and 2 to
avoid presenting ignored constraints as effective. Existing older binaries
already reject version 3. Emit version 3 for new exports that contain any
profile descriptor; preserve existing version selection for exports without
profiles. Handle nested review-action descriptors with the same version rules.
Do not change conditional `configure_session` rules, which are not profile
descriptors. No database migration is required.

## Matching and synchronization

Extend the matcher contract to accept a complete descriptor and return
structured outcomes for unique, missing, ambiguous, and lookup failure. Search
enabled global candidates using the version-specific comparison. An existing
binding can additionally be retained when it matches and is disabled or scoped;
count it once alongside eligible candidates. If it matches and no other eligible
candidate matches, retain it. If another eligible candidate also matches,
report ambiguity. This preserves disabled bindings without allowing the
current-ID shortcut to hide an ambiguous requested identity.

Preflight every explicit workflow, direct-step, and review-action reference
for a desired workflow before creating or updating any part of it. Resolve
all IDs once and use that prepared result during application, including
position remapping and existing session-target validation. For each unresolved
reference, return a bounded warning with source path, workflow and step/action
context, reason, and recovery direction, without option values. Skip the
whole desired workflow and mark it seen so stale-definition cleanup cannot
delete it; continue with other workflows. Repository read errors become a
failure outcome, never a partial candidate set or a missing match. Preserve
existing storage-error guarantees: this preflight does not promise a database
transaction for failures occurring during writes.

Publish warnings through the existing sync result and `last_warnings` status
mechanism. The existing desktop and mobile status surfaces display these
warnings; do not introduce a new control or expose option values in logs.
Ensure repeated reconciliation re-evaluates profiles even if source bytes are
unchanged and success after correction clears the old warning.

## Import and review actions

Export and resolve review-action nested maps without losing options. Update
the sync review preservation path so descriptors with different option maps
cannot share a key or reuse the wrong action's binding. Browser preview uses
the same option-aware comparison, and descriptor equality for submitted
bindings compares both presence and map content. Existing explicit selected
IDs, eligibility and revision validation remain authoritative.

Raw YAML and MCP import keep their documented deterministic tie break and
missing behavior; their comparator includes options when version 3 specifies
them. Browser automatic matching keeps the existing tie break for duplicate
matches and invokes the existing picker for zero matches. This limited
compatibility rule does not weaken the stricter unattended sync preflight.
Amend the import-selection design to point here for the updated descriptor.

## Security and verification

The existing workspace authorization gates remain unchanged. Option values may
be sensitive because providers define them; do not place them in diagnostics,
sync warnings, or logs. Public documentation must tell authors to review files
before sharing. Verify encode/decode and old-reader version rejection; option
comparison, candidate read failures and duplicate matches; real sync create,
update, preserve and recovery; browser bindings and both raw import paths; and
the existing sync status warning in an end-to-end flow.

## Related contracts

- [Requirements](../requirements/workflow-portable-profile-matching.md)
- [Workflow import selection design](workflow-import-profile-selection.md)
- [Profile disable behavior](../../agents/requirements/profile-disable.md)
