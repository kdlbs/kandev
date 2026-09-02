"use client";

import { useMemo } from "react";
import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { useAppStore } from "@/components/state-provider";
import { useTaskLsp } from "@/hooks/domains/lsp/use-task-lsp";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { deriveTaskLspViewModel } from "@/lib/lsp/task-lsp-view-model";
import { TaskLspControl } from "./task-lsp-control";

export function TaskLspTopbarControl({ taskId }: { taskId: string | null }) {
  const appStatusBarEnabled = useAppStore((state) => state.userSettings.appStatusBarEnabled);
  const responsive = useResponsiveBreakpoint();
  const drawer = useAppStatusDrawer();
  const lsp = useTaskLsp(taskId);
  const hiddenLanguages = useAppStore((state) => state.userSettings.lspStatusHiddenLanguages ?? []);
  const hasRelevantLanguage = useMemo(
    () =>
      deriveTaskLspViewModel(lsp.languages, Date.now(), { hiddenLanguages }).relevantRows.length >
      0,
    [hiddenLanguages, lsp.languages],
  );
  if (!taskId) return null;

  const usesTaskDrawer = !responsive.isFinePointer && drawer.enabled;
  if (appStatusBarEnabled && responsive.isFinePointer && hasRelevantLanguage) return null;
  return (
    <TaskLspControl
      taskId={taskId}
      placement="task-topbar"
      touch={usesTaskDrawer}
      externalOpen={drawer.drawerOpen}
      onOpenExternal={usesTaskDrawer ? drawer.openLspStatusDrawer : undefined}
    />
  );
}
