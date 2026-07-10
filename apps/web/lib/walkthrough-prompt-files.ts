import type { WalkthroughPromptFile } from "./walkthrough-request";

type PromptFileInput = {
  path?: string;
  repositoryName?: string;
  repository_name?: string;
};

type CumulativePromptFileInput = PromptFileInput & {
  diff?: string;
};

type PromptFileSource = "uncommitted" | "staged" | "committed" | "pr";

const FILE_KEY_SEP = "\u0000";

function splitCompositeKey(key: string): { repo: string; path: string } {
  const sep = key.indexOf(FILE_KEY_SEP);
  if (sep < 0) return { repo: "", path: key };
  return { repo: key.slice(0, sep), path: key.slice(sep + 1) };
}

function normalizePromptFile(
  source: PromptFileSource,
  file: PromptFileInput,
  fallbackKey?: string,
): WalkthroughPromptFile | null {
  const fallback = fallbackKey ? splitCompositeKey(fallbackKey) : { repo: "", path: "" };
  const path = file.path || fallback.path;
  if (!path) return null;
  const repo = file.repository_name ?? file.repositoryName ?? fallback.repo;
  return {
    path,
    ...(repo ? { repository_name: repo } : {}),
    source,
  };
}

function promptKey(file: WalkthroughPromptFile): string {
  return `${file.repository_name ?? file.repositoryName ?? ""}\0${file.path}`;
}

function addPromptFile(
  out: WalkthroughPromptFile[],
  seen: Set<string>,
  source: PromptFileSource,
  file: PromptFileInput,
  fallbackKey?: string,
) {
  const promptFile = normalizePromptFile(source, file, fallbackKey);
  if (!promptFile) return;
  const key = promptKey(promptFile);
  if (seen.has(key)) return;
  seen.add(key);
  out.push(promptFile);
}

export function buildWalkthroughPromptFiles({
  unstagedFiles,
  stagedFiles,
  committedFiles,
  prFiles,
}: {
  unstagedFiles: PromptFileInput[];
  stagedFiles: PromptFileInput[];
  committedFiles?: Record<string, CumulativePromptFileInput> | null;
  prFiles: PromptFileInput[];
}): WalkthroughPromptFile[] {
  const out: WalkthroughPromptFile[] = [];
  const seen = new Set<string>();
  for (const file of unstagedFiles) addPromptFile(out, seen, "uncommitted", file);
  for (const file of stagedFiles) addPromptFile(out, seen, "staged", file);
  for (const [key, file] of Object.entries(committedFiles ?? {})) {
    addPromptFile(out, seen, "committed", file, key);
  }
  for (const file of prFiles) addPromptFile(out, seen, "pr", file);
  return out;
}
