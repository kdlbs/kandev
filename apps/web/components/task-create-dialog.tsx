"use client";

import { FormEvent } from "react";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import type { Task } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { useKeyboardShortcutHandler } from "@/hooks/use-keyboard-shortcut";
import { TaskCreateDialogFooter } from "@/components/task-create-dialog-footer";
import {
  CreateEditSelectors,
  SessionSelectors,
  WorkflowSection,
} from "@/components/task-create-dialog-form-body";
import {
  useRepositoryOptions,
  useBranchOptions,
  useAgentProfileOptions,
} from "@/components/task-create-dialog-options";
import {
  RepositorySelector,
  BranchSelector,
  AgentSelector,
  ExecutorProfileSelector,
  InlineTaskName,
  TaskFormInputs,
} from "@/components/task-create-dialog-selectors";
import { useTaskSubmitHandlers } from "@/components/task-create-dialog-submit";
import { useToast } from "@/components/toast-provider";
import {
  useDialogFormState,
  useTaskCreateDialogEffects,
  useDialogHandlers,
  useSessionRepoName,
  useTaskCreateDialogData,
  computeIsTaskStarted,
  type DialogFormState,
  type TaskCreateDialogInitialValues,
} from "@/components/task-create-dialog-state";

interface TaskCreateDialogProps {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  mode?: "create" | "edit" | "session";
  workspaceId: string | null;
  workflowId: string | null;
  defaultStepId: string | null;
  steps: Array<{
    id: string;
    title: string;
    events?: {
      on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
      on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
    };
  }>;
  editingTask?: {
    id: string;
    title: string;
    description?: string;
    workflowStepId: string;
    state?: Task["state"];
    repositoryId?: string;
  } | null;
  onSuccess?: (
    task: Task,
    mode: "create" | "edit",
    meta?: { taskSessionId?: string | null },
  ) => void;
  onCreateSession?: (data: { prompt: string; agentProfileId: string; executorId: string }) => void;
  initialValues?: TaskCreateDialogInitialValues;
  taskId?: string | null;
}

function getRepositoryPlaceholder(
  workspaceId: string | null,
  repositoriesLoading: boolean,
  discoverReposLoading: boolean,
) {
  if (!workspaceId) return "Select workspace first";
  if (repositoriesLoading || discoverReposLoading) return "Loading...";
  return "Select repository";
}

type DialogHeaderContentProps = {
  isCreateMode: boolean;
  isEditMode: boolean;
  isTaskStarted: boolean;
  sessionRepoName?: string;
  initialTitle?: string;
  taskName: string;
  repositoryId: string;
  discoveredRepoPath: string;
  workspaceId: string | null;
  repositoriesLoading: boolean;
  discoverReposLoading: boolean;
  headerRepositoryOptions: ReturnType<typeof useRepositoryOptions>["headerRepositoryOptions"];
  onRepositoryChange: (v: string) => void;
  onTaskNameChange: (v: string) => void;
};

function DialogHeaderContent({
  isCreateMode,
  isEditMode,
  isTaskStarted,
  sessionRepoName,
  initialTitle,
  taskName,
  repositoryId,
  discoveredRepoPath,
  workspaceId,
  repositoriesLoading,
  discoverReposLoading,
  headerRepositoryOptions,
  onRepositoryChange,
  onTaskNameChange,
}: DialogHeaderContentProps) {
  if (isCreateMode || isEditMode) {
    return (
      <DialogTitle asChild>
        <div className="flex items-center gap-1 min-w-0 text-sm font-medium">
          <RepositorySelector
            options={headerRepositoryOptions}
            value={repositoryId || discoveredRepoPath}
            onValueChange={onRepositoryChange}
            placeholder={getRepositoryPlaceholder(
              workspaceId,
              repositoriesLoading,
              discoverReposLoading,
            )}
            searchPlaceholder="Search repositories..."
            emptyMessage={
              repositoriesLoading || discoverReposLoading
                ? "Loading repositories..."
                : "No repositories found."
            }
            disabled={isTaskStarted || !workspaceId || repositoriesLoading || discoverReposLoading}
            triggerClassName="w-auto text-sm"
          />
          <span className="text-muted-foreground mr-2">/</span>
          <InlineTaskName value={taskName} onChange={onTaskNameChange} autoFocus={!isEditMode} />
        </div>
      </DialogTitle>
    );
  }
  return (
    <DialogTitle asChild>
      <div className="flex items-center gap-1 min-w-0 text-sm font-medium">
        {sessionRepoName && (
          <>
            <span className="truncate text-muted-foreground">{sessionRepoName}</span>
            <span className="text-muted-foreground mx-0.5">/</span>
          </>
        )}
        <span className="truncate">{initialTitle || "Task"}</span>
        <span className="text-muted-foreground mx-0.5">/</span>
        <span className="text-muted-foreground whitespace-nowrap">new session</span>
      </div>
    </DialogTitle>
  );
}

type DialogFormBodyProps = {
  open: boolean;
  isSessionMode: boolean;
  isCreateMode: boolean;
  isTaskStarted: boolean;
  isPassthroughProfile: boolean;
  initialDescription: string;
  hasDescription: boolean;
  branchOptions: ReturnType<typeof useBranchOptions>;
  branchesLoading: boolean;
  agentProfileOptions: ReturnType<typeof useAgentProfileOptions>;
  executorProfileOptions: Array<{
    value: string;
    label: string;
    renderLabel?: () => React.ReactNode;
  }>;
  agentProfiles: AgentProfileOption[];
  agentProfilesLoading: boolean;
  executorsLoading: boolean;
  isCreatingSession: boolean;
  workflows: unknown[];
  snapshots: unknown;
  effectiveWorkflowId: string | null;
  fs: DialogFormState;
  handleKeyDown: ReturnType<typeof useKeyboardShortcutHandler>;
  onBranchChange: (v: string) => void;
  onAgentProfileChange: (v: string) => void;
  onExecutorProfileChange: (v: string) => void;
  onWorkflowChange: (v: string) => void;
  hasRepositorySelection: boolean;
};

function DialogFormBody({
  open,
  isSessionMode,
  isCreateMode,
  isTaskStarted,
  isPassthroughProfile,
  initialDescription,
  hasDescription,
  branchOptions,
  branchesLoading,
  agentProfileOptions,
  executorProfileOptions,
  agentProfiles,
  agentProfilesLoading,
  executorsLoading,
  isCreatingSession,
  workflows,
  snapshots,
  effectiveWorkflowId,
  fs,
  handleKeyDown,
  onBranchChange,
  onAgentProfileChange,
  onExecutorProfileChange,
  onWorkflowChange,
  hasRepositorySelection,
}: DialogFormBodyProps) {
  return (
    <div className="flex-1 space-y-4 overflow-y-auto pr-1">
      <TaskFormInputs
        key={`${open}-${initialDescription}`}
        isSessionMode={isSessionMode}
        autoFocus={isTaskStarted ? false : true}
        initialDescription={initialDescription}
        onDescriptionChange={fs.setHasDescription}
        onKeyDown={handleKeyDown}
        descriptionValueRef={fs.descriptionInputRef}
        disabled={isTaskStarted || isPassthroughProfile}
        placeholder={
          isPassthroughProfile ? "Sending a prompt is not supported in passthrough mode" : undefined
        }
      />
      {isPassthroughProfile && hasDescription && (
        <p className="text-xs text-amber-500">
          Prompt will be ignored — passthrough sessions don&apos;t support sending a prompt on
          start.
        </p>
      )}
      {!isSessionMode && (
        <CreateEditSelectors
          isTaskStarted={isTaskStarted}
          hasRepositorySelection={hasRepositorySelection}
          repositoryId={fs.repositoryId}
          branchOptions={branchOptions}
          branch={fs.branch}
          onBranchChange={onBranchChange}
          branchesLoading={branchesLoading}
          localBranchesLoading={fs.localBranchesLoading}
          agentProfiles={agentProfiles}
          agentProfilesLoading={agentProfilesLoading}
          agentProfileOptions={agentProfileOptions}
          agentProfileId={fs.agentProfileId}
          onAgentProfileChange={onAgentProfileChange}
          isCreatingSession={isCreatingSession}
          executorProfileOptions={executorProfileOptions}
          executorProfileId={fs.executorProfileId}
          onExecutorProfileChange={onExecutorProfileChange}
          executorsLoading={executorsLoading}
          BranchSelectorComponent={BranchSelector}
          AgentSelectorComponent={AgentSelector}
          ExecutorProfileSelectorComponent={ExecutorProfileSelector}
        />
      )}
      <WorkflowSection
        isCreateMode={isCreateMode}
        isTaskStarted={isTaskStarted}
        workflows={workflows as Parameters<typeof WorkflowSection>[0]["workflows"]}
        snapshots={snapshots as Parameters<typeof WorkflowSection>[0]["snapshots"]}
        effectiveWorkflowId={effectiveWorkflowId}
        onWorkflowChange={onWorkflowChange}
      />
      {isSessionMode && (
        <SessionSelectors
          agentProfileOptions={agentProfileOptions}
          agentProfileId={fs.agentProfileId}
          onAgentProfileChange={onAgentProfileChange}
          agentProfilesLoading={agentProfilesLoading}
          isCreatingSession={isCreatingSession}
          executorProfileOptions={executorProfileOptions}
          executorProfileId={fs.executorProfileId}
          onExecutorProfileChange={onExecutorProfileChange}
          executorsLoading={executorsLoading}
          AgentSelectorComponent={AgentSelector}
          ExecutorProfileSelectorComponent={ExecutorProfileSelector}
        />
      )}
    </div>
  );
}

function useTaskCreateDialogSetup(props: TaskCreateDialogProps) {
  const { open, onOpenChange, mode = "create", workspaceId, workflowId, defaultStepId } = props;
  const { editingTask, onSuccess, onCreateSession, initialValues, taskId = null } = props;
  const isSessionMode = mode === "session";
  const isEditMode = mode === "edit";
  const isCreateMode = mode === "create";
  const isTaskStarted = computeIsTaskStarted(isEditMode, editingTask);
  const fs = useDialogFormState(open, workspaceId, workflowId, initialValues);
  const { toast } = useToast();
  const sessionRepoName = useSessionRepoName(isSessionMode);
  const {
    workflows,
    agentProfiles,
    executors,
    snapshots,
    repositories,
    repositoriesLoading,
    branches,
    branchesLoading,
    computed,
  } = useTaskCreateDialogData(open, workspaceId, workflowId, defaultStepId, fs);
  useTaskCreateDialogEffects(fs, {
    open,
    workspaceId,
    workflowId,
    repositories,
    repositoriesLoading,
    branches,
    agentProfiles,
    executors,
    workspaceDefaults: computed.workspaceDefaults,
    toast,
  });
  const handlers = useDialogHandlers(fs, repositories);
  const submitHandlers = useTaskSubmitHandlers({
    isSessionMode,
    isEditMode,
    isPassthroughProfile: computed.isPassthroughProfile,
    taskName: fs.taskName,
    workspaceId,
    workflowId,
    effectiveWorkflowId: computed.effectiveWorkflowId,
    effectiveDefaultStepId: computed.effectiveDefaultStepId,
    repositoryId: fs.repositoryId,
    selectedLocalRepo: fs.selectedLocalRepo,
    branch: fs.branch,
    agentProfileId: fs.agentProfileId,
    executorId: fs.executorId,
    executorProfileId: fs.executorProfileId,
    editingTask,
    onSuccess,
    onCreateSession,
    onOpenChange,
    taskId,
    descriptionInputRef: fs.descriptionInputRef,
    setIsCreatingSession: fs.setIsCreatingSession,
    setIsCreatingTask: fs.setIsCreatingTask,
    setHasTitle: fs.setHasTitle,
    setHasDescription: fs.setHasDescription,
    setTaskName: fs.setTaskName,
    setRepositoryId: fs.setRepositoryId,
    setBranch: fs.setBranch,
    setAgentProfileId: fs.setAgentProfileId,
    setExecutorId: fs.setExecutorId,
    setSelectedWorkflowId: fs.setSelectedWorkflowId,
    setFetchedSteps: fs.setFetchedSteps,
  });
  const handleKeyDown = useKeyboardShortcutHandler(SHORTCUTS.SUBMIT, (event) => {
    submitHandlers.handleSubmit(event as unknown as FormEvent);
  });
  return {
    fs,
    isSessionMode,
    isEditMode,
    isCreateMode,
    isTaskStarted,
    sessionRepoName,
    workflows,
    agentProfiles,
    snapshots,
    repositoriesLoading,
    branchesLoading,
    computed,
    handlers,
    submitHandlers,
    handleKeyDown,
  };
}

export function TaskCreateDialog(props: TaskCreateDialogProps) {
  const { open, onOpenChange, initialValues, workspaceId } = props;
  const setup = useTaskCreateDialogSetup(props);
  const { fs, isSessionMode, isEditMode, isCreateMode, isTaskStarted } = setup;
  const { sessionRepoName, workflows, agentProfiles, snapshots } = setup;
  const { repositoriesLoading, branchesLoading, computed, handlers, handleKeyDown } = setup;
  const { handleSubmit, handleUpdateWithoutAgent, handleCreateWithoutAgent } = setup.submitHandlers;
  const { handleCreateWithPlanMode, handleCancel } = setup.submitHandlers;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="w-full h-full max-w-full max-h-full rounded-none sm:w-[900px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col">
        <DialogHeader>
          <DialogHeaderContent
            isCreateMode={isCreateMode}
            isEditMode={isEditMode}
            isTaskStarted={isTaskStarted}
            sessionRepoName={sessionRepoName}
            initialTitle={initialValues?.title}
            taskName={fs.taskName}
            repositoryId={fs.repositoryId}
            discoveredRepoPath={fs.discoveredRepoPath}
            workspaceId={workspaceId}
            repositoriesLoading={repositoriesLoading}
            discoverReposLoading={fs.discoverReposLoading}
            headerRepositoryOptions={computed.headerRepositoryOptions}
            onRepositoryChange={handlers.handleRepositoryChange}
            onTaskNameChange={handlers.handleTaskNameChange}
          />
        </DialogHeader>
        <form onSubmit={handleSubmit} className="flex flex-col gap-4 overflow-hidden">
          <DialogFormBody
            open={open}
            isSessionMode={isSessionMode}
            isCreateMode={isCreateMode}
            isTaskStarted={isTaskStarted}
            isPassthroughProfile={computed.isPassthroughProfile}
            initialDescription={initialValues?.description ?? ""}
            hasDescription={fs.hasDescription}
            branchOptions={computed.branchOptions}
            branchesLoading={branchesLoading}
            agentProfileOptions={computed.agentProfileOptions}
            executorProfileOptions={computed.executorProfileOptions}
            agentProfiles={agentProfiles}
            agentProfilesLoading={computed.agentProfilesLoading}
            executorsLoading={computed.executorsLoading}
            isCreatingSession={fs.isCreatingSession}
            workflows={workflows}
            snapshots={snapshots}
            effectiveWorkflowId={computed.effectiveWorkflowId ?? null}
            fs={fs}
            handleKeyDown={handleKeyDown}
            onBranchChange={handlers.handleBranchChange}
            onAgentProfileChange={handlers.handleAgentProfileChange}
            onExecutorProfileChange={handlers.handleExecutorProfileChange}
            onWorkflowChange={handlers.handleWorkflowChange}
            hasRepositorySelection={computed.hasRepositorySelection}
          />
          <DialogFooter className="border-t border-border pt-3 flex-col gap-3 sm:flex-row sm:gap-2">
            <TaskCreateDialogFooter
              isSessionMode={isSessionMode}
              isCreateMode={isCreateMode}
              isEditMode={isEditMode}
              isTaskStarted={isTaskStarted}
              isPassthroughProfile={computed.isPassthroughProfile}
              isCreatingSession={fs.isCreatingSession}
              isCreatingTask={fs.isCreatingTask}
              hasTitle={fs.hasTitle}
              hasDescription={fs.hasDescription}
              hasRepositorySelection={computed.hasRepositorySelection}
              branch={fs.branch}
              agentProfileId={fs.agentProfileId}
              workspaceId={workspaceId}
              effectiveWorkflowId={computed.effectiveWorkflowId ?? null}
              executorHint={computed.executorHint}
              onCancel={handleCancel}
              onUpdateWithoutAgent={handleUpdateWithoutAgent}
              onCreateWithoutAgent={handleCreateWithoutAgent}
              onCreateWithPlanMode={handleCreateWithPlanMode}
            />
          </DialogFooter>
        </form>
      </DialogContent>
    </Dialog>
  );
}
