"use client";

import { useState, memo } from "react";
import {
  IconCheck,
  IconX,
  IconEdit,
  IconFilePlus,
  IconExternalLink,
  IconCopy,
} from "@tabler/icons-react";
import { GridSpinner } from "@/components/grid-spinner";
import { transformPathsInText } from "@/lib/utils";
import type { Message } from "@/lib/types/http";
import { DiffViewBlock } from "./diff-view-block";
import { ExpandableRow } from "./expandable-row";
import { transformFileMutation, type FileMutation } from "@/lib/diff";
import { useExpandState } from "./use-expand-state";

type ModifyFilePayload = {
  file_path?: string;
  mutations?: FileMutation[];
};

type ToolEditMetadata = {
  tool_call_id?: string;
  status?: "pending" | "running" | "complete" | "error";
  normalized?: { modify_file?: ModifyFilePayload };
};

type ToolEditMessageProps = {
  comment: Message;
  worktreePath?: string;
  onOpenFile?: (path: string) => void;
};

function EditStatusIcon({ status }: { status: string | undefined }) {
  if (status === "complete") return <IconCheck className="h-3.5 w-3.5 text-green-500" />;
  if (status === "error") return <IconX className="h-3.5 w-3.5 text-red-500" />;
  if (status === "running") return <GridSpinner className="text-muted-foreground" />;
  return null;
}

function getEditSummary(
  content: string,
  worktreePath: string | undefined,
  isWriteOperation: boolean,
  lineCount: number,
): string {
  const baseSummary = transformPathsInText(content, worktreePath);
  if (isWriteOperation && lineCount > 0) {
    return `${baseSummary} (${lineCount} line${lineCount !== 1 ? "s" : ""})`;
  }
  return baseSummary;
}

type FileActionButtonProps = {
  filePath: string;
  worktreePath: string | undefined;
  onOpenFile: ((path: string) => void) | undefined;
  copied: boolean;
  onCopyPath: (e: React.MouseEvent) => void;
};

function FileActionButton({
  filePath,
  worktreePath,
  onOpenFile,
  copied,
  onCopyPath,
}: FileActionButtonProps) {
  const isFileInWorktree = worktreePath && filePath.startsWith(worktreePath);
  if (onOpenFile && isFileInWorktree) {
    return (
      <button
        type="button"
        onClick={(e) => {
          e.stopPropagation();
          onOpenFile(filePath);
        }}
        className="opacity-0 group-hover/expandable:opacity-100 transition-opacity text-muted-foreground hover:text-foreground shrink-0 cursor-pointer"
        title="Open file"
      >
        <IconExternalLink className="h-3.5 w-3.5" />
      </button>
    );
  }
  if (!isFileInWorktree) {
    return (
      <button
        type="button"
        onClick={onCopyPath}
        className="opacity-0 group-hover/expandable:opacity-100 transition-opacity text-muted-foreground hover:text-foreground shrink-0 cursor-pointer"
        title={copied ? "Copied!" : "Copy path"}
      >
        {copied ? (
          <IconCheck className="h-3.5 w-3.5 text-green-500" />
        ) : (
          <IconCopy className="h-3.5 w-3.5" />
        )}
      </button>
    );
  }
  return null;
}

type EditExpandedContentProps = {
  diffData: import("@/lib/diff/types").FileDiffData | null;
  writeContent: string | undefined;
};

function EditExpandedContent({ diffData, writeContent }: EditExpandedContentProps) {
  if (diffData?.diff) return <DiffViewBlock data={diffData} className="mt-0 border-0 px-0" />;
  if (writeContent) {
    return (
      <div className="pl-4 border-l-2 border-border/30">
        <pre className="text-xs bg-muted/30 rounded p-2 overflow-x-auto max-h-[300px] overflow-y-auto whitespace-pre-wrap">
          {writeContent}
        </pre>
      </div>
    );
  }
  return null;
}

function parseEditMetadata(comment: Message) {
  const metadata = comment.metadata as ToolEditMetadata | undefined;
  const status = metadata?.status;
  const modifyFile = metadata?.normalized?.modify_file;
  const filePath = modifyFile?.file_path;
  const mutation = modifyFile?.mutations?.[0];
  const writeContent = mutation?.content;
  const isWriteOperation = mutation?.type === "create";
  const diffData = filePath && mutation ? transformFileMutation(filePath, mutation) : null;
  const hasExpandableContent = !!(diffData?.diff || writeContent);
  const isSuccess = status === "complete";
  const lineCount = writeContent ? writeContent.split("\n").length : 0;
  return {
    status,
    filePath,
    writeContent,
    isWriteOperation,
    diffData,
    hasExpandableContent,
    isSuccess,
    lineCount,
  };
}

export const ToolEditMessage = memo(function ToolEditMessage({
  comment,
  worktreePath,
  onOpenFile,
}: ToolEditMessageProps) {
  const {
    status,
    filePath,
    writeContent,
    isWriteOperation,
    diffData,
    hasExpandableContent,
    isSuccess,
    lineCount,
  } = parseEditMetadata(comment);
  const [copied, setCopied] = useState(false);
  const autoExpanded = status === "running";
  const { isExpanded, handleToggle } = useExpandState(status, autoExpanded);
  const Icon = isWriteOperation ? IconFilePlus : IconEdit;
  const summary = getEditSummary(comment.content, worktreePath, isWriteOperation, lineCount);

  const handleCopyPath = (e: React.MouseEvent) => {
    e.stopPropagation();
    if (filePath) {
      navigator.clipboard?.writeText(filePath);
      setCopied(true);
      setTimeout(() => setCopied(false), 1500);
    }
  };

  return (
    <ExpandableRow
      icon={<Icon className="h-4 w-4 text-muted-foreground" />}
      header={
        <div className="flex items-center gap-2 text-xs min-w-0">
          <span className="inline-flex items-center gap-1.5 shrink-0 whitespace-nowrap">
            <span className="font-mono text-xs text-muted-foreground">{summary}</span>
            {!isSuccess && <EditStatusIcon status={status} />}
          </span>
          {filePath && (
            <FileActionButton
              filePath={filePath}
              worktreePath={worktreePath}
              onOpenFile={onOpenFile}
              copied={copied}
              onCopyPath={handleCopyPath}
            />
          )}
        </div>
      }
      hasExpandableContent={hasExpandableContent}
      isExpanded={isExpanded}
      onToggle={handleToggle}
    >
      <EditExpandedContent diffData={diffData} writeContent={writeContent} />
    </ExpandableRow>
  );
});
