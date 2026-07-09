import { formatPromptReferenceExpansions } from "@/lib/prompts/expand-prompt-references";

export type WalkthroughPromptFile = {
  path: string;
  repository_name?: string;
  repositoryName?: string;
  source?: "uncommitted" | "committed" | "pr" | string;
};

const MAX_PROMPT_FILES = 80;
export const CHANGES_WALKTHROUGH_PROMPT_NAME = "changes-walkthrough";
const CHANGED_FILES_PLACEHOLDER = "{{changed_files}}";
const CHANGED_FILES_REFERENCE = "The changed files are listed in the user-visible message above.";

function fileKey(file: WalkthroughPromptFile): string {
  return `${file.repository_name ?? file.repositoryName ?? ""}\0${file.path}\0${file.source ?? ""}`;
}

function formatPromptFile(file: WalkthroughPromptFile): string {
  const repo = file.repository_name ?? file.repositoryName ?? "";
  const source = file.source ? ` [${file.source}]` : "";
  return repo ? `${repo}:${file.path}${source}` : `${file.path}${source}`;
}

export function formatChangedFilesForWalkthroughPrompt(files: WalkthroughPromptFile[]): string {
  const uniqueFiles: WalkthroughPromptFile[] = [];
  const seen = new Set<string>();
  for (const file of files) {
    if (!file.path) continue;
    const key = fileKey(file);
    if (seen.has(key)) continue;
    seen.add(key);
    uniqueFiles.push(file);
  }

  const shown = uniqueFiles.slice(0, MAX_PROMPT_FILES);
  const omitted = uniqueFiles.length - shown.length;
  return shown.length > 0
    ? shown.map((file) => `- ${formatPromptFile(file)}`).join("\n") +
        (omitted > 0 ? `\n- ... ${omitted} more file(s) omitted from this prompt` : "")
    : "- No changed files were listed by the UI; inspect the local task state before anchoring.";
}

export function buildChangesWalkthroughPrompt(
  template: string,
  files: WalkthroughPromptFile[],
): string {
  const changedFiles = formatChangedFilesForWalkthroughPrompt(files);
  const trimmedTemplate = template.trim();
  const expansionContent = trimmedTemplate.includes(CHANGED_FILES_PLACEHOLDER)
    ? trimmedTemplate.replaceAll(CHANGED_FILES_PLACEHOLDER, CHANGED_FILES_REFERENCE)
    : trimmedTemplate;
  const expansionContext = formatPromptReferenceExpansions([
    { name: CHANGES_WALKTHROUGH_PROMPT_NAME, content: expansionContent },
  ]);
  return [
    `@${CHANGES_WALKTHROUGH_PROMPT_NAME}`,
    "",
    "Changed files:",
    changedFiles,
    "",
    "<kandev-system>",
    expansionContext,
    "</kandev-system>",
  ].join("\n");
}
