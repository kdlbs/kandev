'use client';

import { memo } from 'react';
import dynamic from 'next/dynamic';
import { useResponsiveBreakpoint } from '@/hooks/use-responsive-breakpoint';
import { SessionMobileLayout, SessionTabletLayout } from './mobile';
import type { Repository, RepositoryScript } from '@/lib/types/http';
import type { Terminal } from '@/hooks/domains/session/use-terminals';
import type { Layout } from 'react-resizable-panels';

// Re-export for backwards compatibility
export type { SelectedDiff } from '@/hooks/use-session-layout-state';

// Dynamic import for dockview (no SSR)
const DockviewDesktopLayout = dynamic(
  () => import('./dockview-desktop-layout').then((mod) => mod.DockviewDesktopLayout),
  { ssr: false }
);

type TaskLayoutProps = {
  workspaceId: string | null;
  boardId: string | null;
  sessionId?: string | null;
  repository?: Repository | null;
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts?: Record<string, Layout>;
  taskTitle?: string;
  baseBranch?: string;
  worktreeBranch?: string | null;
};

export const TaskLayout = memo(function TaskLayout({
  workspaceId,
  boardId,
  sessionId = null,
  repository = null,
  initialScripts = [],
  initialTerminals,
  defaultLayouts = {},
  taskTitle,
  baseBranch,
  worktreeBranch,
}: TaskLayoutProps) {
  const { isMobile, isTablet } = useResponsiveBreakpoint();

  // Mobile layout
  if (isMobile) {
    return (
      <SessionMobileLayout
        workspaceId={workspaceId}
        boardId={boardId}
        sessionId={sessionId}
        baseBranch={baseBranch}
        worktreeBranch={worktreeBranch}
        taskTitle={taskTitle}
      />
    );
  }

  // Tablet layout
  if (isTablet) {
    return (
      <SessionTabletLayout
        workspaceId={workspaceId}
        boardId={boardId}
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
      boardId={boardId}
      sessionId={sessionId}
      repository={repository}
      initialScripts={initialScripts}
      initialTerminals={initialTerminals}
    />
  );
});
