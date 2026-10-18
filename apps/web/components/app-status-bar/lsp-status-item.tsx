"use client";

import { TaskLspControl, TaskLspDisclosure } from "@/components/lsp/task-lsp-control";

type LspStatusItemProps = {
  taskId: string;
  presentation: "bar" | "mobile-drawer";
  focusLanguage?: string | null;
};

export function LspStatusItem({ taskId, presentation, focusLanguage }: LspStatusItemProps) {
  if (presentation === "mobile-drawer") {
    return <TaskLspDisclosure taskId={taskId} touch focusLanguage={focusLanguage} />;
  }
  return <TaskLspControl taskId={taskId} placement="status-bar" hideWhenIrrelevant />;
}
