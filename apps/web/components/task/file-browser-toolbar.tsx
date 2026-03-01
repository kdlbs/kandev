"use client";

import {
  IconSearch,
  IconListTree,
  IconFolderShare,
  IconFolderOpen,
  IconCopy,
  IconCheck,
  IconPlus,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { cn } from "@/lib/utils";
import { PanelHeaderBarSplit } from "./panel-primitives";

type FileBrowserToolbarProps = {
  displayPath: string;
  fullPath: string;
  copied: boolean;
  expandedPathsSize: number;
  onCopyPath: (text: string) => void;
  onStartCreate?: () => void;
  onOpenFolder: () => void;
  onStartSearch: () => void;
  onCollapseAll: () => void;
  showCreateButton: boolean;
};

export function FileBrowserToolbar({
  displayPath,
  fullPath,
  copied,
  expandedPathsSize,
  onCopyPath,
  onStartCreate,
  onOpenFolder,
  onStartSearch,
  onCollapseAll,
  showCreateButton,
}: FileBrowserToolbarProps) {
  return (
    <PanelHeaderBarSplit
      className="group/header"
      left={
        <>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="relative shrink-0 cursor-pointer"
                onClick={() => { if (fullPath) void onCopyPath(fullPath); }}
              >
                <IconFolderOpen
                  className={cn(
                    "h-3.5 w-3.5 text-muted-foreground transition-opacity",
                    copied ? "opacity-0" : "group-hover/header:opacity-0",
                  )}
                />
                {copied ? (
                  <IconCheck className="absolute inset-0 h-3.5 w-3.5 text-green-600/70" />
                ) : (
                  <IconCopy className="absolute inset-0 h-3.5 w-3.5 text-muted-foreground opacity-0 group-hover/header:opacity-100 hover:text-foreground transition-opacity" />
                )}
              </button>
            </TooltipTrigger>
            <TooltipContent>Copy workspace path</TooltipContent>
          </Tooltip>
          <span className="min-w-0 truncate text-xs font-medium text-muted-foreground">
            {displayPath}
          </span>
        </>
      }
      right={
        <>
          {showCreateButton && onStartCreate && (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                  onClick={onStartCreate}
                >
                  <IconPlus className="h-3.5 w-3.5" />
                </button>
              </TooltipTrigger>
              <TooltipContent>New file</TooltipContent>
            </Tooltip>
          )}
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                onClick={onOpenFolder}
              >
                <IconFolderShare className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent>Open workspace folder</TooltipContent>
          </Tooltip>
          <Tooltip>
            <TooltipTrigger asChild>
              <button
                className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                onClick={onStartSearch}
              >
                <IconSearch className="h-3.5 w-3.5" />
              </button>
            </TooltipTrigger>
            <TooltipContent>Search files</TooltipContent>
          </Tooltip>
          {expandedPathsSize > 0 && (
            <Tooltip>
              <TooltipTrigger asChild>
                <button
                  className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
                  onClick={onCollapseAll}
                >
                  <IconListTree className="h-3.5 w-3.5" />
                </button>
              </TooltipTrigger>
              <TooltipContent>Collapse all</TooltipContent>
            </Tooltip>
          )}
        </>
      }
    />
  );
}

