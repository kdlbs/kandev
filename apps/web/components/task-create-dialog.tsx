"use client";

import { FormEvent, useCallback } from "react";
import type { JiraTicket } from "@/lib/types/jira";
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogFooter } from "@kandev/ui/dialog";
import type { Task, Repository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useKeyboardShortcutHandler } from "@/hooks/use-keyboard-shortcut";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { TaskCreateDialogFooter } from "@/components/task-create-dialog-footer";
import {
  SessionSelectors,
  WorkflowSection,
  DialogPromptSection,
} from "@/components/task-create-dialog-form-body";
import {
  useRepositoryOptions,
  useBranchOptions,
  useAgentProfileOptions,
} from "@/components/task-create-dialog-options";
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

type DialogHeaderContentProps = {
  isCreateMode: boolean;
  isEditMode: boolean;
  sessionRepoName?: string;
  initialTitle?: string;
};

function DialogHeaderContent(props: DialogHeaderContentProps) {
  const { isCreateMode, isEditMode, sessionRepoName, initialTitle } = props;

  if (isCreateMode || isEditMode) {
    // Header is intentionally minimal — the dialog itself signals "create"
    // and the task name input lives in the body below the repo chips. The
    // GitHub URL input + toggle moved into the chip row as well.
    return (
      <DialogTitle className="sr-only">{isEditMode ? "Edit task" : "New task"}</DialogTitle>
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
  isSessionMode: boolean;
  isCreateMode: boolean;
  isEditMode: boolean;
  isTaskStarted: boolean;
  isPassthroughProfile: boolean;
  initialDescription: string;
  hasDescription: boolean;
  workspaceId: string | null;
  onJiraImport?: (ticket: JiraTicket) => void;
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
  onToggleGitHubUrl: () => void;
  onGitHubUrlChange: (v: string) => void;
  enhance?: { onEnhance: () => void; isLoading: boolean; isConfigured: boolean };
  workflowAgentLocked: boolean;
  /** Workspace repositories — driven into the chip row for repo + branch picks. */
  repositories: Repository[];
};

function DialogFormBody({
  isSessionMode,
  isCreateMode,
  isEditMode,
  isTaskStarted,
  isPassthroughProfile,
  initialDescription,
  hasDescription,
  workspaceId,
  onJiraImport,
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
  onTaskNameChange,
  onRowRepositoryChange,
  onRowBranchChange,
  onAgentProfileChange,
  onExecutorProfileChange,
  onWorkflowChange,
  onToggleGitHubUrl,
  onGitHubUrlChange,
  enhance,
  workflowAgentLocked,
  repositories,
}: DialogFormBodyProps) {
  // Task name renders below the chip row in create/edit mode (per UX request);
  // session mode and started tasks keep the header-only label and skip this.
  const showTaskName = !isSessionMode && (isCreateMode || isEditMode) && !isTaskStarted;
  return (
    <div className="flex-1 space-y-4 overflow-y-auto pr-1">
      {!isSessionMode && (
        <RepoChipsRow
          fs={fs}
          repositories={repositories}
          isTaskStarted={isTaskStarted}
          workspaceId={workspaceId}
          onRowRepositoryChange={onRowRepositoryChange}
          onRowBranchChange={onRowBranchChange}
          onToggleGitHubUrl={onToggleGitHubUrl}
          onGitHubUrlChange={onGitHubUrlChange}
        />
      )}
      {showTaskName && (
        <InlineTaskName
          value={fs.taskName}
          onChange={onTaskNameChange}
          autoFocus={!isEditMode && !fs.useGitHubUrl}
        />
      )}
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
      />
      {!isSessionMode && (
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
      )}
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

function useTaskCreateDialogSetup(props: TaskCreateDialogProps) {
  const { open, onOpenChange, mode = "create", workspaceId, workflowId, defaultStepId } = props;
  const { editingTask, onSuccess, onCreateSession, initialValues, parentTaskId } = props;
  const taskId = props.taskId ?? null;
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
    repositories: fs.repositories,
    discoveredRepositories: fs.discoveredRepositories,
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
    repositories,
    repositoriesLoading,
    computed,
    handlers,
    submitHandlers,
    handleKeyDown,
    enhance: useEnhanceForDialog(fs),
    handleJiraImport: useJiraImportHandler(fs),
  };
}

export function TaskCreateDialog(props: TaskCreateDialogProps) {
  const { open, onOpenChange, initialValues, workspaceId } = props;
  const setup = useTaskCreateDialogSetup(props);
  const { fs, isSessionMode, isEditMode, isCreateMode, isTaskStarted } = setup;
  const { sessionRepoName, workflows, agentProfiles, snapshots, repositories } = setup;
  const { computed, handlers, handleKeyDown } = setup;
  const { handleSubmit, handleUpdateWithoutAgent, handleCreateWithoutAgent } = setup.submitHandlers;
  const { handleCreateWithPlanMode, handleCancel } = setup.submitHandlers;
  const { handleJiraImport } = setup;
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
        <form onSubmit={handleSubmit} className="flex flex-col gap-4 overflow-hidden">
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
            onToggleGitHubUrl={handlers.handleToggleGitHubUrl}
            onGitHubUrlChange={handlers.handleGitHubUrlChange}
            enhance={setup.enhance}
            workflowAgentLocked={computed.workflowAgentLocked}
            repositories={repositories}
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
              hasAllBranches={
                fs.useGitHubUrl
                  ? !!fs.githubBranch
                  : fs.repositories.length > 0 && fs.repositories.every((r) => !!r.branch)
              }
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
      </DialogContent>
    </Dialog>
  );
}
