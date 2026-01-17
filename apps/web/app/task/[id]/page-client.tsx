'use client';

import { useEffect, useMemo, useState } from 'react';
import '@git-diff-view/react/styles/diff-view.css';
import { TooltipProvider } from '@kandev/ui/tooltip';
import { TaskTopBar } from '@/components/task/task-top-bar';
import { TaskLayout } from '@/components/task/task-layout';
import { DebugOverlay } from '@/components/debug-overlay';
import type { Repository, Task } from '@/lib/types/http';
import { DEBUG_UI } from '@/lib/config';
import { useRepositories } from '@/hooks/use-repositories';
import { useTaskAgent } from '@/hooks/use-task-agent';
import { useSessionResumption } from '@/hooks/use-session-resumption';
import { useSessionAgentctl } from '@/hooks/use-session-agentctl';
import { useAppStore } from '@/components/state-provider';
import { fetchTask } from '@/lib/http';
import { useTasks } from '@/hooks/use-tasks';

type TaskPageClientProps = {
  task: Task | null;
  sessionId?: string | null;
  initialRepositories?: Repository[];
};

export default function TaskPage({
  task: initialTask,
  sessionId = null,
  initialRepositories = [],
}: TaskPageClientProps) {
  const [taskDetails, setTaskDetails] = useState<Task | null>(initialTask);
  const [isMounted, setIsMounted] = useState(false);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const effectiveTaskId = activeTaskId ?? initialTask?.id ?? null;
  const kanbanTask = useAppStore((state) =>
    effectiveTaskId ? state.kanban.tasks.find((item) => item.id === effectiveTaskId) ?? null : null
  );
  const task = useMemo(() => {
    const baseTask = taskDetails ?? initialTask;
    if (!baseTask) return null;
    if (!kanbanTask) return baseTask;
    return {
      ...baseTask,
      title: kanbanTask.title ?? baseTask.title,
      description: kanbanTask.description ?? baseTask.description,
      column_id: (kanbanTask.columnId as string | undefined) ?? baseTask.column_id,
      position: kanbanTask.position ?? baseTask.position,
      state: (kanbanTask.state as Task['state'] | undefined) ?? baseTask.state,
      repositories: baseTask.repositories,
    };
  }, [taskDetails, initialTask, kanbanTask]);
  useTasks(task?.board_id ?? null);
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Regular task agent hook for non-resumed sessions
  const {
    isAgentRunning,
    isAgentLoading,
    worktreePath: agentWorktreePath,
    worktreeBranch: agentWorktreeBranch,
    taskSessionId,
    taskSessionState: agentSessionState,
    handleStartAgent,
    handleStopAgent,
  } = useTaskAgent(task);

  const initialSessionId = sessionId ?? taskSessionId ?? null;
  const effectiveSessionId = activeSessionId ?? initialSessionId;
  const storeSessionState = useAppStore((state) =>
    effectiveSessionId ? state.taskSessions.items[effectiveSessionId]?.state ?? null : null
  );
  const agentctlStatus = useSessionAgentctl(effectiveSessionId);

  // Session resumption hook - handles auto-resume on page reload
  const {
    resumptionState,
    error: resumptionError,
    worktreePath: resumedWorktreePath,
    worktreeBranch: resumedWorktreeBranch,
  } = useSessionResumption(task?.id ?? null, effectiveSessionId);

  // Merge state from resumption and regular agent hooks
  const isResuming = resumptionState === 'checking' || resumptionState === 'resuming';
  const isResumed = resumptionState === 'resumed' || resumptionState === 'running';

  // Both hooks read taskSessionState from the store, so prefer resumedSessionState as it's always
  // updated when we have a sessionId from URL. For worktree, use resumed values when resuming/resumed.
  const taskSessionState = storeSessionState ?? agentSessionState;
  const worktreePath = effectiveSessionId ? (resumedWorktreePath ?? agentWorktreePath) : agentWorktreePath;
  const worktreeBranch = effectiveSessionId ? (resumedWorktreeBranch ?? agentWorktreeBranch) : agentWorktreeBranch;

  // Derive working state for debug overlay
  const isAgentWorking =
    taskSessionState !== null
      ? taskSessionState === 'STARTING' || taskSessionState === 'RUNNING'
      : isAgentRunning && (task?.state === 'IN_PROGRESS' || task?.state === 'SCHEDULING');
  // Messages are loaded inside TaskChatPanel for the active session.

  useEffect(() => {
    queueMicrotask(() => setIsMounted(true));
  }, []);

  useEffect(() => {
    if (!initialTask?.id) return;
    if (activeTaskId && activeTaskId !== initialTask.id) return;
    if (initialSessionId) {
      setActiveSession(initialTask.id, initialSessionId);
    } else {
      setActiveTask(initialTask.id);
    }
  }, [activeTaskId, initialSessionId, initialTask?.id, setActiveSession, setActiveTask]);

  useEffect(() => {
    if (!activeTaskId) return;
    if (taskDetails?.id === activeTaskId) return;
    fetchTask(activeTaskId, { cache: 'no-store' })
      .then((response) => setTaskDetails(response))
      .catch((error) => console.error('Failed to load task details:', error));
  }, [activeTaskId, taskDetails?.id]);
  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const effectiveRepositories = repositories.length ? repositories : initialRepositories;
  const repository = useMemo(
    () => effectiveRepositories.find((item) => item.id === task?.repositories?.[0]?.repository_id) ?? null,
    [effectiveRepositories, task?.repositories]
  );


  if (!isMounted) {
    return <div className="h-screen w-full bg-background" />;
  }

  return (
    <TooltipProvider>
      <div className="h-screen w-full flex flex-col bg-background">
        {DEBUG_UI && (
          <DebugOverlay
            title="Task Debug"
            entries={{
              ws_status: connectionStatus,
              task_id: task?.id ?? null,
              session_id: effectiveSessionId ?? null,
              task_state: task?.state ?? null,
              task_session_state: taskSessionState ?? null,
              is_agent_working: isAgentWorking,
              resumption_state: resumptionState,
              resumption_error: resumptionError,
              agentctl_status: agentctlStatus.status,
              agentctl_ready: agentctlStatus.isReady,
              agentctl_error: agentctlStatus.errorMessage ?? null,
              agentctl_execution_id: agentctlStatus.agentExecutionId ?? null,
            }}
          />
        )}
        <TaskTopBar
          taskId={task?.id ?? null}
          activeSessionId={effectiveSessionId ?? null}
          taskTitle={task?.title}
          taskDescription={task?.description}
          baseBranch={task?.repositories?.[0]?.base_branch ?? undefined}
          onStartAgent={handleStartAgent}
          onStopAgent={handleStopAgent}
          isAgentRunning={isAgentRunning || isResumed}
          isAgentLoading={isAgentLoading || isResuming}
          worktreePath={worktreePath}
          worktreeBranch={worktreeBranch}
          repositoryPath={repository?.local_path ?? null}
          repositoryName={repository?.name ?? null}
        />

        <TaskLayout
          workspaceId={task?.workspace_id ?? null}
          boardId={task?.board_id ?? null}
        />
      </div>
    </TooltipProvider>
  );
}
