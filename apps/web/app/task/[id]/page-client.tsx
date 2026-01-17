'use client';

import { useCallback, useEffect, useMemo, useState } from 'react';
import '@git-diff-view/react/styles/diff-view.css';
import { TooltipProvider } from '@kandev/ui/tooltip';
import { TaskTopBar } from '@/components/task/task-top-bar';
import { TaskLayout } from '@/components/task/task-layout';
import { CommandApprovalDialog } from '@/components/task/command-approval-dialog';
import { DebugOverlay } from '@/components/debug-overlay';
import type { Repository, Task, TaskSession } from '@/lib/types/http';
import { DEBUG_UI } from '@/lib/config';
import { getWebSocketClient } from '@/lib/ws/connection';
import { useRepositories } from '@/hooks/use-repositories';
import { useTaskAgent } from '@/hooks/use-task-agent';
import { useTaskMessages } from '@/hooks/use-task-messages';
import { useSessionResumption } from '@/hooks/use-session-resumption';
import { useAppStore } from '@/components/state-provider';
import { fetchTask, listTaskSessions } from '@/lib/http';
import { linkToTaskSession } from '@/lib/links';

type TaskPageClientProps = {
  task: Task | null;
  sessionId?: string | null;
  initialSessionsByTask?: Record<string, TaskSession[]>;
  initialRepositories?: Repository[];
  initialAgentProfiles?: Array<{ id: string; label: string; agent_id: string }>;
};

export default function TaskPage({
  task: initialTask,
  sessionId = null,
  initialSessionsByTask = {},
  initialRepositories = [],
  initialAgentProfiles = [],
}: TaskPageClientProps) {
  const [activeTaskOverride, setActiveTaskOverride] = useState<Task | null>(null);
  const [activeSessionOverride, setActiveSessionOverride] = useState<string | null>(null);
  const [isMounted, setIsMounted] = useState(false);
  const effectiveTaskId = activeTaskOverride?.id ?? initialTask?.id ?? null;
  const kanbanTask = useAppStore((state) =>
    effectiveTaskId ? state.kanban.tasks.find((item) => item.id === effectiveTaskId) ?? null : null
  );
  const boardTasks = useAppStore((state) => state.kanban.tasks);
  const boardColumns = useAppStore((state) => state.kanban.columns);
  const workspaces = useAppStore((state) => state.workspaces.items);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const task = useMemo(() => {
    const baseTask = activeTaskOverride ?? initialTask;
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
  }, [activeTaskOverride, initialTask, kanbanTask]);
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
  const activeSessionId = activeSessionOverride ?? sessionId ?? taskSessionId;
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
  useTaskMessages(task?.id ?? null, activeSessionId);
  const [sessionsByTask, setSessionsByTask] = useState<Record<string, TaskSession[]>>(
    initialSessionsByTask
  );

  useEffect(() => {
    queueMicrotask(() => setIsMounted(true));
  }, []);
  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const effectiveRepositories = repositories.length ? repositories : initialRepositories;
  const repositoryPathsById = useMemo(
    () => Object.fromEntries(effectiveRepositories.map((repo) => [repo.id, repo.local_path])),
    [effectiveRepositories]
  );
  const repository = useMemo(
    () => effectiveRepositories.find((item) => item.id === task?.repositories?.[0]?.repository_id) ?? null,
    [effectiveRepositories, task?.repositories]
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

  const updateUrl = useCallback((taskId: string, sessionIdToOpen: string) => {
    if (typeof window === 'undefined') return;
    window.history.replaceState({}, '', linkToTaskSession(taskId, sessionIdToOpen));
  }, []);

  const handleSelectSession = useCallback(
    (taskId: string, sessionIdToOpen: string) => {
      setActiveSessionOverride(sessionIdToOpen);
      updateUrl(taskId, sessionIdToOpen);
    },
    [updateUrl]
  );

  const handleLoadTaskSessions = useCallback(async (taskId: string) => {
    try {
      const response = await listTaskSessions(taskId, { cache: 'no-store' });
      setSessionsByTask((prev) => ({ ...prev, [taskId]: response.sessions }));
    } catch (error) {
      console.error('Failed to load task sessions:', error);
    }
  }, []);

  const handleSelectTask = useCallback(
    async (taskId: string, sessionId: string | null) => {
      try {
        const nextTask = await fetchTask(taskId, { cache: 'no-store' });
        setActiveTaskOverride(nextTask);
      } catch (error) {
        console.error('Failed to load task details:', error);
      }

      let targetSessionId = sessionId;
      if (!targetSessionId) {
        try {
          const response = await listTaskSessions(taskId, { cache: 'no-store' });
          targetSessionId = response.sessions[0]?.id ?? null;
        } catch (error) {
          console.error('Failed to load task sessions for navigation:', error);
        }
      }

      if (!targetSessionId) {
        setActiveSessionOverride(null);
        return;
      }

      setActiveSessionOverride(targetSessionId);
      updateUrl(taskId, targetSessionId);
    },
    [updateUrl]
  );

  const handleCreateSessionForTask = useCallback(
    async (
      taskId: string,
      data: { prompt: string; agentProfileId: string; executorId: string; environmentId: string }
    ) => {
      const client = getWebSocketClient();
      if (!client) return;

      try {
        const response = await client.request<{
          success: boolean;
          task_id: string;
          agent_instance_id: string;
          task_session_id?: string;
          state: string;
        }>(
          'orchestrator.start',
          { task_id: taskId, agent_profile_id: data.agentProfileId },
          15000
        );

        if (response?.task_session_id && data.prompt.trim()) {
          await client.request(
            'message.add',
            { task_id: taskId, task_session_id: response.task_session_id, content: data.prompt.trim() },
            10000
          );
        }

        await handleLoadTaskSessions(taskId);

        if (response?.task_session_id) {
          setActiveSessionOverride(response.task_session_id);
          updateUrl(taskId, response.task_session_id);
        }
      } catch (error) {
        console.error('Failed to create task session:', error);
      }
    },
    [handleLoadTaskSessions, updateUrl]
  );

  const workspaceName = useMemo(() => {
    if (!task?.workspace_id) return 'Workspace';
    return workspaces.find((workspace) => workspace.id === task.workspace_id)?.name ?? 'Workspace';
  }, [task, workspaces]);

  const effectiveAgentProfiles = agentProfiles.length ? agentProfiles : initialAgentProfiles;
  const agentLabelsById = useMemo(() => {
    return Object.fromEntries(effectiveAgentProfiles.map((profile) => [profile.id, profile.label]));
  }, [effectiveAgentProfiles]);

  const tasksWithRepositories = useMemo(
    () =>
      boardTasks.map((taskItem) => ({
        ...taskItem,
        repositoryPath: taskItem.repositoryId ? repositoryPathsById[taskItem.repositoryId] : undefined,
      })),
    [boardTasks, repositoryPathsById]
  );

  useEffect(() => {
    if (!task?.id) return;
    if (sessionsByTask[task.id]) return;
    // eslint-disable-next-line react-hooks/set-state-in-effect
    handleLoadTaskSessions(task.id);
  }, [handleLoadTaskSessions, sessionsByTask, task?.id]);

  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;

    const taskIds = boardTasks.map((item) => item.id);
    taskIds.forEach((taskId) => client.subscribe(taskId));

    return () => {
      taskIds.forEach((taskId) => client.unsubscribe(taskId));
    };
  }, [boardTasks]);

  useEffect(() => {
    const client = getWebSocketClient();
    if (!client) return;

    const pending = new Set<string>();
    const unsubscribe = client.on('task_session.state_changed', (message) => {
      const taskId = message.payload?.task_id;
      const sessionId = message.payload?.task_session_id;
      const payload = message.payload as Record<string, unknown> | undefined;
      const nextState = (payload?.new_state ?? payload?.state) as TaskSession['state'] | undefined;
      if (taskId && sessionId && nextState) {
        setSessionsByTask((prev) => {
          const sessions = prev[taskId];
          if (!sessions) return prev;
          const nextSessions = sessions.map((session) =>
            session.id === sessionId ? { ...session, state: nextState } : session
          );
          return { ...prev, [taskId]: nextSessions };
        });
      }
      if (!taskId || pending.has(taskId)) return;
      pending.add(taskId);
      setTimeout(() => {
        pending.delete(taskId);
        handleLoadTaskSessions(taskId);
      }, 250);
    });

    return () => {
      unsubscribe();
    };
  }, [handleLoadTaskSessions]);

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
          tasks={tasksWithRepositories}
          columns={boardColumns.map((column) => ({ id: column.id, title: column.title }))}
          workspaceId={task?.workspace_id ?? null}
          boardId={task?.board_id ?? null}
          workspaceName={workspaceName}
          agentLabelsById={agentLabelsById}
          activeSessionId={activeSessionId ?? null}
          sessionsByTask={sessionsByTask}
          onSelectSession={handleSelectSession}
          onLoadTaskSessions={handleLoadTaskSessions}
          onSelectTask={handleSelectTask}
          onCreateSession={handleCreateSessionForTask}
        />

        {/* Fallback dialog for permissions without matching tool call (e.g., workspace indexing) */}
        {task?.id && <CommandApprovalDialog taskId={task.id} standaloneOnly />}
      </div>
    </TooltipProvider>
  );
}
