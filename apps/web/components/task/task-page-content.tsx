'use client';

import { useEffect, useMemo, useState } from 'react';
import { TaskTopBar } from '@/components/task/task-top-bar';
import { TaskLayout } from '@/components/task/task-layout';
import { DebugOverlay } from '@/components/debug-overlay';
import type { Repository, RepositoryScript, Task } from '@/lib/types/http';
import type { Terminal } from '@/hooks/domains/session/use-terminals';
import type { KanbanState } from '@/lib/state/slices';
import { DEBUG_UI } from '@/lib/config';
import { TooltipProvider } from '@kandev/ui/tooltip';
import { useRepositories } from '@/hooks/domains/workspace/use-repositories';
import { useSessionAgent } from '@/hooks/domains/session/use-session-agent';
import { useSessionResumption } from '@/hooks/domains/session/use-session-resumption';
import { useSessionAgentctl } from '@/hooks/domains/session/use-session-agentctl';
import { useAppStore } from '@/components/state-provider';
import { fetchTask } from '@/lib/api';
import { useTasks } from '@/hooks/use-tasks';
import { useResponsiveBreakpoint } from '@/hooks/use-responsive-breakpoint';
import type { Layout } from 'react-resizable-panels';
import { TaskArchivedProvider } from './task-archived-context';

type TaskPageContentProps = {
  task: Task | null;
  sessionId?: string | null;
  initialRepositories?: Repository[];
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
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
  initialScripts = [],
  initialTerminals,
  defaultLayouts = {},
}: TaskPageContentProps) {
  const [taskDetails, setTaskDetails] = useState<Task | null>(initialTask);
  const [isMounted, setIsMounted] = useState(false);
  const [showDebugOverlay, setShowDebugOverlay] = useState(false);
  const { isMobile } = useResponsiveBreakpoint();
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const effectiveTaskId = activeTaskId ?? initialTask?.id ?? null;
  const kanbanTask = useAppStore((state) =>
    effectiveTaskId ? state.kanban.tasks.find((item: KanbanState['tasks'][number]) => item.id === effectiveTaskId) ?? null : null
  );
  const task = useMemo(() => {
    // Only use taskDetails if it matches the current task ID (avoid mixing old/new data)
    const matchingTaskDetails = taskDetails?.id === effectiveTaskId ? taskDetails : null;
    const matchingInitialTask = initialTask?.id === effectiveTaskId ? initialTask : null;
    const baseTask = matchingTaskDetails ?? matchingInitialTask;

    if (!baseTask && !kanbanTask) return null;

    // If we have full task details, merge with kanban updates
    if (baseTask) {
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
    }

    // Fallback: construct minimal task from kanban data while full details load
    // This allows immediate UI response when switching tasks
    if (kanbanTask) {
      // Reuse workspace/workflow from previous task context (same workflow)
      const prevWorkspaceId = taskDetails?.workspace_id ?? initialTask?.workspace_id;
      const prevBoardId = taskDetails?.workflow_id ?? initialTask?.workflow_id;
      return {
        id: kanbanTask.id,
        title: kanbanTask.title,
        description: kanbanTask.description ?? '',
        workflow_step_id: kanbanTask.workflowStepId,
        position: kanbanTask.position,
        state: kanbanTask.state ?? 'CREATED',
        workspace_id: prevWorkspaceId ?? '',
        workflow_id: prevBoardId ?? '',
        priority: 0,
        repositories: [],
        created_at: '',
        updated_at: kanbanTask.updatedAt ?? '',
      } as Task;
    }

    return null;
  }, [taskDetails, initialTask, kanbanTask, effectiveTaskId]);
  useTasks(task?.workflow_id ?? null);
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  const workflowSteps = useMemo(
    () => kanbanSteps.map((s) => ({
      id: s.id,
      name: s.title,
      color: s.color,
      position: s.position,
      events: s.events,
      allow_manual_move: s.allow_manual_move,
      prompt: s.prompt,
      is_start_step: s.is_start_step,
    })),
    [kanbanSteps]
  );
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
    // Skip fetch if we already have task in kanban state (same workflow) and we have essential data
    // The kanban state already has title, description, state which covers most UI needs
    // Only fetch for additional details like repositories, metadata, etc.
    if (kanbanTask && taskDetails?.workspace_id && taskDetails?.workflow_id) {
      // We have kanban data and can reuse workspace/workflow from current context
      // Fetch in background without blocking (low priority)
      fetchTask(activeTaskId, { cache: 'no-store' })
        .then((response) => setTaskDetails(response))
        .catch((error) => console.error('[TaskPageContent] Failed to load task details:', error));
      return;
    }
    // Full fetch needed for first load or cross-workflow navigation
    fetchTask(activeTaskId, { cache: 'no-store' })
      .then((response) => setTaskDetails(response))
      .catch((error) => console.error('[TaskPageContent] Failed to load task details:', error));
  }, [activeTaskId, taskDetails?.id, taskDetails?.workspace_id, taskDetails?.workflow_id, kanbanTask]);
  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const effectiveRepositories = repositories.length ? repositories : initialRepositories;
  const repository = useMemo(
    () => effectiveRepositories.find((item: Repository) => item.id === task?.repositories?.[0]?.repository_id) ?? null,
    [effectiveRepositories, task?.repositories]
  );


  const archivedValue = useMemo(() => ({
    isArchived: !!task?.archived_at,
    archivedTaskId: task?.archived_at ? task.id : undefined,
    archivedTaskTitle: task?.archived_at ? task.title : undefined,
    archivedTaskRepositoryPath: task?.archived_at ? repository?.local_path ?? undefined : undefined,
    archivedTaskUpdatedAt: task?.archived_at ? task.updated_at : undefined,
  }), [task, repository]);

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
        {/* TaskTopBar is hidden on mobile - mobile layout has its own top bar */}
        {!isMobile && (
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
            showDebugOverlay={showDebugOverlay}
            onToggleDebugOverlay={() => setShowDebugOverlay((prev) => !prev)}
            workflowSteps={workflowSteps}
            currentStepId={task?.workflow_step_id ?? null}
            workflowId={task?.workflow_id ?? null}
            isArchived={!!task?.archived_at}
          />
        )}

        <TaskArchivedProvider value={archivedValue}>
          <TaskLayout
            workspaceId={task?.workspace_id ?? null}
            workflowId={task?.workflow_id ?? null}
            sessionId={effectiveSessionId ?? null}
            repository={repository ?? null}
            initialScripts={initialScripts}
            initialTerminals={initialTerminals}
            defaultLayouts={defaultLayouts}
            taskTitle={task?.title}
            baseBranch={task?.repositories?.[0]?.base_branch}
            worktreeBranch={worktreeBranch}
          />
        </TaskArchivedProvider>
      </div>
    </TooltipProvider>
  );
}
