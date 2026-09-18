import type { ReviewComment } from "./types";

type FileGroup = { key: string; filePath: string; comments: ReviewComment[] };

function fileGroupKey(comment: ReviewComment): string {
  return JSON.stringify([
    comment.repositoryId
      ? ["id", comment.repositoryId]
      : ["name", comment.source === "review-file" ? comment.repositoryName : ""],
    comment.filePath,
  ]);
}

/**
 * Groups comments by file, preserving first-seen file order so the overview
 * mirrors the order comments were added / appear in the file tree.
 */
export function groupCommentsByFile(comments: ReviewComment[]): FileGroup[] {
  const order: string[] = [];
  const byFile = new Map<string, ReviewComment[]>();
  const filePathByKey = new Map<string, string>();
  for (const comment of comments) {
    const key = fileGroupKey(comment);
    const existing = byFile.get(key);
    if (existing) {
      existing.push(comment);
      if (comment.source === "review-file" && comment.repositoryName) {
        filePathByKey.set(key, `${comment.repositoryName}/${comment.filePath}`);
      }
    } else {
      order.push(key);
      filePathByKey.set(
        key,
        comment.source === "review-file"
          ? [comment.repositoryName, comment.filePath].filter(Boolean).join("/")
          : comment.filePath,
      );
      byFile.set(key, [comment]);
    }
  }
  return order.map((key) => ({
    key,
    filePath: filePathByKey.get(key)!,
    comments: byFile.get(key)!,
  }));
}
