"use client";

import { FormEvent, useCallback } from "react";
import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";
import { Dialog, DialogContent, DialogHeader, DialogFooter } from "@kandev/ui/dialog";
import type { Task, Repository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useKeyboardShortcutHandler } from "@/hooks/use-keyboard-shortcut";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { TaskCreateDialogFooter } from "@/components/task-create-dialog-footer";
import { DiscardLocalChangesDialog } from "@/components/discard-local-changes-dialog";
import { DialogHeaderContent } from "@/components/task-create-dialog-header";
import {
  SessionSelectors,
  WorkflowSection,
  DialogPromptSection,
} from "@/components/task-create-dialog-form-body";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import {
  AgentSelector,
  ExecutorProfileSelector,
  InlineTaskName,
} from "@/components/task-create-dialog-selectors";
import { useTaskSubmitHandlers } from "@/components/task-create-dialog-submit";
import { CreateModeSelectors } from "@/components/task-create-dialog-create-mode-selectors";
import { RepoChipsRow } from "@/components/task-create-dialog-repo-chips";
import { useToast } from "@/components/toast-provider";
import {
  useDialogFormState,
  useTaskCreateDialogEffects,
  useDialogHandlers,
  useLockedFieldSync,
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
  /**
   * Pin specific form fields to their initial values (used by feature wrappers
   * like Improve Kandev that fix the repo + branch + workflow). The current
   * implementation just passes the locks through; the chip row's first repo
   * is overwritten on each open. The flags are kept for forward compat with
   * locking the editor UI itself in a future pass.
   */
  lockedFields?: { repository?: boolean; branch?: boolean; workflow?: boolean };
  /** Optional submit hook used by Improve Kandev to wrap the description. */
  transformDescriptionBeforeSubmit?: (description: string) => Promise<string> | string;
  /** Optional override for the description placeholder. */
  descriptionPlaceholder?: string;
  /** Optional render slot above the description editor. */
  aboveDescriptionSlot?: React.ReactNode;
  /** Optional render slot inside the dialog (between body and footer). */
  extraFormSlot?: React.ReactNode;
  /** Optional render slot at the bottom of the dialog footer area. */
  bottomSlot?: React.ReactNode;
  /**
   * When set, every submit button is disabled and the tooltip surfaces this
   * exact reason (e.g. an async bootstrap step from a feature wrapper hasn't
   * completed yet). Takes precedence over the usual missing-field reasons.
   */
  submitBlockedReason?: string | null;
}

type DialogFormBodyProps = {
  isSessionMode: boolean;
  isCreateMode: boolean;
  isEditMode: boolean;
  isTaskStarted: boolean;
  isPassthroughProfile: boolean;
  initialDescription: string;
  hasDescription: boolean;
  workspaceId: string | null;
  onJiraImport?: (ticket: JiraTicket) => void;
  onLinearImport?: (issue: LinearIssue) => void;
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
  onTaskNameChange: (v: string) => void;
  onRowRepositoryChange: (key: string, value: string) => void;
  onRowBranchChange: (key: string, value: string) => void;
  onAgentProfileChange: (v: string) => void;
  onExecutorProfileChange: (v: string) => void;
  onWorkflowChange: (v: string) => void;
  onToggleGitHubUrl?: () => void;
  onGitHubUrlChange: (v: string) => void;
  onToggleFreshBranch: (enabled: boolean) => void;
  onToggleNoRepository?: () => void;
  onWorkspacePathChange: (value: string) => void;
  enhance?: { onEnhance: () => void; isLoading: boolean; isConfigured: boolean };
  workflowAgentLocked: boolean;
  /** Workspace repositories — driven into the chip row for repo + branch picks. */
  repositories: Repository[];
  /** Computed in the parent: single-row + local executor + not URL mode. */
  freshBranchAvailable: boolean;
  /**
   * True when the selected executor profile runs locally on the host. Used
   * to lock the per-row branch pill (the user's checkout dictates the
   * branch for local execution; fresh-branch mode unlocks it).
   */
  isLocalExecutor: boolean;
  /** Optional render slot above the description editor. */
  aboveDescriptionSlot?: React.ReactNode;
  /** Optional render slot inside the dialog body (rendered above the chip row). */
  extraFormSlot?: React.ReactNode;
  /** Optional render slot at the bottom of the dialog body (above the footer). */
  bottomSlot?: React.ReactNode;
  /** Optional override for the description placeholder. */
  descriptionPlaceholder?: string;
  /** When true, hides the workflow picker so the enforced workflow can't be swapped. */
  workflowLocked?: boolean;
};

function computeHasAllBranches(fs: DialogFormState): boolean {
  if (fs.noRepository) return true;
  if (fs.useGitHubUrl) return !!fs.githubBranch;
  return fs.repositories.length > 0 && fs.repositories.every((r) => !!r.branch);
}

function CreateModeBody(props: DialogFormBodyProps) {
  const {
    isCreateMode,
    isEditMode,
    isTaskStarted,
    workspaceId,
    onJiraImport,
    onLinearImport,
    agentProfileOptions,
    executorProfileOptions,
    agentProfiles,
    agentProfilesLoading,
    executorsLoading,
    isCreatingSession,
    fs,
    onTaskNameChange,
    onRowRepositoryChange,
    onRowBranchChange,
    onAgentProfileChange,
    onExecutorProfileChange,
    onToggleGitHubUrl,
    onGitHubUrlChange,
    onToggleFreshBranch,
    workflowAgentLocked,
    repositories,
    freshBranchAvailable,
    isLocalExecutor,
  } = props;
  const showTaskName = (isCreateMode || isEditMode) && !isTaskStarted;
  const taskNameAutoFocus = !isEditMode && !fs.useGitHubUrl;
  return (
    <>
      <RepoChipsRow
        fs={fs}
        repositories={repositories}
        isTaskStarted={isTaskStarted}
        workspaceId={workspaceId}
        onRowRepositoryChange={onRowRepositoryChange}
        onRowBranchChange={onRowBranchChange}
        onToggleGitHubUrl={onToggleGitHubUrl}
        onGitHubUrlChange={onGitHubUrlChange}
        freshBranchAvailable={freshBranchAvailable}
        freshBranchEnabled={fs.freshBranchEnabled}
        onToggleFreshBranch={onToggleFreshBranch}
        isLocalExecutor={isLocalExecutor}
        onToggleNoRepository={props.onToggleNoRepository}
        onWorkspacePathChange={props.onWorkspacePathChange}
      />
      {showTaskName && (
        <InlineTaskName
          value={fs.taskName}
          onChange={onTaskNameChange}
          autoFocus={taskNameAutoFocus}
        />
      )}
      <DialogPromptSection
        isSessionMode={false}
        isTaskStarted={isTaskStarted}
        isPassthroughProfile={props.isPassthroughProfile}
        initialDescription={props.initialDescription}
        hasDescription={props.hasDescription}
        fs={fs}
        handleKeyDown={props.handleKeyDown}
        enhance={props.enhance}
        workspaceId={workspaceId}
        onJiraImport={onJiraImport}
        onLinearImport={onLinearImport}
        descriptionPlaceholder={props.descriptionPlaceholder}
        aboveDescriptionSlot={props.aboveDescriptionSlot}
        extraFormSlot={props.extraFormSlot}
        autoFocusDescription={!isTaskStarted && !(showTaskName && taskNameAutoFocus)}
      />
      <CreateModeSelectors
        isTaskStarted={isTaskStarted}
        agentProfileOptions={agentProfileOptions}
        executorProfileOptions={executorProfileOptions}
        agentProfiles={agentProfiles}
        agentProfilesLoading={agentProfilesLoading}
        executorsLoading={executorsLoading}
        isCreatingSession={isCreatingSession}
        fs={fs}
        onAgentProfileChange={onAgentProfileChange}
        onExecutorProfileChange={onExecutorProfileChange}
        workflowAgentLocked={workflowAgentLocked}
      />
      {props.bottomSlot}
    </>
  );
}

function SessionModeBody(props: DialogFormBodyProps) {
  return (
    <>
      <DialogPromptSection
        isSessionMode
        isTaskStarted={props.isTaskStarted}
        isPassthroughProfile={props.isPassthroughProfile}
        initialDescription={props.initialDescription}
        hasDescription={props.hasDescription}
        fs={props.fs}
        handleKeyDown={props.handleKeyDown}
        enhance={props.enhance}
        workspaceId={props.workspaceId}
        onJiraImport={props.onJiraImport}
      />
      <SessionSelectors
        agentProfileOptions={props.agentProfileOptions}
        agentProfileId={props.fs.agentProfileId}
        onAgentProfileChange={props.onAgentProfileChange}
        agentProfilesLoading={props.agentProfilesLoading}
        isCreatingSession={props.isCreatingSession}
        executorProfileOptions={props.executorProfileOptions}
        executorProfileId={props.fs.executorProfileId}
        onExecutorProfileChange={props.onExecutorProfileChange}
        executorsLoading={props.executorsLoading}
        AgentSelectorComponent={AgentSelector}
        ExecutorProfileSelectorComponent={ExecutorProfileSelector}
      />
    </>
  );
}

function DialogFormBody(props: DialogFormBodyProps) {
  const { isSessionMode, isCreateMode, isTaskStarted, workflows, snapshots } = props;
  return (
    <div className="flex-1 space-y-4 overflow-y-auto pr-1">
      {isSessionMode ? <SessionModeBody {...props} /> : <CreateModeBody {...props} />}
      <WorkflowSection
        isCreateMode={isCreateMode}
        isTaskStarted={isTaskStarted}
        workflows={workflows as Parameters<typeof WorkflowSection>[0]["workflows"]}
        snapshots={snapshots as Parameters<typeof WorkflowSection>[0]["snapshots"]}
        effectiveWorkflowId={props.effectiveWorkflowId}
        onWorkflowChange={props.onWorkflowChange}
        agentProfiles={props.agentProfiles}
        workflowLocked={props.workflowLocked}
      />
    </div>
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
  workspaceRepositories: ReturnType<typeof useTaskCreateDialogData>["repositories"];
  repositoryLocalPath: string;
  isSessionMode: boolean;
  isEditMode: boolean;
};

function useSubmitHandlersWiring({
  props,
  fs,
  computed,
  workspaceRepositories,
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
    repositories: fs.repositories,
    discoveredRepositories: fs.discoveredRepositories,
    workspaceRepositories,
    useGitHubUrl: fs.useGitHubUrl,
    githubUrl: fs.githubUrl,
    githubPrHeadBranch: fs.githubPrHeadBranch,
    githubBranch: fs.githubBranch,
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
    setRepositories: fs.setRepositories,
    setGitHubBranch: fs.setGitHubBranch,
    setAgentProfileId: fs.setAgentProfileId,
    setExecutorId: fs.setExecutorId,
    setSelectedWorkflowId: fs.setSelectedWorkflowId,
    setFetchedSteps: fs.setFetchedSteps,
    clearDraft: fs.clearDraft,
    freshBranchEnabled: fs.freshBranchEnabled,
    isLocalExecutor: computed.isLocalExecutor,
    repositoryLocalPath,
    noRepository: fs.noRepository,
    workspacePath: fs.workspacePath,
  });
}

/**
 * Resolves the on-disk path for the (single) selected row, used by the
 * fresh-branch consent flow. Multi-row tasks hide fresh-branch in the UI,
 * so we only need to resolve a path when there is exactly one row.
 */
function resolveSingleRowLocalPath(fs: DialogFormState, repositories: Repository[]): string {
  if (fs.repositories.length !== 1) return "";
  const row = fs.repositories[0];
  if (row.localPath) return row.localPath;
  if (row.repositoryId) {
    return repositories.find((r) => r.id === row.repositoryId)?.local_path ?? "";
  }
  return "";
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
    computed,
  } = useTaskCreateDialogData(open, workspaceId, workflowId, defaultStepId, fs);
  const repositoryLocalPath = resolveSingleRowLocalPath(fs, repositories);
  useTaskCreateDialogEffects(fs, {
    open,
    workspaceId,
    workflowId,
    repositories,
    repositoriesLoading,
    agentProfiles,
    executors,
    workspaceDefaults: computed.workspaceDefaults,
    toast,
    workflows,
    isLocalExecutor: computed.isLocalExecutor,
  });
  useLockedFieldSync(open, workflowId, initialValues, fs);
  const handlers = useDialogHandlers(fs, repositories);
  const submitHandlers = useSubmitHandlersWiring({
    props,
    fs,
    computed,
    workspaceRepositories: repositories,
    repositoryLocalPath,
    isSessionMode,
    isEditMode,
  });
  const guardedHandleSubmit = useGuardedSubmit(
    submitHandlers.handleSubmit,
    props.submitBlockedReason,
  );
  const handleKeyDown = useKeyboardShortcutHandler(SHORTCUTS.SUBMIT, (event) => {
    guardedHandleSubmit(event as unknown as FormEvent);
  });
  // Fresh-branch is single-row + local executor + not URL mode. The chip row
  // can hold any number of repos; we hide the toggle whenever the question
  // ("which repo do we discard local changes in?") becomes ambiguous.
  const freshBranchAvailable =
    !fs.useGitHubUrl && computed.isLocalExecutor && fs.repositories.length === 1;
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
    repositories,
    repositoriesLoading,
    computed,
    handlers,
    submitHandlers,
    handleKeyDown,
    freshBranchAvailable,
    guardedHandleSubmit,
    enhance: useEnhanceForDialog(fs),
    handleJiraImport: useJiraImportHandler(fs),
    handleLinearImport: useLinearImportHandler(fs),
  };
}

// Buttons are disabled when submitBlockedReason is set, but the form can still
// be submitted via Enter; gate the submit path here so a wrapper's async
// bootstrap step always finishes before any task is created.
function useGuardedSubmit(
  handleSubmit: (e: FormEvent) => void,
  blockedReason: string | null | undefined,
) {
  const blocked = Boolean(blockedReason);
  return useCallback(
    (e: FormEvent) => {
      if (blocked) e.preventDefault();
      else handleSubmit(e);
    },
    [blocked, handleSubmit],
  );
}

export function TaskCreateDialog(props: TaskCreateDialogProps) {
  const { open, onOpenChange, initialValues, workspaceId } = props;
  const setup = useTaskCreateDialogSetup(props);
  const { fs, isSessionMode, isEditMode, isCreateMode, isTaskStarted } = setup;
  const { sessionRepoName, workflows, agentProfiles, snapshots, repositories } = setup;
  const { computed, handlers, handleKeyDown, freshBranchAvailable } = setup;
  const repoLocked = !!props.lockedFields?.repository;
  const { handleUpdateWithoutAgent, handleCreateWithoutAgent } = setup.submitHandlers;
  const { handleCreateWithPlanMode, handleCancel } = setup.submitHandlers;
  const { handleJiraImport, handleLinearImport, guardedHandleSubmit } = setup;
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
            sessionRepoName={sessionRepoName}
            initialTitle={initialValues?.title}
          />
        </DialogHeader>
        <form onSubmit={guardedHandleSubmit} className="flex flex-col gap-4 overflow-hidden">
          <DialogFormBody
            isSessionMode={isSessionMode}
            isCreateMode={isCreateMode}
            isEditMode={isEditMode}
            isTaskStarted={isTaskStarted}
            onTaskNameChange={handlers.handleTaskNameChange}
            onRowRepositoryChange={handlers.handleRowRepositoryChange}
            onRowBranchChange={handlers.handleRowBranchChange}
            isPassthroughProfile={computed.isPassthroughProfile}
            initialDescription={fs.currentDefaults.description}
            hasDescription={fs.hasDescription}
            workspaceId={workspaceId}
            onJiraImport={handleJiraImport}
            onLinearImport={handleLinearImport}
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
            onAgentProfileChange={handlers.handleAgentProfileChange}
            onExecutorProfileChange={handlers.handleExecutorProfileChange}
            onWorkflowChange={handlers.handleWorkflowChange}
            onToggleGitHubUrl={repoLocked ? undefined : handlers.handleToggleGitHubUrl}
            onGitHubUrlChange={handlers.handleGitHubUrlChange}
            onToggleFreshBranch={handlers.handleToggleFreshBranch}
            onToggleNoRepository={repoLocked ? undefined : handlers.handleToggleNoRepository}
            onWorkspacePathChange={handlers.handleWorkspacePathChange}
            enhance={setup.enhance}
            workflowAgentLocked={computed.workflowAgentLocked}
            repositories={repositories}
            freshBranchAvailable={freshBranchAvailable}
            isLocalExecutor={computed.isLocalExecutor}
            extraFormSlot={props.extraFormSlot}
            aboveDescriptionSlot={props.aboveDescriptionSlot}
            bottomSlot={props.bottomSlot}
            descriptionPlaceholder={props.descriptionPlaceholder}
            workflowLocked={props.lockedFields?.workflow}
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
              hasAllBranches={computeHasAllBranches(fs)}
              agentProfileId={computed.effectiveAgentProfileId}
              workspaceId={workspaceId}
              effectiveWorkflowId={computed.effectiveWorkflowId ?? null}
              executorHint={computed.executorHint}
              onCancel={handleCancel}
              onUpdateWithoutAgent={handleUpdateWithoutAgent}
              onCreateWithoutAgent={handleCreateWithoutAgent}
              onCreateWithPlanMode={handleCreateWithPlanMode}
              submitBlockedReason={props.submitBlockedReason}
            />
          </DialogFooter>
        </form>
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
