'use client';

import { memo, useState } from 'react';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@kandev/ui/resizable';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { TaskCenterPanel } from './task-center-panel';
import { TaskRightPanel } from './task-right-panel';
import { TaskFilesPanel } from './task-files-panel';
import { TaskSessionSidebar } from './task-session-sidebar';
import type { OpenFileTab } from '@/lib/types/backend';

const DEFAULT_HORIZONTAL_LAYOUT: [number, number, number] = [18, 57, 25];

type TaskLayoutProps = {
  workspaceId: string | null;
  boardId: string | null;
};

export const TaskLayout = memo(function TaskLayout({
  workspaceId,
  boardId,
}: TaskLayoutProps) {
  const [horizontalLayout, setHorizontalLayout] = useState<[number, number, number]>(
    getLocalStorage('task-layout-horizontal-v2', DEFAULT_HORIZONTAL_LAYOUT)
  );
  const [selectedDiffPath, setSelectedDiffPath] = useState<string | null>(null);
  const [openFileRequest, setOpenFileRequest] = useState<OpenFileTab | null>(null);

  const handleSelectDiffPath = (path: string) => {
    setSelectedDiffPath(path);
  };

  const handleOpenFile = (file: OpenFileTab) => {
    setOpenFileRequest(file);
  };

  const topFilesPanel = (
    <TaskFilesPanel
      onSelectDiffPath={handleSelectDiffPath}
      onOpenFile={handleOpenFile}
    />
  );

  return (
    <div className="flex-1 min-h-0 px-4 pb-4">
      <ResizablePanelGroup
        direction="horizontal"
        className="h-full min-h-0 min-w-0"
        onLayout={(sizes) => {
          setHorizontalLayout(sizes as [number, number, number]);
          setLocalStorage('task-layout-horizontal-v2', sizes);
        }}
      >
        <ResizablePanel defaultSize={horizontalLayout[0]} minSize={12} className="min-h-0 min-w-0">
          <TaskSessionSidebar
            workspaceId={workspaceId}
            boardId={boardId}
          />
        </ResizablePanel>
        <ResizableHandle className="w-px" />
        <ResizablePanel defaultSize={horizontalLayout[1]} minSize={45} className="min-h-0 min-w-0">
          <TaskCenterPanel
            selectedDiffPath={selectedDiffPath}
            openFileRequest={openFileRequest}
            onDiffPathHandled={() => setSelectedDiffPath(null)}
            onFileOpenHandled={() => setOpenFileRequest(null)}
          />
        </ResizablePanel>
        <ResizableHandle className="w-px" />
        <ResizablePanel defaultSize={horizontalLayout[2]} minSize={20} className="min-h-0 min-w-0">
          <TaskRightPanel topPanel={topFilesPanel} />
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  );
});
