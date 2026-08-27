"use client";

import { FormEvent, useCallback, useEffect, useRef, useState } from "react";
import { useTranslation } from "react-i18next";
import { Dialog, DialogContent, DialogHeader, DialogFooter } from "@kandev/ui/dialog";
import type { TaskCreateLastUsedState } from "@/lib/state/slices/settings/types";
import {
  isNativeSubmitDisabled,
  TaskCreateDialogFooter,
} from "@/components/task-create-dialog-footer";
import { DiscardLocalChangesDialog } from "@/components/discard-local-changes-dialog";
import { DialogHeaderContent } from "@/components/task-create-dialog-header";
import {
  SessionSelectors,
  WorkflowSection,
  DialogPromptSection,
} from "@/components/task-create-dialog-form-body";
import { CreateModeSelectors } from "@/components/task-create-dialog-create-mode-selectors";
import {
  AgentSelector,
  ExecutorProfileSelector,
  InlineTaskName,
} from "@/components/task-create-dialog-selectors";
import { RepoChipsRow } from "@/components/task-create-dialog-repo-chips";
import { TaskCreateAdvancedSettings } from "@/components/task-create-dialog-advanced-settings";
import type {
  DialogFormBodyProps,
  TaskCreateDialogProps,
} from "@/components/task-create-dialog-types";
import {
  buildDialogFooterProps,
  buildDialogFormBodyProps,
} from "@/components/task-create-dialog-prop-builders";
import { resetTaskCreateLastUsedSync } from "@/components/task-create-dialog-handlers";
import { useAppStore } from "@/components/state-provider";
import { TaskCreateDialogPopoverContainerProvider } from "@/hooks/use-task-create-dialog-popover-container";
import { shouldShowTaskTitleField } from "@/components/task-create-dialog-helpers";
import { useTaskCreateDialogSetup } from "@/components/task-create-dialog-setup";

export type { TaskCreateDialogProps } from "@/components/task-create-dialog-types";

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
    onRowPolicyChange,
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
        onRowPolicyChange={onRowPolicyChange}
        onPolicySelected={
          isLocalExecutor && freshBranchAvailable ? () => onToggleFreshBranch(true) : undefined
        }
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
        repositorySets={props.repositorySets}
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
        onComposerSubmit={props.onComposerSubmit}
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
        onComposerSubmit={props.onComposerSubmit}
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
      <TaskCreateAdvancedSettings
        isCreateMode={isCreateMode}
        isTaskStarted={isTaskStarted}
        blockedBy={props.fs.blockedBy}
        onBlockedByChange={props.fs.setBlockedBy}
        dependenciesDisabled={props.isCreatingSession}
      />
    </div>
  );
}

// Synthetic submit event used by a plugin composer action's submit. Calling the form
// handler directly (instead of `form.requestSubmit()`) matches the chat
// composer's pattern and avoids the Safari < 16 gap where `requestSubmit` is
// missing on `HTMLFormElement`. `guardedHandleSubmit` only reads
// `preventDefault` off the event, so a stubbed shape is sufficient.
const PROGRAMMATIC_SUBMIT_EVENT = { preventDefault: () => {} } as unknown as FormEvent;

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
  // Programmatic submissions use the native control's preflight before
  // entering the same guarded submit handler as the form button.
  const pendingAttachmentReason = setup.fs.hasPendingAttachmentUploads
    ? t("chat:attachmentUploadPendingSubmit")
    : null;
  const nativeSubmitDisabled = isNativeSubmitDisabled(
    buildDialogFooterProps(setup, props, pendingAttachmentReason),
  );
  const handleComposerSubmit = useCallback(() => {
    if (nativeSubmitDisabled) return false;
    guardedHandleSubmit(PROGRAMMATIC_SUBMIT_EVENT);
    return true;
  }, [guardedHandleSubmit, nativeSubmitDisabled]);
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        ref={setPopoverContainer}
        onEscapeKeyDown={(event) => {
          if (setup.isCreateMode) event.preventDefault();
        }}
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
              onComposerSubmit={handleComposerSubmit}
            />
            <DialogFooter className="border-t border-border pt-3 flex-col gap-3 sm:flex-row sm:gap-2">
              <TaskCreateDialogFooter
                {...buildDialogFooterProps(setup, props, pendingAttachmentReason)}
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
