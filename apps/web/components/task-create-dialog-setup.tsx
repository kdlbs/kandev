"use client";

import { FormEvent, useCallback } from "react";
import type { JiraTicket } from "@/lib/types/jira";
import type { LinearIssue } from "@/lib/types/linear";
import type { Task } from "@/lib/types/http";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useKeyboardShortcutHandler } from "@/hooks/use-keyboard-shortcut";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { useTaskSubmitHandlers } from "@/components/task-create-dialog-submit";
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
    meta?: { taskSessionId?: string | null },
  ) => void;
  onCreateSession?: (data: { prompt: string; agentProfileId: string; executorId: string }) => void;
  initialValues?: TaskCreateDialogInitialValues;
  taskId?: string | null;
  parentTaskId?: string;
  /**
   * Optional UI rendered below the description textarea. Used by feature wrappers
   * such as Improve Kandev to add capture toggles and pre-flight status without
   * forking the dialog.
   */
  extraFormSlot?: React.ReactNode;
  /**
   * Optional async transform applied to the task description before the API
   * payload is built. Awaited; receives the trimmed description and returns
   * the value to send. Used to append context-bundle file paths.
   */
  transformDescriptionBeforeSubmit?: (description: string) => Promise<string> | string;
  /**
   * Optional locks for individual fields. When a flag is true, the
   * corresponding selector is disabled (or hidden, for workflow). Used by
   * feature wrappers such as Improve Kandev to enforce a fixed
   * repository / branch / workflow selection.
   */
  lockedFields?: { repository?: boolean; branch?: boolean; workflow?: boolean };
  /**
   * Optional override for the task description textarea placeholder. Used by
   * feature wrappers (e.g. Improve Kandev) to suggest the kind of input
   * expected.
   */
  descriptionPlaceholder?: string;
  /**
   * Optional slot rendered above the description textarea (e.g. a bug/feature
   * tab toggle for Improve Kandev).
   */
  aboveDescriptionSlot?: React.ReactNode;
  /**
   * Optional slot rendered below the branch/agent/executor selectors (e.g.
   * a workflow preview or contextual help for Improve Kandev).
   */
  bottomSlot?: React.ReactNode;
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
    freshBranchEnabled: fs.freshBranchEnabled,
    isLocalExecutor: computed.isLocalExecutor,
    repositoryLocalPath,
    transformDescriptionBeforeSubmit: props.transformDescriptionBeforeSubmit,
  });
}

export function useTaskCreateDialogSetup(props: TaskCreateDialogProps) {
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
    branchesLoading,
    computed,
  } = useTaskCreateDialogData(open, workspaceId, workflowId, defaultStepId, fs);
  // Multi-repo: the "repo local path" used by the fresh-branch preflight is
  // the first chip's path. Empty when no chip has a repo selected (URL mode
  // or empty rows). Discovered (path-only) chips short-circuit the lookup.
  const repositoryLocalPath = (() => {
    const first = fs.repositories[0];
    if (!first) return "";
    if (first.localPath) return first.localPath;
    if (first.repositoryId) {
      const repo = repositories.find((r) => r.id === first.repositoryId);
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
    agentProfiles,
    executors,
    workspaceDefaults: computed.workspaceDefaults,
    toast,
    workflows,
  });
  useLockedFieldSync(open, workflowId, initialValues, fs);
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
