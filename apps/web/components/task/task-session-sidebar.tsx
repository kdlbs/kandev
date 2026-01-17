'use client';

import { useEffect, useState } from 'react';
import type { TaskSession, TaskState } from '@/lib/types/http';
import { TaskSessionSwitcher } from './task-session-switcher';
import { TaskSwitcher } from './task-switcher';
import { Button } from '@kandev/ui/button';
import { IconPlus } from '@tabler/icons-react';
import { TaskCreateDialog } from '@/components/task-create-dialog';
import { useAppStore, useAppStoreApi } from '@/components/state-provider';

type TaskSummary = {
  id: string;
  title: string;
  state?: TaskState;
  description?: string;
  columnId?: string;
  repositoryPath?: string;
};

type TaskSessionSidebarProps = {
  workspaceName: string;
  tasks: TaskSummary[];
  columns: Array<{ id: string; title: string }>;
  workspaceId: string | null;
  boardId: string | null;
  sessionsByTask: Record<string, TaskSession[]>;
  agentLabelsById: Record<string, string>;
  onSelectSession: (taskId: string, sessionId: string) => void;
  onLoadTaskSessions: (taskId: string) => void;
  onSelectTask: (taskId: string, sessionId: string | null) => void;
  onCreateSession: (taskId: string, data: { prompt: string; agentProfileId: string; executorId: string; environmentId: string }) => void;
};

export function TaskSessionSidebar({
  workspaceName,
  tasks,
  columns,
  workspaceId,
  boardId,
  sessionsByTask,
  agentLabelsById,
  onSelectSession,
  onLoadTaskSessions,
  onSelectTask,
  onCreateSession,
}: TaskSessionSidebarProps) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const [selectedTaskId, setSelectedTaskId] = useState(activeTaskId);
  const selectedSessions = selectedTaskId ? sessionsByTask[selectedTaskId] ?? [] : [];
  const [taskDialogOpen, setTaskDialogOpen] = useState(false);
  const [sessionDialogOpen, setSessionDialogOpen] = useState(false);
  const [sessionTask, setSessionTask] = useState<TaskSummary | null>(null);
  const store = useAppStoreApi();

  useEffect(() => {
    setSelectedTaskId(activeTaskId);
  }, [activeTaskId]);

  return (
    <>
      <div className="h-full min-h-0 min-w-0 bg-card p-4 flex flex-col rounded-lg border border-border/70 border-r-0 mr-[5px]">
        <div className="flex items-center justify-between">
          <span className="text-sm font-medium truncate text-muted-foreground">{workspaceName || 'Workspace'}</span>
          <Button
            size="sm"
            variant="outline"
            className="h-6 gap-1 cursor-pointer"
            onClick={() => setTaskDialogOpen(true)}
          >
            <IconPlus className="h-4 w-4" />
            Task
          </Button>
        </div>
        <div className="flex-1 min-h-0 overflow-y-auto space-y-4 pt-3">
          <TaskSessionSwitcher
            taskId={selectedTaskId}
            activeSessionId={activeSessionId}
            sessions={selectedSessions}
            agentLabelsById={agentLabelsById}
            onSelectSession={onSelectSession}
            onCreateSession={() => {
              if (!selectedTaskId) return;
              const task = tasks.find((item) => item.id === selectedTaskId) ?? null;
              setSessionTask(task);
              setSessionDialogOpen(true);
            }}
          />
          <TaskSwitcher
            tasks={tasks}
            columns={columns}
            activeTaskId={activeTaskId}
            selectedTaskId={selectedTaskId}
            sessionsByTask={sessionsByTask}
            onSelectTask={(taskId, sessionId) => {
              setSelectedTaskId(taskId);
              onSelectTask(taskId, sessionId);
            }}
            onLoadTaskSessions={onLoadTaskSessions}
          />
        </div>
      </div>
      <TaskCreateDialog
        open={taskDialogOpen}
        onOpenChange={setTaskDialogOpen}
        mode="task"
        workspaceId={workspaceId}
        boardId={boardId}
        defaultColumnId={columns[0]?.id ?? null}
        columns={columns.map((column) => ({ id: column.id, title: column.title }))}
        onSuccess={(task) => {
          store.setState((state) => {
            if (state.kanban.boardId !== task.board_id) return state;
            const nextTask = {
              id: task.id,
              columnId: task.column_id,
              title: task.title,
              description: task.description,
              position: task.position ?? 0,
              state: task.state,
              repositoryId: task.repositories?.[0]?.repository_id ?? undefined,
            };
            return {
              ...state,
              kanban: {
                ...state.kanban,
                tasks: state.kanban.tasks.some((item) => item.id === task.id)
                  ? state.kanban.tasks.map((item) => (item.id === task.id ? nextTask : item))
                  : [...state.kanban.tasks, nextTask],
              },
            };
          });
          setSelectedTaskId(task.id);
          onSelectTask(task.id, null);
        }}
      />
      <TaskCreateDialog
        open={sessionDialogOpen}
        onOpenChange={setSessionDialogOpen}
        mode="session"
        workspaceId={workspaceId}
        boardId={boardId}
        defaultColumnId={columns[0]?.id ?? null}
        columns={columns.map((column) => ({ id: column.id, title: column.title }))}
        initialValues={{
          title: sessionTask?.title ?? '',
          description: sessionTask?.description ?? '',
        }}
        onCreateSession={(data) => {
          if (!sessionTask) return;
          onCreateSession(sessionTask.id, data);
        }}
      />
    </>
  );
}
