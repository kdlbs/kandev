import type { ReviewComment } from "./types";

type FileGroup = {
  key: string;
  filePath: string;
  repositoryName?: string;
  comments: ReviewComment[];
};

function repositoryNamesById(comments: ReviewComment[]): Map<string, Set<string>> {
  const names = new Map<string, Set<string>>();
  for (const comment of comments) {
    if (comment.repositoryName === undefined || !comment.repositoryId) continue;
    const scopes = names.get(comment.repositoryId) ?? new Set<string>();
    scopes.add(comment.repositoryName);
    names.set(comment.repositoryId, scopes);
  }
  return names;
}

function commentRepositoryName(
  comment: ReviewComment,
  names: Map<string, Set<string>>,
): string | undefined {
  if (comment.repositoryName !== undefined) return comment.repositoryName;
  const scopes = comment.repositoryId ? names.get(comment.repositoryId) : undefined;
  // Legacy line rows lack scope names. Only bridge an unambiguous ID mapping.
  return scopes?.size === 1 ? [...scopes][0] : undefined;
}

/**
 * Groups comments by file, preserving first-seen file order so the overview
 * mirrors the order comments were added / appear in the file tree.
 */
export function groupCommentsByFile(comments: ReviewComment[]): FileGroup[] {
  const names = repositoryNamesById(comments);
  const byFile = new Map<string, FileGroup>();
  for (const comment of comments) {
    const repositoryName = commentRepositoryName(comment, names);
    const key =
      repositoryName === undefined
        ? JSON.stringify(["legacy", comment.repositoryId ?? null, comment.filePath])
        : JSON.stringify(["name", repositoryName, comment.filePath]);
    const filePath = [repositoryName, comment.filePath].filter(Boolean).join("/");
    const existing = byFile.get(key);
    if (existing) {
      existing.comments.push(comment);
    } else {
      byFile.set(key, { key, filePath, repositoryName, comments: [comment] });
    }
  }
  return [...byFile.values()];
}
