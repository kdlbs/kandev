"use client";

import {
  IconArrowBackUp,
  IconPlus,
  IconMinus,
  IconCheck,
  IconLoader2,
  IconPencil,
} from "@tabler/icons-react";

import { Button } from "@kandev/ui/button";
import { cn } from "@/lib/utils";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import { LineStat } from "@/components/diff-stat";
import { FileStatusIcon } from "@/components/shared/file-status-icon";
import { FileIcon } from "@/components/ui/file-icon";
import { getFileCategory } from "@/lib/utils/file-types";
import type { ChangedFile } from "./changes-panel-helpers";
import type { OpenDiffOptions } from "./changes-diff-target";
import { useTranslation } from "react-i18next";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";

const COARSE_POINTER_TARGET_CLASS = "min-h-11 min-w-11";

const splitPath = (path: string) => {
  const lastSlash = path.lastIndexOf("/");
  if (lastSlash === -1) return { folder: "", file: path };
  return {
    folder: path.slice(0, lastSlash),
    file: path.slice(lastSlash + 1),
  };
};

type FileRowProps = {
  file: ChangedFile;
  isPending: boolean;
  isSelected?: boolean;
  /** True when this file's diff/editor tab is the currently active dockview panel. */
  isActive?: boolean;
  onSelect?: (path: string, e: React.MouseEvent) => boolean;
  onOpenDiff: (path: string, options?: OpenDiffOptions) => void;
  // Multi-repo: handlers receive the file's repository_name so the per-file
  // staging op routes to the right git repo. Same-named files (e.g. README.md
  // in two repos) cannot be disambiguated by path alone.
  onStage: (path: string, repo?: string) => void;
  onUnstage: (path: string, repo?: string) => void;
  onDiscard: (path: string, repo?: string, anchor?: HTMLElement) => void;
  onEditFile: (path: string, repo?: string) => void;
  /**
   * Tree mode: skip the folder prefix, swap the left-side stage button for a
   * filetype icon, and surface the stage action only on row hover (VS Code-
   * style). The Unstaged / Staged section split keeps the staged state
   * visually obvious even without the always-on left chip.
   */
  treeMode?: boolean;
  /** Tree mode: left padding in pixels driven by depth. */
  indentPx?: number;
};

export function FileRow({
  file,
  isPending,
  isSelected,
  isActive,
  onSelect,
  onOpenDiff,
  onStage,
  onUnstage,
  onDiscard,
  onEditFile,
  treeMode,
  indentPx,
}: FileRowProps) {
  const { isFinePointer } = useResponsiveBreakpoint();
  const { folder, file: name } = splitPath(file.path);
  const showFolder = !treeMode && folder;

  const handleClick = (e: React.MouseEvent) => {
    if (e.button === 2) return;
    const consumed = onSelect?.(file.path, e);
    if (!consumed) {
      if (getFileCategory(file.path) === "image") {
        onEditFile(file.path, file.repositoryName);
        return;
      }
      onOpenDiff(file.path, {
        source: "uncommitted",
        repositoryName: file.repositoryName,
        ...(file.changeLayer ? { changeLayer: file.changeLayer } : {}),
      });
    }
  };

  return (
    <li
      data-testid={`file-row-${file.path.replace(/[/\\]/g, "-")}`}
      data-changes-file={file.path}
      data-selected={isSelected ? "true" : "false"}
      data-active={isActive ? "true" : "false"}
      className={cn(
        "group flex items-center justify-between gap-2 rounded-md border border-transparent px-2 py-1.5 -mx-1 text-sm cursor-pointer",
        "md:px-1 md:py-0.5",
        isSelected || isActive
          ? "border-primary/50 bg-card text-foreground hover:bg-muted/70"
          : "hover:bg-muted/60",
      )}
      onClick={handleClick}
    >
      <div
        className="flex items-center gap-2 min-w-0"
        style={indentPx ? { paddingLeft: indentPx } : undefined}
      >
        {treeMode ? (
          <TreeModeFileActionSlot
            name={name}
            isPending={isPending}
            isFinePointer={isFinePointer}
            staged={file.staged}
            path={file.path}
            repo={file.repositoryName}
            onStage={onStage}
            onUnstage={onUnstage}
          />
        ) : (
          <StageButton
            isPending={isPending}
            staged={file.staged}
            touchSized={!isFinePointer}
            path={file.path}
            repo={file.repositoryName}
            onStage={onStage}
            onUnstage={onUnstage}
          />
        )}
        <button type="button" className="min-w-0 text-left cursor-pointer" title={file.path}>
          <p className="flex text-foreground text-xs min-w-0">
            {showFolder && (
              <span className="text-foreground/60 truncate min-w-0 [flex-shrink:9999]">
                {folder}/
              </span>
            )}
            <span className="font-medium text-foreground truncate min-w-0">{name}</span>
          </p>
        </button>
      </div>
      <div className="grid items-center shrink-0 [&>*]:col-start-1 [&>*]:row-start-1">
        <FileRowStats file={file} isFinePointer={isFinePointer} />
        <FileRowActions
          path={file.path}
          repo={file.repositoryName}
          isFinePointer={isFinePointer}
          onDiscard={onDiscard}
          onEditFile={onEditFile}
        />
      </div>
    </li>
  );
}

function FileRowStats({ file, isFinePointer }: { file: ChangedFile; isFinePointer: boolean }) {
  return (
    <div
      className={cn(
        "flex items-center gap-2 justify-end transition-opacity pointer-events-none",
        isFinePointer ? "group-hover:opacity-0" : "opacity-0",
      )}
    >
      <LineStat added={file.plus} removed={file.minus} />
      <FileStatusIcon status={file.status} oldPath={file.oldPath} />
    </div>
  );
}

function TreeModeFileActionSlot({
  name,
  isPending,
  isFinePointer,
  staged,
  path,
  repo,
  onStage,
  onUnstage,
}: {
  name: string;
  isPending: boolean;
  isFinePointer: boolean;
  staged: boolean;
  path: string;
  repo?: string;
  onStage: (path: string, repo?: string) => void;
  onUnstage: (path: string, repo?: string) => void;
}) {
  return (
    <div
      data-testid="file-row-icon-action-slot"
      className={cn(
        "grid shrink-0 items-center justify-center [&>*]:col-start-1 [&>*]:row-start-1",
        isFinePointer ? "size-4" : "size-11",
      )}
    >
      {isFinePointer && (
        <FileIcon
          fileName={name}
          className="size-4 transition-opacity group-hover:opacity-0 pointer-events-none"
        />
      )}
      <div
        className={cn(
          isFinePointer &&
            "opacity-0 transition-opacity pointer-events-none group-hover:opacity-100 group-hover:pointer-events-auto",
        )}
      >
        <StageButton
          isPending={isPending}
          staged={staged}
          touchSized={!isFinePointer}
          path={path}
          repo={repo}
          onStage={onStage}
          onUnstage={onUnstage}
        />
      </div>
    </div>
  );
}

function StageButton({
  isPending,
  staged,
  touchSized,
  path,
  repo,
  onStage,
  onUnstage,
}: {
  isPending: boolean;
  staged: boolean;
  touchSized?: boolean;
  path: string;
  repo?: string;
  onStage: (path: string, repo?: string) => void;
  onUnstage: (path: string, repo?: string) => void;
}) {
  const { t } = useTranslation();
  if (isPending) {
    return (
      <div
        className={cn(
          "flex-shrink-0 flex items-center justify-center size-4",
          touchSized && COARSE_POINTER_TARGET_CLASS,
        )}
      >
        <IconLoader2 className="h-3 w-3 animate-spin text-muted-foreground" />
      </div>
    );
  }
  if (staged) {
    return (
      <button
        type="button"
        title={t("task:unstageFile")}
        className={cn(
          "group/unstage flex-shrink-0 flex items-center justify-center size-4 rounded bg-emerald-500/20 text-emerald-600 hover:bg-rose-500/20 hover:text-rose-600 cursor-pointer",
          touchSized && COARSE_POINTER_TARGET_CLASS,
        )}
        onClick={(e) => {
          e.stopPropagation();
          onUnstage(path, repo);
        }}
      >
        <IconCheck className="h-3 w-3 group-hover/unstage:hidden" />
        <IconMinus className="h-2.5 w-2.5 hidden group-hover/unstage:block" />
      </button>
    );
  }
  return (
    <button
      type="button"
      title={t("task:stageFile")}
      className={cn(
        "flex-shrink-0 flex items-center justify-center size-4 rounded border border-dashed border-muted-foreground/50 text-muted-foreground hover:border-emerald-500 hover:text-emerald-500 hover:bg-emerald-500/10 cursor-pointer",
        touchSized && COARSE_POINTER_TARGET_CLASS,
      )}
      onClick={(e) => {
        e.stopPropagation();
        onStage(path, repo);
      }}
    >
      <IconPlus className="h-2.5 w-2.5" />
    </button>
  );
}

function FileRowActions({
  path,
  repo,
  isFinePointer,
  onDiscard,
  onEditFile,
}: {
  path: string;
  repo?: string;
  isFinePointer: boolean;
  onDiscard: (path: string, repo?: string, anchor?: HTMLElement) => void;
  onEditFile: (path: string, repo?: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      data-testid="file-row-hover-actions"
      className={cn(
        "flex items-center gap-1 justify-end transition-opacity",
        isFinePointer
          ? "opacity-0 group-hover:opacity-100 pointer-events-none group-hover:pointer-events-auto"
          : "opacity-100 pointer-events-auto",
      )}
    >
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={t("task:discardChanges2")}
            className={cn(
              "text-muted-foreground hover:text-foreground cursor-pointer",
              !isFinePointer && COARSE_POINTER_TARGET_CLASS,
            )}
            onClick={(e) => {
              e.stopPropagation();
              onDiscard(path, repo, e.currentTarget);
            }}
          >
            <IconArrowBackUp className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent>{t("task:discardChanges2")}</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            aria-label={t("common:edit")}
            className={cn(
              "text-muted-foreground hover:text-foreground cursor-pointer",
              !isFinePointer && COARSE_POINTER_TARGET_CLASS,
            )}
            onClick={(e) => {
              e.stopPropagation();
              onEditFile(path, repo);
            }}
          >
            <IconPencil className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent>{t("common:edit")}</TooltipContent>
      </Tooltip>
    </div>
  );
}

// --- Bulk action components (used by FileListSection) ---

export function DefaultActionButtons({
  actionLabel,
  isActionLoading,
  onAction,
  secondaryActionLabel,
  isSecondaryActionLoading,
  onSecondaryAction,
}: {
  actionLabel: string;
  isActionLoading?: boolean;
  onAction: () => void;
  secondaryActionLabel?: string;
  isSecondaryActionLoading?: boolean;
  onSecondaryAction?: () => void;
}) {
  return (
    <>
      <Button
        size="sm"
        variant="outline"
        className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
        onClick={onAction}
        disabled={isActionLoading}
      >
        {isActionLoading && <IconLoader2 className="h-3 w-3 animate-spin" />}
        {actionLabel}
      </Button>
      {onSecondaryAction && secondaryActionLabel && (
        <Button
          size="sm"
          variant="outline"
          className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
          onClick={onSecondaryAction}
          disabled={isSecondaryActionLoading}
        >
          {isSecondaryActionLoading && <IconLoader2 className="h-3 w-3 animate-spin" />}
          {secondaryActionLabel}
        </Button>
      )}
    </>
  );
}

export function BulkActionBar({
  variant,
  selectionCount,
  selectedPaths,
  onBulkStage,
  onBulkUnstage,
  onBulkDiscard,
}: {
  variant: "unstaged" | "staged";
  selectionCount: number;
  selectedPaths: Set<string>;
  onBulkStage?: (paths: string[]) => void;
  onBulkUnstage?: (paths: string[]) => void;
  onBulkDiscard?: (paths: string[], anchor?: HTMLElement) => void;
}) {
  const { t } = useTranslation();
  const paths = [...selectedPaths];

  return (
    <div data-testid={`bulk-actions-${variant}`} className="flex items-center gap-1.5">
      <span className="text-[11px] text-muted-foreground">{selectionCount} selected</span>
      {variant === "unstaged" && onBulkStage && (
        <Button
          data-testid="bulk-stage"
          size="sm"
          variant="outline"
          className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
          onClick={() => onBulkStage(paths)}
        >
          {t("task:stageCount", { selectionCount })}
        </Button>
      )}
      {variant === "staged" && onBulkUnstage && (
        <Button
          data-testid={`bulk-unstage-${variant}`}
          size="sm"
          variant="outline"
          className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer"
          onClick={() => onBulkUnstage(paths)}
        >
          {t("task:unstageCount", { selectionCount })}
        </Button>
      )}
      {onBulkDiscard && (
        <Button
          data-testid={`bulk-discard-${variant}`}
          size="sm"
          variant="outline"
          className="h-6 text-[11px] px-2.5 gap-1 cursor-pointer text-destructive hover:text-destructive"
          onClick={(e) => onBulkDiscard(paths, e.currentTarget)}
        >
          {t("task:discardCount", { selectionCount })}
        </Button>
      )}
    </div>
  );
}
