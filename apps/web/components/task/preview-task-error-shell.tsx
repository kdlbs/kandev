"use client";

import type { ReactNode } from "react";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useTaskStatusSummary } from "@/hooks/domains/task/use-task-status-summary";
import { TaskLaunchErrorProvider } from "./task-launch-error-context";
import { TaskSharedError } from "./task-shared-error";

export function PreviewTaskErrorShell({
  taskId,
  workspaceId,
  statusSummary,
  resumption,
  children,
}: {
  taskId: string;
  workspaceId?: string | null;
  statusSummary?: ReturnType<typeof useTaskStatusSummary>;
  resumption: ReturnType<typeof useSessionResumption>;
  children: ReactNode;
}) {
  return (
    <TaskLaunchErrorProvider
      value={{
        taskId,
        workspaceId: workspaceId ?? "",
        statusSummary,
        automaticRecovery: resumption,
      }}
    >
      <div className="flex h-full min-h-0 flex-col">
        <TaskSharedError />
        {children}
      </div>
    </TaskLaunchErrorProvider>
  );
}
