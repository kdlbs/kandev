'use client';

import { useEffect, useMemo, useState } from 'react';
import '@git-diff-view/react/styles/diff-view.css';
import { TooltipProvider } from '@kandev/ui/tooltip';
import { TaskTopBar } from '@/components/task/task-top-bar';
import { TaskLayout } from '@/components/task/task-layout';
import { DebugOverlay } from '@/components/debug-overlay';
import type { Repository, Task } from '@/lib/types/http';
import type { KanbanState } from '@/lib/state/slices';
import { DEBUG_UI } from '@/lib/config';
import { useRepositories } from '@/hooks/domains/workspace/use-repositories';
import { useSessionAgent } from '@/hooks/domains/session/use-session-agent';
import { useSessionResumption } from '@/hooks/domains/session/use-session-resumption';
import { useSessionAgentctl } from '@/hooks/domains/session/use-session-agentctl';
import { useAppStore } from '@/components/state-provider';
import { fetchTask } from '@/lib/api';
import { useTasks } from '@/hooks/use-tasks';
import type { Layout } from 'react-resizable-panels';

type TaskPageContentProps = {
  task: Task | null;
  sessionId?: string | null;
  initialRepositories?: Repository[];
  defaultLayouts?: Record<string, Layout>;
};

/**
 * TaskPageContent is the main content component for the task/session page.
 * It handles task state, agent management, and renders the task layout.
 *
 * This is a shared component used by the session page (/s/[sessionId]).
 */
export function TaskPageContent({
  task: initialTask,
  sessionId = null,
  initialRepositories = [],
  defaultLayouts = {},
}: TaskPageContentProps) {
  const [taskDetails, setTaskDetails] = useState<Task | null>(initialTask);
  const [isMounted, setIsMounted] = useState(false);
  const [showDebugOverlay, setShowDebugOverlay] = useState(false);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const effectiveTaskId = activeTaskId ?? initialTask?.id ?? null;
  const kanbanTask = useAppStore((state) =>
    effectiveTaskId ? state.kanban.tasks.find((item: KanbanState['tasks'][number]) => item.id === effectiveTaskId) ?? null : null
  );
  const task = useMemo(() => {
    const baseTask = taskDetails ?? initialTask;
    if (!baseTask) return null;
    if (!kanbanTask) return baseTask;
    return {
      ...baseTask,
      title: kanbanTask.title ?? baseTask.title,
      description: kanbanTask.description ?? baseTask.description,
      workflow_step_id: (kanbanTask.workflowStepId as string | undefined) ?? baseTask.workflow_step_id,
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
  } = useSessionAgent(task);

  const initialSessionId = sessionId ?? taskSessionId ?? null;
  const effectiveSessionId = activeSessionId ?? initialSessionId;
  const storeSessionState = useAppStore((state) =>
    effectiveSessionId ? state.taskSessions.items[effectiveSessionId]?.state ?? null : null
  );
  const agentctlStatus = useSessionAgentctl(effectiveSessionId);
  const previewOpen = useAppStore((state) =>
    effectiveSessionId ? state.previewPanel.openBySessionId[effectiveSessionId] ?? false : false
  );
  const previewStage = useAppStore((state) =>
    effectiveSessionId ? state.previewPanel.stageBySessionId[effectiveSessionId] ?? 'closed' : 'closed'
  );
  const previewUrl = useAppStore((state) =>
    effectiveSessionId ? state.previewPanel.urlBySessionId[effectiveSessionId] ?? '' : ''
  );
  const devProcessId = useAppStore((state) =>
    effectiveSessionId ? state.processes.devProcessBySessionId[effectiveSessionId] : undefined
  );
  const devProcessStatus = useAppStore((state) =>
    devProcessId ? state.processes.processesById[devProcessId]?.status ?? null : null
  );

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
    // Sync SSR data to store on mount or when SSR props change.
    // Don't include activeTaskId in deps - we only want to sync FROM SSR, not react to store changes.
    if (initialSessionId) {
      setActiveSession(initialTask.id, initialSessionId);
    } else {
      setActiveTask(initialTask.id);
    }
  }, [initialTask?.id, initialSessionId, setActiveSession, setActiveTask]);

  useEffect(() => {
    if (!activeTaskId) return;
    if (taskDetails?.id === activeTaskId) return;
    fetchTask(activeTaskId, { cache: 'no-store' })
      .then((response) => setTaskDetails(response))
      .catch((error) => console.error('[TaskPageContent] Failed to load task details:', error));
  }, [activeTaskId, taskDetails?.id]);
  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const effectiveRepositories = repositories.length ? repositories : initialRepositories;
  const repository = useMemo(
    () => effectiveRepositories.find((item: Repository) => item.id === task?.repositories?.[0]?.repository_id) ?? null,
    [effectiveRepositories, task?.repositories]
  );


  if (!isMounted) {
    return <div className="h-screen w-full bg-background" />;
  }

  return (
    <TooltipProvider>
      <div className="h-screen w-full flex flex-col bg-background">
        {DEBUG_UI && showDebugOverlay && (
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
              preview_open: previewOpen,
              preview_stage: previewStage,
              preview_url: previewUrl || null,
              dev_process_id: devProcessId ?? null,
              dev_process_status: devProcessStatus ?? null,
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
          hasDevScript={Boolean(repository?.dev_script?.trim())}
          showDebugOverlay={showDebugOverlay}
          onToggleDebugOverlay={() => setShowDebugOverlay((prev) => !prev)}
        />

        <TaskLayout
          workspaceId={task?.workspace_id ?? null}
          boardId={task?.board_id ?? null}
          sessionId={effectiveSessionId ?? null}
          repository={repository ?? null}
          defaultLayouts={defaultLayouts}
        />
      </div>
    </TooltipProvider>
  );
}
