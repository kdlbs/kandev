'use client';

import { memo, useMemo } from 'react';
import { Group, Panel } from 'react-resizable-panels';
import { SessionTaskSwitcherSheet } from './session-task-switcher-sheet';
import { TaskCenterPanel } from '../task-center-panel';
import { TaskRightPanel } from '../task-right-panel';
import { TaskFilesPanel } from '../task-files-panel';
import { PreviewPanel } from '@/components/task/preview/preview-panel';
import { PreviewController } from '@/components/task/preview/preview-controller';
import { useDefaultLayout } from '@/lib/layout/use-default-layout';
import { useLayoutStore } from '@/lib/state/layout-store';
import { useSessionLayoutState } from '@/hooks/use-session-layout-state';
import type { Repository } from '@/lib/types/http';
import type { Layout } from 'react-resizable-panels';
import type { OpenFileTab } from '@/lib/types/backend';

const DEFAULT_TABLET_LAYOUT: Record<string, number> = {
  left: 60,
  right: 40,
};

const DEFAULT_PREVIEW_LAYOUT: Record<string, number> = {
  chat: 60,
  preview: 40,
};

type SessionTabletLayoutProps = {
  workspaceId: string | null;
  workflowId: string | null;
  sessionId?: string | null;
  repository?: Repository | null;
  defaultLayouts?: Record<string, Layout>;
};

type TabletLeftPanelProps = {
  layoutStatePreview: boolean;
  previewLayoutKey: string;
  defaultPreviewLayout: Layout | undefined;
  onPreviewLayoutChange: (layout: Layout) => void;
  selectedDiff: { path: string; content?: string } | null;
  openFileRequest: OpenFileTab | null;
  handleClearSelectedDiff: () => void;
  handleFileOpenHandled: () => void;
  sessionId?: string | null;
  sessionForPreview: string | null;
  hasDevScript: boolean;
};

function TabletLeftPanel({
  layoutStatePreview, previewLayoutKey, defaultPreviewLayout, onPreviewLayoutChange,
  selectedDiff, openFileRequest, handleClearSelectedDiff, handleFileOpenHandled,
  sessionId, sessionForPreview, hasDevScript,
}: TabletLeftPanelProps) {
  const centerPanel = (
    <TaskCenterPanel
      selectedDiff={selectedDiff}
      openFileRequest={openFileRequest}
      onDiffHandled={handleClearSelectedDiff}
      onFileOpenHandled={handleFileOpenHandled}
    />
  );
  if (layoutStatePreview) {
    return (
      <Group orientation="horizontal" className="h-full min-h-0 min-w-0" id={previewLayoutKey} key={previewLayoutKey} defaultLayout={defaultPreviewLayout} onLayoutChanged={onPreviewLayoutChange}>
        <Panel id="chat" minSize="300px" className="min-h-0 min-w-0">{centerPanel}</Panel>
        <Panel id="preview" minSize="300px" className="min-h-0 min-w-0">
          <PreviewPanel sessionId={sessionForPreview} hasDevScript={hasDevScript} />
        </Panel>
      </Group>
    );
  }
  return (
    <TaskCenterPanel
      selectedDiff={selectedDiff}
      openFileRequest={openFileRequest}
      onDiffHandled={handleClearSelectedDiff}
      onFileOpenHandled={handleFileOpenHandled}
      sessionId={sessionId}
    />
  );
}

export const SessionTabletLayout = memo(function SessionTabletLayout({
  workspaceId,
  workflowId,
  sessionId = null,
  repository = null,
  defaultLayouts = {},
}: SessionTabletLayoutProps) {
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
    isTaskSwitcherOpen,
    setMobileSessionTaskSwitcherOpen,
  } = useSessionLayoutState({ sessionId });

  const layoutBySession = useLayoutStore((state) => state.columnsBySessionId);
  const layoutState = useMemo(
    () => layoutBySession[sessionKey] ?? { left: true, chat: true, right: true, preview: false },
    [layoutBySession, sessionKey]
  );

  const hasDevScript = Boolean(repository?.dev_script?.trim());
  const sessionForPreview = effectiveSessionId;

  const topFilesPanel = (
    <TaskFilesPanel
      onSelectDiff={handleSelectDiff}
      onOpenFile={handleOpenFile}
    />
  );

  // Tablet layout: two columns (left for chat/plan/changes, right for files+terminal)
  const tabletLayoutKey = 'task-layout-tablet-v1';
  const { defaultLayout: tabletLayout, onLayoutChanged: onTabletLayoutChange } = useDefaultLayout({
    id: tabletLayoutKey,
    panelIds: ['left', 'right'],
    baseLayout: DEFAULT_TABLET_LAYOUT,
    serverDefaultLayout: defaultLayouts[tabletLayoutKey],
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

  return (
    <div className="flex-1 min-h-0 px-2 pb-2">
      <PreviewController sessionId={sessionForPreview} hasDevScript={hasDevScript} />
      <Group
        orientation="horizontal"
        className="h-full min-h-0"
        id={tabletLayoutKey}
        key={tabletLayoutKey}
        defaultLayout={tabletLayout}
        onLayoutChanged={onTabletLayoutChange}
      >
        {/* Left Panel: Chat/Plan/Changes with tabs */}
        <Panel id="left" minSize="300px" className="min-h-0 min-w-0">
          <TabletLeftPanel
            layoutStatePreview={layoutState.preview}
            previewLayoutKey={previewLayoutKey}
            defaultPreviewLayout={defaultPreviewLayout}
            onPreviewLayoutChange={onPreviewLayoutChange}
            selectedDiff={selectedDiff}
            openFileRequest={openFileRequest}
            handleClearSelectedDiff={handleClearSelectedDiff}
            handleFileOpenHandled={handleFileOpenHandled}
            sessionId={sessionId}
            sessionForPreview={sessionForPreview}
            hasDevScript={hasDevScript}
          />
        </Panel>

        {/* Right Panel: Files + Terminal stacked */}
        <Panel id="right" minSize="250px" className="min-h-0 min-w-0">
          <TaskRightPanel topPanel={topFilesPanel} sessionId={sessionForPreview} repositoryId={repository?.id ?? null} />
        </Panel>
      </Group>

      {/* Task Switcher Sheet - same as mobile */}
      <SessionTaskSwitcherSheet
        open={isTaskSwitcherOpen}
        onOpenChange={setMobileSessionTaskSwitcherOpen}
        workspaceId={workspaceId}
        workflowId={workflowId}
      />
    </div>
  );
});
