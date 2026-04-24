"use client";

import { useState, useRef, useEffect, useCallback } from "react";
import { IconGitBranch, IconCopy, IconCheck, IconPencil } from "@tabler/icons-react";
import { Input } from "@kandev/ui/input";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { ScrollOnOverflow } from "@kandev/ui/scroll-on-overflow";
import { formatUserHomePath } from "@/lib/utils";
import { useCopyToClipboard } from "@/hooks/use-copy-to-clipboard";

/** Copy button with check/copy icon toggle */
function CopyIconButton({
  copied,
  onClick,
  className,
}: {
  copied: boolean;
  onClick: (e: React.MouseEvent) => void;
  className?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      className={`cursor-pointer ${className ?? ""}`}
      aria-label={copied ? "Copied" : "Copy to clipboard"}
    >
      {copied ? (
        <IconCheck className="h-3 w-3 text-green-500" />
      ) : (
        <IconCopy className="h-3 w-3 text-muted-foreground hover:text-foreground" />
      )}
    </button>
  );
}

/** Copiable path row used in the branch popover */
function PathRow({ label, path }: { label: string; path: string }) {
  const { copied, copy: handleCopy } = useCopyToClipboard();
  return (
    <div className="space-y-0.5">
      <div className="text-xs text-muted-foreground">{label}</div>
      <div className="relative group/row overflow-hidden">
        <div className="text-xs text-muted-foreground bg-muted/50 px-2 py-1.5 pr-9 rounded-sm select-text cursor-text whitespace-nowrap">
          {formatUserHomePath(path)}
        </div>
        <CopyIconButton
          copied={copied}
          onClick={(e) => {
            e.stopPropagation();
            handleCopy(formatUserHomePath(path));
          }}
          className="absolute right-1 top-1 p-1 rounded bg-background/80 backdrop-blur-sm hover:bg-background transition-all shadow-sm"
        />
      </div>
    </div>
  );
}

/** Inline branch rename input */
function BranchRenameInput({
  editValue,
  onEditValueChange,
  onConfirm,
  onCancel,
  isRenaming,
}: {
  editValue: string;
  onEditValueChange: (v: string) => void;
  onConfirm: () => void;
  onCancel: () => void;
  isRenaming: boolean;
}) {
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  const handleKeyDown = useCallback(
    (e: React.KeyboardEvent) => {
      if (e.key === "Enter") {
        e.preventDefault();
        onConfirm();
      } else if (e.key === "Escape") {
        onCancel();
      }
    },
    [onConfirm, onCancel],
  );

  return (
    <div className="flex items-center gap-1.5 rounded-md px-1 h-7 bg-muted/40 min-w-0 max-w-full">
      <IconGitBranch className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
      <Input
        ref={inputRef}
        value={editValue}
        onChange={(e) => onEditValueChange(e.target.value)}
        onKeyDown={handleKeyDown}
        onBlur={onConfirm}
        disabled={isRenaming}
        className="h-5 text-xs px-1 py-0 w-32 bg-background border-primary/50"
      />
    </div>
  );
}

/** Hook for branch rename editing state */
function useBranchRenameEdit(
  displayBranch: string | undefined,
  onRenameBranch: ((newName: string) => Promise<void>) | undefined,
) {
  const [isEditing, setIsEditing] = useState(false);
  const [editValue, setEditValue] = useState(displayBranch ?? "");
  const [isRenaming, setIsRenaming] = useState(false);

  useEffect(() => {
    if (!isEditing) setEditValue(displayBranch ?? "");
  }, [displayBranch, isEditing]);

  const handleStartEdit = useCallback((e: React.MouseEvent) => {
    e.preventDefault();
    e.stopPropagation();
    setIsEditing(true);
  }, []);

  const handleCancelEdit = useCallback(() => {
    setIsEditing(false);
    setEditValue(displayBranch ?? "");
  }, [displayBranch]);

  const handleConfirmRename = useCallback(async () => {
    const trimmed = editValue.trim();
    if (!onRenameBranch || !trimmed || trimmed === displayBranch?.trim() || isRenaming) {
      handleCancelEdit();
      return;
    }
    setIsRenaming(true);
    try {
      await onRenameBranch(trimmed);
      setIsEditing(false);
    } catch {
      // Error is handled by onRenameBranch (shows toast), keep edit mode open
    } finally {
      setIsRenaming(false);
    }
  }, [onRenameBranch, editValue, displayBranch, handleCancelEdit, isRenaming]);

  return {
    isEditing,
    editValue,
    setEditValue,
    isRenaming,
    handleStartEdit,
    handleCancelEdit,
    handleConfirmRename,
  };
}

function BranchPillTrigger({
  displayBranch,
  copiedBranch,
  onCopyBranch,
  onStartEdit,
  canRename,
}: {
  displayBranch: string;
  copiedBranch: boolean;
  onCopyBranch: (text: string) => void;
  onStartEdit: (e: React.MouseEvent) => void;
  canRename: boolean;
}) {
  return (
    <div className="group flex items-center gap-1.5 rounded-md px-2 h-7 bg-muted/40 hover:bg-muted/60 cursor-pointer transition-colors min-w-0 max-w-64">
      <IconGitBranch className="h-3.5 w-3.5 text-muted-foreground shrink-0" />
      <ScrollOnOverflow data-testid="topbar-branch-name" className="text-xs text-muted-foreground min-w-0">
        {displayBranch}
      </ScrollOnOverflow>
      {canRename && (
        <button
          type="button"
          onClick={onStartEdit}
          className="opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer p-0.5 hover:bg-muted rounded"
          aria-label="Rename branch"
          title="Rename branch"
        >
          <IconPencil className="h-3 w-3 text-muted-foreground" />
        </button>
      )}
      <CopyIconButton
        copied={copiedBranch}
        onClick={(e) => {
          e.stopPropagation();
          onCopyBranch(displayBranch);
        }}
        className="opacity-0 group-hover:opacity-100 transition-opacity cursor-pointer ml-0.5"
      />
    </div>
  );
}

export function BranchPathPopover({
  displayBranch,
  repositoryPath,
  worktreePath,
  onRenameBranch,
}: {
  displayBranch?: string;
  repositoryPath?: string | null;
  worktreePath?: string | null;
  onRenameBranch?: (newName: string) => Promise<void>;
}) {
  const { copied: copiedBranch, copy: handleCopyBranch } = useCopyToClipboard();
  const [popoverOpen, setPopoverOpen] = useState(false);
  const [tooltipOpen, setTooltipOpen] = useState(false);
  const recentlyClosedRef = useRef(false);
  const {
    isEditing,
    editValue,
    setEditValue,
    isRenaming,
    handleStartEdit: startEdit,
    handleCancelEdit,
    handleConfirmRename,
  } = useBranchRenameEdit(displayBranch, onRenameBranch);
  const handleStartEdit = useCallback(
    (e: React.MouseEvent) => {
      startEdit(e);
      setPopoverOpen(false);
    },
    [startEdit],
  );

  const handlePopoverOpenChange = useCallback((open: boolean) => {
    setPopoverOpen(open);
    if (!open) {
      recentlyClosedRef.current = true;
      setTooltipOpen(false);
      setTimeout(() => {
        recentlyClosedRef.current = false;
      }, 200);
    }
  }, []);

  const handleTooltipOpenChange = useCallback(
    (open: boolean) => {
      if (open && (popoverOpen || recentlyClosedRef.current)) return;
      setTooltipOpen(open);
    },
    [popoverOpen],
  );

  if (!displayBranch) return null;

  if (isEditing) {
    return (
      <BranchRenameInput
        editValue={editValue}
        onEditValueChange={setEditValue}
        onConfirm={handleConfirmRename}
        onCancel={handleCancelEdit}
        isRenaming={isRenaming}
      />
    );
  }

  return (
    <Popover open={popoverOpen} onOpenChange={handlePopoverOpenChange}>
      <Tooltip open={tooltipOpen} onOpenChange={handleTooltipOpenChange}>
        <TooltipTrigger asChild>
          <PopoverTrigger asChild>
            <BranchPillTrigger
              displayBranch={displayBranch}
              copiedBranch={copiedBranch}
              onCopyBranch={handleCopyBranch}
              onStartEdit={handleStartEdit}
              canRename={!!onRenameBranch}
            />
          </PopoverTrigger>
        </TooltipTrigger>
        <TooltipContent side="right">
          {onRenameBranch ? "Click to see paths, or click pencil to rename" : "Click to see paths"}
        </TooltipContent>
      </Tooltip>
      <PopoverContent side="bottom" sideOffset={5} className="p-0 w-auto max-w-[600px] gap-1">
        <div className="px-2 pt-1 pb-2 space-y-1.5">
          {repositoryPath && <PathRow label="Repository" path={repositoryPath} />}
          {worktreePath && <PathRow label="Worktree" path={worktreePath} />}
        </div>
      </PopoverContent>
    </Popover>
  );
}
