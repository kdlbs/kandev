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
  setIsCreating: (v: boolean) => void;
  onClose: () => void;
  /** Workspace mode for the new subtask (handoffs phase 5). */
  workspaceMode: SubtaskWorkspaceMode;
};

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

  const performCreate = useCallback(
    async (trimmedTitle: string, prompt: string) => {
      // inherit_parent: omit repositories — backend inherits parent repos
      // and records workspace-group membership for launch reuse.
      const repositories =
        workspaceMode === "inherit_parent"
          ? undefined
          : buildRepositoriesPayload({
              useGitHubUrl: fs.useGitHubUrl,
              githubUrl: fs.githubUrl,
              githubBranch: fs.githubBranch,
              githubPrHeadBranch: fs.githubPrHeadBranch,
              repositories: fs.repositories,
              discoveredRepositories: fs.discoveredRepositories,
              workspaceRepositories: availableRepositories,
            });
      const response = await createTask({
        workspace_id: workspaceId!,
        workflow_id: workflowId!,
        title: trimmedTitle,
        description: prompt,
        repositories,
        start_agent: true,
        agent_profile_id: fs.agentProfileId || defaultProfileId || undefined,
        executor_profile_id: fs.executorProfileId || undefined,
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
    },
    // The fs object is referenced as a whole so React's deep dependency
    // tracking re-runs performCreate when any of its fields change.
    [
      fs,
      availableRepositories,
      defaultProfileId,
      workspaceId,
      workflowId,
      parentTaskId,
      attachments,
      setActiveTask,
      setActiveSession,
      workspaceMode,
    ],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      if (isSubmittingRef.current) return;
      const trimmedTitle = title.trim();
      if (!trimmedTitle || !workspaceId || !workflowId) return;
      const prompt = resolvePrompt();
      if (!prompt) return;

      isSubmittingRef.current = true;
      setIsCreating(true);
      try {
        await performCreate(trimmedTitle, prompt);
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
    [title, workspaceId, workflowId, resolvePrompt, performCreate, setIsCreating, onClose, toast],
  );

  return { handleSubmit };
}

/**
 * Bundles the prompt textarea ref, attachments, enhance-prompt action, and
 * derived context items used by the subtask form. Returns the values the form
 * needs without spreading hook/state plumbing across the parent component.
 */
export function useSubtaskPromptZone(opts: {
  taskTitle: string;
  inputDisabled: boolean;
  contextValue: string;
  initialPrompt: string | null;
  setHasPrompt: (v: boolean) => void;
}) {
  const { taskTitle, inputDisabled, contextValue, initialPrompt, setHasPrompt } = opts;
  const promptRef = useRef<HTMLTextAreaElement>(null);
  const attachments = useDialogAttachments(inputDisabled);
  const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({
    sessionId: null,
    taskTitle,
  });
  const handleEnhancePrompt = useCallback(() => {
    const current = promptRef.current?.value?.trim();
    if (!current) return;
    enhancePrompt(current, (enhanced) => {
      if (promptRef.current) {
        promptRef.current.value = enhanced;
        setHasPrompt(true);
      }
    });
  }, [enhancePrompt, setHasPrompt]);
  const contextItems = useMemo(
    () => toContextItems(attachments.attachments, attachments.handleRemoveAttachment),
    [attachments.attachments, attachments.handleRemoveAttachment],
  );
  const resolvePrompt = useCallback(() => {
    const typed = promptRef.current?.value?.trim() ?? "";
    if (contextValue === "copy_prompt" && !typed && initialPrompt) return initialPrompt;
    return typed;
  }, [contextValue, initialPrompt]);
  return {
    promptRef,
    attachments,
    contextItems,
    handleEnhancePrompt,
    isEnhancingPrompt,
    resolvePrompt,
  };
}
