"use client";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogHeader, DialogFooter } from "@kandev/ui/dialog";
import type { Task } from "@/lib/types/http";
import type { TaskCreateLastUsedState } from "@/lib/state/slices/settings/types";
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
import { CreateModeSelectors } from "@/components/task-create-dialog-create-mode-selectors";
import { RepoChipsRow } from "@/components/task-create-dialog-repo-chips";
import type { TaskCreateDialogInitialValues } from "@/components/task-create-dialog-state";
import type { DialogFormBodyProps } from "@/components/task-create-dialog-types";
import {
  buildDialogFooterProps,
  buildDialogFormBodyProps,
} from "@/components/task-create-dialog-prop-builders";
import { resetTaskCreateLastUsedSync } from "@/components/task-create-dialog-handlers";
import { useAppStore } from "@/components/state-provider";
import { TaskCreateDialogPopoverContainerProvider } from "@/hooks/use-task-create-dialog-popover-container";
import { shouldShowTaskTitleField } from "@/components/task-create-dialog-helpers";
import { useTaskCreateDialogSetup } from "@/components/task-create-dialog-setup";

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
  onCreateSession?: (data: {
    prompt: string;
    agentProfileId: string;
    executorId: string;
    attachments?: ReturnType<
      typeof import("@/components/task-create-dialog-helpers").toMessageAttachments
    >;
  }) => void;
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
    autoTitle,
    isEditMode,
    isTaskStarted,
    workspaceId,
    onJiraImport,
    onLinearImport,
    fs,
    onTaskNameChange,
    onRowRepositoryChange,
    onRowBranchChange,
    onToggleRemote,
    onToggleFreshBranch,
    repositories,
    onRefreshRepositories,
    repositoriesRefreshing,
    freshBranchAvailable,
    isLocalExecutor,
    localRepositoryCreation,
  } = props;
  const showTaskName =
    shouldShowTaskTitleField(isCreateMode, isEditMode, isTaskStarted) && !autoTitle;
  const taskNameAutoFocus = !autoTitle && !isEditMode && !fs.useRemote;
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
        onPendingAttachmentUploadsChange={fs.setHasPendingAttachmentUploads}
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
      <CreateModeAgentSelectors {...props} />
      {props.bottomSlot}
    </>
  );
}

function CreateModeAgentSelectors(props: DialogFormBodyProps) {
  return (
    <CreateModeSelectors
      isTaskStarted={props.isTaskStarted}
      agentProfileOptions={props.agentProfileOptions}
      executorProfileOptions={props.executorProfileOptions}
      agentProfiles={props.agentProfiles}
      agentProfilesLoading={props.agentProfilesLoading}
      executorsLoading={props.executorsLoading}
      isCreatingSession={props.isCreatingSession}
      fs={props.fs}
      onAgentProfileChange={props.onAgentProfileChange}
      onExecutorProfileChange={props.onExecutorProfileChange}
      workflowAgentLocked={props.workflowAgentLocked}
      noCompatibleAgent={props.noCompatibleAgent}
      executorProfileName={props.executorProfileName}
    />
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
        onPendingAttachmentUploadsChange={props.fs.setHasPendingAttachmentUploads}
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

// Synthetic submit event used by the voice auto-send path. Calling the form
// handler directly (instead of `form.requestSubmit()`) matches the chat
// composer's pattern and avoids the Safari < 16 gap where `requestSubmit` is
// missing on `HTMLFormElement`. `guardedHandleSubmit` only reads
// `preventDefault` off the event, so a stubbed shape is sufficient.
const VOICE_SUBMIT_EVENT = { preventDefault: () => {} } as unknown as FormEvent;

export function TaskCreateDialog(props: TaskCreateDialogProps) {
  const { t } = useTranslation("chat");
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
        data-webkit-safe-motion="true"
        showCloseButton={false}
        overlayClassName="create-task-dialog-overlay"
        className="w-full h-full min-w-0 max-w-full max-h-full overflow-visible rounded-none pt-0 sm:w-[900px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col"
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
              <TaskCreateDialogFooter
                {...buildDialogFooterProps(
                  setup,
                  props,
                  setup.fs.hasPendingAttachmentUploads
                    ? t("chat:attachmentUploadPendingSubmit")
                    : null,
                )}
              />
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
  pending: ReturnType<typeof useTaskCreateDialogSetup>["submitHandlers"]["pendingDiscard"];
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
