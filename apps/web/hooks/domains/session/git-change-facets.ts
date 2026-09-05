import type {
  FileChangeFacet,
  FileInfo,
  GitChangeLayer,
} from "@/lib/state/slices/session-runtime/types";

function projectFileChange(
  file: FileInfo,
  layer: GitChangeLayer,
  facet: FileChangeFacet,
): FileInfo {
  return {
    ...file,
    status: facet.status,
    staged: layer === "staged",
    additions: facet.additions,
    deletions: facet.deletions,
    old_path: facet.old_path,
    diff: facet.diff,
    diff_skip_reason: facet.diff_skip_reason,
    change_layer: layer,
  };
}

/**
 * Builds section-specific file views while preserving one canonical raw file
 * per repository/path for counts and mutations.
 */
export function splitFilesByChangeLayer(allFiles: FileInfo[]): {
  stagedFiles: FileInfo[];
  unstagedFiles: FileInfo[];
} {
  const stagedFiles: FileInfo[] = [];
  const unstagedFiles: FileInfo[] = [];

  for (const file of allFiles) {
    const hasFacets = Boolean(file.staged_change || file.unstaged_change);
    if (!hasFacets) {
      (file.staged ? stagedFiles : unstagedFiles).push(file);
      continue;
    }
    if (file.staged_change) {
      stagedFiles.push(projectFileChange(file, "staged", file.staged_change));
    }
    if (file.unstaged_change) {
      unstagedFiles.push(projectFileChange(file, "unstaged", file.unstaged_change));
    }
  }

  return { stagedFiles, unstagedFiles };
}
