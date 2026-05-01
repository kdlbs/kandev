"use client";

import { useCallback } from "react";
import {
  IconArrowBackUp,
  IconCopy,
  IconEye,
  IconFold,
  IconFoldDown,
  IconLayoutColumns,
  IconLayoutRows,
  IconPencil,
  IconTextWrap,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { FileActionsDropdown } from "@/components/editors/file-actions-dropdown";
import { useGlobalViewMode } from "@/hooks/use-global-view-mode";

const iconBtn = "h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100";
const iconBtnActive = "h-6 w-6 p-0 cursor-pointer opacity-100 bg-muted";

function isMarkdownPath(filePath: string): boolean {
  const ext = filePath.split(".").pop()?.toLowerCase();
  return ext === "md" || ext === "mdx";
}

export type FileDiffToolbarProps = {
  diff: string;
  filePath: string;
  sessionId: string;
  source: string;
  wordWrap: boolean;
  expandUnchanged: boolean;
  onDiscard: () => void;
  onOpenFile?: (filePath: string) => void;
  onPreviewMarkdown?: (filePath: string) => void;
  onToggleExpandUnchanged: () => void;
  onToggleWordWrap: () => void;
};

function ToolbarIconBtn({
  onClick,
  tooltip,
  active,
  children,
  className,
}: {
  onClick: () => void;
  tooltip: string;
  active?: boolean;
  children: React.ReactNode;
  className?: string;
}) {
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          type="button"
          aria-label={tooltip}
          aria-pressed={active}
          variant="ghost"
          size="sm"
          className={className ?? (active ? iconBtnActive : iconBtn)}
          onClick={onClick}
        >
          {children}
        </Button>
      </TooltipTrigger>
      <TooltipContent>{tooltip}</TooltipContent>
    </Tooltip>
  );
}

export function FileDiffToolbar(props: FileDiffToolbarProps) {
  const {
    diff,
    filePath,
    sessionId,
    source,
    wordWrap,
    expandUnchanged,
    onDiscard,
    onOpenFile,
    onPreviewMarkdown,
    onToggleExpandUnchanged,
    onToggleWordWrap,
  } = props;
  const [globalViewMode, setGlobalViewMode] = useGlobalViewMode();
  const handleCopyDiff = useCallback(() => {
    navigator.clipboard.writeText(diff || "");
  }, [diff]);
  const handleToggleViewMode = useCallback(
    () => setGlobalViewMode(globalViewMode === "split" ? "unified" : "split"),
    [globalViewMode, setGlobalViewMode],
  );
  return (
    <div className="flex items-center gap-0.5">
      <ToolbarIconBtn onClick={handleCopyDiff} tooltip="Copy diff">
        <IconCopy className="h-3.5 w-3.5" />
      </ToolbarIconBtn>
      <ToolbarIconBtn
        onClick={onToggleExpandUnchanged}
        tooltip={expandUnchanged ? "Collapse unchanged" : "Expand all"}
        active={expandUnchanged}
      >
        {expandUnchanged ? (
          <IconFold className="h-3.5 w-3.5" />
        ) : (
          <IconFoldDown className="h-3.5 w-3.5" />
        )}
      </ToolbarIconBtn>
      <ToolbarIconBtn onClick={onToggleWordWrap} tooltip="Toggle word wrap" active={wordWrap}>
        <IconTextWrap className="h-3.5 w-3.5" />
      </ToolbarIconBtn>
      <ToolbarIconBtn
        onClick={handleToggleViewMode}
        tooltip={globalViewMode === "split" ? "Switch to unified view" : "Switch to split view"}
      >
        {globalViewMode === "split" ? (
          <IconLayoutRows className="h-3.5 w-3.5" />
        ) : (
          <IconLayoutColumns className="h-3.5 w-3.5" />
        )}
      </ToolbarIconBtn>
      {onPreviewMarkdown && isMarkdownPath(filePath) && (
        <ToolbarIconBtn onClick={() => onPreviewMarkdown(filePath)} tooltip="Preview markdown">
          <IconEye className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
      {onOpenFile && (
        <ToolbarIconBtn onClick={() => onOpenFile(filePath)} tooltip="Edit">
          <IconPencil className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
      <FileActionsDropdown filePath={filePath} sessionId={sessionId} size="xs" />
      {source === "uncommitted" && (
        <ToolbarIconBtn
          onClick={onDiscard}
          tooltip="Revert changes"
          className="h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100 hover:text-destructive"
        >
          <IconArrowBackUp className="h-3.5 w-3.5" />
        </ToolbarIconBtn>
      )}
    </div>
  );
}
