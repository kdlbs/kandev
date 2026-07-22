"use client";

import { memo } from "react";
import { IconCheck, IconX, IconFileCode2 } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { FilePathButton } from "./file-path-button";
import type { Message } from "@/lib/types/http";
import { ExpandableRow } from "./expandable-row";
import { useExpandState } from "./use-expand-state";
import { useOpenFileAtLine } from "@/hooks/use-file-editors";
import { splitReadFiles, type ReadFileRef } from "@/lib/read-selector";

type ReadFileOutput = {
  content?: string;
  line_count?: number;
  truncated?: boolean;
  language?: string;
};

type ReadFilePayload = {
  file_path?: string;
  offset?: number;
  limit?: number;
  output?: ReadFileOutput;
};

type ToolReadMetadata = {
  tool_call_id?: string;
  title?: string;
  status?: "pending" | "running" | "complete" | "error";
  normalized?: { read_file?: ReadFilePayload };
};

type ToolReadMessageProps = {
  comment: Message;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string) => void;
};

// ReadStatusIcon renders the status glyph (complete/error/running) for a read card.
function ReadStatusIcon({ status }: { status: string | undefined }) {
  if (status === "complete") return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  if (status === "error") return <IconX className="h-3.5 w-3.5 text-red-500" />;
  if (status === "running") return <GridSpinner className="text-muted-foreground" />;
  return null;
}

// getReadSummary returns the short "Read N lines" header label for the card.
function getReadSummary(lineCount: number | undefined): string {
  if (lineCount) return `Read ${lineCount} line${lineCount !== 1 ? "s" : ""}`;
  return "Read";
}

// formatLineRange renders the range the agent read (carried separately from
// the file path so the link stays openable). offset is the 1-based start line;
// limit is the line count (0 when open-ended / a single anchor).
function formatLineRange(offset: number | undefined, limit: number | undefined): string {
  if (!offset || offset <= 0) return "";
  if (limit && limit > 0) return `:${offset}-${offset + limit - 1}`;
  return `:${offset}`;
}

// parseReadMetadata pulls the read status, file path, line range, and output
// out of a tool_read message's normalized metadata for rendering.
function parseReadMetadata(comment: Message) {
  const metadata = comment.metadata as ToolReadMetadata | undefined;
  const status = metadata?.status;
  const readFile = metadata?.normalized?.read_file;
  const readOutput = readFile?.output;
  const filePath = readFile?.file_path;
  const offset = readFile?.offset;
  const limit = readFile?.limit;
  const hasOutput = !!readOutput?.content;
  const isSuccess = status === "complete";
  return { status, readOutput, filePath, offset, limit, hasOutput, isSuccess };
}

// ReadFileLink renders one openable file link plus its line-range badge. Used
// for each file when a single read references several comma-joined files; the
// per-file startLine drives the scroll-to-line on open.
function ReadFileLink({
  file,
  worktreePath,
  sessionId,
  onOpenFile,
}: {
  file: ReadFileRef;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string) => void;
}) {
  const handleOpenFile = useOpenFileAtLine(onOpenFile, file.startLine, worktreePath, sessionId);
  const lineRange = formatLineRange(file.startLine, file.lineCount);
  return (
    <span className="inline-flex items-baseline min-w-0">
      <FilePathButton
        filePath={file.path}
        worktreePath={worktreePath}
        onOpenFile={onOpenFile ? handleOpenFile : undefined}
      />
      {lineRange && (
        <span className="font-mono text-xs text-muted-foreground/70 shrink-0">{lineRange}</span>
      )}
    </span>
  );
}

// ToolReadMessage renders a read tool call: the file link, line-range badge, and
// the (expandable) read output.
export const ToolReadMessage = memo(function ToolReadMessage({
  comment,
  worktreePath,
  sessionId,
  onOpenFile,
}: ToolReadMessageProps) {
  const { status, readOutput, filePath, offset, limit, hasOutput, isSuccess } =
    parseReadMetadata(comment);
  const autoExpanded = status === "running";
  const { isExpanded, handleToggle } = useExpandState(status, autoExpanded);
  // splitReadFiles (pure, cheap — the React Compiler memoizes) yields the clean,
  // openable path + parsed range for every file. This also fixes legacy/raw
  // reads whose selector is still glued to the path ("foo.sh:88-137"). For a
  // single file the backend may instead carry the range in offset/limit, so
  // merge that in as a fallback.
  const files = filePath ? splitReadFiles(filePath) : [];
  const resolvedFiles =
    files.length === 1
      ? [
          {
            path: files[0].path,
            startLine: files[0].startLine || offset || 0,
            lineCount: files[0].lineCount || limit || 0,
          },
        ]
      : files;

  return (
    <ExpandableRow
      icon={<IconFileCode2 className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs min-w-0">
          <span className="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap">
            <span className="font-mono text-xs text-muted-foreground">
              {getReadSummary(readOutput?.line_count)}
            </span>
            {!isSuccess && <ReadStatusIcon status={status} />}
          </span>
          {filePath && (
            <span className="inline-flex flex-wrap items-baseline gap-x-1 min-w-0">
              {resolvedFiles.map((file, idx) => (
                <span key={`${file.path}-${idx}`} className="inline-flex items-baseline min-w-0">
                  <ReadFileLink
                    file={file}
                    worktreePath={worktreePath}
                    sessionId={sessionId}
                    onOpenFile={onOpenFile}
                  />
                  {idx < resolvedFiles.length - 1 && (
                    <span className="text-muted-foreground/50">,</span>
                  )}
                </span>
              ))}
            </span>
          )}
          {readOutput?.truncated && (
            <span className="text-xs text-amber-500/80 shrink-0">(truncated)</span>
          )}
        </div>
      }
      hasExpandableContent={hasOutput}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      {readOutput?.content && (
        <div className="relative rounded-md border border-border/50 overflow-hidden bg-muted/20">
          <pre className="text-xs p-3 overflow-x-auto max-h-[200px] overflow-y-auto">
            <code>{readOutput.content}</code>
          </pre>
        </div>
      )}
    </ExpandableRow>
  );
});
