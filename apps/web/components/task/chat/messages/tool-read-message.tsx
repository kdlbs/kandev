"use client";

import { memo } from "react";
import { IconCheck, IconX, IconFileCode2 } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { FilePathButton } from "./file-path-button";
import type { Message } from "@/lib/types/http";
import { ExpandableRow } from "./expandable-row";
import { useExpandState } from "./use-expand-state";

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
  status?: "pending" | "running" | "complete" | "error";
  normalized?: { read_file?: ReadFilePayload };
};

type ToolReadMessageProps = {
  comment: Message;
  worktreePath?: string;
  sessionId?: string;
  onOpenFile?: (path: string) => void;
};

function ReadStatusIcon({ status }: { status: string | undefined }) {
  if (status === "complete") return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  if (status === "error") return <IconX className="h-3.5 w-3.5 text-red-500" />;
  if (status === "running") return <GridSpinner className="text-muted-foreground" />;
  return null;
}

function getReadSummary(lineCount: number | undefined): string {
  if (lineCount) return `Read ${lineCount} line${lineCount !== 1 ? "s" : ""}`;
  return "Read";
}

function parseReadMetadata(comment: Message) {
  const metadata = comment.metadata as ToolReadMetadata | undefined;
  const status = metadata?.status;
  const readFile = metadata?.normalized?.read_file;
  const readOutput = readFile?.output;
  const filePath = readFile?.file_path;
  const hasOutput = !!readOutput?.content;
  const isSuccess = status === "complete";
  return { status, readOutput, filePath, hasOutput, isSuccess };
}

export const ToolReadMessage = memo(function ToolReadMessage({
  comment,
  worktreePath,
  onOpenFile,
}: ToolReadMessageProps) {
  const { status, readOutput, filePath, hasOutput, isSuccess } = parseReadMetadata(comment);
  const autoExpanded = status === "running";
  const { isExpanded, handleToggle } = useExpandState(status, autoExpanded);

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
            <span className="min-w-0">
              <FilePathButton
                filePath={filePath}
                worktreePath={worktreePath}
                onOpenFile={onOpenFile}
              />
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
