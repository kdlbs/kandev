"use client";

import { IconBrain, IconCode, IconListCheck } from "@tabler/icons-react";
import { cn } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import type { RichMetadata } from "@/components/task/chat/types";
import { DiffViewBlock } from "@/components/task/chat/messages/diff-view-block";
import { normalizeDiffString } from "@/lib/diff";
import type { FileDiffData } from "@/lib/diff/types";

/**
 * Resolve old diff payload format to new FileDiffData format
 */
function resolveDiffPayload(diff: unknown): FileDiffData | null {
  if (!diff) return null;

  // Handle string diff
  if (typeof diff === "string") {
    const normalized = normalizeDiffString(diff, "file");
    if (!normalized) return null;
    return {
      filePath: "file",
      oldContent: "",
      newContent: "",
      diff: normalized,
      additions: 0,
      deletions: 0,
    };
  }

  // Handle array of hunks (legacy format)
  if (Array.isArray(diff)) {
    const hunkStrings = diff.map((hunk) => String(hunk)).join("\n");
    const normalized = normalizeDiffString(hunkStrings, "file");
    if (!normalized) return null;
    return {
      filePath: "file",
      oldContent: "",
      newContent: "",
      diff: normalized,
      additions: 0,
      deletions: 0,
    };
  }

  // Handle object with hunks array (legacy format)
  if (typeof diff === "object" && diff !== null) {
    const candidate = diff as {
      hunks?: unknown[];
      oldFile?: { fileName?: string };
      newFile?: { fileName?: string };
    };
    if (Array.isArray(candidate.hunks)) {
      const hunkStrings = candidate.hunks.map((hunk) => String(hunk)).join("\n");
      const filePath = candidate.newFile?.fileName || candidate.oldFile?.fileName || "file";
      const normalized = normalizeDiffString(hunkStrings, filePath);
      if (!normalized) return null;
      return {
        filePath,
        oldContent: "",
        newContent: "",
        diff: normalized,
        additions: 0,
        deletions: 0,
      };
    }
  }

  return null;
}

export function RichBlocks({ comment }: { comment: Message }) {
  const metadata = comment.metadata as RichMetadata | undefined;
  if (!metadata) return null;

  const todos = metadata.todos ?? [];
  const todoItems = todos
    .map((item) => (typeof item === "string" ? { text: item, done: false } : item))
    .filter((item) => item.text);
  const diffData = resolveDiffPayload(metadata.diff);
  const diffText = typeof metadata.diff === "string" ? metadata.diff : null;

  return (
    <>
      {metadata.thinking && (
        <div className="mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs">
          <div className="flex items-center gap-2 text-muted-foreground mb-1 uppercase tracking-wide">
            <IconBrain className="h-3.5 w-3.5" />
            <span>Thinking</span>
          </div>
          <div className="whitespace-pre-wrap text-foreground/80">{metadata.thinking}</div>
        </div>
      )}
      {todoItems.length > 0 && (
        <div className="mt-3 rounded-md border border-border/49 bg-background/60 px-3 py-2 text-xs">
          <div className="flex items-center gap-2 text-muted-foreground mb-1 uppercase tracking-wide">
            <IconListCheck className="h-3.5 w-3.5" />
            <span>Todos</span>
          </div>
          <div className="space-y-1">
            {todoItems.map((todo) => (
              <div key={todo.text} className="flex items-center gap-2">
                <span
                  className={cn(
                    "h-1.5 w-1.5 rounded-full",
                    todo.done ? "bg-green-500" : "bg-muted-foreground/60",
                  )}
                />
                <span className={cn(todo.done && "line-through text-muted-foreground")}>
                  {todo.text}
                </span>
              </div>
            ))}
          </div>
        </div>
      )}
      {diffData && <DiffViewBlock data={diffData} />}
      {!diffData && diffText && (
        <div className="mt-3 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs">
          <div className="flex items-center gap-2 text-muted-foreground mb-1 uppercase tracking-wide">
            <IconCode className="h-3.5 w-3.5" />
            <span>Diff</span>
          </div>
          <pre className="whitespace-pre-wrap break-words text-[11px] text-foreground/80">
            {diffText}
          </pre>
        </div>
      )}
    </>
  );
}
