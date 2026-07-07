export type WalkthroughPromptFile = {
  path: string;
  repository_name?: string;
  repositoryName?: string;
  source?: "uncommitted" | "committed" | "pr" | string;
};

const MAX_PROMPT_FILES = 80;

function fileKey(file: WalkthroughPromptFile): string {
  return `${file.repository_name ?? file.repositoryName ?? ""}\0${file.path}\0${file.source ?? ""}`;
}

function formatPromptFile(file: WalkthroughPromptFile): string {
  const repo = file.repository_name ?? file.repositoryName ?? "";
  const source = file.source ? ` [${file.source}]` : "";
  return repo ? `${repo}:${file.path}${source}` : `${file.path}${source}`;
}

export function buildChangesWalkthroughPrompt(files: WalkthroughPromptFile[]): string {
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
  const availableFiles =
    shown.length > 0
      ? shown.map((file) => `- ${formatPromptFile(file)}`).join("\n") +
        (omitted > 0 ? `\n- ... ${omitted} more file(s) omitted from this prompt` : "")
      : "- No changed files were listed by the UI; inspect the local task state before anchoring.";

  return [
    "Please create an agent-authored walkthrough of the current changes using `show_walkthrough_kandev`.",
    "",
    "Walkthrough requirements:",
    "- Use only files listed below or files you verify exist in this task's local worktree/review diff.",
    "- For PR-only files, do not assume the PR head is checked out locally; anchor to the review diff when available, and avoid editor-only/current-worktree claims.",
    "- Anchor steps to changed lines or changed line ranges whenever possible.",
    "- Use `line_end` whenever a logical explanation spans multiple lines; prefer one range step over adjacent single-line steps.",
    "- Keep each step concise and direct. Do not include a `Justification:` preamble.",
    "- If a good local/review anchor is unavailable, omit that step instead of referencing a remote-only path.",
    "",
    "Available changed files:",
    availableFiles,
  ].join("\n");
}
