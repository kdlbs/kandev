import type { CustomPrompt } from "@/lib/types/http";
import type { PromptVersionsSave } from "@/lib/types/prompt-sync";

/**
 * The Prompts page editor draft. For a synced prompt or an override,
 * `content` is the local version and `useLocal` is the Local override switch;
 * `useLocal` is absent for manual prompts.
 */
export type PromptFormState = { name: string; content: string; useLocal?: boolean };

/** A synced prompt or an override: one with a repository version. */
export function isLinkedPrompt(prompt: CustomPrompt): boolean {
  return prompt.source === "sync" || prompt.source === "override";
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
  if (!isLinkedPrompt(prompt)) return { name: prompt.name, content: prompt.content };
  return {
    name: prompt.name,
    content: localVersion(prompt),
    useLocal: prompt.source === "override",
  };
}

/**
 * The versions save for a linked prompt's draft, based on the revision the
 * draft started from; the local version is sent only when edited.
 */
export function versionsSavePayload(
  prompt: CustomPrompt,
  form: PromptFormState,
  expectedUpdatedAt: string,
): PromptVersionsSave {
  const content = form.content.trim();
  return {
    use_local: form.useLocal === true,
    ...(content !== localVersion(prompt) ? { local_content: content } : {}),
    expected_updated_at: expectedUpdatedAt,
  };
}
