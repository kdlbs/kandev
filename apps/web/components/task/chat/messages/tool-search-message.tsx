"use client";

import { memo } from "react";
import { IconCheck, IconX, IconSearch } from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { toRelativePath } from "@/lib/utils";
import { FilePathButton } from "./file-path-button";
import type { Message } from "@/lib/types/http";
import { ExpandableRow } from "./expandable-row";
import { useExpandState } from "./use-expand-state";

type CodeSearchOutput = {
  files?: string[];
  file_count?: number;
  truncated?: boolean;
};

type CodeSearchPayload = {
  query?: string;
  pattern?: string;
  path?: string;
  glob?: string;
  output?: CodeSearchOutput;
};

type ToolSearchMetadata = {
  tool_call_id?: string;
  status?: "pending" | "running" | "complete" | "error";
  normalized?: { code_search?: CodeSearchPayload };
};

type ToolSearchMessageProps = {
  comment: Message;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

function SearchStatusIcon({ status }: { status: string | undefined }) {
  if (status === "complete") return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  if (status === "error") return <IconX className="h-3.5 w-3.5 text-red-500" />;
  if (status === "running") return <GridSpinner className="text-muted-foreground" />;
  return null;
}

function getSearchSummary(searchOutput: CodeSearchOutput | undefined): string {
  if (searchOutput?.files && searchOutput.files.length > 0) {
    const count = searchOutput.file_count || searchOutput.files.length;
    return `Found ${count} file${count !== 1 ? "s" : ""}`;
  }
  return "Searching";
}

type SearchResultsProps = {
  files: string[];
  worktreePath: string | undefined;
  onOpenFile: ((path: string) => void) | undefined;
  truncated: boolean | undefined;
};

function SearchResultFiles({ files, worktreePath, onOpenFile, truncated }: SearchResultsProps) {
  return (
    <div className="rounded-md border border-border/50 overflow-hidden bg-muted/20">
      <div className="text-xs space-y-0.5 max-h-[200px] overflow-y-auto p-1">
        {files.map((file) => (
          <FilePathButton
            key={file}
            filePath={file}
            worktreePath={worktreePath}
            onOpenFile={onOpenFile}
            variant="list-item"
          />
        ))}
        {truncated && (
          <div className="text-amber-500/80 mt-1 px-2">...and more files (truncated)</div>
        )}
      </div>
    </div>
  );
}

function parseSearchMetadata(comment: Message) {
  const metadata = comment.metadata as ToolSearchMetadata | undefined;
  const status = metadata?.status;
  const codeSearch = metadata?.normalized?.code_search;
  const searchOutput = codeSearch?.output;
  const searchPath = codeSearch?.path;
  const searchPattern = codeSearch?.glob || codeSearch?.pattern || codeSearch?.query;
  const hasOutput = !!(searchOutput?.files && searchOutput.files.length > 0);
  const isSuccess = status === "complete";
  return { status, searchOutput, searchPath, searchPattern, hasOutput, isSuccess };
}

export const ToolSearchMessage = memo(function ToolSearchMessage({
  comment,
  worktreePath,
  onOpenFile,
}: ToolSearchMessageProps) {
  const { status, searchOutput, searchPath, searchPattern, hasOutput, isSuccess } =
    parseSearchMetadata(comment);
  const autoExpanded = status === "running";
  const { isExpanded, handleToggle } = useExpandState(status, autoExpanded);

  return (
    <ExpandableRow
      icon={<IconSearch className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs">
          <span className="inline-flex items-center gap-1.5">
            <span className="font-mono text-xs text-muted-foreground">
              {getSearchSummary(searchOutput)}
            </span>
            {!isSuccess && <SearchStatusIcon status={status} />}
          </span>
          {searchPattern && (
            <span className="text-xs text-muted-foreground/60 font-mono">{searchPattern}</span>
          )}
          {searchPath && (
            <span className="text-xs text-muted-foreground/60 truncate font-mono bg-muted/30 px-1.5 py-0.5 rounded">
              {toRelativePath(searchPath, worktreePath)}
            </span>
          )}
        </div>
      }
      hasExpandableContent={hasOutput}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      {searchOutput?.files && (
        <SearchResultFiles
          files={searchOutput.files}
          worktreePath={worktreePath}
          onOpenFile={onOpenFile}
          truncated={searchOutput.truncated}
        />
      )}
    </ExpandableRow>
  );
});
