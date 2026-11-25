---
status: current
system: integrations
requirements:
  - REQ-INTEGRATIONS-SHARED-PROMPT-WRITES-001
---

# Shared prompt MCP writes

## Boundaries and mapping

This design replaces the per-prompt permission design.
It extends [saved prompt reads](external-mcp-shared-prompts.md).
The integration system owns tool discovery, authorization, instructions, and results.
`internal/prompts` owns persistence, validation, and reference expansion.

The policy decision is
[Agent editing for every custom prompt](../../../decisions/2026-11-11-custom-prompt-agent-editing.md).
The [implementation package](../../../plans/custom-prompt-agent-editing/plan.md)
tracks delivery. The earlier package remains a historical delivery record.

| Acceptance criteria | Design section |
| --- | --- |
| .1, .2, .3, .11, .14 | Tools and authority |
| .9, .10, .13, .16 | Persistence and concurrent writes |
| .3, .15 | Compatibility projections |
| .7, .8, .12 | Live delivery and Settings |

All suffixes refer to `AC-INTEGRATIONS-SHARED-PROMPT-WRITES-001`.
Retired criteria .4, .5, and .6 describe the previous policy only.

## Tools and authority

The configuration and external catalogs retain `create_shared_prompt_kandev`
and `update_shared_prompt_kandev`. Their required inputs remain `name` and `content`.
Other catalogs gain no dedicated prompt-write tools.
The backend continues to require `org.config.manage` from trusted authentication context.
Existing transport authentication and automation dispatch restrictions remain.

Creation rejects an existing exact name. Dedicated updates replace content by
an exact, case-sensitive name. Generic settings retains its existing rename
support for custom prompts through `settingsOperations.updatePrompt`.
Both paths call the guarded prompt service and reject built-ins.
Name and content validation retain their 512-byte and 1 MiB limits.

Configuration context and tool descriptions require explicitly requested or
agreed prompt changes, followed by readback. They describe effects on future references.
Request intent remains an agent instruction. There is no additional backend
approval token, prompt attestation, or automatic rewrite process.

## Persistence and concurrent writes

Remove `models.Prompt.AllowAgentEdits` and the service permission branch.
`CreatePromptForAgent` can delegate to ordinary creation without a separate default.
Operator updates use `UpdatePrompt` without a permission argument.
`UpdatePromptForAgent` retains its built-in check and existing field validation.

The repository's conditional agent update matches the observed ID, name,
`builtin = 0`, and `updated_at`. It no longer tests a permission value.
Renames, content edits, deletion, or built-in reclassification reject stale writes.
Prompt Settings captures the prompt's `updated_at` when editing begins and sends
it as `expected_updated_at` with every save. The service rejects a mismatched
revision with the existing write-conflict error before applying fields. The
repository's conditional update still catches a concurrent change after that
comparison. HTTP returns 409 and preserves the newer prompt. The request field
remains optional for older clients. The response serializes `updated_at` with
RFC3339Nano precision so distinct revisions remain distinct.

Prompt SQL no longer selects, scans, inserts, or updates `allow_agent_edits`.
Fresh schema initialization omits that column and removes `migrateAgentPermission`.
Existing databases can retain an unused column. Its value never affects reads or writes.
This is the final compatibility policy, without a later cleanup dependency.
No startup backfill or table reconstruction changes prompt content or timestamps.

Upgrade tests cover databases before and after introduction of the old column.
Previously stored zero and one values produce the same custom-prompt eligibility.
The shared dialect-aware repository must support SQLite and PostgreSQL.
Conformance assertions must stop requiring a retired permission value in the target schema.
Original upgrade fixtures remain historical inputs.

## Compatibility projections

HTTP `PromptDTO`, MCP summaries and detail results, and generic settings reads
retain `allow_agent_edits`. Each projection derives it from `!prompt.Builtin`.
This field describes effective eligibility, without storing a configurable permission.

HTTP `UpdatePromptRequest` can decode its deprecated permission field for old clients.
The controller ignores that input and passes name, content, and the optional
expected revision to the service. A permission-only HTTP patch returns the current
representation without a mutation. Neither true nor false input changes eligibility.
Generic settings continues to reject undeclared permission changes.
No frontend write sends the deprecated field.

## Live delivery and Settings

Successful mutations retain content-free `prompts.changed` invalidation and the
existing WebSocket broadcaster. Failed writes publish no successful change.
Cache generations, reconnection invalidation, and bounded retry keep their
current behavior. When a remote change arrives, a pristine open editor adopts
the saved name, content, and revision. A dirty editor keeps its local draft and
original revision; saving it returns the existing 409 conflict rather than
replacing a newer remote change. Cancel and reopen adopts the latest revision.
Later reference expansions see saved changes. Running turns retain captured instructions.

Remove `PromptAgentPermission` and its permission-only form, dirty-state,
save-payload, and draft-revision fields from `prompts-settings.tsx`.
The editor retains name, content, Cancel, and the floating Discard/Save changes action.
Remove the two unused permission strings from every locale, including pseudo.
There is no new user-facing copy or replacement control.

Desktop and phone use the existing inline editor at `/settings/prompts`.
The nearest shipped mobile exemplar is the prompt editor and floating save bar
covered by `mobile-prompts-settings.spec.ts`.
The mobile composition retains one page scroll owner and the existing action order.
Inline editing suits this persistent settings form and requires no new overlay.
Existing safe-area behavior, touch targets, keyboard access, and error states remain required.
The shared form and save logic stay common to both viewports.

The plan's UI-01 preview defines the affected region for both viewport sizes.
Desktop and mobile E2E checks cover content save, absent permission controls,
and a subsequent agent edit without an opt-in action.

## Failures and observability

Invalid, duplicate, missing, protected, unauthorized, and conflicting writes
retain explicit errors. Unexpected storage errors retain structured logging without content.
Remove the permission-disabled service error and its MCP mapping.
Existing events and logs provide the required evidence. No new metric is necessary.

## Verification

Repository tests cover both historical permission values, fresh schema,
upgrade replay, preserved timestamps, built-in rejection, and concurrent changes.
Service and handler tests cover Settings-created and MCP-created prompts, all
write adapters, configuration authority, compatibility fields, and legacy input.
Catalog and embedded-context tests cover explicit-request instructions and unchanged tool availability.

Component tests cover name/content save payloads, edit-start revision tracking,
pristine refresh, and draft preservation during remote refresh. Handler tests
prove stale revisions return 409 without replacing content. Existing loader and
WebSocket tests cover invalidation. Chromium and mobile-Chrome E2E prove the
simplified editor and stale-save conflict after a live agent edit.
The work order owns exact commands and public-documentation updates.
