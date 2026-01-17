'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import '@git-diff-view/react/styles/diff-view.css';
import { TooltipProvider } from '@kandev/ui/tooltip';
import { TaskTopBar } from '@/components/task/task-top-bar';
import { TaskLayout } from '@/components/task/task-layout';
import { CommandApprovalDialog } from '@/components/task/command-approval-dialog';
import { DebugOverlay } from '@/components/debug-overlay';
import type { Task } from '@/lib/types/http';
import { DEBUG_UI } from '@/lib/config';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useRepositories } from '@/hooks/use-repositories';
import { useTaskAgent } from '@/hooks/use-task-agent';
import { useSessionResumption } from '@/hooks/use-session-resumption';
import { useAppStore } from '@/components/state-provider';

type TaskPageClientProps = {
  task: Task | null;
  sessionId?: string | null;
};

export default function TaskPage({ task: initialTask, sessionId = null }: TaskPageClientProps) {
  const [isMounted, setIsMounted] = useState(false);
  const kanbanTask = useAppStore((state) =>
    initialTask?.id ? state.kanban.tasks.find((item) => item.id === initialTask.id) ?? null : null
  );
  const task = useMemo(() => {
    if (!initialTask) return null;
    if (!kanbanTask) return initialTask;
    return {
      ...initialTask,
      title: kanbanTask.title ?? initialTask.title,
      description: kanbanTask.description ?? initialTask.description,
      column_id: (kanbanTask.columnId as string | undefined) ?? initialTask.column_id,
      position: kanbanTask.position ?? initialTask.position,
      state: (kanbanTask.state as Task['state'] | undefined) ?? initialTask.state,
      repositories: initialTask.repositories,
    };
  }, [initialTask, kanbanTask]);
  const connectionStatus = useAppStore((state) => state.connection.status);

  // Session resumption hook - handles auto-resume on page reload
  const {
    resumptionState,
    error: resumptionError,
    taskSessionState: resumedSessionState,
    worktreePath: resumedWorktreePath,
    worktreeBranch: resumedWorktreeBranch,
  } = useSessionResumption(task?.id ?? null, sessionId);

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

  // Merge state from resumption and regular agent hooks
  const activeSessionId = sessionId ?? taskSessionId;
  const isResuming = resumptionState === 'checking' || resumptionState === 'resuming';
  const isResumed = resumptionState === 'resumed' || resumptionState === 'running';

  // Both hooks read taskSessionState from the store, so prefer resumedSessionState as it's always
  // updated when we have a sessionId from URL. For worktree, use resumed values when resuming/resumed.
  const taskSessionState = resumedSessionState ?? agentSessionState;
  const worktreePath = sessionId ? (resumedWorktreePath ?? agentWorktreePath) : agentWorktreePath;
  const worktreeBranch = sessionId ? (resumedWorktreeBranch ?? agentWorktreeBranch) : agentWorktreeBranch;

  // Derive working state for debug overlay
  const isAgentWorking =
    taskSessionState !== null
      ? taskSessionState === 'STARTING' || taskSessionState === 'RUNNING'
      : isAgentRunning && (task?.state === 'IN_PROGRESS' || task?.state === 'SCHEDULING');

  useEffect(() => {
    queueMicrotask(() => setIsMounted(true));
  }, []);


  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const repository = useMemo(
    () => repositories.find((item) => item.id === task?.repositories?.[0]?.repository_id) ?? null,
    [repositories, task?.repositories]
  );

  const handleSendMessage = useCallback(async (content: string) => {
    if (!task?.id) return;

    const client = getWebSocketClient();
    if (!client) return;

    if (!activeSessionId) {
      console.error('No active agent session. Start an agent before sending a message.');
      return;
    }

    try {
      await client.request(
        'message.add',
        { task_id: task.id, task_session_id: activeSessionId, content },
        10000
      );
    } catch (error) {
      console.error('Failed to send message:', error);
    }
  }, [activeSessionId, task]);

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
              task_session_id: activeSessionId ?? null,
              task_state: task?.state ?? null,
              task_session_state: taskSessionState ?? null,
              is_agent_working: isAgentWorking,
              resumption_state: resumptionState,
              resumption_error: resumptionError,
            }}
          />
        )}
        <TaskTopBar
          taskId={task?.id ?? null}
          activeSessionId={activeSessionId ?? null}
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
          taskId={task?.id ?? null}
          sessionId={activeSessionId ?? null}
          onSendMessage={handleSendMessage}
        />

        {/* Fallback dialog for permissions without matching tool call (e.g., workspace indexing) */}
        {task?.id && <CommandApprovalDialog taskId={task.id} standaloneOnly />}
      </div>
    </TooltipProvider>
  );
}
