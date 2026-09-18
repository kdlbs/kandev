import type { ReviewComment } from "./types";

type FileGroup = { key: string; filePath: string; comments: ReviewComment[] };

function repositoryNamesById(comments: ReviewComment[]): Map<string, Set<string>> {
  const names = new Map<string, Set<string>>();
  for (const comment of comments) {
    if (comment.source !== "review-file" || !comment.repositoryId) continue;
    const scopes = names.get(comment.repositoryId) ?? new Set<string>();
    scopes.add(comment.repositoryName);
    names.set(comment.repositoryId, scopes);
  }
  return names;
}

function fileGroupKey(comment: ReviewComment, names: Map<string, Set<string>>): string {
  if (comment.source === "review-file") {
    return JSON.stringify(["name", comment.repositoryName, comment.filePath]);
  }
  if (!comment.repositoryId) return JSON.stringify(["name", "", comment.filePath]);
  const scopes = names.get(comment.repositoryId);
  // Legacy line rows lack scope names. Only bridge an unambiguous ID mapping.
  return scopes?.size === 1
    ? JSON.stringify(["name", [...scopes][0], comment.filePath])
    : JSON.stringify(["id", comment.repositoryId, comment.filePath]);
}

/**
 * Groups comments by file, preserving first-seen file order so the overview
 * mirrors the order comments were added / appear in the file tree.
 */
export function groupCommentsByFile(comments: ReviewComment[]): FileGroup[] {
  const names = repositoryNamesById(comments);
  const byFile = new Map<string, FileGroup>();
  for (const comment of comments) {
    const key = fileGroupKey(comment, names);
    const filePath =
      comment.source === "review-file"
        ? [comment.repositoryName, comment.filePath].filter(Boolean).join("/")
        : comment.filePath;
    const existing = byFile.get(key);
    if (existing) {
      existing.comments.push(comment);
      if (filePath !== comment.filePath) existing.filePath = filePath;
    } else {
      byFile.set(key, { key, filePath, comments: [comment] });
    }
  }
  return [...byFile.values()];
}
