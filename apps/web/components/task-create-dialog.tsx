"use client";

import { FormEvent, useCallback } from "react";
import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";
import { Dialog, DialogContent, DialogHeader, DialogFooter } from "@kandev/ui/dialog";
import type { Task } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useKeyboardShortcutHandler } from "@/hooks/use-keyboard-shortcut";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { TaskCreateDialogFooter } from "@/components/task-create-dialog-footer";
import { DiscardLocalChangesDialog } from "@/components/discard-local-changes-dialog";
import { DialogHeaderContent } from "@/components/task-create-dialog-header";
import {
  CreateEditSelectors,
  SessionSelectors,
  WorkflowSection,
  DialogPromptSection,
} from "@/components/task-create-dialog-form-body";
import { useBranchOptions, useAgentProfileOptions } from "@/components/task-create-dialog-options";
import {
  BranchSelector,
  AgentSelector,
  ExecutorProfileSelector,
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
  parentTaskId?: string;
}

type DialogFormBodyProps = {
  isSessionMode: boolean;
  isCreateMode: boolean;
  isTaskStarted: boolean;
  isPassthroughProfile: boolean;
  initialDescription: string;
  hasDescription: boolean;
  workspaceId: string | null;
  onJiraImport?: (ticket: JiraTicket) => void;
  onLinearImport?: (issue: LinearIssue) => void;
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
  isLocalExecutor: boolean;
  enhance?: { onEnhance: () => void; isLoading: boolean; isConfigured: boolean };
  workflowAgentLocked: boolean;
  onToggleFreshBranch: (enabled: boolean) => void;
};

function DialogFormBody({
  isSessionMode,
  isCreateMode,
  isTaskStarted,
  isPassthroughProfile,
  initialDescription,
  hasDescription,
  workspaceId,
  onJiraImport,
  onLinearImport,
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
  isLocalExecutor,
  enhance,
  workflowAgentLocked,
  onToggleFreshBranch,
}: DialogFormBodyProps) {
  return (
    <div className="flex-1 space-y-4 overflow-y-auto pr-1">
      <DialogPromptSection
        isSessionMode={isSessionMode}
        isTaskStarted={isTaskStarted}
        isPassthroughProfile={isPassthroughProfile}
        initialDescription={initialDescription}
        hasDescription={hasDescription}
        fs={fs}
        handleKeyDown={handleKeyDown}
        enhance={enhance}
        workspaceId={workspaceId}
        onJiraImport={onJiraImport}
        onLinearImport={onLinearImport}
      />
      {!isSessionMode &&
        renderCreateEditSelectors({
          isTaskStarted,
          hasRepositorySelection,
          branchOptions,
          branchesLoading,
          agentProfiles,
          agentProfilesLoading,
          agentProfileOptions,
          isCreatingSession,
          executorProfileOptions,
          executorsLoading,
          isLocalExecutor,
          workflowAgentLocked,
          fs,
          onBranchChange,
          onAgentProfileChange,
          onExecutorProfileChange,
          onToggleFreshBranch,
        })}
      <WorkflowSection
        isCreateMode={isCreateMode}
        isTaskStarted={isTaskStarted}
        workflows={workflows as Parameters<typeof WorkflowSection>[0]["workflows"]}
        snapshots={snapshots as Parameters<typeof WorkflowSection>[0]["snapshots"]}
        effectiveWorkflowId={effectiveWorkflowId}
        onWorkflowChange={onWorkflowChange}
        agentProfiles={agentProfiles}
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

type CreateEditSelectorsRenderArgs = Pick<
  DialogFormBodyProps,
  | "isTaskStarted"
  | "hasRepositorySelection"
  | "branchOptions"
  | "branchesLoading"
  | "agentProfiles"
  | "agentProfilesLoading"
  | "agentProfileOptions"
  | "isCreatingSession"
  | "executorProfileOptions"
  | "executorsLoading"
  | "isLocalExecutor"
  | "workflowAgentLocked"
  | "fs"
  | "onBranchChange"
  | "onAgentProfileChange"
  | "onExecutorProfileChange"
  | "onToggleFreshBranch"
>;

function renderCreateEditSelectors(args: CreateEditSelectorsRenderArgs) {
  const { fs } = args;
  return (
    <CreateEditSelectors
      isTaskStarted={args.isTaskStarted}
      hasRepositorySelection={args.hasRepositorySelection}
      branchOptions={args.branchOptions}
      branch={fs.branch}
      onBranchChange={args.onBranchChange}
      branchesLoading={args.branchesLoading}
      localBranchesLoading={fs.localBranchesLoading}
      agentProfiles={args.agentProfiles}
      agentProfilesLoading={args.agentProfilesLoading}
      agentProfileOptions={args.agentProfileOptions}
      agentProfileId={fs.agentProfileId}
      onAgentProfileChange={args.onAgentProfileChange}
      isCreatingSession={args.isCreatingSession}
      executorProfileOptions={args.executorProfileOptions}
      executorProfileId={fs.executorProfileId}
      onExecutorProfileChange={args.onExecutorProfileChange}
      executorsLoading={args.executorsLoading}
      isLocalExecutor={args.isLocalExecutor}
      useGitHubUrl={fs.useGitHubUrl}
      workflowAgentLocked={args.workflowAgentLocked}
      freshBranchEnabled={fs.freshBranchEnabled}
      onToggleFreshBranch={args.onToggleFreshBranch}
      currentLocalBranch={fs.currentLocalBranch}
      BranchSelectorComponent={BranchSelector}
      AgentSelectorComponent={AgentSelector}
      ExecutorProfileSelectorComponent={ExecutorProfileSelector}
    />
  );
}

function useEnhanceForDialog(fs: DialogFormState) {
  const isConfigured = useIsUtilityConfigured();
  const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({
    sessionId: null,
    taskTitle: fs.taskName,
  });
  const onEnhance = useCallback(() => {
    const current = fs.descriptionInputRef.current?.getValue()?.trim();
    if (!current) return;
    enhancePrompt(current, (enhanced) => {
      fs.descriptionInputRef.current?.setValue(enhanced);
      fs.setHasDescription(true);
    });
  }, [enhancePrompt, fs]);
  return { onEnhance, isLoading: isEnhancingPrompt, isConfigured };
}

function useJiraImportHandler(fs: DialogFormState) {
  return useCallback(
    (ticket: JiraTicket) => {
      const title = `[${ticket.key}] ${ticket.summary}`;
      fs.setTaskName(title);
      fs.setHasTitle(true);
      const description = ticket.description?.trim()
        ? `${ticket.description}\n\n---\nJira: ${ticket.url}`
        : `Jira: ${ticket.url}`;
      fs.descriptionInputRef.current?.setValue(description);
      fs.setHasDescription(true);
    },
    [fs],
  );
}

function useLinearImportHandler(fs: DialogFormState) {
  return useCallback(
    (issue: LinearIssue) => {
      const title = `[${issue.identifier}] ${issue.title}`;
      fs.setTaskName(title);
      fs.setHasTitle(true);
      const description = issue.description?.trim()
        ? `${issue.description}\n\n---\nLinear: ${issue.url}`
        : `Linear: ${issue.url}`;
      fs.descriptionInputRef.current?.setValue(description);
      fs.setHasDescription(true);
    },
    [fs],
  );
}

type SubmitWiringArgs = {
  props: TaskCreateDialogProps;
  fs: ReturnType<typeof useDialogFormState>;
  computed: ReturnType<typeof useTaskCreateDialogData>["computed"];
  repositoryLocalPath: string;
  isSessionMode: boolean;
  isEditMode: boolean;
};

function useSubmitHandlersWiring({
  props,
  fs,
  computed,
  repositoryLocalPath,
  isSessionMode,
  isEditMode,
}: SubmitWiringArgs) {
  const { workspaceId, workflowId, editingTask, onSuccess, onCreateSession, onOpenChange } = props;
  const { parentTaskId } = props;
  const taskId = props.taskId ?? null;
  return useTaskSubmitHandlers({
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
    useGitHubUrl: fs.useGitHubUrl,
    githubUrl: fs.githubUrl,
    githubPrHeadBranch: fs.githubPrHeadBranch,
    branch: fs.branch,
    agentProfileId: computed.effectiveAgentProfileId,
    executorId: fs.executorId,
    executorProfileId: fs.executorProfileId,
    editingTask,
    onSuccess,
    onCreateSession,
    onOpenChange,
    taskId,
    parentTaskId,
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
    clearDraft: fs.clearDraft,
    freshBranchEnabled: fs.freshBranchEnabled,
    isLocalExecutor: computed.isLocalExecutor,
    repositoryLocalPath,
  });
}

function useTaskCreateDialogSetup(props: TaskCreateDialogProps) {
  const { open, mode = "create", workspaceId, workflowId, defaultStepId } = props;
  const { editingTask, initialValues } = props;
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
  const repositoryLocalPath = (() => {
    if (fs.selectedLocalRepo) return fs.selectedLocalRepo.path;
    if (fs.repositoryId) {
      const repo = repositories.find((r) => r.id === fs.repositoryId);
      return repo?.local_path ?? "";
    }
    return "";
  })();
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
    workflows,
  });
  const handlers = useDialogHandlers(fs, repositories);
  const submitHandlers = useSubmitHandlersWiring({
    props,
    fs,
    computed,
    repositoryLocalPath,
    isSessionMode,
    isEditMode,
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
    enhance: useEnhanceForDialog(fs),
    handleJiraImport: useJiraImportHandler(fs),
    handleLinearImport: useLinearImportHandler(fs),
  };
}

type DialogFormProps = {
  setup: ReturnType<typeof useTaskCreateDialogSetup>;
  workspaceId: string | null;
};

function DialogForm({ setup, workspaceId }: DialogFormProps) {
  const { fs, isSessionMode, isCreateMode, isTaskStarted, workflows, agentProfiles } = setup;
  const { snapshots, branchesLoading, computed, handlers, handleKeyDown } = setup;
  const { handleJiraImport, handleLinearImport } = setup;
  const { handleSubmit, handleUpdateWithoutAgent, handleCreateWithoutAgent } = setup.submitHandlers;
  const { handleCreateWithPlanMode, handleCancel } = setup.submitHandlers;
  return (
    <form onSubmit={handleSubmit} className="flex flex-col gap-4 overflow-hidden">
      <DialogFormBody
        isSessionMode={isSessionMode}
        isCreateMode={isCreateMode}
        isTaskStarted={isTaskStarted}
        isPassthroughProfile={computed.isPassthroughProfile}
        initialDescription={fs.currentDefaults.description}
        hasDescription={fs.hasDescription}
        workspaceId={workspaceId}
        onJiraImport={handleJiraImport}
        onLinearImport={handleLinearImport}
        branchOptions={computed.branchOptions}
        branchesLoading={branchesLoading || (fs.useGitHubUrl && fs.githubBranchesLoading)}
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
        isLocalExecutor={computed.isLocalExecutor}
        enhance={setup.enhance}
        workflowAgentLocked={computed.workflowAgentLocked}
        onToggleFreshBranch={handlers.handleToggleFreshBranch}
      />
      <DialogFooter className="border-t border-border pt-3 flex-col gap-3 sm:flex-row sm:gap-2">
        <TaskCreateDialogFooter
          isSessionMode={isSessionMode}
          isCreateMode={isCreateMode}
          isEditMode={setup.isEditMode}
          isTaskStarted={isTaskStarted}
          isPassthroughProfile={computed.isPassthroughProfile}
          isCreatingSession={fs.isCreatingSession}
          isCreatingTask={fs.isCreatingTask}
          hasTitle={fs.hasTitle}
          hasDescription={fs.hasDescription}
          hasRepositorySelection={computed.hasRepositorySelection}
          branch={fs.branch}
          agentProfileId={computed.effectiveAgentProfileId}
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
  );
}

export function TaskCreateDialog(props: TaskCreateDialogProps) {
  const { open, onOpenChange, initialValues, workspaceId } = props;
  const setup = useTaskCreateDialogSetup(props);
  const { fs, isCreateMode, isEditMode, isTaskStarted } = setup;
  const { repositoriesLoading, computed, handlers } = setup;
  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="create-task-dialog"
        className="w-full h-full max-w-full max-h-full rounded-none sm:w-[900px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col"
      >
        <DialogHeader>
          <DialogHeaderContent
            isCreateMode={isCreateMode}
            isEditMode={isEditMode}
            isTaskStarted={isTaskStarted}
            sessionRepoName={setup.sessionRepoName}
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
            useGitHubUrl={fs.useGitHubUrl}
            githubUrl={fs.githubUrl}
            githubUrlError={fs.githubUrlError}
            onToggleGitHubUrl={handlers.handleToggleGitHubUrl}
            onGitHubUrlChange={handlers.handleGitHubUrlChange}
          />
        </DialogHeader>
        <DialogForm setup={setup} workspaceId={workspaceId} />
        <PendingDiscardModal pending={setup.submitHandlers.pendingDiscard} />
      </DialogContent>
    </Dialog>
  );
}

function PendingDiscardModal({
  pending,
}: {
  pending: ReturnType<typeof useTaskSubmitHandlers>["pendingDiscard"];
}) {
  if (!pending) return null;
  return (
    <DiscardLocalChangesDialog
      open
      onOpenChange={(next) => {
        if (!next) pending.resolve(false);
      }}
      dirtyFiles={pending.dirtyFiles}
      repoPath={pending.repoPath}
      onConfirm={() => pending.resolve(true)}
      onCancel={() => pending.resolve(false)}
    />
  );
}
