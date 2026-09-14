import type { FileInfo, GitStatusEntry } from "./types";

const FILE_KEY_SEPARATOR = "\u0000";

type ParsedFileKey = {
  path: string;
  repositoryName?: string;
};

function parseFileKey(key: string): ParsedFileKey {
  const separator = key.indexOf(FILE_KEY_SEPARATOR);
  if (separator < 0) return { path: key };
  return {
    repositoryName: key.slice(0, separator),
    path: key.slice(separator + 1),
  };
}

/**
 * Repairs file records from older Git-status snapshots.
 *
 * New records carry `path` and `repository_name` on each value. Older
 * cumulative-diff records stored those values only in the map key.
 */
export function normalizeGitStatusFiles(
  files: Record<string, FileInfo> | undefined,
): Record<string, FileInfo> | undefined {
  if (!files) return files;

  let normalized: Record<string, FileInfo> | undefined;
  for (const [key, file] of Object.entries(files)) {
    const parsed = parseFileKey(key);
    const path = typeof file.path === "string" && file.path.length > 0 ? file.path : parsed.path;
    const repositoryName = file.repository_name ?? parsed.repositoryName;
    if (path === file.path && repositoryName === file.repository_name) continue;

    normalized ??= { ...files };
    normalized[key] = {
      ...file,
      path,
      ...(repositoryName === undefined ? {} : { repository_name: repositoryName }),
    };
  }
  return normalized ?? files;
}

/** Normalize the file contract before a Git-status entry enters the store. */
export function normalizeGitStatusEntry(gitStatus: GitStatusEntry): GitStatusEntry {
  const files = normalizeGitStatusFiles(gitStatus.files);
  if (files === gitStatus.files && files !== undefined) return gitStatus;
  return { ...gitStatus, files: files ?? {} };
}
