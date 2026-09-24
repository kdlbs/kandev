"use client";

import { useCallback, useMemo, useRef } from "react";
import { createTask } from "@/lib/api/domains/kanban-api";
import { replaceTaskUrl } from "@/lib/links";
import { useAppStore } from "@/components/state-provider";
import {
  buildWorkspaceSourcesPayload,
  hasPendingAttachmentUploads,
  toMessageAttachments,
} from "@/components/task-create-dialog-helpers";
import { useToast } from "@/components/toast-provider";
import { usePromptResultDelivery } from "@/hooks/use-prompt-result-delivery";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import type { Repository } from "@/lib/types/http";
import { hasUnavailablePickerRemoteProvider } from "@/components/task-create-dialog-remote-provider-readiness";
import { resolveRepositorySelections } from "@/components/task-create-dialog-repositories-state";
import type { TaskRepositorySelection } from "@/components/task-create-dialog-types";
import type { SubtaskWorkspaceMode, useSubtaskFormState } from "./new-subtask-form-state";
import { toContextItems, useDialogAttachments } from "./session-dialog-shared";
import { t } from "@/lib/i18n";

type UseSubtaskSubmitOpts = {
  fs: ReturnType<typeof useSubtaskFormState>;
  parentTaskId: string;
  defaultProfileId: string;
  workspaceId: string | null;
  workflowId: string | null;
  availableRepositories: Repository[];
  attachments: ReturnType<typeof useDialogAttachments>["attachments"];
  resolvePrompt: () => string;
  title: string;
  autoTitle?: boolean;
  autopilot?: boolean;
  setIsCreating: (v: boolean) => void;
  onClose: () => void;
  /** Workspace mode for the new subtask (handoffs phase 5). */
  workspaceMode: SubtaskWorkspaceMode;
  /** Whether the selected executor profile runs directly on the local clone. */
  isLocalExecutor?: boolean;
  /** True when local repositories must be resolved from their remote origin. */
  remoteOriginMode?: boolean;
  /** Shared executor/source compatibility gate from the dialog. */
  sourcePolicyInvalid?: boolean;
};

type CreateSubtaskArgs = {
  fs: UseSubtaskSubmitOpts["fs"];
  parentTaskId: string;
  defaultProfileId: string;
  workspaceId: string;
  workflowId: string;
  availableRepositories: Repository[];
  attachments: UseSubtaskSubmitOpts["attachments"];
  trimmedTitle: string;
  prompt: string;
  autoTitle: boolean;
  autopilot: boolean;
  workspaceMode: SubtaskWorkspaceMode;
  isLocalExecutor: boolean;
  remoteOriginMode: boolean;
  freshBranchEnabled: boolean;
  onClose: () => void;
  setActiveTask: (taskId: string) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
};

export function shouldSubmitFreshBranch({
  selections,
  freshBranchEnabled,
  workspaceMode,
  isLocalExecutor,
}: {
  selections: TaskRepositorySelection[];
  freshBranchEnabled: boolean;
  workspaceMode: SubtaskWorkspaceMode;
  isLocalExecutor: boolean;
}): boolean {
  if (!freshBranchEnabled || workspaceMode !== "new_workspace" || !isLocalExecutor) return false;
  if (selections.length !== 1) return false;
  const selection = selections[0];
  return selection.kind === "local" && Boolean(selection.repositoryId || selection.localPath);
}

async function createSubtask({
  fs,
  parentTaskId,
  defaultProfileId,
  workspaceId,
  workflowId,
  availableRepositories,
  attachments,
  trimmedTitle,
  prompt,
  autoTitle,
  autopilot,
  workspaceMode,
  isLocalExecutor,
  remoteOriginMode,
  freshBranchEnabled,
  onClose,
  setActiveTask,
  setActiveSession,
}: CreateSubtaskArgs) {
  const workspaceSources =
    workspaceMode === "inherit_parent"
      ? undefined
      : buildWorkspaceSourcesPayload({
          selections: fs.repositorySelections,
          useRemote: fs.useRemote,
          remoteRepos: fs.remoteRepos,
          prInfoByUrl: fs.prInfoByUrl,
          repositories: fs.repositories,
          discoveredRepositories: fs.discoveredRepositories,
          workspaceRepositories: availableRepositories,
          isLocalExecutor,
          remoteOriginMode,
          freshBranch: freshBranchEnabled
            ? { confirmDiscard: false, consentedDirtyFiles: [] }
            : undefined,
        });
  const response = await createTask({
    workspace_id: workspaceId,
    workflow_id: workflowId,
    ...(autoTitle ? { auto_title: true } : { title: trimmedTitle }),
    description: prompt,
    ...(workspaceSources !== undefined ? { workspace_sources: workspaceSources } : {}),
    start_agent: true,
    agent_profile_id: fs.agentProfileId || defaultProfileId || undefined,
    executor_profile_id:
      workspaceMode === "inherit_parent" ? undefined : fs.executorProfileId || undefined,
    parent_id: parentTaskId,
    attachments: toMessageAttachments(attachments),
    workspace_mode: workspaceMode,
    autopilot: autopilot || undefined,
  });
  const newSessionId = response.session_id ?? response.primary_session_id ?? null;
  // Close the dialog before navigation. Navigation can remount the sidebar
  // that owns the dialog state, which makes a later close update a stale owner.
  onClose();
  if (newSessionId) {
    setActiveTask(response.id);
    setActiveSession(response.id, newSessionId);
    replaceTaskUrl(response.id);
  }
}

function canStartSubtaskSubmission({
  fs,
  trimmedTitle,
  prompt,
  autoTitle,
  workspaceId,
  workflowId,
  attachments,
  workspaceMode,
  sourcePolicyInvalid,
}: {
  fs: UseSubtaskSubmitOpts["fs"];
  trimmedTitle: string;
  prompt: string;
  autoTitle: boolean;
  workspaceId: string | null;
  workflowId: string | null;
  attachments: UseSubtaskSubmitOpts["attachments"];
  workspaceMode: SubtaskWorkspaceMode;
  sourcePolicyInvalid: boolean;
}): boolean {
  if ((!autoTitle && !trimmedTitle) || !prompt || !workspaceId || !workflowId) return false;
  if (hasPendingAttachmentUploads(attachments)) return false;
  if (workspaceMode !== "inherit_parent" && sourcePolicyInvalid) return false;
  if (workspaceMode === "inherit_parent") return true;
  return !hasUnavailablePickerRemoteProvider(
    resolveRepositorySelections(fs),
    fs.remoteProviderReadiness,
  );
}

/**
 * Encapsulates the subtask creation flow: builds the repositories payload,
 * calls createTask, and activates the new session. Returns `handleSubmit`
 * so the surrounding component stays under the per-function complexity cap.
 */
// eslint-disable-next-line max-lines-per-function -- the hook owns one atomic submit lifecycle.
export function useSubtaskSubmit(opts: UseSubtaskSubmitOpts) {
  const {
    fs,
    parentTaskId,
    defaultProfileId,
    workspaceId,
    workflowId,
    availableRepositories,
    attachments,
    resolvePrompt,
    title,
    autoTitle = false,
    autopilot = false,
    setIsCreating,
    onClose,
    workspaceMode,
    isLocalExecutor = false,
    remoteOriginMode = false,
    sourcePolicyInvalid = false,
  } = opts;
  const { toast } = useToast();
  const setActiveTask = useAppStore((s) => s.setActiveTask);
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const isSubmittingRef = useRef(false);
  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (isSubmittingRef.current) return;
      const trimmedTitle = title.trim();
      const prompt = resolvePrompt().trim();
      if (
        !canStartSubtaskSubmission({
          fs,
          trimmedTitle,
          prompt,
          autoTitle,
          workspaceId,
          workflowId,
          attachments,
          workspaceMode,
          sourcePolicyInvalid,
        })
      )
        return;
      if (!workspaceId || !workflowId) return;
      isSubmittingRef.current = true;
      setIsCreating(true);
      try {
        await createSubtask({
          fs,
          parentTaskId,
          defaultProfileId,
          workspaceId,
          workflowId,
          availableRepositories,
          attachments,
          trimmedTitle,
          prompt,
          autoTitle,
          autopilot,
          workspaceMode,
          isLocalExecutor,
          remoteOriginMode,
          freshBranchEnabled: shouldSubmitFreshBranch({
            selections: resolveRepositorySelections(fs),
            freshBranchEnabled: fs.freshBranchEnabled,
            workspaceMode,
            isLocalExecutor,
          }),
          onClose,
          setActiveTask,
          setActiveSession,
        });
      } catch (error) {
        toast({
          title: t("task:failedToCreateSubtask"),
          description: error instanceof Error ? error.message : t("common:unknownError"),
          variant: "error",
        });
      } finally {
        isSubmittingRef.current = false;
        setIsCreating(false);
      }
    },
    [
      title,
      autoTitle,
      autopilot,
      workspaceId,
      workflowId,
      resolvePrompt,
      fs,
      parentTaskId,
      defaultProfileId,
      availableRepositories,
      attachments,
      setActiveTask,
      setActiveSession,
      workspaceMode,
      isLocalExecutor,
      remoteOriginMode,
      sourcePolicyInvalid,
      setIsCreating,
      onClose,
      toast,
    ],
  );

  return { handleSubmit };
}

/**
 * Bundles the prompt textarea ref, attachments, enhance-prompt action, and
 * derived context items used by the subtask form. Returns the values the form
 * needs without spreading hook/state plumbing across the parent component.
 */
export function useSubtaskPromptZone(opts: {
  parentTaskId: string;
  workspaceId?: string | null;
  taskTitle: string;
  inputDisabled: boolean;
  contextValue: string;
  initialPrompt: string | null;
  promptValue: string;
  setPromptValue: (value: string) => void;
  setHasPrompt: (v: boolean) => void;
}) {
  const {
    parentTaskId,
    workspaceId,
    taskTitle,
    inputDisabled,
    contextValue,
    initialPrompt,
    promptValue,
    setPromptValue,
    setHasPrompt,
  } = opts;
  const promptRef = useRef<HTMLTextAreaElement>(null);
  const latestPromptValueRef = useRef(promptValue);
  latestPromptValueRef.current = promptValue;
  const { toast } = useToast();
  const attachments = useDialogAttachments(inputDisabled, workspaceId);
  const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({
    sessionId: null,
    taskTitle,
  });
  const promptResultDelivery = usePromptResultDelivery({
    scopeKey: `new-subtask:${parentTaskId}`,
    getCurrent: () => latestPromptValueRef.current,
    apply: (value) => {
      if (!promptRef.current) {
        return false;
      }

      setPromptValue(value);
      setHasPrompt(value.trim().length > 0);
      return true;
    },
  });
  const handleEnhancePrompt = useCallback(async () => {
    const current = latestPromptValueRef.current;
    if (!current.trim()) return;
    const generation = promptResultDelivery.captureScope();

    await enhancePrompt(current, (enhanced) => {
      const delivered = promptResultDelivery.deliver(current, enhanced, generation);
      if (delivered) {
        toast({ description: t("task:enhancedPromptApplied"), variant: "success" });
      }

      return delivered;
    });
  }, [enhancePrompt, promptResultDelivery, toast]);
  const contextItems = useMemo(
    () =>
      toContextItems(
        attachments.attachments,
        attachments.handleRemoveAttachment,
        attachments.handleRetryAttachment,
      ),
    [
      attachments.attachments,
      attachments.handleRemoveAttachment,
      attachments.handleRetryAttachment,
    ],
  );
  const resolvePrompt = useCallback(() => {
    const typed = promptValue.trim();
    if (contextValue === "copy_prompt" && !typed && initialPrompt) return initialPrompt;
    return typed;
  }, [contextValue, initialPrompt, promptValue]);
  return {
    promptRef,
    attachments,
    contextItems,
    handleEnhancePrompt,
    isEnhancingPrompt,
    pendingResult: promptResultDelivery.pendingResult,
    applyPending: promptResultDelivery.applyPending,
    copyPending: promptResultDelivery.copyPending,
    resolvePrompt,
  };
}
