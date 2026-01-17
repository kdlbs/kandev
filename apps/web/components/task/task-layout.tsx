'use client';

import { memo, useMemo, useState } from 'react';
import { ResizableHandle, ResizablePanel, ResizablePanelGroup } from '@kandev/ui/resizable';
import { getLocalStorage, setLocalStorage } from '@/lib/local-storage';
import { TaskCenterPanel } from './task-center-panel';
import { TaskRightPanel } from './task-right-panel';
import { TaskFilesPanel } from './task-files-panel';
import { TaskSessionSidebar } from './task-session-sidebar';
import type { OpenFileTab } from '@/lib/types/backend';
import type { TaskSession, TaskState } from '@/lib/types/http';

const DEFAULT_HORIZONTAL_LAYOUT: [number, number, number] = [18, 57, 25];

type TaskLayoutProps = {
  taskId: string | null;
  onSendMessage: (content: string) => Promise<void>;
  tasks: Array<{
    id: string;
    title: string;
    state?: TaskState;
    description?: string;
    columnId?: string;
    repositoryPath?: string;
  }>;
  columns: Array<{ id: string; title: string }>;
  workspaceId: string | null;
  boardId: string | null;
  workspaceName: string;
  agentLabelsById: Record<string, string>;
  sessionsByTask: Record<string, TaskSession[]>;
  onSelectSession: (taskId: string, sessionId: string) => void;
  onLoadTaskSessions: (taskId: string) => void;
  onSelectTask: (taskId: string, sessionId: string | null) => void;
  onCreateSession: (taskId: string, data: { prompt: string; agentProfileId: string; executorId: string; environmentId: string }) => void;
};

export const TaskLayout = memo(function TaskLayout({
  taskId,
  onSendMessage,
  tasks,
  columns,
  workspaceId,
  boardId,
  workspaceName,
  agentLabelsById,
  sessionsByTask,
  onSelectSession,
  onLoadTaskSessions,
  onSelectTask,
  onCreateSession,
}: TaskLayoutProps) {
  const [horizontalLayout, setHorizontalLayout] = useState<[number, number, number]>(
    getLocalStorage('task-layout-horizontal-v2', DEFAULT_HORIZONTAL_LAYOUT)
  );
  const [selectedDiffPath, setSelectedDiffPath] = useState<string | null>(null);
  const [openFileRequest, setOpenFileRequest] = useState<OpenFileTab | null>(null);

  const switcherTasks = useMemo(
    () =>
      tasks.map((task) => ({
        id: task.id,
        title: task.title,
        state: task.state,
        description: task.description,
        columnId: task.columnId,
        repositoryPath: task.repositoryPath,
      })),
    [tasks]
  );

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
        className="h-full min-h-0 min-w-0"
        onLayout={(sizes) => {
          setHorizontalLayout(sizes as [number, number, number]);
          setLocalStorage('task-layout-horizontal-v2', sizes);
        }}
      >
        <ResizablePanel defaultSize={horizontalLayout[0]} minSize={12} className="min-h-0 min-w-0">
          <TaskSessionSidebar
            workspaceName={workspaceName}
            tasks={switcherTasks}
            columns={columns}
            workspaceId={workspaceId}
            boardId={boardId}
            sessionsByTask={sessionsByTask}
            onSelectSession={onSelectSession}
            onLoadTaskSessions={onLoadTaskSessions}
            agentLabelsById={agentLabelsById}
            onSelectTask={onSelectTask}
            onCreateSession={onCreateSession}
          />
        </ResizablePanel>
        <ResizableHandle className="w-px" />
        <ResizablePanel defaultSize={horizontalLayout[1]} minSize={45} className="min-h-0 min-w-0">
          <TaskCenterPanel
            taskId={taskId}
            onSendMessage={onSendMessage}
            selectedDiffPath={selectedDiffPath}
            openFileRequest={openFileRequest}
            onDiffPathHandled={() => setSelectedDiffPath(null)}
            onFileOpenHandled={() => setOpenFileRequest(null)}
          />
        </ResizablePanel>
        <ResizableHandle className="w-px" />
        <ResizablePanel defaultSize={horizontalLayout[2]} minSize={20} className="min-h-0 min-w-0">
          <TaskRightPanel topPanel={topFilesPanel} taskId={taskId ?? ''} />
        </ResizablePanel>
      </ResizablePanelGroup>
    </div>
  );
});
