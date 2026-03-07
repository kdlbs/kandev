"use client";

import { IconBrain, IconCode, IconExternalLink, IconPhoto } from "@tabler/icons-react";
import type { Message } from "@/lib/types/http";
import type { ContentBlock, RichMetadata } from "@/components/task/chat/types";
import { DiffViewBlock } from "@/components/task/chat/messages/diff-view-block";
import { TodoMessage } from "@/components/task/chat/messages/todo-message";
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

  const hasTodos = (metadata.todos ?? []).length > 0;
  const diffData = resolveDiffPayload(metadata.diff);
  const diffText = typeof metadata.diff === "string" ? metadata.diff : null;
  const contentBlocks = (metadata.content_blocks ?? []).filter(
    (b) => b.type === "image" || b.type === "resource_link",
  );

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
      {hasTodos && (
        <div className="mt-3">
          <TodoMessage comment={comment} />
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
      {contentBlocks.length > 0 &&
        contentBlocks.map((block, i) => <ContentBlockView key={i} block={block} />)}
    </>
  );
}

function ContentBlockView({ block }: { block: ContentBlock }) {
  switch (block.type) {
    case "image":
      return (
        <div className="mt-3 rounded-md border border-border/50 bg-background/60 p-2">
          <div className="flex items-center gap-2 text-muted-foreground mb-1 text-xs uppercase tracking-wide">
            <IconPhoto className="h-3.5 w-3.5" />
            <span>Image</span>
          </div>
          {block.data && (
            // eslint-disable-next-line @next/next/no-img-element -- base64 data URIs are not optimizable by next/image
            <img
              src={block.uri || `data:${block.mime_type || "image/png"};base64,${block.data}`}
              alt="Agent image"
              className="max-w-full max-h-96 rounded"
            />
          )}
          {block.uri && !block.data && (
            // eslint-disable-next-line @next/next/no-img-element -- external agent image URIs
            <img src={block.uri} alt="Agent image" className="max-w-full max-h-96 rounded" />
          )}
        </div>
      );
    case "resource_link":
      return (
        <a
          href={block.uri}
          target="_blank"
          rel="noopener noreferrer"
          className="mt-2 flex items-center gap-2 rounded-md border border-border/50 bg-background/60 px-3 py-2 text-xs hover:bg-muted/50 transition-colors cursor-pointer"
        >
          <IconExternalLink className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
          <div className="min-w-0">
            <div className="font-medium text-foreground truncate">
              {block.title || block.name || block.uri}
            </div>
            {block.description && (
              <div className="text-muted-foreground truncate">{block.description}</div>
            )}
          </div>
        </a>
      );
    default:
      return null;
  }
}
