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
import type { FileDiffData } from "@/lib/diff/types";
import type { ViewMode } from "@/hooks/use-global-view-mode";

interface DiffViewerToolbarProps {
  data: FileDiffData;
  foldUnchanged: boolean;
  setFoldUnchanged: (v: boolean) => void;
  wordWrap: boolean;
  setWordWrap: (v: boolean) => void;
  globalViewMode: ViewMode;
  setGlobalViewMode: (v: ViewMode) => void;
  onCopyDiff: () => void;
  onOpenFile?: (filePath: string) => void;
  onRevert?: (filePath: string) => void;
}

const iconBtn = "h-6 w-6 p-0 cursor-pointer opacity-60 hover:opacity-100";

type DiffViewerActionsProps = Omit<DiffViewerToolbarProps, "data"> & { filePath: string };

function DiffViewerActions({
  filePath,
  foldUnchanged,
  setFoldUnchanged,
  wordWrap,
  setWordWrap,
  globalViewMode,
  setGlobalViewMode,
  onCopyDiff,
  onOpenFile,
  onRevert,
}: DiffViewerActionsProps) {
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
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={cn(iconBtn, foldUnchanged && "opacity-100 bg-muted")}
            onClick={() => setFoldUnchanged(!foldUnchanged)}
          >
            {foldUnchanged ? (
              <IconFoldDown className="h-3.5 w-3.5" />
            ) : (
              <IconFold className="h-3.5 w-3.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>{foldUnchanged ? "Show all lines" : "Fold unchanged lines"}</TooltipContent>
      </Tooltip>
      {onRevert && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="sm" className={iconBtn} onClick={() => onRevert(filePath)}>
              <IconArrowBackUp className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Revert changes</TooltipContent>
        </Tooltip>
      )}
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={cn(iconBtn, wordWrap && "opacity-100 bg-muted")}
            onClick={() => setWordWrap(!wordWrap)}
          >
            <IconTextWrap className="h-3.5 w-3.5" />
          </Button>
        </TooltipTrigger>
        <TooltipContent>Toggle word wrap</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <Button
            variant="ghost"
            size="sm"
            className={cn(iconBtn)}
            onClick={() => setGlobalViewMode(globalViewMode === "split" ? "unified" : "split")}
          >
            {globalViewMode === "split" ? (
              <IconLayoutRows className="h-3.5 w-3.5" />
            ) : (
              <IconLayoutColumns className="h-3.5 w-3.5" />
            )}
          </Button>
        </TooltipTrigger>
        <TooltipContent>
          {globalViewMode === "split" ? "Switch to unified view" : "Switch to split view"}
        </TooltipContent>
      </Tooltip>
      {onOpenFile && (
        <Tooltip>
          <TooltipTrigger asChild>
            <Button variant="ghost" size="sm" className={iconBtn} onClick={() => onOpenFile(filePath)}>
              <IconPencil className="h-3.5 w-3.5" />
            </Button>
          </TooltipTrigger>
          <TooltipContent>Edit</TooltipContent>
        </Tooltip>
      )}
    </div>
  );
}

export function DiffViewerToolbar({
  data,
  foldUnchanged,
  setFoldUnchanged,
  wordWrap,
  setWordWrap,
  globalViewMode,
  setGlobalViewMode,
  onCopyDiff,
  onOpenFile,
  onRevert,
}: DiffViewerToolbarProps) {
  return (
    <div className="flex items-center justify-between px-3 py-1.5 border-b border-border/50 bg-card/50 rounded-t-md text-xs text-muted-foreground">
      <span className="font-mono truncate">{data.filePath}</span>
      <DiffViewerActions
        filePath={data.filePath}
        foldUnchanged={foldUnchanged}
        setFoldUnchanged={setFoldUnchanged}
        wordWrap={wordWrap}
        setWordWrap={setWordWrap}
        globalViewMode={globalViewMode}
        setGlobalViewMode={setGlobalViewMode}
        onCopyDiff={onCopyDiff}
        onOpenFile={onOpenFile}
        onRevert={onRevert}
      />
    </div>
  );
}
