"use client";

import { useMemo, useRef, useState } from "react";
import {
  IconAdjustments,
  IconArrowLeft,
  IconCheck,
  IconChevronDown,
  IconPlus,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { Drawer, DrawerContent, DrawerHeader, DrawerTitle } from "@kandev/ui/drawer";
import { Popover, PopoverContent, PopoverTrigger } from "@kandev/ui/popover";
import { useTranslation } from "react-i18next";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import {
  DEFAULT_THREAD_VIEW,
  MAX_THREAD_VIEWS,
  threadViewName,
} from "@/lib/state/slices/ui/thread-view-builtins";
import type { ThreadCandidate, ThreadViewQueryResult } from "@/lib/threads/thread-view-query";
import type { ThreadView, ThreadViewDraft } from "@/lib/state/slices/ui/thread-view-types";
import type { Repository } from "@/lib/types/http";
import { ThreadsViewEditor } from "./threads-view-editor";

type Props = Pick<ThreadViewQueryResult, "matchingCount" | "hiddenCount"> & {
  candidates: ThreadCandidate[];
  repositories?: ReadonlyArray<Pick<Repository, "id" | "name">>;
  admittedCount: number;
};

// eslint-disable-next-line max-lines-per-function -- Coordinates the desktop and touch entry points for one saved-view state adapter.
export function ThreadsViewControls({
  candidates,
  repositories = [],
  admittedCount,
  matchingCount,
  hiddenCount,
}: Props) {
  const { t } = useTranslation();
  const { usesDesktopWorkbench } = useResponsiveBreakpoint();
  const views = useAppStore((state) => state.threadViews.views);
  const activeViewId = useAppStore((state) => state.threadViews.activeViewId);
  const draft = useAppStore((state) => state.threadViews.draft);
  const syncError = useAppStore((state) => state.threadViews.syncError);
  const setActiveView = useAppStore((state) => state.setThreadActiveView);
  const createView = useAppStore((state) => state.createThreadView);
  const updateDraft = useAppStore((state) => state.updateThreadViewDraft);
  const save = useAppStore((state) => state.saveThreadViewDraftOverwrite);
  const saveAs = useAppStore((state) => state.saveThreadViewDraftAs);
  const discard = useAppStore((state) => state.discardThreadViewDraft);
  const deleteView = useAppStore((state) => state.deleteThreadView);
  const renameView = useAppStore((state) => state.renameThreadView);
  const duplicateView = useAppStore((state) => state.duplicateThreadView);
  const reapplySort = useAppStore((state) => state.reapplyThreadViewSort);
  const retrySync = useAppStore((state) => state.retryThreadViewSync);
  const clearSyncError = useAppStore((state) => state.clearThreadViewSyncError);
  const [settingsOpen, setSettingsOpen] = useState(false);
  const openSettingsAfterPickerClose = useRef(false);
  const activeView = useMemo(
    () => views.find((view) => view.id === activeViewId) ?? DEFAULT_THREAD_VIEW,
    [activeViewId, views],
  );
  const activeViewName = threadViewName(activeView, t);
  const hasDraft = !!draft && draft.baseViewId === activeView.id;
  let disabledReason: string | null = null;
  if (draft) disabledReason = t("threads:saveOrDiscardBeforeNewView");
  else if (views.length >= MAX_THREAD_VIEWS) {
    disabledReason = t("threads:viewLimitReached", { count: MAX_THREAD_VIEWS });
  }

  function startNewView() {
    if (disabledReason) return;
    if (!createView()) return;
    openSettingsAfterPickerClose.current = true;
  }

  if (!usesDesktopWorkbench) {
    return (
      <MobileThreadsViewControls
        activeView={activeView}
        views={views}
        draft={draft}
        candidates={candidates}
        repositories={repositories}
        admittedCount={admittedCount}
        matchingCount={matchingCount}
        hiddenCount={hiddenCount}
        syncError={syncError}
        disabledReason={disabledReason}
        viewCount={views.length}
        canDelete={views.length > 1}
        onSetActiveView={setActiveView}
        onCreateView={createView}
        onUpdate={updateDraft}
        onSave={save}
        onSaveAs={saveAs}
        onDiscard={discard}
        onRename={(name) => renameView(activeView.id, name)}
        onDelete={() => deleteView(activeView.id)}
        onDuplicate={() => duplicateView(activeView.id, "")}
        onReapplySort={reapplySort}
        onRetrySync={retrySync}
        onDismissSyncError={clearSyncError}
      />
    );
  }

  return (
    <div className="flex items-center gap-1" data-testid="threads-view-controls">
      {syncError && (
        <ThreadViewSyncError error={syncError} onRetry={retrySync} onDismiss={clearSyncError} />
      )}
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="sm"
            className="h-8 max-w-[9rem] cursor-pointer gap-1 px-2 text-xs"
            aria-label={t("threads:viewPickerLabel", { name: activeViewName })}
            data-testid="threads-view-picker"
          >
            <span className="truncate">{activeViewName}</span>
            <IconChevronDown className="h-3.5 w-3.5 shrink-0" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent
          align="start"
          className="w-56"
          onCloseAutoFocus={(event) => {
            if (!openSettingsAfterPickerClose.current) return;
            event.preventDefault();
            openSettingsAfterPickerClose.current = false;
            setSettingsOpen(true);
          }}
        >
          {views.map((view) => (
            <DropdownMenuItem
              key={view.id}
              onSelect={() => setActiveView(view.id)}
              data-testid={`threads-view-option-${view.id}`}
              className="cursor-pointer gap-2 text-xs"
            >
              <IconCheck
                className={view.id === activeView.id ? "h-3.5 w-3.5" : "h-3.5 w-3.5 opacity-0"}
              />
              <span className="truncate">{threadViewName(view, t)}</span>
            </DropdownMenuItem>
          ))}
          <DropdownMenuSeparator />
          <DropdownMenuItem
            disabled={!!disabledReason}
            onSelect={startNewView}
            data-testid="threads-new-view"
            className="cursor-pointer gap-2 text-xs"
            title={disabledReason ?? undefined}
          >
            <IconPlus className="h-3.5 w-3.5" />
            <span>{t("threads:newView")}</span>
            {disabledReason && (
              <span className="ml-auto text-[10px] text-muted-foreground">{disabledReason}</span>
            )}
          </DropdownMenuItem>
        </DropdownMenuContent>
      </DropdownMenu>
      <Popover open={settingsOpen} onOpenChange={setSettingsOpen}>
        <PopoverTrigger asChild>
          <Button
            type="button"
            variant="ghost"
            size="icon"
            className="relative h-8 w-8 cursor-pointer"
            aria-label={t("threads:viewSettingsFor", { name: activeViewName })}
            data-testid="threads-view-settings"
          >
            <IconAdjustments className="h-4 w-4" />
            {hasDraft && (
              <span
                className="absolute right-1 top-1 h-1.5 w-1.5 rounded-full bg-amber-500"
                data-testid="threads-view-dirty"
              />
            )}
          </Button>
        </PopoverTrigger>
        <PopoverContent
          align="start"
          className="max-h-[calc(100dvh-1rem)] w-[min(42rem,calc(100vw-1rem))] overflow-y-auto border border-border/80 p-0 shadow-xl ring-1 ring-foreground/20"
          data-testid="threads-view-settings-popover"
        >
          <ThreadsViewEditor
            activeView={activeView}
            draft={draft}
            candidates={candidates}
            repositories={repositories}
            viewCount={views.length}
            canDelete={views.length > 1}
            onUpdate={updateDraft}
            onSave={save}
            onSaveAs={saveAs}
            onDiscard={discard}
            onRename={(name) => renameView(activeView.id, name)}
            onDelete={() => {
              deleteView(activeView.id);
              setSettingsOpen(false);
            }}
            onDuplicate={() => duplicateView(activeView.id, "")}
            onReapplySort={reapplySort}
          />
        </PopoverContent>
      </Popover>
      <span
        className="hidden max-w-[9rem] truncate text-[11px] text-muted-foreground xl:inline"
        data-testid="threads-view-count"
      >
        {t("threads:columnsSummary", { admitted: admittedCount, matching: matchingCount })}
        {hiddenCount > 0 && (
          <span className="ml-1">{t("threads:hiddenCount", { count: hiddenCount })}</span>
        )}
      </span>
    </div>
  );
}

type ThreadViewDraftUpdate = (
  patch: Partial<Pick<ThreadView, "taskScope" | "filters" | "sort" | "maxColumns">>,
) => void;

type MobileThreadsViewControlsProps = {
  activeView: ThreadView;
  views: ThreadView[];
  draft: ThreadViewDraft | null;
  candidates: ThreadCandidate[];
  repositories: ReadonlyArray<Pick<Repository, "id" | "name">>;
  admittedCount: number;
  matchingCount: number;
  hiddenCount: number;
  syncError: string | null;
  disabledReason: string | null;
  viewCount: number;
  canDelete: boolean;
  onSetActiveView: (id: string) => void;
  onCreateView: () => string | null;
  onUpdate: ThreadViewDraftUpdate;
  onSave: () => void;
  onSaveAs: (name: string) => void;
  onDiscard: () => void;
  onRename: (name: string) => void;
  onDelete: () => void;
  onDuplicate: () => void;
  onReapplySort: () => void;
  onRetrySync: () => void;
  onDismissSyncError: () => void;
};

// eslint-disable-next-line max-lines-per-function -- Keeps the single mobile drawer lifecycle and its shared editor wiring together.
function MobileThreadsViewControls({
  activeView,
  views,
  draft,
  candidates,
  repositories,
  admittedCount,
  matchingCount,
  hiddenCount,
  syncError,
  disabledReason,
  viewCount,
  canDelete,
  onSetActiveView,
  onCreateView,
  onUpdate,
  onSave,
  onSaveAs,
  onDiscard,
  onRename,
  onDelete,
  onDuplicate,
  onReapplySort,
  onRetrySync,
  onDismissSyncError,
}: MobileThreadsViewControlsProps) {
  const { t } = useTranslation();
  const activeViewName = threadViewName(activeView, t);
  const [open, setOpen] = useState(false);
  const [page, setPage] = useState<"views" | "editor">("views");
  const triggerRef = useRef<HTMLButtonElement>(null);

  function openViews() {
    setPage("views");
    setOpen(true);
  }

  function startNewView() {
    if (disabledReason) return;
    if (onCreateView()) setPage("editor");
  }

  function closeDrawer() {
    setOpen(false);
    triggerRef.current?.focus();
  }

  function selectView(id: string) {
    onSetActiveView(id);
    closeDrawer();
  }

  return (
    <>
      {syncError && (
        <ThreadViewSyncError
          error={syncError}
          mobile
          onRetry={onRetrySync}
          onDismiss={onDismissSyncError}
        />
      )}
      <Button
        type="button"
        variant="outline"
        className="min-h-11 max-w-[12rem] shrink-0 cursor-pointer gap-1 px-3 text-xs"
        onClick={openViews}
        aria-label={t("threads:viewPickerLabel", { name: activeViewName })}
        data-testid="threads-mobile-view-trigger"
        ref={triggerRef}
      >
        <IconAdjustments className="h-4 w-4 shrink-0" />
        <span className="truncate">{activeViewName}</span>
        <IconChevronDown className="h-3.5 w-3.5 shrink-0" />
      </Button>
      <Drawer
        open={open}
        onOpenChange={(nextOpen) => {
          setOpen(nextOpen);
          if (nextOpen) setPage("views");
        }}
      >
        <DrawerContent
          className="h-[min(90dvh,48rem)] max-h-[calc(100dvh-1rem)] overflow-hidden pb-[max(0.75rem,env(safe-area-inset-bottom))]"
          data-testid="threads-mobile-view-drawer"
          onCloseAutoFocus={(event) => {
            event.preventDefault();
            triggerRef.current?.focus();
          }}
          onAnimationEnd={(event) => {
            if (event.currentTarget.dataset.state === "closed") {
              triggerRef.current?.focus();
            }
          }}
        >
          <DrawerHeader className="shrink-0 border-b px-4 pb-3 pt-2 text-left">
            <div className="flex items-center gap-2">
              {page === "editor" && (
                <Button
                  type="button"
                  variant="ghost"
                  size="icon"
                  className="h-11 w-11 shrink-0 cursor-pointer"
                  onClick={() => setPage("views")}
                  aria-label={t("threads:backToViewEditor")}
                  data-testid="threads-mobile-view-back"
                >
                  <IconArrowLeft className="h-4 w-4" />
                </Button>
              )}
              <DrawerTitle className="text-base">
                {page === "editor" ? t("threads:viewSettings") : t("threads:title")}
              </DrawerTitle>
            </div>
          </DrawerHeader>
          <div
            className="min-h-0 flex-1 overflow-y-auto overscroll-contain"
            data-testid="threads-mobile-view-drawer-scroll-region"
          >
            {page === "views" ? (
              <MobileThreadViewList
                activeView={activeView}
                views={views}
                admittedCount={admittedCount}
                matchingCount={matchingCount}
                hiddenCount={hiddenCount}
                disabledReason={disabledReason}
                onSelect={selectView}
                onNewView={startNewView}
                onOpenSettings={() => setPage("editor")}
              />
            ) : (
              <ThreadsViewEditor
                activeView={activeView}
                draft={draft}
                candidates={candidates}
                repositories={repositories}
                viewCount={viewCount}
                canDelete={canDelete}
                mobile
                onUpdate={onUpdate}
                onSave={onSave}
                onSaveAs={onSaveAs}
                onDiscard={onDiscard}
                onRename={onRename}
                onDelete={() => {
                  onDelete();
                  closeDrawer();
                }}
                onDuplicate={onDuplicate}
                onReapplySort={onReapplySort}
              />
            )}
          </div>
        </DrawerContent>
      </Drawer>
    </>
  );
}

function ThreadViewSyncError({
  error,
  mobile = false,
  onRetry,
  onDismiss,
}: {
  error: string;
  mobile?: boolean;
  onRetry: () => void;
  onDismiss: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div
      className={`flex min-w-0 items-center gap-1 rounded-md border border-destructive/40 bg-destructive/10 px-2 py-1 text-xs text-destructive${mobile ? " mx-3 mb-2 mt-2 min-h-11" : ""}`}
      data-testid="threads-view-sync-error"
      role="alert"
    >
      <span className="min-w-0 flex-1 truncate">
        {t("threads:failedToSyncViews")}: {error}
      </span>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className={mobile ? "min-h-11 shrink-0 cursor-pointer" : "h-7 shrink-0 cursor-pointer"}
        onClick={onRetry}
        data-testid="threads-view-sync-retry"
      >
        {t("task:retry")}
      </Button>
      <Button
        type="button"
        variant="ghost"
        size="sm"
        className={mobile ? "min-h-11 shrink-0 cursor-pointer" : "h-7 shrink-0 cursor-pointer"}
        onClick={onDismiss}
        data-testid="threads-view-sync-dismiss"
      >
        {t("task:dismiss")}
      </Button>
    </div>
  );
}

function MobileThreadViewList({
  activeView,
  views,
  admittedCount,
  matchingCount,
  hiddenCount,
  disabledReason,
  onSelect,
  onNewView,
  onOpenSettings,
}: {
  activeView: ThreadView;
  views: ThreadView[];
  admittedCount: number;
  matchingCount: number;
  hiddenCount: number;
  disabledReason: string | null;
  onSelect: (id: string) => void;
  onNewView: () => void;
  onOpenSettings: () => void;
}) {
  const { t } = useTranslation();
  return (
    <div className="space-y-3 p-3" data-testid="threads-mobile-view-list">
      <div className="rounded-lg bg-muted/50 px-3 py-2 text-xs text-muted-foreground">
        {t("threads:columnsSummary", { admitted: admittedCount, matching: matchingCount })}
        {hiddenCount > 0 && (
          <span className="ml-1">{t("threads:hiddenCount", { count: hiddenCount })}</span>
        )}
      </div>
      <div className="space-y-1">
        {views.map((view) => (
          <Button
            key={view.id}
            type="button"
            variant="ghost"
            className="min-h-11 w-full cursor-pointer justify-start gap-2 px-3 text-left text-sm"
            onClick={() => onSelect(view.id)}
            data-testid={`threads-mobile-view-option-${view.id}`}
          >
            <IconCheck className={view.id === activeView.id ? "h-4 w-4" : "h-4 w-4 opacity-0"} />
            <span className="truncate">{threadViewName(view, t)}</span>
          </Button>
        ))}
      </div>
      <div className="grid gap-2 sm:grid-cols-2">
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer justify-start"
          onClick={onNewView}
          disabled={!!disabledReason}
          title={disabledReason ?? undefined}
          data-testid="threads-mobile-new-view"
        >
          <IconPlus className="mr-2 h-4 w-4" />
          {t("threads:newView")}
        </Button>
        <Button
          type="button"
          variant="outline"
          className="min-h-11 cursor-pointer justify-start"
          onClick={onOpenSettings}
          data-testid="threads-mobile-view-settings"
        >
          <IconAdjustments className="mr-2 h-4 w-4" />
          {t("threads:viewSettings")}
        </Button>
      </div>
    </div>
  );
}
