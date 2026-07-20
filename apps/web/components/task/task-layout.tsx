"use client";

import { memo } from "react";
import dynamic from "@/lib/routing/client-dynamic";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import { SessionMobileLayout, SessionTabletLayout } from "./mobile";
import type { Repository, RepositoryScript } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import type { Layout } from "react-resizable-panels";

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
