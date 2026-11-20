import type { CustomPrompt } from "@/lib/types/http";
import type { PromptConflictChoice, PromptVersionsSave } from "@/lib/types/prompt-sync";

/**
 * The Prompts page editor draft for the prompt open for editing. For a synced
 * prompt or an override, `content` is its local version.
 */
export type PromptFormState = { name: string; content: string };

/** Pending version choices by prompt ID, only where they differ from the saved one. */
export type PendingVersions = Record<string, PromptConflictChoice>;

/** A synced prompt or an override: one with a repository version. */
export function isLinkedPrompt(prompt: CustomPrompt): boolean {
  return prompt.source === "sync" || prompt.source === "override";
}

/** The local version: the override's content, or a synced prompt's kept copy. */
export function localVersion(prompt: CustomPrompt): string {
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
    content: isLinkedPrompt(prompt) ? localVersion(prompt) : prompt.content,
  };
}

/** The version a linked prompt uses: pending when switched, otherwise saved. */
export function choiceFor(prompt: CustomPrompt, pending: PendingVersions): PromptConflictChoice {
  return pending[prompt.id] ?? (prompt.source === "override" ? "local" : "repository");
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

/** The prompt open for editing, its draft, and the revision the draft started from. */
export type EditingDraft = { prompt: CustomPrompt; form: PromptFormState; updatedAt: string };

export type PromptSave =
  | { kind: "versions"; prompt: CustomPrompt; payload: PromptVersionsSave }
  | { kind: "update"; prompt: CustomPrompt; form: PromptFormState; expectedUpdatedAt: string };

function editedLocal(prompt: CustomPrompt, editing: EditingDraft | undefined): string | undefined {
  if (!editing || editing.prompt.id !== prompt.id) return undefined;
  const content = editing.form.content.trim();
  return content === localVersion(prompt) ? undefined : content;
}

/**
 * The writes one Save makes: a versions save for every switched linked
 * prompt and for an edited linked prompt (carrying the edit and the draft's
 * starting revision), and a plain update for an edited manual prompt.
 */
export function planPromptSaves({
  prompts,
  pending,
  editing,
}: {
  prompts: CustomPrompt[];
  pending: PendingVersions;
  editing?: EditingDraft;
}): PromptSave[] {
  const plan: PromptSave[] = [];
  for (const prompt of prompts) {
    if (isLinkedPrompt(prompt)) {
      const local = editedLocal(prompt, editing);
      if (pending[prompt.id] === undefined && local === undefined) continue;
      plan.push({
        kind: "versions",
        prompt,
        payload: {
          use_local: choiceFor(prompt, pending) === "local",
          ...(local === undefined ? {} : { local_content: local }),
          ...(local === undefined || !editing ? {} : { expected_updated_at: editing.updatedAt }),
        },
      });
    } else if (editing?.prompt.id === prompt.id) {
      const saved = formFromPrompt(prompt);
      if (editing.form.name === saved.name && editing.form.content === saved.content) continue;
      plan.push({
        kind: "update",
        prompt,
        form: editing.form,
        expectedUpdatedAt: editing.updatedAt,
      });
    }
  }
  return plan;
}
