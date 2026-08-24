"use client";

import { memo } from "react";
import dynamic from "@/lib/routing/client-dynamic";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { SessionMobileLayout, SessionTabletLayout } from "./mobile";
import type { Repository, RepositoryScript } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import type { Layout } from "react-resizable-panels";
import { isTypedTaskLaunchError } from "./simple/components/task-launch-error-entry";
import { TaskChatLaunchError } from "./simple/components/task-chat-launch-error";
import { useTaskLaunchErrorContext } from "./task-launch-error-context";

// Re-export for backwards compatibility
export type { SelectedDiff } from "@/hooks/use-session-layout-state";

// Dynamic import for dockview (no SSR)
const DockviewDesktopLayout = dynamic(
  () => import("./dockview-desktop-layout").then((mod) => mod.DockviewDesktopLayout),
  { ssr: false },
);

type TaskLayoutProps = {
  workspaceId: string | null;
  workflowId: string | null;
  sessionId?: string | null;
  repository?: Repository | null;
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts?: Record<string, Layout>;
  taskTitle?: string;
  /** `owner/repo` (or the repository name) of the task's primary repository. */
  repositoryLabel?: string | null;
  baseBranch?: string;
  worktreeBranch?: string | null;
  isRemoteExecutor?: boolean;
  remoteExecutorType?: string | null;
  remoteExecutorName?: string | null;
  remoteState?: string | null;
  remoteCreatedAt?: string | null;
  remoteCheckedAt?: string | null;
  remoteStatusError?: string | null;
  initialLayout?: string | null;
  isArchived?: boolean;
};

export const TaskLayout = memo(function TaskLayout({
  workspaceId,
  workflowId,
  sessionId = null,
  repository = null,
  initialScripts = [],
  initialTerminals,
  defaultLayouts = {},
  taskTitle,
  repositoryLabel,
  baseBranch,
  worktreeBranch,
  isRemoteExecutor,
  remoteExecutorType,
  remoteExecutorName,
  remoteState,
  remoteCreatedAt,
  remoteCheckedAt,
  remoteStatusError,
  initialLayout,
  isArchived,
}: TaskLayoutProps) {
  const { isMobile, usesDesktopWorkbench, isFullDesktop } = useResponsiveBreakpoint();
  const launchErrorContext = useTaskLaunchErrorContext();
  const activeLaunchError = launchErrorContext?.statusSummary?.active_error;

  if (launchErrorContext && !sessionId && isTypedTaskLaunchError(activeLaunchError)) {
    return (
      <div
        className="flex h-full min-h-0 min-w-0 flex-col overflow-auto px-4"
        data-testid="session-chat"
      >
        <TaskChatLaunchError
          taskId={launchErrorContext.taskId}
          workspaceId={launchErrorContext.workspaceId}
          statusSummary={launchErrorContext.statusSummary}
          runErrors={[]}
          repositories={launchErrorContext.repositories}
        />
      </div>
    );
  }

  // Mobile layout
  if (isMobile) {
    return (
      <SessionMobileLayout
        workspaceId={workspaceId}
        workflowId={workflowId}
        sessionId={sessionId}
        baseBranch={baseBranch}
        worktreeBranch={worktreeBranch}
        taskTitle={taskTitle}
        repositoryLabel={repositoryLabel}
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorType={remoteExecutorType}
        remoteExecutorName={remoteExecutorName}
        remoteState={remoteState}
        remoteCreatedAt={remoteCreatedAt}
        remoteCheckedAt={remoteCheckedAt}
        remoteStatusError={remoteStatusError}
        isArchived={isArchived}
      />
    );
  }

  // Tablet fallback for coarse-pointer half-screen devices.
  if (!usesDesktopWorkbench) {
    return (
      <SessionTabletLayout
        workspaceId={workspaceId}
        workflowId={workflowId}
        sessionId={sessionId}
        repository={repository}
        defaultLayouts={defaultLayouts}
      />
    );
  }

  // Desktop layout - dockview
  return (
    <DockviewDesktopLayout
      workspaceId={workspaceId}
      workflowId={workflowId}
      sessionId={sessionId}
      repository={repository}
      initialScripts={initialScripts}
      initialTerminals={initialTerminals}
      initialLayout={initialLayout}
      compact={!isFullDesktop}
    />
  );
});
