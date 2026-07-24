"use client";

import { useCallback, useEffect, useRef, type ReactNode, type Ref, type RefObject } from "react";
import {
  IconSearch,
  IconListTree,
  IconFolderOpen,
  IconCopy,
  IconCheck,
  IconPlus,
  IconDots,
} from "@tabler/icons-react";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Skeleton } from "@kandev/ui/skeleton";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
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
  onAddSources?: (opener: HTMLButtonElement) => void;
  addSourcesButtonRef?: Ref<HTMLButtonElement>;
  addSourcesDisabledReason?: string;
};

function CopyWorkspacePathButton({
  fullPath,
  copied,
  onCopyPath,
}: Pick<FileBrowserToolbarProps, "fullPath" | "copied" | "onCopyPath">) {
  return (
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
  );
}

function useMobileDrawerFocusRestoration(triggerRef: RefObject<HTMLButtonElement | null>) {
  const { isMobile } = useResponsiveBreakpoint();
  const restoreAfterDrawerCloseRef = useRef(false);

  useEffect(() => {
    const restoreFocus = (event: AnimationEvent) => {
      if (!restoreAfterDrawerCloseRef.current) return;
      const drawer = event.target;
      if (
        !(drawer instanceof HTMLElement) ||
        drawer.dataset.testid !== "add-workspace-sources-drawer" ||
        drawer.dataset.state !== "closed"
      )
        return;
      restoreAfterDrawerCloseRef.current = false;
      requestAnimationFrame(() => {
        if (triggerRef.current?.isConnected && !triggerRef.current.disabled) {
          triggerRef.current.focus();
        }
      });
    };
    document.addEventListener("animationend", restoreFocus, true);
    return () => document.removeEventListener("animationend", restoreFocus, true);
  }, [triggerRef]);

  return useCallback(() => {
    restoreAfterDrawerCloseRef.current = isMobile;
  }, [isMobile]);
}

function WorkspaceActionsMenu({
  onAddSources,
  onOpenFolder,
  addSourcesButtonRef,
  addSourcesDisabledReason,
}: Pick<
  FileBrowserToolbarProps,
  "onAddSources" | "onOpenFolder" | "addSourcesButtonRef" | "addSourcesDisabledReason"
>) {
  const triggerRef = useRef<HTMLButtonElement>(null);
  const openSourcesAfterCloseRef = useRef(false);
  const restoreMobileFocusAfterDrawerClose = useMobileDrawerFocusRestoration(triggerRef);
  const setTriggerRef = useCallback(
    (node: HTMLButtonElement | null) => {
      triggerRef.current = node;
      if (typeof addSourcesButtonRef === "function") {
        addSourcesButtonRef(node);
      } else if (addSourcesButtonRef) {
        (addSourcesButtonRef as { current: HTMLButtonElement | null }).current = node;
      }
    },
    [addSourcesButtonRef],
  );
  const disabledReason =
    addSourcesDisabledReason ??
    (onAddSources ? undefined : "This task needs a repository before sources can be added.");

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <button
              ref={setTriggerRef}
              type="button"
              aria-label="Workspace actions"
              data-testid="files-workspace-actions"
              className="inline-flex size-11 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer sm:size-8"
            >
              <IconDots className="h-4 w-4" />
            </button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>Workspace actions</TooltipContent>
      </Tooltip>
      <DropdownMenuContent
        align="end"
        className="w-72"
        onCloseAutoFocus={(event) => {
          if (!openSourcesAfterCloseRef.current) return;
          event.preventDefault();
          openSourcesAfterCloseRef.current = false;
          if (triggerRef.current && onAddSources) onAddSources(triggerRef.current);
        }}
      >
        <DropdownMenuItem
          disabled={Boolean(disabledReason)}
          className="min-h-11 cursor-pointer gap-2 sm:min-h-8"
          onSelect={() => {
            openSourcesAfterCloseRef.current = true;
            restoreMobileFocusAfterDrawerClose();
          }}
        >
          <IconPlus className="h-3.5 w-3.5" />
          <span className="min-w-0">
            <span className="block">Add Repositories to workspace</span>
            {disabledReason && (
              <span className="block text-[10px] text-muted-foreground normal-case">
                {disabledReason}
              </span>
            )}
          </span>
        </DropdownMenuItem>
        <DropdownMenuItem
          className="min-h-11 cursor-pointer gap-2 sm:min-h-8"
          onSelect={onOpenFolder}
        >
          <IconFolderOpen className="h-3.5 w-3.5" />
          Open workspace folder
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

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
  onAddSources,
  addSourcesButtonRef,
  addSourcesDisabledReason,
}: FileBrowserToolbarProps) {
  return (
    <PanelHeaderBarSplit
      className="group/header"
      left={
        <>
          <CopyWorkspacePathButton fullPath={fullPath} copied={copied} onCopyPath={onCopyPath} />
          {displayPath ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span className="min-w-0 truncate text-xs font-medium text-muted-foreground">
                  {displayPath}
                </span>
              </TooltipTrigger>
              <TooltipContent className="break-all max-w-[min(420px,90vw)]">
                {fullPath || displayPath}
              </TooltipContent>
            </Tooltip>
          ) : (
            <Skeleton className="h-3 w-24" />
          )}
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
          <WorkspaceActionsMenu
            onAddSources={onAddSources}
            onOpenFolder={onOpenFolder}
            addSourcesButtonRef={addSourcesButtonRef}
            addSourcesDisabledReason={addSourcesDisabledReason}
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
