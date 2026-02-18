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
import { SessionCommands } from '@/components/session-commands';
import { VcsDialogsProvider } from '@/components/vcs/vcs-dialogs';

type TaskPageContentProps = {
  task: Task | null;
  sessionId?: string | null;
  initialRepositories?: Repository[];
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts?: Record<string, Layout>;
};

function buildDebugEntries(params: {
  connectionStatus: string;
  task: Task | null;
  effectiveSessionId: string | null | undefined;
  taskSessionState: string | null;
  isAgentWorking: boolean;
  resumptionState: string;
  resumptionError: string | null;
  agentctlStatus: { status: string; isReady: boolean; errorMessage?: string | null; agentExecutionId?: string | null };
  previewOpen: boolean;
  previewStage: string;
  previewUrl: string;
  devProcessId: string | undefined;
  devProcessStatus: string | null;
}): Record<string, unknown> {
  const { connectionStatus, task, effectiveSessionId, taskSessionState,
    isAgentWorking, resumptionState, resumptionError, agentctlStatus,
    previewOpen, previewStage, previewUrl, devProcessId, devProcessStatus } = params;
  return {
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
  };
}

function resolveEffectiveTask(
  taskDetails: Task | null,
  initialTask: Task | null,
  kanbanTask: KanbanState['tasks'][number] | null,
  effectiveTaskId: string | null,
): Task | null {
  const matchingTaskDetails = taskDetails?.id === effectiveTaskId ? taskDetails : null;
  const matchingInitialTask = initialTask?.id === effectiveTaskId ? initialTask : null;
  const baseTask = matchingTaskDetails ?? matchingInitialTask;

  if (!baseTask && !kanbanTask) return null;
  if (baseTask) return mergeBaseWithKanban(baseTask, kanbanTask);
  if (kanbanTask) return buildTaskFromKanban(kanbanTask, taskDetails, initialTask);
  return null;
}

function mergeBaseWithKanban(baseTask: Task, kanbanTask: KanbanState['tasks'][number] | null): Task {
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

function buildTaskFromKanban(kanbanTask: KanbanState['tasks'][number], taskDetails: Task | null, initialTask: Task | null): Task {
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

function useWorkflowStepsMapped() {
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  return useMemo(
    () => kanbanSteps.map((s) => ({
      id: s.id, name: s.title, color: s.color, position: s.position,
      events: s.events, allow_manual_move: s.allow_manual_move,
      prompt: s.prompt, is_start_step: s.is_start_step,
    })),
    [kanbanSteps]
  );
}

function useSessionPanelState(effectiveSessionId: string | null | undefined) {
  const storeSessionState = useAppStore((state) =>
    effectiveSessionId ? state.taskSessions.items[effectiveSessionId]?.state ?? null : null
  );
  const isSessionPassthrough = useAppStore((state) =>
    effectiveSessionId ? state.taskSessions.items[effectiveSessionId]?.is_passthrough === true : false
  );
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
  return { storeSessionState, isSessionPassthrough, previewOpen, previewStage, previewUrl, devProcessId, devProcessStatus };
}

function deriveIsAgentWorking(taskSessionState: string | null, isAgentRunning: boolean, taskState: string | null): boolean {
  if (taskSessionState !== null) return taskSessionState === 'STARTING' || taskSessionState === 'RUNNING';
  return isAgentRunning && (taskState === 'IN_PROGRESS' || taskState === 'SCHEDULING');
}

function useMergedAgentState(
  agent: ReturnType<typeof useSessionAgent>,
  resumption: ReturnType<typeof useSessionResumption>,
  sessionPanel: ReturnType<typeof useSessionPanelState>,
  effectiveSessionId: string | null | undefined,
  task: Task | null,
) {
  const isResuming = resumption.resumptionState === 'checking' || resumption.resumptionState === 'resuming';
  const isResumed = resumption.resumptionState === 'resumed' || resumption.resumptionState === 'running';
  const taskSessionState = sessionPanel.storeSessionState ?? agent.taskSessionState;
  const worktreePath = effectiveSessionId ? (resumption.worktreePath ?? agent.worktreePath) : agent.worktreePath;
  const worktreeBranch = effectiveSessionId ? (resumption.worktreeBranch ?? agent.worktreeBranch) : agent.worktreeBranch;
  const isAgentWorking = deriveIsAgentWorking(taskSessionState, agent.isAgentRunning, task?.state ?? null);
  return { isResuming, isResumed, taskSessionState, worktreePath, worktreeBranch, isAgentWorking };
}

function buildArchivedValue(task: Task | null, repository: Repository | null) {
  const isArchived = !!task?.archived_at;
  return {
    isArchived,
    archivedTaskId: isArchived ? task?.id : undefined,
    archivedTaskTitle: isArchived ? task?.title : undefined,
    archivedTaskRepositoryPath: isArchived ? (repository?.local_path ?? undefined) : undefined,
    archivedTaskUpdatedAt: isArchived ? task?.updated_at : undefined,
  };
}

type TaskPageInnerProps = {
  task: Task | null;
  effectiveSessionId: string | null;
  repository: Repository | null;
  agent: ReturnType<typeof useSessionAgent>;
  merged: ReturnType<typeof useMergedAgentState>;
  resumption: ReturnType<typeof useSessionResumption>;
  sessionPanel: ReturnType<typeof useSessionPanelState>;
  agentctlStatus: ReturnType<typeof useSessionAgentctl>;
  connectionStatus: string;
  workflowSteps: ReturnType<typeof useWorkflowStepsMapped>;
  archivedValue: ReturnType<typeof buildArchivedValue>;
  isMobile: boolean;
  showDebugOverlay: boolean;
  onToggleDebugOverlay: () => void;
  initialScripts: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts: Record<string, Layout>;
};

function resolveTaskIds(task: Task | null) {
  return {
    taskId: task?.id ?? null,
    workflowId: task?.workflow_id ?? null,
    workspaceId: task?.workspace_id ?? null,
    workflowStepId: task?.workflow_step_id ?? null,
    baseBranch: task?.repositories?.[0]?.base_branch,
    isArchived: !!task?.archived_at,
  };
}

function resolveTaskProps(task: Task | null, repository: Repository | null) {
  const ids = resolveTaskIds(task);
  return {
    ...ids,
    taskTitle: task?.title,
    taskDescription: task?.description,
    repositoryPath: repository?.local_path ?? null,
    repositoryName: repository?.name ?? null,
  };
}

function TaskPageInner({
  task, effectiveSessionId, repository, agent, merged, resumption, sessionPanel,
  agentctlStatus, connectionStatus, workflowSteps, archivedValue, isMobile,
  showDebugOverlay, onToggleDebugOverlay, initialScripts, initialTerminals, defaultLayouts,
}: TaskPageInnerProps) {
  const tp = resolveTaskProps(task, repository);
  return (
    <TooltipProvider>
      <VcsDialogsProvider sessionId={effectiveSessionId} baseBranch={tp.baseBranch} taskTitle={tp.taskTitle} displayBranch={merged.worktreeBranch}>
        <div className="h-screen w-full flex flex-col bg-background">
          <SessionCommands sessionId={effectiveSessionId} baseBranch={tp.baseBranch} isAgentRunning={merged.isAgentWorking} hasWorktree={Boolean(merged.worktreeBranch)} isPassthrough={sessionPanel.isSessionPassthrough} />
          {DEBUG_UI && showDebugOverlay && (
            <DebugOverlay title="Task Debug" entries={buildDebugEntries({
              connectionStatus, task, effectiveSessionId, taskSessionState: merged.taskSessionState,
              isAgentWorking: merged.isAgentWorking, resumptionState: resumption.resumptionState,
              resumptionError: resumption.error, agentctlStatus, previewOpen: sessionPanel.previewOpen,
              previewStage: sessionPanel.previewStage, previewUrl: sessionPanel.previewUrl,
              devProcessId: sessionPanel.devProcessId, devProcessStatus: sessionPanel.devProcessStatus,
            })} />
          )}
          {!isMobile && (
            <TaskTopBar
              taskId={tp.taskId} activeSessionId={effectiveSessionId} taskTitle={tp.taskTitle}
              taskDescription={tp.taskDescription} baseBranch={tp.baseBranch ?? undefined}
              onStartAgent={agent.handleStartAgent} onStopAgent={agent.handleStopAgent}
              isAgentRunning={agent.isAgentRunning || merged.isResumed} isAgentLoading={agent.isAgentLoading || merged.isResuming}
              worktreePath={merged.worktreePath} worktreeBranch={merged.worktreeBranch}
              repositoryPath={tp.repositoryPath} repositoryName={tp.repositoryName}
              showDebugOverlay={showDebugOverlay} onToggleDebugOverlay={onToggleDebugOverlay}
              workflowSteps={workflowSteps} currentStepId={tp.workflowStepId} workflowId={tp.workflowId} isArchived={tp.isArchived}
            />
          )}
          <TaskArchivedProvider value={archivedValue}>
            <TaskLayout
              workspaceId={tp.workspaceId} workflowId={tp.workflowId} sessionId={effectiveSessionId}
              repository={repository ?? null} initialScripts={initialScripts} initialTerminals={initialTerminals}
              defaultLayouts={defaultLayouts} taskTitle={tp.taskTitle} baseBranch={tp.baseBranch} worktreeBranch={merged.worktreeBranch}
            />
          </TaskArchivedProvider>
        </div>
      </VcsDialogsProvider>
    </TooltipProvider>
  );
}

function syncActiveTaskSession(params: {
  initialTaskId: string | undefined;
  initialSessionId: string | null;
  setActiveSession: (taskId: string, sessionId: string) => void;
  setActiveTask: (taskId: string) => void;
}) {
  if (!params.initialTaskId) return;
  if (params.initialSessionId) params.setActiveSession(params.initialTaskId, params.initialSessionId);
  else params.setActiveTask(params.initialTaskId);
}

function useTaskDetails(activeTaskId: string | null, initialTask: Task | null) {
  const [taskDetails, setTaskDetails] = useState<Task | null>(initialTask);
  const kanbanTask = useAppStore((state) =>
    activeTaskId ? state.kanban.tasks.find((item: KanbanState['tasks'][number]) => item.id === activeTaskId) ?? null : null
  );
  const effectiveTaskId = activeTaskId ?? initialTask?.id ?? null;
  const task = useMemo(() => resolveEffectiveTask(taskDetails, initialTask, kanbanTask, effectiveTaskId), [taskDetails, initialTask, kanbanTask, effectiveTaskId]);
  useTasks(task?.workflow_id ?? null);

  useEffect(() => {
    if (!activeTaskId || taskDetails?.id === activeTaskId) return;
    fetchTask(activeTaskId, { cache: 'no-store' })
      .then((response) => setTaskDetails(response))
      .catch((error) => console.error('[TaskPageContent] Failed to load task details:', error));
  }, [activeTaskId, taskDetails?.id, taskDetails?.workspace_id, taskDetails?.workflow_id, kanbanTask, setTaskDetails]);

  return { task, kanbanTask };
}

function useTaskPageData(initialTask: Task | null, sessionId: string | null, initialRepositories: Repository[]) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const setActiveTask = useAppStore((state) => state.setActiveTask);

  const { task } = useTaskDetails(activeTaskId, initialTask);

  const agent = useSessionAgent(task);
  const initialSessionId = sessionId ?? agent.taskSessionId ?? null;
  const effectiveSessionId = activeSessionId ?? initialSessionId;

  useEffect(() => {
    syncActiveTaskSession({ initialTaskId: initialTask?.id, initialSessionId, setActiveSession, setActiveTask });
  }, [initialTask?.id, initialSessionId, setActiveSession, setActiveTask]);

  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const effectiveRepositories = repositories.length ? repositories : initialRepositories;
  const repository = useMemo(
    () => effectiveRepositories.find((item: Repository) => item.id === task?.repositories?.[0]?.repository_id) ?? null,
    [effectiveRepositories, task?.repositories]
  );

  return { task, agent, effectiveSessionId, repository };
}

export function TaskPageContent({
  task: initialTask,
  sessionId = null,
  initialRepositories = [],
  initialScripts = [],
  initialTerminals,
  defaultLayouts = {},
}: TaskPageContentProps) {
  const [isMounted, setIsMounted] = useState(false);
  const [showDebugOverlay, setShowDebugOverlay] = useState(false);
  const { isMobile } = useResponsiveBreakpoint();
  const connectionStatus = useAppStore((state) => state.connection.status);

  const { task, agent, effectiveSessionId, repository } = useTaskPageData(initialTask, sessionId, initialRepositories);

  const workflowSteps = useWorkflowStepsMapped();
  const sessionPanel = useSessionPanelState(effectiveSessionId);
  const agentctlStatus = useSessionAgentctl(effectiveSessionId);
  const resumption = useSessionResumption(task?.id ?? null, effectiveSessionId);
  const merged = useMergedAgentState(agent, resumption, sessionPanel, effectiveSessionId, task);
  const archivedValue = useMemo(() => buildArchivedValue(task, repository), [task, repository]);

  useEffect(() => { queueMicrotask(() => setIsMounted(true)); }, []);

  if (!isMounted) return <div className="h-screen w-full bg-background" />;

  return (
    <TaskPageInner
      task={task} effectiveSessionId={effectiveSessionId ?? null} repository={repository}
      agent={agent} merged={merged} resumption={resumption} sessionPanel={sessionPanel}
      agentctlStatus={agentctlStatus} connectionStatus={connectionStatus}
      workflowSteps={workflowSteps} archivedValue={archivedValue} isMobile={isMobile}
      showDebugOverlay={showDebugOverlay} onToggleDebugOverlay={() => setShowDebugOverlay((prev) => !prev)}
      initialScripts={initialScripts} initialTerminals={initialTerminals} defaultLayouts={defaultLayouts}
    />
  );
}
