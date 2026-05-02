"use client";

import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";
import { Dialog, DialogContent, DialogHeader, DialogFooter } from "@kandev/ui/dialog";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { useKeyboardShortcutHandler } from "@/hooks/use-keyboard-shortcut";
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
import { type DialogFormState } from "@/components/task-create-dialog-state";
import {
  useTaskCreateDialogSetup,
  type TaskCreateDialogProps,
} from "@/components/task-create-dialog-setup";

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
  onRefreshBranches: () => void;
  branchesFetchedAt?: string;
  branchesFetchError?: string;
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
  extraFormSlot?: React.ReactNode;
  lockedFields?: TaskCreateDialogProps["lockedFields"];
  descriptionPlaceholder?: string;
  aboveDescriptionSlot?: React.ReactNode;
  bottomSlot?: React.ReactNode;
};

// eslint-disable-next-line max-lines-per-function -- thin pass-through; each section already factored into its own component
function DialogFormBody(p: DialogFormBodyProps) {
  const {
    isSessionMode,
    isCreateMode,
    isTaskStarted,
    branchOptions,
    branchesLoading,
    onRefreshBranches,
    branchesFetchedAt,
    branchesFetchError,
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
    onBranchChange,
    onAgentProfileChange,
    onExecutorProfileChange,
    onWorkflowChange,
    hasRepositorySelection,
    isLocalExecutor,
    workflowAgentLocked,
    onToggleFreshBranch,
    lockedFields,
  } = p;
  return (
    <div className="flex-1 space-y-4 overflow-y-auto pr-1">
      <DialogPromptSection
        isSessionMode={isSessionMode}
        isTaskStarted={isTaskStarted}
        isPassthroughProfile={p.isPassthroughProfile}
        initialDescription={p.initialDescription}
        hasDescription={p.hasDescription}
        fs={fs}
        handleKeyDown={p.handleKeyDown}
        enhance={p.enhance}
        workspaceId={p.workspaceId}
        descriptionPlaceholder={p.descriptionPlaceholder}
        onJiraImport={p.onJiraImport}
        onLinearImport={p.onLinearImport}
        extraFormSlot={p.extraFormSlot}
        aboveDescriptionSlot={p.aboveDescriptionSlot}
      />

      {!isSessionMode &&
        renderCreateEditSelectors({
          isTaskStarted,
          hasRepositorySelection,
          branchOptions,
          branchesLoading,
          onRefreshBranches,
          branchesFetchedAt,
          branchesFetchError,
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
          branchLocked: !!lockedFields?.branch,
        })}
      {!lockedFields?.workflow && (
        <WorkflowSection
          isCreateMode={isCreateMode}
          isTaskStarted={isTaskStarted}
          workflows={workflows as Parameters<typeof WorkflowSection>[0]["workflows"]}
          snapshots={snapshots as Parameters<typeof WorkflowSection>[0]["snapshots"]}
          effectiveWorkflowId={effectiveWorkflowId}
          onWorkflowChange={onWorkflowChange}
          agentProfiles={agentProfiles}
        />
      )}
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
      {p.bottomSlot}
    </div>
  );
}

type CreateEditSelectorsRenderArgs = Pick<
  DialogFormBodyProps,
  | "isTaskStarted"
  | "hasRepositorySelection"
  | "branchOptions"
  | "branchesLoading"
  | "onRefreshBranches"
  | "branchesFetchedAt"
  | "branchesFetchError"
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
> & { branchLocked?: boolean };

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
      onRefreshBranches={args.onRefreshBranches}
      branchesFetchedAt={args.branchesFetchedAt}
      branchesFetchError={args.branchesFetchError}
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
      branchLocked={args.branchLocked}
      BranchSelectorComponent={BranchSelector}
      AgentSelectorComponent={AgentSelector}
      ExecutorProfileSelectorComponent={ExecutorProfileSelector}
    />
  );
}

type DialogFormProps = {
  setup: ReturnType<typeof useTaskCreateDialogSetup>;
  workspaceId: string | null;
  extraFormSlot?: React.ReactNode;
  lockedFields?: TaskCreateDialogProps["lockedFields"];
  descriptionPlaceholder?: string;
  aboveDescriptionSlot?: React.ReactNode;
  bottomSlot?: React.ReactNode;
};

function DialogForm({
  setup,
  workspaceId,
  extraFormSlot,
  lockedFields,
  descriptionPlaceholder,
  aboveDescriptionSlot,
  bottomSlot,
}: DialogFormProps) {
  const { fs, isSessionMode, isCreateMode, isTaskStarted, workflows, agentProfiles } = setup;
  const { snapshots, branchesLoading, computed, handlers, handleKeyDown } = setup;
  const { handleJiraImport, handleLinearImport, refreshBranches, branchesFetchedAt } = setup;
  const { branchesFetchError } = setup;
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
        onRefreshBranches={refreshBranches}
        branchesFetchedAt={branchesFetchedAt}
        branchesFetchError={branchesFetchError}
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
        extraFormSlot={extraFormSlot}
        lockedFields={lockedFields}
        descriptionPlaceholder={descriptionPlaceholder}
        aboveDescriptionSlot={aboveDescriptionSlot}
        bottomSlot={bottomSlot}
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
  const setup = useTaskCreateDialogSetup(props);
  const { fs, computed, handlers } = setup;
  return (
    <Dialog open={props.open} onOpenChange={props.onOpenChange}>
      <DialogContent
        data-testid="create-task-dialog"
        className="w-full h-full max-w-full max-h-full rounded-none sm:w-[900px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col"
      >
        <DialogHeader>
          <RenderHeader
            initialTitle={props.initialValues?.title}
            workspaceId={props.workspaceId}
            isCreateMode={setup.isCreateMode}
            isEditMode={setup.isEditMode}
            isTaskStarted={setup.isTaskStarted}
            sessionRepoName={setup.sessionRepoName}
            fs={fs}
            repositoriesLoading={setup.repositoriesLoading}
            computed={computed}
            handlers={handlers}
            repositoryLocked={!!props.lockedFields?.repository}
          />
        </DialogHeader>
        <DialogForm
          setup={setup}
          workspaceId={props.workspaceId}
          extraFormSlot={props.extraFormSlot}
          lockedFields={props.lockedFields}
          descriptionPlaceholder={props.descriptionPlaceholder}
          aboveDescriptionSlot={props.aboveDescriptionSlot}
          bottomSlot={props.bottomSlot}
        />
        <PendingDiscardModal pending={setup.submitHandlers.pendingDiscard} />
      </DialogContent>
    </Dialog>
  );
}

type RenderHeaderProps = {
  initialTitle: string | undefined;
  workspaceId: string | null | undefined;
  isCreateMode: boolean;
  isEditMode: boolean;
  isTaskStarted: boolean;
  sessionRepoName: string | null | undefined;
  fs: ReturnType<typeof useTaskCreateDialogSetup>["fs"];
  repositoriesLoading: boolean;
  computed: ReturnType<typeof useTaskCreateDialogSetup>["computed"];
  handlers: ReturnType<typeof useTaskCreateDialogSetup>["handlers"];
  repositoryLocked?: boolean;
};

function RenderHeader(props: RenderHeaderProps) {
  const { fs, computed, handlers } = props;
  return (
    <DialogHeaderContent
      isCreateMode={props.isCreateMode}
      isEditMode={props.isEditMode}
      isTaskStarted={props.isTaskStarted}
      sessionRepoName={props.sessionRepoName ?? undefined}
      initialTitle={props.initialTitle}
      taskName={fs.taskName}
      repositoryId={fs.repositoryId}
      discoveredRepoPath={fs.discoveredRepoPath}
      workspaceId={props.workspaceId ?? null}
      repositoriesLoading={props.repositoriesLoading}
      discoverReposLoading={fs.discoverReposLoading}
      headerRepositoryOptions={computed.headerRepositoryOptions}
      onRepositoryChange={handlers.handleRepositoryChange}
      onTaskNameChange={handlers.handleTaskNameChange}
      useGitHubUrl={fs.useGitHubUrl}
      githubUrl={fs.githubUrl}
      githubUrlError={fs.githubUrlError}
      onToggleGitHubUrl={handlers.handleToggleGitHubUrl}
      onGitHubUrlChange={handlers.handleGitHubUrlChange}
      repositoryLocked={props.repositoryLocked}
    />
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
