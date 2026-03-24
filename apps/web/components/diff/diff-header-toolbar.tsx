"use client";

import { useCallback, type ReactNode } from "react";
import { cn } from "@kandev/ui/lib/utils";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipTrigger, TooltipContent } from "@kandev/ui/tooltip";
import {
  IconCopy,
  IconTextWrap,
  IconLayoutRows,
  IconLayoutColumns,
  IconPencil,
  IconArrowBackUp,
  IconFoldDown,
  IconFold,
  IconEye,
} from "@tabler/icons-react";
import type { RenderHeaderMetadataProps } from "@pierre/diffs";
import type { ViewMode } from "@/hooks/use-global-view-mode";

const iconBtn = "h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100";

function ToolbarBtn({
  onClick,
  tooltip,
  className,
  children,
}: {
  onClick: () => void;
  tooltip: string;
  className?: string;
  children: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          variant="ghost"
          size="sm"
          className={cn(iconBtn, className)}
          onClick={onClick}
          aria-label={tooltip}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

interface DiffHeaderToolbarOptions {
  filePath: string;
  diff?: string;
  wordWrap: boolean;
  onToggleWordWrap: () => void;
  viewMode: ViewMode;
  onToggleViewMode: () => void;
  onOpenFile?: (filePath: string) => void;
  onPreviewMarkdown?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  expandUnchanged?: boolean;
  onToggleExpandUnchanged?: () => void;
}

type ToolbarButtonsProps = Omit<DiffHeaderToolbarOptions, "filePath" | "diff"> & {
  resolvedPath: string;
  onCopyDiff: () => void;
  isMarkdownFile: boolean;
};

function DiffHeaderToolbarButtons({
  resolvedPath,
  onCopyDiff,
  onRevert,
  expandUnchanged,
  onToggleExpandUnchanged,
  wordWrap,
  onToggleWordWrap,
  viewMode,
  onToggleViewMode,
  onOpenFile,
  onPreviewMarkdown,
  isMarkdownFile,
}: ToolbarButtonsProps) {
  return (
    <div className="flex items-center gap-1">
      <ToolbarBtn onClick={onCopyDiff} tooltip="Copy diff">
        <IconCopy className="h-3.5 w-3.5" />
      </ToolbarBtn>

      {onRevert && (
        <ToolbarBtn onClick={() => onRevert(resolvedPath)} tooltip="Revert changes">
          <IconArrowBackUp className="h-3.5 w-3.5" />
        </ToolbarBtn>
      )}

      {onToggleExpandUnchanged && (
        <ToolbarBtn
          onClick={onToggleExpandUnchanged}
          tooltip={expandUnchanged ? "Collapse unchanged lines" : "Expand all lines"}
          className={expandUnchanged ? "opacity-100 bg-muted" : undefined}
        >
          {expandUnchanged ? (
            <IconFold className="h-3.5 w-3.5" />
          ) : (
            <IconFoldDown className="h-3.5 w-3.5" />
          )}
        </ToolbarBtn>
      )}

      <ToolbarBtn
        onClick={onToggleWordWrap}
        tooltip="Toggle word wrap"
        className={wordWrap ? "opacity-100 bg-muted" : undefined}
      >
        <IconTextWrap className="h-3.5 w-3.5" />
      </ToolbarBtn>

      <ToolbarBtn
        onClick={onToggleViewMode}
        tooltip={viewMode === "split" ? "Switch to unified view" : "Switch to split view"}
      >
        {viewMode === "split" ? (
          <IconLayoutRows className="h-3.5 w-3.5" />
        ) : (
          <IconLayoutColumns className="h-3.5 w-3.5" />
        )}
      </ToolbarBtn>

      {isMarkdownFile && onPreviewMarkdown && (
        <ToolbarBtn
          onClick={() => onPreviewMarkdown(resolvedPath)}
          tooltip="Preview markdown"
          className={iconBtn}
        >
          <IconEye className="h-3.5 w-3.5" />
        </ToolbarBtn>
      )}

      {onOpenFile && (
        <ToolbarBtn onClick={() => onOpenFile(resolvedPath)} tooltip="Edit">
          <IconPencil className="h-3.5 w-3.5" />
        </ToolbarBtn>
      )}
    </div>
  );
}

function checkIsMarkdown(filePath: string): boolean {
  const ext = filePath.split(".").pop()?.toLowerCase();
  return ext === "md" || ext === "mdx";
}

export function useDiffHeaderToolbar(opts: DiffHeaderToolbarOptions) {
  const {
    filePath,
    diff,
    wordWrap,
    onToggleWordWrap,
    viewMode,
    onToggleViewMode,
    onOpenFile,
    onPreviewMarkdown,
    onRevert,
    expandUnchanged,
    onToggleExpandUnchanged,
  } = opts;

  return useCallback(
    (props: RenderHeaderMetadataProps): ReactNode => {
      const resolvedPath = props.fileDiff?.name || filePath;
      return (
        <DiffHeaderToolbarButtons
          resolvedPath={resolvedPath}
          isMarkdownFile={checkIsMarkdown(resolvedPath)}
          onCopyDiff={() => navigator.clipboard.writeText(diff || "")}
          wordWrap={wordWrap}
          onToggleWordWrap={onToggleWordWrap}
          viewMode={viewMode}
          onToggleViewMode={onToggleViewMode}
          onOpenFile={onOpenFile}
          onPreviewMarkdown={onPreviewMarkdown}
          onRevert={onRevert}
          expandUnchanged={expandUnchanged}
          onToggleExpandUnchanged={onToggleExpandUnchanged}
        />
      );
    },
    [
      filePath,
      diff,
      wordWrap,
      onToggleWordWrap,
      viewMode,
      onToggleViewMode,
      onOpenFile,
      onPreviewMarkdown,
      onRevert,
      expandUnchanged,
      onToggleExpandUnchanged,
    ],
  );
}
