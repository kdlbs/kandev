"use client";

import { IconMessage } from "@tabler/icons-react";
import type { DiffComment } from "@/lib/diff/types";
import { formatLineRange } from "@/lib/diff";

type FileGroup = { key: string; filePath: string; comments: DiffComment[] };

function fileGroupKey(comment: DiffComment): string {
  return JSON.stringify([comment.repositoryId ?? null, comment.filePath]);
}

/**
 * Groups comments by file, preserving first-seen file order so the overview
 * mirrors the order comments were added / appear in the file tree.
 */
export function groupCommentsByFile(comments: DiffComment[]): FileGroup[] {
  const order: string[] = [];
  const byFile = new Map<string, DiffComment[]>();
  const filePathByKey = new Map<string, string>();
  for (const comment of comments) {
    const key = fileGroupKey(comment);
    const existing = byFile.get(key);
    if (existing) {
      existing.push(comment);
    } else {
      order.push(key);
      filePathByKey.set(key, comment.filePath);
      byFile.set(key, [comment]);
    }
  }
  return order.map((key) => ({
    key,
    filePath: filePathByKey.get(key)!,
    comments: byFile.get(key)!,
  }));
}

function fileDir(filePath: string): string {
  const idx = filePath.lastIndexOf("/");
  return idx === -1 ? "" : filePath.slice(0, idx);
}

function fileName(filePath: string): string {
  const idx = filePath.lastIndexOf("/");
  return idx === -1 ? filePath : filePath.slice(idx + 1);
}

/**
 * Scrollable overview of pending review comments, grouped per file. Rendered
 * inside the "Fix Comments" hover popover on the review top bar.
 */
export function ReviewCommentsOverview({ comments }: { comments: DiffComment[] }) {
  const groups = groupCommentsByFile(comments);
  const total = comments.length;

  if (total === 0) {
    return (
      <div className="px-3 py-4 text-center text-xs text-muted-foreground">
        No comments to fix yet.
      </div>
    );
  }

  return (
    <div className="flex max-h-[min(60vh,26rem)] flex-col" data-testid="review-comments-overview">
      <div className="flex items-center gap-2 border-b border-border/60 px-3 py-2">
        <IconMessage className="h-4 w-4 shrink-0 text-blue-500" />
        <span className="text-sm font-medium">
          {total} pending review comment{total !== 1 ? "s" : ""}
        </span>
        <span className="text-xs text-muted-foreground">
          across {groups.length} file{groups.length !== 1 ? "s" : ""}
        </span>
      </div>

      <div
        className="min-h-0 flex-1 overflow-y-auto overscroll-contain px-2 py-1.5"
        data-testid="review-comments-overview-scroll"
      >
        {groups.map((group) => (
          <div key={group.key} className="mb-2 last:mb-0">
            <div className="flex items-baseline gap-1.5 px-1 pb-1">
              <span className="truncate text-xs font-medium text-foreground" title={group.filePath}>
                {fileName(group.filePath)}
              </span>
              {fileDir(group.filePath) && (
                <span className="min-w-0 flex-1 truncate text-[11px] text-muted-foreground/70">
                  {fileDir(group.filePath)}
                </span>
              )}
              <span className="ml-auto shrink-0 rounded-full bg-muted px-1.5 py-0.5 text-[10px] font-medium text-muted-foreground">
                {group.comments.length}
              </span>
            </div>

            <div className="space-y-1">
              {group.comments.map((comment) => (
                <div
                  key={comment.id}
                  className="rounded-md border border-border/50 bg-muted/30 px-2 py-1.5"
                >
                  <div className="mb-0.5 flex items-center gap-1 text-[10px] font-medium text-muted-foreground">
                    <span>{formatLineRange(comment.startLine, comment.endLine)}</span>
                    <span className="text-muted-foreground/60">
                      · {comment.side === "additions" ? "new" : "old"}
                    </span>
                  </div>
                  <p className="line-clamp-2 whitespace-pre-wrap text-xs leading-snug text-foreground/90">
                    {comment.text}
                  </p>
                </div>
              ))}
            </div>
          </div>
        ))}
      </div>
    </div>
  );
}
