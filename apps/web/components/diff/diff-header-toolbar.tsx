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
} from "@tabler/icons-react";
import type { RenderHeaderMetadataProps } from "@pierre/diffs";
import type { ViewMode } from "@/hooks/use-global-view-mode";

const iconBtn = "h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100";

interface DiffHeaderToolbarOptions {
  filePath: string;
  diff?: string;
  wordWrap: boolean;
  onToggleWordWrap: () => void;
  viewMode: ViewMode;
  onToggleViewMode: () => void;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
  expandUnchanged?: boolean;
  onToggleExpandUnchanged?: () => void;
}

type ToolbarButtonsProps = Omit<DiffHeaderToolbarOptions, "filePath" | "diff"> & {
  resolvedPath: string;
  onCopyDiff: () => void;
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
}: ToolbarButtonsProps) {
  return (
    <div className="flex items-center gap-1">
      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="sm" className={iconBtn} onClick={onCopyDiff}>
            <IconCopy className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Copy diff</TooltipContent>
      </Tooltip>

      {onRevert && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="sm" className={iconBtn} onClick={() => onRevert(resolvedPath)}>
              <IconArrowBackUp className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Revert changes</TooltipContent>
        </Tooltip>
      )}

      {onToggleExpandUnchanged && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button
              variant="ghost"
              size="sm"
              className={cn(iconBtn, expandUnchanged && "opacity-100 bg-muted")}
              onClick={onToggleExpandUnchanged}
            >
              {expandUnchanged ? <IconFold className="h-3.5 w-3.5" /> : <IconFoldDown className="h-3.5 w-3.5" />}
            </Button>
          </TooltipTrigger>
          <TooltipContent>{expandUnchanged ? "Collapse unchanged lines" : "Expand all lines"}</TooltipContent>
        </Tooltip>
      )}

      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={cn(iconBtn, wordWrap && "opacity-100 bg-muted")}
            onClick={onToggleWordWrap}
          >
            <IconTextWrap className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Toggle word wrap</TooltipContent>
      </Tooltip>

      <Tooltip>
        <TooltipTrigger asChild>
          <Button variant="ghost" size="sm" className={iconBtn} onClick={onToggleViewMode}>
            {viewMode === "split" ? <IconLayoutRows className="h-3.5 w-3.5" /> : <IconLayoutColumns className="h-3.5 w-3.5" />}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{viewMode === "split" ? "Switch to unified view" : "Switch to split view"}</TooltipContent>
      </Tooltip>

      {onOpenFile && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="sm" className={iconBtn} onClick={() => onOpenFile(resolvedPath)}>
              <IconPencil className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Edit</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

export function useDiffHeaderToolbar(opts: DiffHeaderToolbarOptions) {
  const {
    filePath, diff, wordWrap, onToggleWordWrap, viewMode, onToggleViewMode,
    onOpenFile, onRevert, expandUnchanged, onToggleExpandUnchanged,
  } = opts;

  return useCallback(
    (props: RenderHeaderMetadataProps): ReactNode => {
      const resolvedPath = props.fileDiff?.name || filePath;
      return (
        <DiffHeaderToolbarButtons
          resolvedPath={resolvedPath}
          onCopyDiff={() => navigator.clipboard.writeText(diff || "")}
          wordWrap={wordWrap}
          onToggleWordWrap={onToggleWordWrap}
          viewMode={viewMode}
          onToggleViewMode={onToggleViewMode}
          onOpenFile={onOpenFile}
          onRevert={onRevert}
          expandUnchanged={expandUnchanged}
          onToggleExpandUnchanged={onToggleExpandUnchanged}
        />
      );
    },
    [filePath, diff, wordWrap, onToggleWordWrap, viewMode, onToggleViewMode,
      onOpenFile, onRevert, expandUnchanged, onToggleExpandUnchanged],
  );
}
