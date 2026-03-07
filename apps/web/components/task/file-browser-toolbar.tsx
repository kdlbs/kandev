"use client";

import type { ReactNode } from "react";
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

function ToolbarButton({
  onClick,
  label,
  icon,
}: {
  onClick: () => void;
  label: string;
  icon: ReactNode;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          className="text-muted-foreground hover:bg-muted hover:text-foreground rounded p-1 cursor-pointer"
          aria-label={label}
          onClick={onClick}
        >
          {icon}
        </button>
      </TooltipTrigger>
      <TooltipContent>{label}</TooltipContent>
    </Tooltip>
  );
}

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
                aria-label="Copy workspace path"
                onClick={() => {
                  if (fullPath) void onCopyPath(fullPath);
                }}
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
            <ToolbarButton
              onClick={onStartCreate}
              label="New file"
              icon={<IconPlus className="h-3.5 w-3.5" />}
            />
          )}
          <ToolbarButton
            onClick={onOpenFolder}
            label="Open workspace folder"
            icon={<IconFolderShare className="h-3.5 w-3.5" />}
          />
          <ToolbarButton
            onClick={onStartSearch}
            label="Search files"
            icon={<IconSearch className="h-3.5 w-3.5" />}
          />
          {expandedPathsSize > 0 && (
            <ToolbarButton
              onClick={onCollapseAll}
              label="Collapse all"
              icon={<IconListTree className="h-3.5 w-3.5" />}
            />
          )}
        </>
      }
    />
  );
}
