---
title: "Prompt Sync"
description: "Keep saved prompts as Markdown files in a GitHub or GitLab repository and sync them into Kandev."
---

# Prompt Sync

Prompt Sync makes one repository directory the source of truth for saved prompts. Each Markdown file becomes a saved prompt that workflow steps, chat, and agents can reference by `@name`. Changes go through pull-request review in the same repository as the workflows that use them, and every workspace sees the same text.

Saved prompts are shared across the whole installation, so there is exactly one Prompt Sync source per installation. It is configured on its own, separately from each workspace's [Workflow Sync](workflow-sync.md). One source means two repositories can never define the same prompt name.

## Quick path

1. Commit one `<name>.md` file per prompt to a directory such as `.kandev/prompts/` in a GitHub or GitLab repository. That directory can sit next to `.kandev/workflows/`.
2. Open **Settings → Prompts → Prompt Sync** and select **Set up**.
3. Choose the provider and the **credential workspace**. Prompt Sync reads the repository through that workspace's existing GitHub or GitLab connection.
4. Enter the repository, branch, and directory, then **Save**.
5. If the repository has prompts with the same names as prompts you already have, choose which version to use for each (see [Name conflicts](#name-conflicts)).

**Save** first reads the repository without changing anything. If it cannot be read, nothing is saved and the dialog shows the error. Otherwise Kandev saves the configuration and runs the first sync straight away. Synced prompts show a **Synced** badge.

## Prompt files

```text
.kandev/
├── workflows/
│   └── delivery.yml        # Workflow Sync reads this directory
└── prompts/
    ├── principles.md       # becomes @principles
    └── code-review-strict.md
```

- Only immediate files ending in `.md` (any letter case) are read. Subdirectories and other files are ignored.
- The prompt name is the file name without `.md`. The content is the file text with leading and trailing whitespace trimmed.
- A file is invalid when its name is empty, longer than 512 bytes, or starts or ends with whitespace; or when its content is not UTF-8, is empty, longer than 1 MiB, or too large for the provider to return. An invalid file produces a warning, and the prompt last synced under that name stays unchanged; other files still sync.
- Two files that resolve to the same name (for example `review.md` and `review.MD`) are both skipped with a warning, even if one of them is invalid.

## Reconciliation rules

Every sync compares the files with the prompts Prompt Sync owns:

- A new file creates a synced prompt. A changed file updates it. An unchanged file writes nothing.
- A removed file deletes its prompt only when nothing still references it: no workflow prompt or step prompt in any workspace, and no remaining saved prompt (including prompts kept by this rule). A prompt that is still referenced is kept, and the sync reports which prompt and how many workflows or prompts use it. Remove the references, then sync again.
- A file whose name matches a prompt created in Kandev, or a built-in prompt, is a [name conflict](#name-conflicts). Prompt Sync never skips it silently and never replaces your text without your choice.
- After applying changes, Prompt Sync warns about any `@name` inside a remaining synced prompt (including kept and unchanged ones) that does not resolve to a saved prompt.
- A successful listing with no prompt files is an empty source and removes unreferenced synced prompts.
- A listing or download failure fails the run before anything changes.

Renaming a file is a delete of the old name and a create of the new one.

## Read-only prompts

Synced prompts are owned by the repository:

- Settings shows a **Synced** badge with the source file. Instead of edit and delete, a **View** button (eye icon) opens the full content read-only.
- The prompt HTTP API rejects updates and deletes with `409`.
- Agent prompt-write tools reject changes, and `allow_agent_edits` reports `false`. See [Agents](#agents).
- Creating a prompt with a synced prompt's name fails as a duplicate.

To change a synced prompt, edit its file and sync again.

## Local overrides

A shared repository can ship a default prompt at a chosen point, for example a `@post-pr-review` step that runs an extra reviewer. Each installation can keep that default or replace it locally:

1. On the synced prompt in **Settings → Prompts**, turn on the **Local override** switch. The prompt keeps its name and content but becomes an editable, deletable local copy with an **Overridden** badge.
2. Edit it like any prompt. Syncs leave it alone and list it under **Overridden locally** on the status card instead of warning.
3. To go back to the shared default, turn the switch off and confirm **Use repository version**. Kandev returns the prompt to sync and runs a sync, so the repository text is used. Your local version is kept: turning the switch on again brings it back. Deleting the override instead drops your version, and the next sync recreates the synced prompt.

The switch is always the first control on the right of the row, so it stays in the same place whether the prompt is synced or overridden. Removing Prompt Sync turns overrides into normal prompts.

## Name conflicts

A name conflict is a repository file whose name matches a prompt you created in Kandev or a built-in prompt. Built-in prompts are treated the same way as your own.

- **When you save the configuration**, Kandev lists the conflicts before saving anything. For each prompt, choose **Keep local** (the default) or **Use repository version**; **Compare versions** shows both texts. **Back** returns to the fields; closing the dialog saves nothing. **Save and sync** applies your choices:
  - **Keep local** makes your prompt a local override of the file. Your text stays in use.
  - **Use repository version** makes it a synced prompt with the repository text. Your text is kept, and turning on **Local override** brings it back.
- **When a later sync finds a new conflict** (for example, someone adds a matching file), your prompt becomes an override marked **Conflict** and keeps your text. It stays flagged until you choose: the row shows a **Conflict** badge, **Settings → Prompts** shows a banner with **Review**, and the Prompts settings entry, the sidebar **Settings** button, and the phone menu's **Settings** row show an amber dot. **Review** opens the same choices; **Apply choices** runs a sync with them. Turning the row's **Local override** switch off also resolves the conflict in favour of the repository version.
- If the conflicting file is removed from the repository, the conflict is cleared and your override stays.
- If a synced prompt whose local version Kandev kept is removed from the repository, it becomes a normal prompt with your local text again. Sync never deletes a built-in prompt.

## Agents

- Agents cannot change synced prompts. The error tells the agent that a local change would have no effect, that you can turn on **Local override** for the prompt in **Settings → Prompts**, and that agents cannot change that setting, so the agent can pass the steps on to you.
- Agent prompt reads report each prompt's source (`manual`, `sync`, or `override`), whether it has an unresolved conflict, and editing guidance for synced and overridden prompts.
- Agents can edit overrides; the edit changes your local version, which is in use. It does not resolve a conflict.
- No agent tool can turn **Local override** on or off or resolve a conflict.

## Workflow Sync reference warnings

[Workflow Sync](workflow-sync.md) warns when a synced workflow prompt or step prompt references an `@name` that is not a saved prompt. The warning names the file, the workflow, the steps, and the missing name. It does not block the sync. Add the prompt (for example through Prompt Sync) and sync again.

## Automatic sync and failures

With **Auto-sync** on, Kandev checks the configuration every 60 seconds (first check one minute after startup) and syncs once the interval has passed. The interval is 60 seconds to 30 days; the default is 300 seconds.

Failures use the same recovery policy as Workflow Sync:

- Temporary failures back off exponentially, up to one hour, and never retry earlier than a provider's retry time.
- A GitHub credential or access failure, or a missing repository, branch, or directory, suspends auto-sync. GitLab failures follow bounded circuit retries.
- A deleted credential workspace suspends auto-sync with an explicit reason.
- Saving the configuration, **Sync now**, or a credential change resumes polling.

Error messages never include tokens or provider response bodies.

## Stop syncing

Select **Configure → Remove sync**. Kandev first turns every synced prompt and override into a normal, editable prompt, keeping its name and current content, and then removes the configuration. Kept local versions and conflict flags are dropped. If that fails, the configuration stays so you can retry.

Changing the provider, credential workspace, repository, branch, or directory keeps existing synced prompts. The next sync reconciles them against the new source.

## Permissions

Reading the configuration and status follows saved-prompt read access. Saving, removing, and **Sync now** require configuration-management permission and access to the credential workspace when authentication is enabled.

<details>
<summary>HTTP API</summary>

| Method | Route | Behavior |
|--------|-------|----------|
| `GET` | `/api/v1/prompt-sync/config` | `200` with the configuration and status, or `204` when not configured. |
| `POST` | `/api/v1/prompt-sync/check` | List the name conflicts a source would cause, without saving anything. Send the configuration as the body, or no body to check the saved one. `422` with `error` if the repository cannot be read. |
| `POST` | `/api/v1/prompt-sync/config` | Validate, save, and run one sync. Optional `resolutions` maps prompt names to `local` or `repository`. Returns `config` plus `result` or `error`. |
| `DELETE` | `/api/v1/prompt-sync/config` | Release synced prompts, then delete. Returns `{"deleted":true}`. |
| `POST` | `/api/v1/prompt-sync/sync` | Run now, with optional `{"resolutions": {...}}`. Returns `config` plus `result` or `error`; `404` when not configured. |
| `POST` | `/api/v1/prompt-sync/prompts/{id}/override` | Turn a synced prompt into a local override. `409` if the prompt is not synced. |
| `POST` | `/api/v1/prompt-sync/prompts/{id}/revert` | Return an override to sync, keeping its text as the local version, and run a sync. `409` if the prompt is not an override. |

```bash
curl -fsS -X POST -H 'Content-Type: application/json' \
  -d '{"workspace_id":"WORKSPACE_ID","provider":"gitlab","project_path":"acme/engineering",
       "branch":"main","path":".kandev/prompts","interval_seconds":300,"poll_enabled":true}' \
  'http://localhost:38429/api/v1/prompt-sync/config'
```

A completed sync returns HTTP `200` even when the response contains `error`; inspect the JSON and `config.last_ok`.

</details>

## Troubleshooting

- **A prompt shows a Conflict badge:** a repository file has the same name. Select **Review** on the banner and choose a version.
- **A removed prompt is still listed:** a workflow or another saved prompt still references it. The warning gives the number of referrers; remove the references and sync again.
- **Suspended after deleting a workspace:** choose another credential workspace and save.
- **Edits revert:** synced prompts are repository-owned. Change the file instead.

Related guides: [Workflow Sync](workflow-sync.md), [Workflow Tips](workflow-tips.md), and [Integrations](integrations.md).
