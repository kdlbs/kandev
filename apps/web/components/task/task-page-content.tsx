"use client";

import { useEffect, useMemo, useState } from "react";
import { TaskTopBar } from "@/components/task/task-top-bar";
import { TaskLayout } from "@/components/task/task-layout";
import { DebugOverlay } from "@/components/debug-overlay";
import type { Repository, RepositoryScript, Task } from "@/lib/types/http";
import type { Terminal } from "@/hooks/domains/session/use-terminals";
import type { KanbanState } from "@/lib/state/slices";
import { DEBUG_UI } from "@/lib/config";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useSessionAgent } from "@/hooks/domains/session/use-session-agent";
import { useSessionResumption } from "@/hooks/domains/session/use-session-resumption";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { useAppStore } from "@/components/state-provider";
import { fetchTask } from "@/lib/api";
import { useTasks } from "@/hooks/use-tasks";
import { useResponsiveBreakpoint } from "@/hooks/use-responsive-breakpoint";
import type { Layout } from "react-resizable-panels";
import { TaskArchivedProvider } from "./task-archived-context";
import { SessionCommands } from "@/components/session-commands";
import { VcsDialogsProvider } from "@/components/vcs/vcs-dialogs";
import {
  buildDebugEntries,
  deriveIsAgentWorking,
  buildArchivedValue,
  resolveTaskProps,
} from "@/components/task/task-page-content-helpers";

type TaskPageContentProps = {
  task: Task | null;
  sessionId?: string | null;
  initialRepositories?: Repository[];
  initialScripts?: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts?: Record<string, Layout>;
  initialLayout?: string | null;
};

function resolveEffectiveTask(
  taskDetails: Task | null,
  initialTask: Task | null,
  kanbanTask: KanbanState["tasks"][number] | null,
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

function mergeBaseWithKanban(
  baseTask: Task,
  kanbanTask: KanbanState["tasks"][number] | null,
): Task {
  if (!kanbanTask) return baseTask;
  return {
    ...baseTask,
    title: kanbanTask.title ?? baseTask.title,
    description: kanbanTask.description ?? baseTask.description,
    workflow_step_id:
      (kanbanTask.workflowStepId as string | undefined) ?? baseTask.workflow_step_id,
    position: kanbanTask.position ?? baseTask.position,
    state: (kanbanTask.state as Task["state"] | undefined) ?? baseTask.state,
    repositories: baseTask.repositories,
  };
}

function buildTaskFromKanban(
  kanbanTask: KanbanState["tasks"][number],
  taskDetails: Task | null,
  initialTask: Task | null,
): Task {
  const prevWorkspaceId = taskDetails?.workspace_id ?? initialTask?.workspace_id;
  const prevBoardId = taskDetails?.workflow_id ?? initialTask?.workflow_id;
  return {
    id: kanbanTask.id,
    title: kanbanTask.title,
    description: kanbanTask.description ?? "",
    workflow_step_id: kanbanTask.workflowStepId,
    position: kanbanTask.position,
    state: kanbanTask.state ?? "CREATED",
    workspace_id: prevWorkspaceId ?? "",
    workflow_id: prevBoardId ?? "",
    priority: 0,
    repositories: [],
    created_at: "",
    updated_at: kanbanTask.updatedAt ?? "",
  } as Task;
}

function useWorkflowStepsMapped() {
  const kanbanSteps = useAppStore((state) => state.kanban.steps);
  return useMemo(
    () =>
      kanbanSteps.map((s) => ({
        id: s.id,
        name: s.title,
        color: s.color,
        position: s.position,
        events: s.events,
        allow_manual_move: s.allow_manual_move,
        prompt: s.prompt,
        is_start_step: s.is_start_step,
      })),
    [kanbanSteps],
  );
}

function useSessionPanelState(effectiveSessionId: string | null | undefined) {
  const storeSessionState = useAppStore((state) =>
    effectiveSessionId ? (state.taskSessions.items[effectiveSessionId]?.state ?? null) : null,
  );
  const isSessionPassthrough = useAppStore((state) =>
    effectiveSessionId
      ? state.taskSessions.items[effectiveSessionId]?.is_passthrough === true
      : false,
  );
  const previewOpen = useAppStore((state) =>
    effectiveSessionId ? (state.previewPanel.openBySessionId[effectiveSessionId] ?? false) : false,
  );
  const previewStage = useAppStore((state) =>
    effectiveSessionId
      ? (state.previewPanel.stageBySessionId[effectiveSessionId] ?? "closed")
      : "closed",
  );
  const previewUrl = useAppStore((state) =>
    effectiveSessionId ? (state.previewPanel.urlBySessionId[effectiveSessionId] ?? "") : "",
  );
  const devProcessId = useAppStore((state) =>
    effectiveSessionId ? state.processes.devProcessBySessionId[effectiveSessionId] : undefined,
  );
  const devProcessStatus = useAppStore((state) =>
    devProcessId ? (state.processes.processesById[devProcessId]?.status ?? null) : null,
  );
  return {
    storeSessionState,
    isSessionPassthrough,
    previewOpen,
    previewStage,
    previewUrl,
    devProcessId,
    devProcessStatus,
  };
}

function useMergedAgentState(
  agent: ReturnType<typeof useSessionAgent>,
  resumption: ReturnType<typeof useSessionResumption>,
  sessionPanel: ReturnType<typeof useSessionPanelState>,
  effectiveSessionId: string | null | undefined,
  task: Task | null,
) {
  const isResuming =
    resumption.resumptionState === "checking" || resumption.resumptionState === "resuming";
  const isResumed =
    resumption.resumptionState === "resumed" || resumption.resumptionState === "running";
  const taskSessionState = sessionPanel.storeSessionState ?? agent.taskSessionState;
  const worktreePath = effectiveSessionId
    ? (resumption.worktreePath ?? agent.worktreePath)
    : agent.worktreePath;
  const worktreeBranch = effectiveSessionId
    ? (resumption.worktreeBranch ?? agent.worktreeBranch)
    : agent.worktreeBranch;
  const isAgentWorking = deriveIsAgentWorking(
    taskSessionState,
    agent.isAgentRunning,
    task?.state ?? null,
  );
  return { isResuming, isResumed, taskSessionState, worktreePath, worktreeBranch, isAgentWorking };
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
  initialLayout?: string | null;
};

type RemoteExecutorStatus = {
  is_remote_executor?: boolean;
  executor_type?: string | null;
  executor_name?: string | null;
  remote_name?: string | null;
  remote_state?: string | null;
  remote_created_at?: string | null;
  remote_checked_at?: string | null;
  remote_status_error?: string | null;
};

function toNullable(value: string | null | undefined): string | null {
  return value ?? null;
}

function resolveRemoteExecutor(status?: RemoteExecutorStatus | null) {
  const remoteExecutorName = status?.remote_name ?? status?.executor_name ?? null;
  return {
    isRemoteExecutor: status?.is_remote_executor ?? false,
    remoteExecutorType: toNullable(status?.executor_type),
    remoteExecutorName,
    remoteState: toNullable(status?.remote_state),
    remoteCreatedAt: toNullable(status?.remote_created_at),
    remoteCheckedAt: toNullable(status?.remote_checked_at),
    remoteStatusError: toNullable(status?.remote_status_error),
  };
}

function buildTaskTopBarProps(params: {
  taskProps: ReturnType<typeof resolveTaskProps>;
  agent: ReturnType<typeof useSessionAgent>;
  merged: ReturnType<typeof useMergedAgentState>;
  workflowSteps: ReturnType<typeof useWorkflowStepsMapped>;
  showDebugOverlay: boolean;
  onToggleDebugOverlay: () => void;
  effectiveSessionId: string | null;
  remote: ReturnType<typeof resolveRemoteExecutor>;
}) {
  const { taskProps, agent, merged, workflowSteps, showDebugOverlay, onToggleDebugOverlay } =
    params;
  return {
    taskId: taskProps.taskId,
    activeSessionId: params.effectiveSessionId,
    taskTitle: taskProps.taskTitle,
    taskDescription: taskProps.taskDescription,
    baseBranch: taskProps.baseBranch ?? undefined,
    onStartAgent: agent.handleStartAgent,
    onStopAgent: agent.handleStopAgent,
    isAgentRunning: agent.isAgentRunning || merged.isResumed,
    isAgentLoading: agent.isAgentLoading || merged.isResuming,
    worktreePath: merged.worktreePath,
    worktreeBranch: merged.worktreeBranch,
    repositoryPath: taskProps.repositoryPath,
    repositoryName: taskProps.repositoryName,
    showDebugOverlay,
    onToggleDebugOverlay,
    workflowSteps,
    currentStepId: taskProps.workflowStepId,
    workflowId: taskProps.workflowId,
    isArchived: taskProps.isArchived,
    isRemoteExecutor: params.remote.isRemoteExecutor,
    remoteExecutorType: params.remote.remoteExecutorType,
    remoteExecutorName: params.remote.remoteExecutorName,
    remoteState: params.remote.remoteState,
    remoteCreatedAt: params.remote.remoteCreatedAt,
    remoteCheckedAt: params.remote.remoteCheckedAt,
    remoteStatusError: params.remote.remoteStatusError,
  };
}

function buildTaskLayoutProps(params: {
  taskProps: ReturnType<typeof resolveTaskProps>;
  repository: Repository | null;
  effectiveSessionId: string | null;
  initialScripts: RepositoryScript[];
  initialTerminals?: Terminal[];
  defaultLayouts: Record<string, Layout>;
  merged: ReturnType<typeof useMergedAgentState>;
  remote: ReturnType<typeof resolveRemoteExecutor>;
  initialLayout?: string | null;
}) {
  const { taskProps, repository, effectiveSessionId, initialScripts, initialTerminals } = params;
  return {
    workspaceId: taskProps.workspaceId,
    workflowId: taskProps.workflowId,
    sessionId: effectiveSessionId,
    repository: repository ?? null,
    initialScripts,
    initialTerminals,
    defaultLayouts: params.defaultLayouts,
    initialLayout: params.initialLayout,
    taskTitle: taskProps.taskTitle,
    baseBranch: taskProps.baseBranch,
    worktreeBranch: params.merged.worktreeBranch,
    isRemoteExecutor: params.remote.isRemoteExecutor,
    remoteExecutorType: params.remote.remoteExecutorType,
    remoteExecutorName: params.remote.remoteExecutorName,
    remoteState: params.remote.remoteState,
    remoteCreatedAt: params.remote.remoteCreatedAt,
    remoteCheckedAt: params.remote.remoteCheckedAt,
    remoteStatusError: params.remote.remoteStatusError,
  };
}

function maybeBuildDebugEntries(params: {
  isVisible: boolean;
  connectionStatus: string;
  task: Task | null;
  effectiveSessionId: string | null | undefined;
  merged: ReturnType<typeof useMergedAgentState>;
  resumption: ReturnType<typeof useSessionResumption>;
  sessionPanel: ReturnType<typeof useSessionPanelState>;
  agentctlStatus: ReturnType<typeof useSessionAgentctl>;
}) {
  if (!params.isVisible) return null;
  return buildDebugEntries({
    connectionStatus: params.connectionStatus,
    task: params.task,
    effectiveSessionId: params.effectiveSessionId,
    taskSessionState: params.merged.taskSessionState,
    isAgentWorking: params.merged.isAgentWorking,
    resumptionState: params.resumption.resumptionState,
    resumptionError: params.resumption.error,
    agentctlStatus: params.agentctlStatus,
    previewOpen: params.sessionPanel.previewOpen,
    previewStage: params.sessionPanel.previewStage,
    previewUrl: params.sessionPanel.previewUrl,
    devProcessId: params.sessionPanel.devProcessId,
    devProcessStatus: params.sessionPanel.devProcessStatus,
  });
}

function TaskPageInner({
  task,
  effectiveSessionId,
  repository,
  agent,
  merged,
  resumption,
  sessionPanel,
  agentctlStatus,
  connectionStatus,
  workflowSteps,
  archivedValue,
  isMobile,
  showDebugOverlay,
  onToggleDebugOverlay,
  initialScripts,
  initialTerminals,
  defaultLayouts,
  initialLayout,
}: TaskPageInnerProps) {
  const taskProps = resolveTaskProps(task, repository);
  const remote = resolveRemoteExecutor(resumption.sessionStatus as RemoteExecutorStatus | null);
  const debugEntries = maybeBuildDebugEntries({
    isVisible: DEBUG_UI && showDebugOverlay,
    connectionStatus,
    task,
    effectiveSessionId,
    merged,
    resumption,
    sessionPanel,
    agentctlStatus,
  });
  const topBarProps = buildTaskTopBarProps({
    taskProps,
    agent,
    merged,
    workflowSteps,
    showDebugOverlay,
    onToggleDebugOverlay,
    effectiveSessionId,
    remote,
  });
  const layoutProps = buildTaskLayoutProps({
    taskProps,
    repository,
    effectiveSessionId,
    initialScripts,
    initialTerminals,
    defaultLayouts,
    merged,
    remote,
    initialLayout,
  });

  return (
    <TooltipProvider>
      <VcsDialogsProvider
        sessionId={effectiveSessionId}
        baseBranch={taskProps.baseBranch}
        taskTitle={taskProps.taskTitle}
        displayBranch={merged.worktreeBranch}
      >
        <div className="h-screen w-full flex flex-col bg-background">
          <SessionCommands
            sessionId={effectiveSessionId}
            baseBranch={taskProps.baseBranch}
            isAgentRunning={merged.isAgentWorking}
            hasWorktree={Boolean(merged.worktreeBranch)}
            isPassthrough={sessionPanel.isSessionPassthrough}
          />
          {debugEntries && <DebugOverlay title="Task Debug" entries={debugEntries} />}
          {!isMobile && <TaskTopBar {...topBarProps} />}
          <TaskArchivedProvider value={archivedValue}>
            <TaskLayout {...layoutProps} />
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
  if (params.initialSessionId)
    params.setActiveSession(params.initialTaskId, params.initialSessionId);
  else params.setActiveTask(params.initialTaskId);
}

function useTaskDetails(activeTaskId: string | null, initialTask: Task | null) {
  const [taskDetails, setTaskDetails] = useState<Task | null>(initialTask);
  const kanbanTask = useAppStore((state) =>
    activeTaskId
      ? (state.kanban.tasks.find(
          (item: KanbanState["tasks"][number]) => item.id === activeTaskId,
        ) ?? null)
      : null,
  );
  const effectiveTaskId = activeTaskId ?? initialTask?.id ?? null;
  const task = useMemo(
    () => resolveEffectiveTask(taskDetails, initialTask, kanbanTask, effectiveTaskId),
    [taskDetails, initialTask, kanbanTask, effectiveTaskId],
  );
  useTasks(task?.workflow_id ?? null);

  useEffect(() => {
    if (!activeTaskId || taskDetails?.id === activeTaskId) return;
    fetchTask(activeTaskId, { cache: "no-store" })
      .then((response) => setTaskDetails(response))
      .catch((error) => console.error("[TaskPageContent] Failed to load task details:", error));
  }, [
    activeTaskId,
    taskDetails?.id,
    taskDetails?.workspace_id,
    taskDetails?.workflow_id,
    kanbanTask,
    setTaskDetails,
  ]);

  return { task, kanbanTask };
}

function useTaskPageData(
  initialTask: Task | null,
  sessionId: string | null,
  initialRepositories: Repository[],
) {
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const setActiveTask = useAppStore((state) => state.setActiveTask);

  const { task } = useTaskDetails(activeTaskId, initialTask);

  const agent = useSessionAgent(task);
  const initialSessionId = sessionId ?? agent.taskSessionId ?? null;
  const effectiveSessionId = activeSessionId ?? initialSessionId;

  useEffect(() => {
    syncActiveTaskSession({
      initialTaskId: initialTask?.id,
      initialSessionId,
      setActiveSession,
      setActiveTask,
    });
  }, [initialTask?.id, initialSessionId, setActiveSession, setActiveTask]);

  const { repositories } = useRepositories(task?.workspace_id ?? null, Boolean(task?.workspace_id));
  const effectiveRepositories = repositories.length ? repositories : initialRepositories;
  const repository = useMemo(
    () =>
      effectiveRepositories.find(
        (item: Repository) => item.id === task?.repositories?.[0]?.repository_id,
      ) ?? null,
    [effectiveRepositories, task?.repositories],
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
  initialLayout,
}: TaskPageContentProps) {
  const [isMounted, setIsMounted] = useState(false);
  const [showDebugOverlay, setShowDebugOverlay] = useState(false);
  const { isMobile } = useResponsiveBreakpoint();
  const connectionStatus = useAppStore((state) => state.connection.status);

  const { task, agent, effectiveSessionId, repository } = useTaskPageData(
    initialTask,
    sessionId,
    initialRepositories,
  );

  const workflowSteps = useWorkflowStepsMapped();
  const sessionPanel = useSessionPanelState(effectiveSessionId);
  const agentctlStatus = useSessionAgentctl(effectiveSessionId);
  const resumption = useSessionResumption(task?.id ?? null, effectiveSessionId);
  const merged = useMergedAgentState(agent, resumption, sessionPanel, effectiveSessionId, task);
  const archivedValue = useMemo(() => buildArchivedValue(task, repository), [task, repository]);

  useEffect(() => {
    queueMicrotask(() => setIsMounted(true));
  }, []);

  if (!isMounted) return <div className="h-screen w-full bg-background" />;

  return (
    <TaskPageInner
      task={task}
      effectiveSessionId={effectiveSessionId ?? null}
      repository={repository}
      agent={agent}
      merged={merged}
      resumption={resumption}
      sessionPanel={sessionPanel}
      agentctlStatus={agentctlStatus}
      connectionStatus={connectionStatus}
      workflowSteps={workflowSteps}
      archivedValue={archivedValue}
      isMobile={isMobile}
      showDebugOverlay={showDebugOverlay}
      onToggleDebugOverlay={() => setShowDebugOverlay((prev) => !prev)}
      initialScripts={initialScripts}
      initialTerminals={initialTerminals}
      defaultLayouts={defaultLayouts}
      initialLayout={initialLayout}
    />
  );
}
