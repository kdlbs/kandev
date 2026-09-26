"use client";

import { memo, useCallback, useState } from "react";
import dynamic from "@/lib/routing/client-dynamic";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { useRouter } from "@/lib/routing/client-router";
import { canvasHref, type Canvas } from "@/lib/api/domains/canvas-api";
import { TaskSheetSelectionProvider } from "./mobile/task-sheet-selection-context";
import { ResponsiveTaskPicker } from "./mobile/responsive-task-picker";
import { SessionMobileLayout, SessionTabletLayout } from "./mobile";
import type { Repository, RepositoryScript } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import type { Layout } from "react-resizable-panels";
import { useTaskCanvasLifecycleActivation } from "./dockview-canvas-activation";
import { statusSummaryTaskError } from "@/lib/task-status-summary";
import { useTaskLaunchErrorContext } from "./task-launch-error-context";
import type { TaskCanvasesLoadStatus } from "@/hooks/domains/task/use-task-canvases";
import type { TaskTopbarRepository } from "./task-page-content-helpers";

// Re-export for backwards compatibility
export type { SelectedDiff } from "@/hooks/use-session-layout-state";

// Dynamic import for dockview (no SSR)
const DockviewDesktopLayout = dynamic(
  () => import("./dockview-desktop-layout").then((mod) => mod.DockviewDesktopLayout),
  { ssr: false },
);

type TaskLayoutProps = {
  taskId?: string | null;
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
  topbarRepository?: TaskTopbarRepository | null;
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
  onTaskUnarchived?: (taskId: string) => void;
  taskCanvases?: Canvas[];
  taskCanvasesStatus?: TaskCanvasesLoadStatus;
};

export const TaskLayout = memo(function TaskLayout(props: TaskLayoutProps) {
  const [pickerWorkspaceId, setPickerWorkspaceId] = useState(props.workspaceId);
  if (props.workspaceId && props.workspaceId !== pickerWorkspaceId) {
    setPickerWorkspaceId(props.workspaceId);
  }
  return (
    <TaskSheetSelectionProvider workspaceId={props.workspaceId}>
      <ResponsiveTaskLayout {...props} />
      <ResponsiveTaskPicker
        key={pickerWorkspaceId}
        workspaceId={props.workspaceId}
        workflowId={props.workflowId}
      />
    </TaskSheetSelectionProvider>
  );
});

const ResponsiveTaskLayout = memo(function ResponsiveTaskLayout({
  taskId = null,
  workspaceId,
  workflowId,
  sessionId = null,
  repository = null,
  initialScripts = [],
  initialTerminals,
  defaultLayouts = {},
  taskTitle,
  repositoryLabel,
  topbarRepository,
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
  onTaskUnarchived,
  taskCanvases,
  taskCanvasesStatus,
}: TaskLayoutProps) {
  const { isMobile, usesDesktopWorkbench, isFullDesktop } = useResponsiveBreakpoint();
  const launchErrorContext = useTaskLaunchErrorContext();
  const hasSharedTaskError = Boolean(statusSummaryTaskError(launchErrorContext?.statusSummary));
  useTaskCanvasLifecycleActivation({
    taskId,
    workspaceId,
    sessionId,
    isMobile,
    taskCanvases,
    taskCanvasesStatus,
  });
  const router = useRouter();
  const onOpenCanvas = useCallback(
    (canvasId: string) => router.push(canvasHref(canvasId)),
    [router],
  );
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
        topbarRepository={topbarRepository}
        isRemoteExecutor={isRemoteExecutor}
        remoteExecutorType={remoteExecutorType}
        remoteExecutorName={remoteExecutorName}
        remoteState={remoteState}
        remoteCreatedAt={remoteCreatedAt}
        remoteCheckedAt={remoteCheckedAt}
        remoteStatusError={remoteStatusError}
        isArchived={isArchived}
        onTaskUnarchived={onTaskUnarchived}
        taskCanvases={taskCanvases ?? []}
        onOpenCanvas={onOpenCanvas}
        hasSharedTaskError={hasSharedTaskError}
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
