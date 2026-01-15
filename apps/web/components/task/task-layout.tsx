'use client';

import { memo, useState } from 'react';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@kandev/ui/resizable';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { TaskLeftPanel } from './task-left-panel';
import { TaskRightPanel } from './task-right-panel';
import { TaskFilesPanel } from './task-files-panel';
import type { OpenFileTab } from '@/lib/types/backend';

const DEFAULT_HORIZONTAL_LAYOUT: [number, number] = [75, 25];

type TaskLayoutProps = {
  taskId: string | null;
  taskDescription?: string;
  isLoadingComments: boolean;
  isAgentWorking: boolean;
  onSendMessage: (content: string) => Promise<void>;
};

export const TaskLayout = memo(function TaskLayout({
  taskId,
  taskDescription,
  isLoadingComments,
  isAgentWorking,
  onSendMessage,
}: TaskLayoutProps) {
  const [horizontalLayout, setHorizontalLayout] = useState<[number, number]>(
    getLocalStorage('task-layout-horizontal', DEFAULT_HORIZONTAL_LAYOUT)
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
      taskId={taskId}
      onSelectDiffPath={handleSelectDiffPath}
      onOpenFile={handleOpenFile}
    />
  );

  return (
    <div className="flex-1 min-h-0 px-4 pb-4">
      <ResizablePanelGroup
        direction="horizontal"
        className="h-full"
        onLayout={(sizes) => {
          setHorizontalLayout(sizes as [number, number]);
          setLocalStorage('task-layout-horizontal', sizes);
        }}
      >
        <ResizablePanel defaultSize={horizontalLayout[0]} minSize={55}>
          <TaskLeftPanel
            taskId={taskId}
            taskDescription={taskDescription}
            isLoadingComments={isLoadingComments}
            isAgentWorking={isAgentWorking}
            onSendMessage={onSendMessage}
            selectedDiffPath={selectedDiffPath}
            openFileRequest={openFileRequest}
            onDiffPathHandled={() => setSelectedDiffPath(null)}
            onFileOpenHandled={() => setOpenFileRequest(null)}
          />
        </ResizablePanel>
        <ResizableHandle className="w-px" />
        <ResizablePanel defaultSize={horizontalLayout[1]} minSize={20}>
          <TaskRightPanel topPanel={topFilesPanel} />
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  );
});

