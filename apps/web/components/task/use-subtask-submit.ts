"use client";

import { useCallback, useMemo, useRef } from "react";
import { createTask } from "@/lib/api/domains/kanban-api";
import { replaceTaskUrl } from "@/lib/links";
import { useAppStore } from "@/components/state-provider";
import {
  buildRepositoriesPayload,
  toMessageAttachments,
} from "@/components/task-create-dialog-helpers";
import { useToast } from "@/components/toast-provider";
import { usePromptResultDelivery } from "@/hooks/use-prompt-result-delivery";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import type { Repository } from "@/lib/types/http";
import type { SubtaskWorkspaceMode, useSubtaskFormState } from "./new-subtask-form-state";
import { toContextItems, useDialogAttachments } from "./session-dialog-shared";

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
  setIsCreating: (v: boolean) => void;
  onClose: () => void;
  /** Workspace mode for the new subtask (handoffs phase 5). */
  workspaceMode: SubtaskWorkspaceMode;
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
  workspaceMode: SubtaskWorkspaceMode;
  setActiveTask: (taskId: string) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
};

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
  workspaceMode,
  setActiveTask,
  setActiveSession,
}: CreateSubtaskArgs) {
  const repositories =
    workspaceMode === "inherit_parent"
      ? undefined
      : buildRepositoriesPayload({
          useRemote: fs.useRemote,
          remoteRepos: fs.remoteRepos,
          prInfoByUrl: fs.prInfoByUrl,
          repositories: fs.repositories,
          discoveredRepositories: fs.discoveredRepositories,
          workspaceRepositories: availableRepositories,
        });
  const response = await createTask({
    workspace_id: workspaceId,
    workflow_id: workflowId,
    ...(autoTitle ? { auto_title: true } : { title: trimmedTitle }),
    description: prompt,
    repositories,
    start_agent: true,
    agent_profile_id: fs.agentProfileId || defaultProfileId || undefined,
    executor_profile_id:
      workspaceMode === "inherit_parent" ? undefined : fs.executorProfileId || undefined,
    parent_id: parentTaskId,
    attachments: toMessageAttachments(attachments),
    workspace_mode: workspaceMode,
  });
  const newSessionId = response.session_id ?? response.primary_session_id ?? null;
  if (newSessionId) {
    setActiveTask(response.id);
    setActiveSession(response.id, newSessionId);
    replaceTaskUrl(response.id);
  }
}

/**
 * Encapsulates the subtask creation flow: builds the repositories payload,
 * calls createTask, and activates the new session. Returns `handleSubmit`
 * so the surrounding component stays under the per-function complexity cap.
 */
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
    setIsCreating,
    onClose,
    workspaceMode,
  } = opts;
  const { toast } = useToast();
  const setActiveTask = useAppStore((s) => s.setActiveTask);
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  // Synchronous guard: setIsCreating(true) won't reflect into the disabled
  // submit button until React commits, so a fast double-submit (Enter + click,
  // double-click) can re-enter handleSubmit and call createTask twice.
  const isSubmittingRef = useRef(false);

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (isSubmittingRef.current) return;
      const trimmedTitle = title.trim();
      const prompt = resolvePrompt().trim();
      if ((!autoTitle && !trimmedTitle) || !prompt || !workspaceId || !workflowId) return;

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
          workspaceMode,
          setActiveTask,
          setActiveSession,
        });
        onClose();
      } catch (error) {
        toast({
          title: "Failed to create subtask",
          description: error instanceof Error ? error.message : "Unknown error",
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
  const attachments = useDialogAttachments(inputDisabled);
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
        toast({ description: "Enhanced prompt applied.", variant: "success" });
      }

      return delivered;
    });
  }, [enhancePrompt, promptResultDelivery, toast]);
  const contextItems = useMemo(
    () => toContextItems(attachments.attachments, attachments.handleRemoveAttachment),
    [attachments.attachments, attachments.handleRemoveAttachment],
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
