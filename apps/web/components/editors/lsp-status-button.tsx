"use client";

import { useAppStatusDrawer } from "@/components/app-status-bar/app-status-surface-provider";
import { TaskLspControl } from "@/components/lsp/task-lsp-control";
import { useAppStore } from "@/components/state-provider";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { LspStatus } from "@/lib/lsp/lsp-client-manager";
import type { LspProgressSnapshot } from "@/lib/lsp/lsp-progress";

type LspStatusButtonProps = {
  status: LspStatus;
  progress: LspProgressSnapshot;
  lspLanguage: string | null;
  onToggle: () => void;
};

export function LspStatusButton(props: LspStatusButtonProps) {
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const drawer = useAppStatusDrawer();
  const { isFinePointer } = useResponsiveBreakpoint();
  if (!props.lspLanguage) return null;
  const usesTaskDrawer = !isFinePointer && drawer.enabled;
  return (
    <TaskLspControl
      taskId={taskId}
      language={props.lspLanguage}
      placement="editor-toolbar"
      touch={usesTaskDrawer}
      externalOpen={drawer.drawerOpen}
      onOpenExternal={usesTaskDrawer ? drawer.openLspStatusDrawer : undefined}
    />
  );
}
