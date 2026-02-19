"use client";

import { useCallback } from "react";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useLayoutStore } from "@/lib/state/layout-store";
import { useAppStore } from "@/components/state-provider";
import { useFileEditors } from "@/hooks/use-file-editors";

/**
 * Unified hook returning add-only panel action functions.
 * On desktop: delegates to dockview store.
 * On mobile/tablet: delegates to layout store (kept for backward compat).
 */
export function usePanelActions() {
  const { isMobile, isTablet } = useResponsiveBreakpoint();
  const isDesktop = !isMobile && !isTablet;

  // Desktop: dockview store
  const dockAddBrowser = useDockviewStore((s) => s.addBrowserPanel);
  const dockAddPlan = useDockviewStore((s) => s.addPlanPanel);
  const dockAddChat = useDockviewStore((s) => s.addChatPanel);
  const dockAddChanges = useDockviewStore((s) => s.addChangesPanel);
  const dockAddTerminal = useDockviewStore((s) => s.addTerminalPanel);

  // File editors (works on desktop through dockview)
  const { openFile: dockOpenFile } = useFileEditors();

  // Mobile/Tablet: layout store
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const openDocument = useLayoutStore((s) => s.openDocument);
  const setActiveDocument = useAppStore((s) => s.setActiveDocument);
  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const setPlanMode = useAppStore((s) => s.setPlanMode);

  const addBrowser = useCallback(
    (url?: string) => {
      if (isDesktop) {
        dockAddBrowser(url);
      } else if (activeSessionId) {
        // Mobile/tablet: use layout store to open preview
        useLayoutStore.getState().openPreview(activeSessionId);
      }
    },
    [isDesktop, dockAddBrowser, activeSessionId],
  );

  const addPlan = useCallback(() => {
    if (isDesktop) {
      dockAddPlan();
    } else if (activeSessionId && activeTaskId) {
      // Mobile/tablet: open document panel with plan
      setActiveDocument(activeSessionId, { type: "plan", taskId: activeTaskId });
      openDocument(activeSessionId);
      setPlanMode(activeSessionId, true);
    }
  }, [
    isDesktop,
    dockAddPlan,
    activeSessionId,
    activeTaskId,
    setActiveDocument,
    openDocument,
    setPlanMode,
  ]);

  const addChat = useCallback(() => {
    if (isDesktop) {
      dockAddChat();
    }
  }, [isDesktop, dockAddChat]);

  const addChanges = useCallback(() => {
    if (isDesktop) {
      dockAddChanges();
    }
  }, [isDesktop, dockAddChanges]);

  const addTerminal = useCallback(
    (terminalId?: string) => {
      if (isDesktop) {
        dockAddTerminal(terminalId);
      }
    },
    [isDesktop, dockAddTerminal],
  );

  const openFile = useCallback(
    (filePath: string) => {
      if (isDesktop) {
        dockOpenFile(filePath);
      }
    },
    [isDesktop, dockOpenFile],
  );

  return {
    addBrowser,
    addPlan,
    addChat,
    addChanges,
    addTerminal,
    openFile,
  };
}
