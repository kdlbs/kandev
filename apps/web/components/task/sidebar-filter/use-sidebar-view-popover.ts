"use client";

import { useCallback, useEffect, useState } from "react";
import { useAppStore } from "@/components/state-provider";
import { MAX_SIDEBAR_VIEWS } from "@/lib/state/slices/ui/sidebar-view-builtins";

export function getNewViewDisabledReason(viewCount: number, hasDraft: boolean): string | null {
  if (hasDraft) return "Save or discard changes before creating a new view.";
  if (viewCount >= MAX_SIDEBAR_VIEWS) {
    return `You can save up to ${MAX_SIDEBAR_VIEWS} views.`;
  }
  return null;
}

export function useSidebarViewPopover() {
  const views = useAppStore((state) => state.sidebarViews.views);
  const draft = useAppStore((state) => state.sidebarViews.draft);
  const createSidebarView = useAppStore((state) => state.createSidebarView);
  const [open, setOpen] = useState(false);
  const [renameRequestedViewId, setRenameRequestedViewId] = useState<string | null>(null);
  const newViewDisabledReason = getNewViewDisabledReason(views.length, draft !== null);

  const startNewView = useCallback(
    (options?: { openPopover?: boolean }): boolean => {
      if (newViewDisabledReason) return false;
      const createdViewId = createSidebarView();
      if (!createdViewId) return false;
      setRenameRequestedViewId(createdViewId);
      if (options?.openPopover !== false) setOpen(true);
      return true;
    },
    [createSidebarView, newViewDisabledReason],
  );

  const onOpenChange = useCallback((nextOpen: boolean) => {
    setOpen(nextOpen);
    if (!nextOpen) setRenameRequestedViewId(null);
  }, []);

  const consumeRenameRequest = useCallback((viewId: string) => {
    setRenameRequestedViewId((current) => (current === viewId ? null : current));
  }, []);

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
