"use client";

import { selectSidebarViews } from "@/lib/state/slices/ui/sidebar-workspace-state";

import { useCallback, useEffect, useState } from "react";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { MAX_SIDEBAR_VIEWS } from "@/lib/state/slices/ui/sidebar-view-builtins";
import { t } from "@/lib/i18n";

export function getNewViewDisabledReason(viewCount: number, hasDraft: boolean): string | null {
  if (hasDraft) return t("task:saveOrDiscardBeforeNewView");
  if (viewCount >= MAX_SIDEBAR_VIEWS) {
    return t("task:viewLimitReached", { count: MAX_SIDEBAR_VIEWS });
  }
  return null;
}

export function useSidebarViewPopover() {
  const store = useAppStoreApi();
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const views = useAppStore((state) => selectSidebarViews(state).views);
  const draft = useAppStore((state) => selectSidebarViews(state).draft);
  const createSidebarView = useAppStore((state) => state.createSidebarView);
  const [openWorkspaceId, setOpenWorkspaceId] = useState<string | null>(null);
  const open = Boolean(workspaceId && openWorkspaceId === workspaceId);
  const [renameRequestedViewId, setRenameRequestedViewId] = useState<string | null>(null);
  const newViewDisabledReason = getNewViewDisabledReason(views.length, draft !== null);

  const startNewView = useCallback(
    (options?: { openPopover?: boolean }): boolean => {
      if (
        newViewDisabledReason ||
        !workspaceId ||
        store.getState().workspaces.activeId !== workspaceId
      )
        return false;
      const createdViewId = createSidebarView();
      if (!createdViewId) return false;
      setRenameRequestedViewId(createdViewId);
      if (options?.openPopover !== false) setOpenWorkspaceId(workspaceId);
      return true;
    },
    [createSidebarView, newViewDisabledReason, store, workspaceId],
  );

  const onOpenChange = useCallback(
    (nextOpen: boolean) => {
      if (store.getState().workspaces.activeId !== workspaceId) return;
      setOpenWorkspaceId(nextOpen ? workspaceId : null);
      if (!nextOpen) setRenameRequestedViewId(null);
    },
    [store, workspaceId],
  );

  const consumeRenameRequest = useCallback((viewId: string) => {
    setRenameRequestedViewId((current) => (current === viewId ? null : current));
  }, []);

  useEffect(() => {
    setOpenWorkspaceId(null);
    setRenameRequestedViewId(null);
  }, [workspaceId]);

  useEffect(() => {
    if (renameRequestedViewId && !views.some((view) => view.id === renameRequestedViewId)) {
      setRenameRequestedViewId(null);
    }
  }, [renameRequestedViewId, views]);

  return {
    open,
    onOpenChange,
    startNewView,
    renameRequestedViewId,
    consumeRenameRequest,
    newViewDisabledReason,
  };
}
