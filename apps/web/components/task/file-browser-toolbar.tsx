"use client";

import { useCallback, useEffect, useRef, type ReactNode, type Ref, type RefObject } from "react";
import {
  IconSearch,
  IconListTree,
  IconFolderOpen,
  IconCopy,
  IconCheck,
  IconFilePlus,
  IconFolderUp,
  IconPlus,
  IconUpload,
  IconDots,
  IconLoader2,
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
import { useTranslation } from "react-i18next";

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
          className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:w-11 [@media(pointer:coarse)]:w-11"
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
  onOpenFolder: (opener?: HTMLButtonElement) => void;
  isOpeningFolder?: boolean;
  isFolderDisabled?: boolean;
  onStartSearch: () => void;
  onCollapseAll: () => void;
  showCreateButton: boolean;
  onUploadFiles?: (mode: "files" | "folder") => void;
  onAddSources?: (opener: HTMLButtonElement) => void;
  addSourcesButtonRef?: Ref<HTMLButtonElement>;
  addSourcesDisabledReason?: string;
};

function CopyWorkspacePathButton({
  fullPath,
  copied,
  onCopyPath,
}: Pick<FileBrowserToolbarProps, "fullPath" | "copied" | "onCopyPath">) {
  const { t } = useTranslation();
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          className="relative inline-flex h-6 w-6 shrink-0 items-center justify-center cursor-pointer max-md:h-11 [@media(pointer:coarse)]:h-11 max-md:w-11 [@media(pointer:coarse)]:w-11"
          aria-label={t("task:copyWorkspacePath")}
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
      <TooltipContent>{t("task:copyWorkspacePath")}</TooltipContent>
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

function CreateMenu({
  onStartCreate,
  onUploadFiles,
}: {
  onStartCreate: () => void;
  onUploadFiles?: (mode: "files" | "folder") => void;
}) {
  const { t } = useTranslation();
  const createAfterCloseRef = useRef(false);
  const handleCreateSelect = useCallback(() => {
    createAfterCloseRef.current = true;
  }, []);
  const handleCloseAutoFocus = useCallback(
    (event: Event) => {
      if (!createAfterCloseRef.current) return;
      event.preventDefault();
      createAfterCloseRef.current = false;
      onStartCreate();
    },
    [onStartCreate],
  );

  // Without an upload handler there is only one action, so keep the original
  // one-click button rather than burying New File behind a menu.
  if (!onUploadFiles) {
    return (
      <ToolbarButton
        onClick={onStartCreate}
        label={t("task:newFile")}
        icon={<IconPlus className="h-3.5 w-3.5" />}
      />
    );
  }

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <button
              type="button"
              aria-label={t("task:addToWorkspace")}
              data-testid="files-create-menu"
              className="inline-flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer max-md:size-11 [@media(pointer:coarse)]:size-11"
            >
              <IconPlus className="h-3.5 w-3.5" />
            </button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{t("task:addToWorkspace")}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent align="end" className="w-56" onCloseAutoFocus={handleCloseAutoFocus}>
        <DropdownMenuItem
          className="min-h-8 cursor-pointer gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onSelect={handleCreateSelect}
        >
          <IconFilePlus className="h-3.5 w-3.5" />
          {t("task:newFile")}
        </DropdownMenuItem>
        <DropdownMenuItem
          className="min-h-8 cursor-pointer gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onSelect={() => onUploadFiles("files")}
        >
          <IconUpload className="h-3.5 w-3.5" />
          {t("task:uploadFiles")}
        </DropdownMenuItem>
        <DropdownMenuItem
          className="min-h-8 cursor-pointer gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onSelect={() => onUploadFiles("folder")}
        >
          <IconFolderUp className="h-3.5 w-3.5" />
          {t("task:uploadFolder")}
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

function OpenWorkspaceFolderMenuItem({
  isOpening,
  disabled,
  onSelect,
}: {
  isOpening?: boolean;
  disabled?: boolean;
  onSelect: () => void;
}) {
  const { t } = useTranslation();
  return (
    <DropdownMenuItem
      className="min-h-8 cursor-pointer gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
      disabled={disabled || isOpening}
      aria-busy={isOpening}
      onSelect={onSelect}
    >
      {isOpening ? (
        <IconLoader2 className="h-3.5 w-3.5 animate-spin" aria-hidden />
      ) : (
        <IconFolderOpen className="h-3.5 w-3.5" aria-hidden />
      )}
      {t("task:openWorkspaceFolder")}
    </DropdownMenuItem>
  );
}

function WorkspaceActionsMenu({
  onAddSources,
  onOpenFolder,
  isOpeningFolder,
  isFolderDisabled,
  addSourcesButtonRef,
  addSourcesDisabledReason,
}: Pick<
  FileBrowserToolbarProps,
  | "onAddSources"
  | "onOpenFolder"
  | "isOpeningFolder"
  | "isFolderDisabled"
  | "addSourcesButtonRef"
  | "addSourcesDisabledReason"
>) {
  const { t } = useTranslation();
  const triggerRef = useRef<HTMLButtonElement>(null);
  const openSourcesAfterCloseRef = useRef(false);
  const openFolderAfterCloseRef = useRef(false);
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
    (onAddSources ? undefined : t("task:taskNeedsRepositoryForSources"));

  return (
    <DropdownMenu>
      <Tooltip>
        <TooltipTrigger asChild>
          <DropdownMenuTrigger asChild>
            <button
              ref={setTriggerRef}
              type="button"
              aria-label={t("task:workspaceActions")}
              data-testid="files-workspace-actions"
              className="inline-flex size-6 shrink-0 items-center justify-center rounded text-muted-foreground hover:bg-muted hover:text-foreground cursor-pointer max-md:size-11 [@media(pointer:coarse)]:size-11"
            >
              <IconDots className="h-4 w-4" />
            </button>
          </DropdownMenuTrigger>
        </TooltipTrigger>
        <TooltipContent>{t("task:workspaceActions")}</TooltipContent>
      </Tooltip>
      <DropdownMenuContent
        align="end"
        className="w-72"
        onCloseAutoFocus={(event) => {
          if (openFolderAfterCloseRef.current) {
            event.preventDefault();
            openFolderAfterCloseRef.current = false;
            triggerRef.current?.focus();
            onOpenFolder(triggerRef.current ?? undefined);
            return;
          }
          if (!openSourcesAfterCloseRef.current) return;
          event.preventDefault();
          openSourcesAfterCloseRef.current = false;
          if (triggerRef.current && onAddSources) onAddSources(triggerRef.current);
        }}
      >
        <DropdownMenuItem
          disabled={Boolean(disabledReason)}
          className="min-h-8 cursor-pointer gap-2 max-md:min-h-11 [@media(pointer:coarse)]:min-h-11"
          onSelect={() => {
            openSourcesAfterCloseRef.current = true;
            restoreMobileFocusAfterDrawerClose();
          }}
        >
          <IconPlus className="h-3.5 w-3.5" />
          <span className="min-w-0">
            <span className="block">{t("task:addRepositoriesToWorkspace")}</span>
            {disabledReason && (
              <span className="block text-[10px] text-muted-foreground normal-case">
                {disabledReason}
              </span>
            )}
          </span>
        </DropdownMenuItem>
        <OpenWorkspaceFolderMenuItem
          isOpening={isOpeningFolder}
          disabled={isFolderDisabled}
          onSelect={() => {
            openFolderAfterCloseRef.current = true;
          }}
        />
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
  isOpeningFolder,
  isFolderDisabled,
  onStartSearch,
  onCollapseAll,
  showCreateButton,
  onUploadFiles,
  onAddSources,
  addSourcesButtonRef,
  addSourcesDisabledReason,
}: FileBrowserToolbarProps) {
  const { t } = useTranslation();
  return (
    <PanelHeaderBarSplit
      className="group/header"
      left={
        <>
          <CopyWorkspacePathButton fullPath={fullPath} copied={copied} onCopyPath={onCopyPath} />
          {displayPath ? (
            <Tooltip>
              <TooltipTrigger asChild>
                <span
                  className="min-w-0 truncate text-xs font-medium text-muted-foreground"
                  data-testid="file-browser-workspace-path"
                >
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
            <CreateMenu onStartCreate={onStartCreate} onUploadFiles={onUploadFiles} />
          )}
          <WorkspaceActionsMenu
            onAddSources={onAddSources}
            onOpenFolder={onOpenFolder}
            isOpeningFolder={isOpeningFolder}
            isFolderDisabled={isFolderDisabled}
            addSourcesButtonRef={addSourcesButtonRef}
            addSourcesDisabledReason={addSourcesDisabledReason}
          />
          <ToolbarButton
            onClick={onStartSearch}
            label={t("task:searchFiles")}
            icon={<IconSearch className="h-3.5 w-3.5" />}
          />
          {expandedPathsSize > 0 && (
            <ToolbarButton
              onClick={onCollapseAll}
              label={t("task:collapseAll")}
              icon={<IconListTree className="h-3.5 w-3.5" />}
            />
          )}
        </>
      }
    />
  );
}
