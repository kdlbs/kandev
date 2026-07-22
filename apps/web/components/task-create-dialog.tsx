"use client";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";
import { Dialog, DialogContent, DialogHeader, DialogFooter } from "@kandev/ui/dialog";
import type { Task, Repository } from "@/lib/types/http";
import type { TaskCreateLastUsedState } from "@/lib/state/slices/settings/types";
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
import type { DialogFormBodyProps } from "@/components/task-create-dialog-types";
import {
  buildDialogFooterProps,
  buildDialogFormBodyProps,
} from "@/components/task-create-dialog-prop-builders";
import { resetTaskCreateLastUsedSync } from "@/components/task-create-dialog-handlers";
import { useAppStore } from "@/components/state-provider";
import { TaskCreateDialogPopoverContainerProvider } from "@/hooks/use-task-create-dialog-popover-container";
import { shouldShowTaskTitleField } from "@/components/task-create-dialog-helpers";

export interface TaskCreateDialogProps {
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
    meta?: { taskSessionId?: string | null; willNavigate?: boolean },
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
    onToggleRemote,
    onToggleFreshBranch,
    workflowAgentLocked,
    repositories,
    onRefreshRepositories,
    repositoriesRefreshing,
    freshBranchAvailable,
    isLocalExecutor,
    localRepositoryCreation,
  } = props;
  const showTaskName = shouldShowTaskTitleField(isCreateMode, isEditMode, isTaskStarted);
  const taskNameAutoFocus = !isEditMode && !fs.useRemote;
  return (
    <>
      <RepoChipsRow
        fs={fs}
        repositories={repositories}
        isTaskStarted={isTaskStarted}
        workspaceId={workspaceId}
        onRowRepositoryChange={onRowRepositoryChange}
        onRowBranchChange={onRowBranchChange}
        onToggleRemote={onToggleRemote}
        freshBranchAvailable={freshBranchAvailable}
        freshBranchEnabled={fs.freshBranchEnabled}
        onToggleFreshBranch={onToggleFreshBranch}
        isLocalExecutor={isLocalExecutor}
        lastUsedBranch={props.lastUsedBranch}
        userSettingsLoaded={props.userSettingsLoaded}
        onToggleNoRepository={props.onToggleNoRepository}
        onWorkspacePathChange={props.onWorkspacePathChange}
        localRepositoryCreation={localRepositoryCreation}
        onRefreshRepositories={onRefreshRepositories}
        repositoriesRefreshing={repositoriesRefreshing}
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
        initialDescription={props.initialDescription}
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
        onVoiceAutoSend={props.onVoiceAutoSend}
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
        noCompatibleAgent={props.noCompatibleAgent}
        executorProfileName={props.executorProfileName}
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
        initialDescription={props.initialDescription}
        fs={props.fs}
        handleKeyDown={props.handleKeyDown}
        enhance={props.enhance}
        workspaceId={props.workspaceId}
        onJiraImport={props.onJiraImport}
        onVoiceAutoSend={props.onVoiceAutoSend}
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
  preserveQueuedLastUsedOnClose: () => void;
};

function useSubmitHandlersWiring({
  props,
  fs,
  computed,
  workspaceRepositories,
  repositoryLocalPath,
  isSessionMode,
  isEditMode,
  preserveQueuedLastUsedOnClose,
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
    useRemote: fs.useRemote,
    remoteRepos: fs.remoteRepos,
    prInfoByUrl: fs.prInfoByUrl,
    agentProfileId: computed.effectiveAgentProfileId,
    executorId: fs.executorId,
    executorProfileId: fs.executorProfileId,
    editingTask,
    onSuccess,
    onCreateSession,
    onOpenChange,
    preserveTaskCreateLastUsedOnClose: preserveQueuedLastUsedOnClose,
    taskId,
    parentTaskId,
    descriptionInputRef: fs.descriptionInputRef,
    setIsCreatingSession: fs.setIsCreatingSession,
    setIsCreatingTask: fs.setIsCreatingTask,
    setHasTitle: fs.setHasTitle,
    setHasDescription: fs.setHasDescription,
    setTaskName: fs.setTaskName,
    setRepositories: fs.setRepositories,
    setRemoteRepos: fs.setRemoteRepos,
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

export function useTaskCreateDialogSetup(
  props: TaskCreateDialogProps,
  options: { preserveQueuedLastUsedOnClose?: () => void } = {},
) {
  const { open, mode = "create", workspaceId, workflowId, defaultStepId } = props;
  const { editingTask, initialValues } = props;
  const isSessionMode = mode === "session";
  const isEditMode = mode === "edit";
  const isCreateMode = mode === "create";
  const isTaskStarted = computeIsTaskStarted(isEditMode, editingTask);
  const fs = useDialogFormState(open, workspaceId, workflowId, initialValues);
  const upsertWorkspaceRepository = useAppStore((state) => state.upsertRepository);
  const { toast } = useToast();
  const sessionRepoName = useSessionRepoName(isSessionMode);
  const {
    workflows,
    agentProfiles,
    executors,
    snapshots,
    repositories,
    repositoriesLoading,
    refreshRepositories,
    taskCreateLastUsed,
    userSettingsLoaded,
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
    compatibleAgentProfiles: computed.compatibleAgentProfiles,
    authLoaded: computed.authLoaded,
    executors,
    workspaceDefaults: computed.workspaceDefaults,
    toast,
    workflows,
    isLocalExecutor: computed.isLocalExecutor,
    lastUsedRepositoryId: taskCreateLastUsed.repositoryId,
    userSettingsLoaded,
    lastUsedAgentProfileId: taskCreateLastUsed.agentProfileId,
    lastUsedExecutorProfileId: taskCreateLastUsed.executorProfileId,
    lastUsedBranch: taskCreateLastUsed.branch,
    preserveBranch: initialValues?.checkoutBranch || initialValues?.branch,
  });
  useLockedFieldSync(open, workflowId, initialValues, fs);
  const handlers = useDialogHandlers(fs, repositories, {
    workspaceId,
    executors,
    upsertWorkspaceRepository,
  });
  const submitHandlers = useSubmitHandlersWiring({
    props,
    fs,
    computed,
    workspaceRepositories: repositories,
    repositoryLocalPath,
    isSessionMode,
    isEditMode,
    preserveQueuedLastUsedOnClose: options.preserveQueuedLastUsedOnClose ?? (() => undefined),
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
    !fs.useRemote && computed.isLocalExecutor && fs.repositories.length === 1;
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
    refreshRepositories,
    computed,
    handlers,
    submitHandlers,
    handleKeyDown,
    freshBranchAvailable,
    taskCreateLastUsed,
    userSettingsLoaded,
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

// Synthetic submit event used by the voice auto-send path. Calling the form
// handler directly (instead of `form.requestSubmit()`) matches the chat
// composer's pattern and avoids the Safari < 16 gap where `requestSubmit` is
// missing on `HTMLFormElement`. `guardedHandleSubmit` only reads
// `preventDefault` off the event, so a stubbed shape is sufficient.
const VOICE_SUBMIT_EVENT = { preventDefault: () => {} } as unknown as FormEvent;

export function TaskCreateDialog(props: TaskCreateDialogProps) {
  const syncedTaskCreateLastUsed = useAppStore((state) => state.userSettings.taskCreateLastUsed);
  const preserveQueuedLastUsedOnCloseRef = useRef<{
    syncedSettings: TaskCreateLastUsedState | null | undefined;
  } | null>(null);
  const queuedLastUsedResetHandledRef = useRef(false);
  const preserveQueuedLastUsedOnClose = useCallback(() => {
    preserveQueuedLastUsedOnCloseRef.current = { syncedSettings: syncedTaskCreateLastUsed };
  }, [syncedTaskCreateLastUsed]);
  const resetQueuedLastUsedOnClose = useCallback(() => {
    const preserveQueued = preserveQueuedLastUsedOnCloseRef.current;
    resetTaskCreateLastUsedSync({
      clearQueued: !preserveQueued,
      syncedSettings: preserveQueued?.syncedSettings,
    });
    preserveQueuedLastUsedOnCloseRef.current = null;
    queuedLastUsedResetHandledRef.current = true;
  }, []);
  const setup = useTaskCreateDialogSetup(props, { preserveQueuedLastUsedOnClose });
  const { guardedHandleSubmit } = setup;
  const [popoverContainer, setPopoverContainer] = useState<HTMLDivElement | null>(null);
  useEffect(() => {
    if (props.open) {
      preserveQueuedLastUsedOnCloseRef.current = null;
      queuedLastUsedResetHandledRef.current = false;
      return resetQueuedLastUsedOnClose;
    }
    if (queuedLastUsedResetHandledRef.current) {
      queuedLastUsedResetHandledRef.current = false;
      return;
    }
    resetQueuedLastUsedOnClose();
  }, [props.open, resetQueuedLastUsedOnClose]);
  // Voice auto-send invokes the same submit handler as the in-form Submit
  // button. Every existing validation gate (missing title/repo/branch/agent,
  // `submitBlockedReason`, in-flight create) still applies because they live
  // inside `handleSubmit` itself, so a dictation with incomplete fields
  // silently no-ops rather than creating a malformed task.
  const handleVoiceAutoSend = useCallback(() => {
    guardedHandleSubmit(VOICE_SUBMIT_EVENT);
  }, [guardedHandleSubmit]);
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        ref={setPopoverContainer}
        data-testid="create-task-dialog"
        className="w-full h-full min-w-0 max-w-full max-h-full overflow-visible rounded-none sm:w-[900px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col"
      >
        <TaskCreateDialogPopoverContainerProvider container={popoverContainer}>
          <DialogHeader>
            <DialogHeaderContent
              isCreateMode={setup.isCreateMode}
              isEditMode={setup.isEditMode}
              sessionRepoName={setup.sessionRepoName}
              initialTitle={props.initialValues?.title}
            />
          </DialogHeader>
          <form
            onSubmit={guardedHandleSubmit}
            className="flex min-w-0 flex-col gap-4 overflow-hidden"
          >
            <DialogFormBody
              {...buildDialogFormBodyProps(setup, props)}
              onVoiceAutoSend={handleVoiceAutoSend}
            />
            <DialogFooter className="border-t border-border pt-3 flex-col gap-3 sm:flex-row sm:gap-2">
              <TaskCreateDialogFooter {...buildDialogFooterProps(setup, props)} />
            </DialogFooter>
          </form>
          <PendingDiscardModal pending={setup.submitHandlers.pendingDiscard} />
        </TaskCreateDialogPopoverContainerProvider>
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
