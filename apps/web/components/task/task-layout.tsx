'use client';

import { memo, useMemo, useState, useCallback } from 'react';
import { Group, Panel, type Layout } from 'react-resizable-panels';
import { useDefaultLayout } from '@/lib/layout/use-default-layout';
import { TaskCenterPanel } from './task-center-panel';
import { TaskRightPanel } from './task-right-panel';
import { TaskFilesPanel } from './task-files-panel';
import { TaskSessionSidebar } from './task-session-sidebar';
import { PreviewPanel } from '@/components/task/preview/preview-panel';
import { PreviewController } from '@/components/task/preview/preview-controller';
import { useLayoutStore } from '@/lib/state/layout-store';
import type { OpenFileTab } from '@/lib/types/backend';
import type { Repository } from '@/lib/types/http';
import { useAppStore } from '@/components/state-provider';

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
  defaultLayouts?: Record<string, Layout>;
};

export type SelectedDiff = {
  path: string;
  content?: string; // Optional: if provided, use this instead of looking up from git status
};

export const TaskLayout = memo(function TaskLayout({
  workspaceId,
  boardId,
  sessionId = null,
  repository = null,
  defaultLayouts = {},
}: TaskLayoutProps) {
  const [selectedDiff, setSelectedDiff] = useState<SelectedDiff | null>(null);
  const [openFileRequest, setOpenFileRequest] = useState<OpenFileTab | null>(null);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const layoutBySession = useLayoutStore((state) => state.columnsBySessionId);
  const sessionKey = activeSessionId ?? sessionId ?? '';
  const layoutState = useMemo(
    () => layoutBySession[sessionKey] ?? DEFAULT_LAYOUT_STATE,
    [layoutBySession, sessionKey]
  );
  const hasDevScript = Boolean(repository?.dev_script?.trim());

  const handleSelectDiff = useCallback((path: string, content?: string) => {
    setSelectedDiff({ path, content });
  }, []);

  const handleOpenFile = useCallback((file: OpenFileTab) => {
    setOpenFileRequest(file);
  }, []);

  const handleDiffHandled = useCallback(() => {
    setSelectedDiff(null);
  }, []);

  const handleFileOpenHandled = useCallback(() => {
    setOpenFileRequest(null);
  }, []);

  const topFilesPanel = (
    <TaskFilesPanel
      onSelectDiff={handleSelectDiff}
      onOpenFile={handleOpenFile}
    />
  );

  const sessionForPreview = activeSessionId ?? sessionId ?? null;

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
  const previewLayoutKey = `task-layout-preview-v2:${sessionKey}`;
  const { defaultLayout: defaultPreviewLayout, onLayoutChanged: onPreviewLayoutChange } =
    useDefaultLayout({
      id: previewLayoutKey,
      panelIds: previewPanelIds,
      baseLayout: DEFAULT_PREVIEW_LAYOUT,
      serverDefaultLayout: defaultLayouts[previewLayoutKey],
    });

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
                  onDiffHandled={handleDiffHandled}
                  onFileOpenHandled={handleFileOpenHandled}
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
              onDiffHandled={handleDiffHandled}
              onFileOpenHandled={handleFileOpenHandled}
              sessionId={sessionId}
            />
          </Panel>
        )}

        {layoutState.right ? (
          <>
            <Panel id="right" minSize="310px" className="min-h-0 min-w-0">
              <TaskRightPanel topPanel={topFilesPanel} sessionId={sessionForPreview} />
            </Panel>
          </>
        ) : null}
      </Group>
    </div>
  );
});
