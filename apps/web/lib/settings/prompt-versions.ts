import type { CustomPrompt } from "@/lib/types/http";
import type { PromptConflictChoice, PromptVersionsSave } from "@/lib/types/prompt-sync";

/**
 * A prompt's editable fields. For a synced prompt or an override, `content`
 * is its local version.
 */
export type PromptFormState = { name: string; content: string };

/** Pending version choices by prompt ID, only where they differ from the saved one. */
export type PendingVersions = Record<string, PromptConflictChoice>;

/** A synced prompt or an override: one with a repository version. */
export function isLinkedPrompt(prompt: CustomPrompt): boolean {
  return prompt.source === "sync" || prompt.source === "override";
}

/**
 * Whether a linked prompt has a repository version to switch to. An override
 * whose file left the sync source keeps only its local version.
 */
export function hasRepositoryVersion(prompt: CustomPrompt): boolean {
  if (prompt.source === "override") return Boolean(prompt.alternate_content);
  return prompt.source === "sync";
}

/** The local version: the override's content, or a synced prompt's kept copy. */
function localVersion(prompt: CustomPrompt): string {
  if (prompt.source === "sync") return prompt.alternate_content || prompt.content;
  return prompt.content;
}

/** The repository version: a synced prompt's content, or an override's kept copy. */
export function repositoryVersion(prompt: CustomPrompt): string {
  if (prompt.source === "override") return prompt.alternate_content || prompt.content;
  return prompt.content;
}

/** The draft a saved prompt starts from. */
export function formFromPrompt(prompt: CustomPrompt): PromptFormState {
  return {
    name: prompt.name,
    content: localVersion(prompt),
  };
}

/**
 * The version a linked prompt uses: pending when switched, otherwise saved.
 * An override whose file left the repository has only its local version, so
 * a pending Repository choice no longer applies.
 */
export function choiceFor(prompt: CustomPrompt, pending: PendingVersions): PromptConflictChoice {
  if (prompt.source === "override" && !hasRepositoryVersion(prompt)) return "local";
  return pending[prompt.id] ?? (prompt.source === "override" ? "local" : "repository");
}

/** Whether a linked prompt's pending choice differs from the saved one. */
export function isSwitched(prompt: CustomPrompt, pending: PendingVersions): boolean {
  return choiceFor(prompt, pending) !== choiceFor(prompt, {});
}

/** Records a switch; a choice equal to the saved one is no longer pending. */
export function withChoice(
  pending: PendingVersions,
  prompt: CustomPrompt,
  choice: PromptConflictChoice,
): PendingVersions {
  const { [prompt.id]: _previous, ...rest } = pending;
  if (choice === choiceFor(prompt, {})) return rest;
  return { ...rest, [prompt.id]: choice };
}

/** An unsaved edit to a saved prompt and the revision it started from. */
type PromptDraft = { form: PromptFormState; updatedAt: string };

/** Unsaved edits by prompt ID, only where they differ from the saved prompt. */
export type PromptDrafts = Record<string, PromptDraft>;

function sameForm(a: PromptFormState, b: PromptFormState): boolean {
  return a.name === b.name && a.content === b.content;
}

/** What a prompt's editor shows: its unsaved edit, otherwise the saved prompt. */
export function draftForm(prompt: CustomPrompt, drafts: PromptDrafts): PromptFormState {
  return drafts[prompt.id]?.form ?? formFromPrompt(prompt);
}

/**
 * Applies an edit. A draft keeps the revision of its first edit, so a save
 * made against a newer revision is rejected instead of overwriting it; an
 * edit back to the saved prompt is no longer a draft.
 */
export function withDraftEdit(
  drafts: PromptDrafts,
  prompt: CustomPrompt,
  patch: Partial<PromptFormState>,
): PromptDrafts {
  const { [prompt.id]: previous, ...rest } = drafts;
  const form = { ...(previous?.form ?? formFromPrompt(prompt)), ...patch };
  if (sameForm(form, formFromPrompt(prompt))) return rest;
  return { ...rest, [prompt.id]: { form, updatedAt: previous?.updatedAt ?? prompt.updated_at } };
}

/** Whether an open row edits text: a manual prompt, or a linked prompt on Local. */
export function showsEditor(prompt: CustomPrompt, pending: PendingVersions): boolean {
  return !isLinkedPrompt(prompt) || choiceFor(prompt, pending) === "local";
}

/** A manual prompt needs a name; every prompt needs content. */
export function isDraftValid(prompt: CustomPrompt, form: PromptFormState): boolean {
  if (!form.content.trim()) return false;
  return isLinkedPrompt(prompt) || Boolean(form.name.trim());
}

export type PromptSave =
  | { kind: "versions"; prompt: CustomPrompt; payload: PromptVersionsSave }
  | { kind: "update"; prompt: CustomPrompt; form: PromptFormState; expectedUpdatedAt: string }
  | { kind: "recreate"; id: string; form: PromptFormState };

/**
 * Drafts whose prompt left the list while being edited (for example, a sync
 * removed its file). Their text is kept so Save can recreate the prompt.
 */
export function orphanedDrafts(
  prompts: CustomPrompt[],
  drafts: PromptDrafts,
): { id: string; form: PromptFormState }[] {
  const listed = new Set(prompts.map((prompt) => prompt.id));
  return Object.entries(drafts)
    .filter(([id]) => !listed.has(id))
    .map(([id, draft]) => ({ id, form: draft.form }));
}

function editedLocal(prompt: CustomPrompt, draft: PromptDraft | undefined): string | undefined {
  if (!draft) return undefined;
  const content = draft.form.content.trim();
  return content === localVersion(prompt) ? undefined : content;
}

function linkedSave(
  prompt: CustomPrompt,
  pending: PendingVersions,
  draft: PromptDraft | undefined,
): PromptSave | undefined {
  const local = editedLocal(prompt, draft);
  if (!isSwitched(prompt, pending) && local === undefined) return undefined;
  return {
    kind: "versions",
    prompt,
    payload: {
      use_local: choiceFor(prompt, pending) === "local",
      ...(local === undefined || !draft
        ? {}
        : { local_content: local, expected_updated_at: draft.updatedAt }),
    },
  };
}

/**
 * The writes one Save makes, in list order: a versions save for every
 * switched or edited linked prompt (carrying the edit and its starting
 * revision), a plain update for every edited manual prompt, then a new
 * prompt for every draft whose prompt was removed while edited.
 */
export function planPromptSaves({
  prompts,
  pending,
  drafts,
}: {
  prompts: CustomPrompt[];
  pending: PendingVersions;
  drafts: PromptDrafts;
}): PromptSave[] {
  const plan: PromptSave[] = [];
  for (const prompt of prompts) {
    const draft = drafts[prompt.id];
    if (isLinkedPrompt(prompt)) {
      const save = linkedSave(prompt, pending, draft);
      if (save) plan.push(save);
    } else if (draft && !sameForm(draft.form, formFromPrompt(prompt))) {
      plan.push({ kind: "update", prompt, form: draft.form, expectedUpdatedAt: draft.updatedAt });
    }
  }
  for (const orphan of orphanedDrafts(prompts, drafts)) plan.push({ kind: "recreate", ...orphan });
  return plan;
}
