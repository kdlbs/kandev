'use client';

import { memo, useMemo, useState, useCallback } from 'react';
import { Group, Panel, type Layout } from 'react-resizable-panels';
import { useDefaultLayout } from '@/lib/layout/use-default-layout';
import { useResponsiveBreakpoint } from '@/hooks/use-responsive-breakpoint';
import { useSessionLayoutState } from '@/hooks/use-session-layout-state';
import { TaskCenterPanel } from './task-center-panel';
import { TaskRightPanel } from './task-right-panel';
import { TaskFilesPanel } from './task-files-panel';
import { TaskSessionSidebar } from './task-session-sidebar';
import { SessionMobileLayout, SessionTabletLayout } from './mobile';
import { PreviewPanel } from '@/components/task/preview/preview-panel';
import { PreviewController } from '@/components/task/preview/preview-controller';
import { useLayoutStore } from '@/lib/state/layout-store';
import type { Repository, RepositoryScript } from '@/lib/types/http';
import type { Terminal } from '@/hooks/domains/session/use-terminals';

// Re-export for backwards compatibility
export type { SelectedDiff } from '@/hooks/use-session-layout-state';

const DEFAULT_HORIZONTAL_LAYOUT: Record<string, number> = {
  left: 18,
  chat: 57,
  preview: 57,
  right: 25,
};
const DEFAULT_PREVIEW_LAYOUT: Record<string, number> = {
  chat: 60,
  preview: 40,
};
const DEFAULT_LAYOUT_STATE = { left: true, chat: true, right: true, preview: false };

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

  // Use shared layout state hook
  const {
    effectiveSessionId,
    sessionKey,
    selectedDiff,
    handleSelectDiff,
    handleClearSelectedDiff,
    openFileRequest,
    handleOpenFile,
    handleFileOpenHandled,
  } = useSessionLayoutState({ sessionId });

  // Track active file path for highlighting in file tree
  const [activeFilePath, setActiveFilePath] = useState<string | null>(null);

  const handleActiveFileChange = useCallback((filePath: string | null) => {
    setActiveFilePath(filePath);
  }, []);

  const layoutBySession = useLayoutStore((state) => state.columnsBySessionId);
  const layoutState = useMemo(
    () => layoutBySession[sessionKey] ?? DEFAULT_LAYOUT_STATE,
    [layoutBySession, sessionKey]
  );
  const hasDevScript = Boolean(repository?.dev_script?.trim());
  const sessionForPreview = effectiveSessionId;

  const horizontalPanelIds = useMemo(() => {
    const ids: string[] = [];
    if (layoutState.left) ids.push('left');
    ids.push(layoutState.preview ? 'preview' : 'chat');
    if (layoutState.right) ids.push('right');
    return ids;
  }, [layoutState.left, layoutState.preview, layoutState.right]);

  const horizontalLayoutKey = `task-layout-horizontal-v3:${horizontalPanelIds.join('|')}`;
  const { defaultLayout: defaultHorizontalLayout, onLayoutChanged: onHorizontalLayoutChange } =
    useDefaultLayout({
      id: horizontalLayoutKey,
      panelIds: horizontalPanelIds,
      baseLayout: DEFAULT_HORIZONTAL_LAYOUT,
      serverDefaultLayout: defaultLayouts[horizontalLayoutKey],
    });

  const previewPanelIds = ['chat', 'preview'];
  const previewLayoutKey = 'task-layout-preview-v2';
  const { defaultLayout: defaultPreviewLayout, onLayoutChanged: onPreviewLayoutChange } =
    useDefaultLayout({
      id: previewLayoutKey,
      panelIds: previewPanelIds,
      baseLayout: DEFAULT_PREVIEW_LAYOUT,
      serverDefaultLayout: defaultLayouts[previewLayoutKey],
    });

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

  // Desktop layout
  const topFilesPanel = (
    <TaskFilesPanel
      onSelectDiff={handleSelectDiff}
      onOpenFile={handleOpenFile}
      activeFilePath={activeFilePath}
    />
  );

  return (
    <div className="flex-1 min-h-0 px-4 pb-4">
      <PreviewController sessionId={sessionForPreview} hasDevScript={hasDevScript} />
      <Group
        orientation="horizontal"
        className="h-full min-h-0"
        id={horizontalLayoutKey}
        key={horizontalLayoutKey}
        defaultLayout={defaultHorizontalLayout}
        onLayoutChanged={onHorizontalLayoutChange}
      >
        {layoutState.left ? (
          <>
            <Panel id="left" minSize="180px" className="min-h-0">
              <TaskSessionSidebar workspaceId={workspaceId} boardId={boardId} />
            </Panel>
          </>
        ) : null}

        {layoutState.preview ? (
          <Panel id="preview" className="min-h-0 min-w-0">
            <Group
              orientation="horizontal"
              className="h-full min-h-0 min-w-0"
              id={previewLayoutKey}
              key={previewLayoutKey}
              defaultLayout={defaultPreviewLayout}
              onLayoutChanged={onPreviewLayoutChange}
            >
              <Panel id="chat" minSize="400px" className="min-h-0 min-w-0">
                <TaskCenterPanel
                  selectedDiff={selectedDiff}
                  openFileRequest={openFileRequest}
                  onDiffHandled={handleClearSelectedDiff}
                  onFileOpenHandled={handleFileOpenHandled}
                  onActiveFileChange={handleActiveFileChange}
                />
              </Panel>
              <Panel id="preview" minSize="470px" className="min-h-0 min-w-0">
                <PreviewPanel sessionId={sessionForPreview} hasDevScript={hasDevScript} />
              </Panel>
            </Group>
          </Panel>
        ) : (
          <Panel id="chat" minSize="400px" className="min-h-0 min-w-0">
            <TaskCenterPanel
              selectedDiff={selectedDiff}
              openFileRequest={openFileRequest}
              onDiffHandled={handleClearSelectedDiff}
              onFileOpenHandled={handleFileOpenHandled}
              onActiveFileChange={handleActiveFileChange}
              sessionId={sessionId}
            />
          </Panel>
        )}

        {layoutState.right ? (
          <>
            <Panel id="right" minSize="310px" className="min-h-0 min-w-0">
              <TaskRightPanel topPanel={topFilesPanel} sessionId={sessionForPreview} repositoryId={repository?.id ?? null} initialScripts={initialScripts} initialTerminals={initialTerminals} />
            </Panel>
          </>
        ) : null}
      </Group>
    </div>
  );
});
