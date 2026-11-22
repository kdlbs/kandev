---
status: active
system: integrations
created: 2026-11-19
owners:
  - kandev
---

# Prompt Sync Requirements

## Overview

Workflow step prompts reference saved prompts by `@name`, but saved prompts
live only in the database. A repository can define a workflow but not the text
its agents act on. A shared rule cannot be reviewed in the same pull request
as the workflow that uses it, and a new installation that syncs the workflows
gets `@name` references that resolve to nothing or to an old local copy
(GitHub issue #5391).

Prompt Sync makes one repository directory the source of truth for a set of
saved prompts. Saved prompts are install-wide, so Prompt Sync has exactly one
install-wide source. It is configured separately from per-workspace
[Workflow Sync](../../../public/workflow-sync.md), which stays unchanged. With
a single source per install, two sync sources can never define the same prompt
name.

The integration system owns this contract because Prompt Sync is a
provider-backed repository reconciliation. It reads through an existing
GitHub or GitLab workspace connection and stores no credential. The prompt
service still owns saved-prompt storage, validation, and `@name` expansion.
The MCP write rules in
[Shared prompt writes over MCP](shared-prompt-writes.md) still apply, with the
added rule that synced prompts are read-only.

## Terminology

- **Prompt source:** The install-wide repository location Prompt Sync reads
  from: provider, repository, branch, and directory.
- **Credential workspace:** The workspace whose existing GitHub or GitLab
  connection Prompt Sync uses to read the prompt source.
- **Prompt file:** An immediate file in the prompt source directory whose
  extension is `.md`, compared case-insensitively.
- **Synced prompt:** A saved prompt created by Prompt Sync and still owned by
  it.
- **Manual prompt:** Any saved prompt that is neither synced nor an override,
  including built-in prompts.
- **Override:** A local, editable prompt that an operator created from a
  synced prompt to replace the repository version on this installation.
- **Prompt reference:** An `@name` in prompt text, matched with the same rules
  as saved-prompt expansion.

## Requirements

### REQ-INTEGRATIONS-PROMPT-SYNC-001: Install-wide prompt source

**Intent:** An operator points the installation at one repository directory of
prompt files, without new credentials and without touching Workflow Sync.

**User story:** As a maintainer, I want to configure where shared prompts come
from once for the installation, so that every workspace uses the same reviewed
text.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-001.1:** Settings > Prompts shall offer one
  install-wide Prompt Sync configuration with these fields: provider (GitHub
  or GitLab), credential workspace, repository, branch, directory, auto-sync,
  and interval. Defaults are branch `main`, directory `.kandev/prompts`,
  auto-sync on, and interval 300 seconds. The interval range is 60 to 2592000
  seconds.
- **AC-INTEGRATIONS-PROMPT-SYNC-001.2:** Repository reads shall use only the
  credential workspace's existing GitHub or GitLab connection. Prompt Sync
  shall store no credential. A GitHub configuration shall keep its repository
  host, which must match that workspace's connection host. Repository, branch,
  and directory validation shall match Workflow Sync.
- **AC-INTEGRATIONS-PROMPT-SYNC-001.3:** Saving the configuration shall first
  check the source under AC-INTEGRATIONS-PROMPT-SYNC-008.1, then store it and
  run one sync immediately, applying the conflict choices made in the dialog.
  **Sync now** shall run one sync immediately and return its result.
- **AC-INTEGRATIONS-PROMPT-SYNC-001.4:** When authentication is enforced,
  saving, removing, and **Sync now** shall require configuration-management
  authority plus access to the credential workspace. Anyone who can read saved
  prompts shall be able to read the configuration and its status. When
  authentication is disabled, behavior shall match other settings.
- **AC-INTEGRATIONS-PROMPT-SYNC-001.5:** Configuring, running, or removing
  Prompt Sync shall not change Workflow Sync configuration, scheduling, or
  results. The only Workflow Sync change in this package is
  REQ-INTEGRATIONS-PROMPT-SYNC-006.
- **AC-INTEGRATIONS-PROMPT-SYNC-001.6:** When GitHub quota blocks a manual
  sync, that sync shall return a retryable error before changing prompts.
  Pending saves and removal shall be able to proceed while quota is
  unavailable. Settings on desktop and phone shall identify the quota
  block and tell the operator to retry when quota is available.

### REQ-INTEGRATIONS-PROMPT-SYNC-002: Prompt files

**Intent:** One Markdown file per prompt, so a review covers exactly the text
that agents receive.

**User story:** As a maintainer, I want each shared prompt in its own Markdown
file, so that prompt changes are easy to review in a pull request.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-002.1:** A sync shall read only the prompt
  files that are immediate children of the configured directory, in sorted
  path order. It shall ignore subdirectories and files with other extensions.
- **AC-INTEGRATIONS-PROMPT-SYNC-002.2:** A prompt's name shall be its file
  name without the `.md` extension. Its content shall be the file text with
  surrounding whitespace trimmed, which is the normalization saved prompts
  already use.
- **AC-INTEGRATIONS-PROMPT-SYNC-002.3:** A prompt file is invalid when it
  meets any of these conditions:
  - its name is empty or longer than the saved-prompt name limit;
  - its content is not valid UTF-8 or contains a NUL character;
  - its content is empty after trimming;
  - its content is longer than the saved-prompt content limit;
  - the provider refuses to return it because of its size.

  An invalid file shall produce a warning. A synced prompt with that name
  shall stay unchanged for that run.
- **AC-INTEGRATIONS-PROMPT-SYNC-002.4:** When two prompt files resolve to the
  same name (for example `review.md` and `review.MD`), the sync shall report a
  warning, apply neither file, and keep any existing synced prompt with that
  name unchanged, even when one of the files is invalid.
- **AC-INTEGRATIONS-PROMPT-SYNC-002.5:** A directory listing or file download
  failure shall fail the run before any saved prompt changes. A file refused
  only for its size is not a download failure; it follows
  AC-INTEGRATIONS-PROMPT-SYNC-002.3. A listing that may be truncated (a
  GitHub directory with 1,000 or more entries, the most its contents API
  returns) shall fail the run the same way, so no file missing from it is
  treated as removed.

### REQ-INTEGRATIONS-PROMPT-SYNC-003: Reconciliation

**Intent:** Saved prompts follow the repository without breaking workflows
that still depend on them, and without taking over prompts a person wrote.

**User story:** As a maintainer, I want merged prompt changes to reach the
installation automatically, so that agents act on the reviewed text.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-003.1:** A sync shall create a synced prompt
  for each valid file whose name has no saved prompt. It shall update a synced
  prompt whose content differs from its file. A synced prompt whose content
  already matches shall not be written, and no change event shall be
  published for it.
- **AC-INTEGRATIONS-PROMPT-SYNC-003.2:** When a synced prompt's file is no
  longer in the source, the sync shall delete it only when nothing still
  references it. A reference is a workflow prompt or workflow step prompt in
  any workspace, or a surviving saved prompt; a prompt kept by this rule
  survives too, so its own references are followed. Surviving prompts are
  read after the run's other writes, so a reference added by a file created
  or updated in the same run counts. A kept prompt shall be reported with a
  warning that names it and the number of referrers.
- **AC-INTEGRATIONS-PROMPT-SYNC-003.3:** When a prompt file's name matches an
  existing manual or built-in prompt, that is a conflict, handled under
  REQ-INTEGRATIONS-PROMPT-SYNC-008. The sync shall never skip it silently or
  overwrite the existing content without the user's choice. A prompt renamed
  or edited after the sync read it shall not be adopted by that run. A file
  whose name matches an override follows AC-INTEGRATIONS-PROMPT-SYNC-007.3
  instead.
- **AC-INTEGRATIONS-PROMPT-SYNC-003.4:** A successful listing with no prompt
  files shall be treated as an empty source. It shall remove unreferenced
  synced prompts under REQ-INTEGRATIONS-PROMPT-SYNC-003.2.
- **AC-INTEGRATIONS-PROMPT-SYNC-003.5:** After applying changes, the sync
  shall report a warning for each prompt reference that does not resolve to a
  saved prompt in the content of every prompt still owned by sync, including
  frozen prompts and prompts kept because a workflow references them.
- **AC-INTEGRATIONS-PROMPT-SYNC-003.6:** Prompt changes made by a sync shall
  show up in an open Settings > Prompts page without a reload, including
  changes saved before a later step of the same run failed. Later `@name`
  expansion shall use them. Unsaved editor drafts shall be preserved.
- **AC-INTEGRATIONS-PROMPT-SYNC-003.7:** A sync result shall report created,
  updated, and deleted prompt names, warnings, and whether nothing changed.
  The stored status shall record the last sync time, whether it succeeded, the
  last error, and the last warnings.

### REQ-INTEGRATIONS-PROMPT-SYNC-004: Ownership and read-only prompts

**Intent:** The repository is the only place a synced prompt changes, so a
sync never overwrites someone's edit.

**User story:** As a maintainer, I want synced prompts locked in Kandev, so
that every change goes through review.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-004.1:** Settings, HTTP updates, HTTP deletes,
  and agent MCP writes shall reject changes to a synced prompt. The rejection
  shall tell the user to edit the prompt in the synced repository; the agent
  rejection follows AC-INTEGRATIONS-PROMPT-SYNC-010.1. The
  prompt's content shall stay unchanged. The prompt's `allow_agent_edits`
  value shall be false.
- **AC-INTEGRATIONS-PROMPT-SYNC-004.2:** Prompt reads (HTTP, boot payload, and
  MCP reads) shall report whether a prompt is synced and its repository file
  path.
- **AC-INTEGRATIONS-PROMPT-SYNC-004.3:** On desktop and phone, Settings >
  Prompts shall show a synced badge on each synced prompt, with a tooltip
  naming its source file. Edit and delete controls shall not be offered for
  synced prompts. Instead, a View control shall open the prompt's full content
  read-only, with a note naming the source file where it is edited; on phones
  and coarse pointers the control shall be touch-sized. Every read-only
  prompt editor shall look read-only (a dashed frame, no cursor, and a
  surface distinct from an editable one) while keeping its text at a WCAG AA
  contrast of at least 4.5:1. The Prompt Sync section shall show no empty
  content area while Prompt Sync is not set up, and a single outline around
  its status once configured; the custom prompt list likewise draws no group
  outline around its search bar and rows. Manual and built-in prompts shall keep their
  controls, with **Edit** showing and hiding the editor.
- **AC-INTEGRATIONS-PROMPT-SYNC-004.4:** Creating a prompt manually (through
  Settings, HTTP, or MCP) with a name that a synced prompt already uses shall
  fail with the existing duplicate-name error.
- **AC-INTEGRATIONS-PROMPT-SYNC-004.5:** Removing the configuration shall
  first release every synced prompt and every override to a manual, editable
  prompt, keeping its identity and current content (or its kept local
  content where the user chose it under AC-INTEGRATIONS-PROMPT-SYNC-009.5)
  and clearing any conflict and kept local content, except prompts the user
  chose to delete under AC-INTEGRATIONS-PROMPT-SYNC-009.6, and then delete
  the configuration. If any release
  fails, the configuration shall be kept so the user can retry.
- **AC-INTEGRATIONS-PROMPT-SYNC-004.6:** Changing the provider, credential
  workspace, repository, branch, or directory shall keep existing synced
  prompts. The next sync shall reconcile them against the new source under
  REQ-INTEGRATIONS-PROMPT-SYNC-003.

### REQ-INTEGRATIONS-PROMPT-SYNC-005: Automatic sync and failures

**Intent:** Prompt Sync keeps running unattended and backs off from a broken
source instead of retrying it constantly.

**User story:** As an operator, I want automatic prompt sync to recover from
temporary failures and to stop repeating permanent ones.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-005.1:** With auto-sync enabled, the backend
  shall check the configuration on a 60-second outer tick, waiting one full
  tick after startup. It shall run a sync once the interval has passed since
  the last sync.
- **AC-INTEGRATIONS-PROMPT-SYNC-005.2:** Transient failures shall back off
  with the same policy as Workflow Sync: equal-jitter exponential backoff from
  the larger of the interval and one minute, with growth capped at one hour.
  A provider retry time may push the next attempt later, never earlier.
- **AC-INTEGRATIONS-PROMPT-SYNC-005.3:** These failures shall suspend
  automatic polling with an actionable reason:
  - a definite GitHub credential or access failure;
  - a missing GitHub repository, branch, directory, or ref;
  - a missing credential workspace, for either provider.

  GitLab credential and configuration failures shall follow the same bounded
  circuit retries as Workflow Sync. Saving the configuration, **Sync now**, or
  a credential change shall resume polling.
- **AC-INTEGRATIONS-PROMPT-SYNC-005.4:** Stored and returned error messages
  shall be sanitized. They shall contain no token or credential material.

### REQ-INTEGRATIONS-PROMPT-SYNC-006: Unresolved references in Workflow Sync

**Intent:** A synced workflow that names a prompt the installation does not
have is caught at sync time, not when an agent runs it.

**User story:** As a maintainer, I want Workflow Sync to tell me when a step
references a prompt that does not exist, so that I can add it before tasks
run.

#### Acceptance criteria

- **AC-INTEGRATIONS-PROMPT-SYNC-006.1:** After a Workflow Sync run applies its
  files, it shall report one warning for each distinct combination of source
  file, workflow, and prompt name where a workflow prompt or step prompt in a
  parsed definition contains a reference that does not resolve to a saved
  prompt. The warning shall name the file, the workflow, the referencing
  steps, and the missing name. These warnings shall not block or change the
  apply.
- **AC-INTEGRATIONS-PROMPT-SYNC-006.2:** A Workflow Sync run whose references
  all resolve, or that has none, shall return the same result and status as
  before this change.

### Local versions, conflicts, and agent access

REQ-INTEGRATIONS-PROMPT-SYNC-007 through REQ-INTEGRATIONS-PROMPT-SYNC-010
(local overrides, name conflicts, lossless switching, and agent access) are
defined in [Prompt Sync local versions](prompt-sync-local-versions.md).

## Out of scope

- Prompts scoped to a workspace, or more than one prompt source per
  installation.
- Running prompt sync as part of a Workflow Sync run, or reading prompts from
  the Workflow Sync directory.
- Recursive directories, YAML or JSON prompt envelopes, and front-matter
  metadata in prompt files.
- Installation-level GitHub or GitLab credentials.
- Renaming detection: a renamed file is a delete of the old name and a create
  of the new one.
- Blocking deletion on references from Office skills or chat history. Only
  workflow prompts, workflow step prompts, and surviving saved prompts block
  deletion.
