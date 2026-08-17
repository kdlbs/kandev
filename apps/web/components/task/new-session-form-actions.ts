import { useCallback, type RefObject } from "react";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import {
  hasPendingAttachmentUploads,
  toMessageAttachments,
} from "@/components/task-create-dialog-helpers";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { SummarizeSessionResult } from "@/hooks/use-summarize-session";
import { applySummarizeSessionResult, type SummaryToastFn } from "./session-context-summary";
import { t } from "@/lib/i18n";

type SessionContextChangeOpts = {
  promptRef: RefObject<TaskFormInputsHandle | null>;
  initialPrompt: string | null;
  summarize: (sessionId: string) => Promise<SummarizeSessionResult>;
  toast: SummaryToastFn;
  setContextValue: (v: string) => void;
  setHasPrompt: (v: boolean) => void;
};

function launchErrorDescription(error: unknown): string {
  if (error instanceof Error) return error.message;
  return t("common:unknownError");
}

export function useSessionContextChange(opts: SessionContextChangeOpts) {
  const { promptRef, initialPrompt, summarize, toast, setContextValue, setHasPrompt } = opts;
  return useCallback(
    async (value: string) => {
      if (!value) return;
      setContextValue(value);
      if (value === "copy_prompt" && initialPrompt && promptRef.current) {
        promptRef.current.setValue(initialPrompt);
        setHasPrompt(true);
      } else if (value === "blank" && promptRef.current) {
        promptRef.current.setValue("");
        setHasPrompt(false);
      } else if (value.startsWith("summarize:")) {
        const sessionId = value.slice("summarize:".length);
        const result = await summarize(sessionId);
        applySummarizeSessionResult({ result, promptRef, setContextValue, setHasPrompt, toast });
      }
    },
    [initialPrompt, promptRef, summarize, setContextValue, setHasPrompt, toast],
  );
}

export function useSessionLaunchSubmit({
  promptRef,
  taskId,
  selectedProfileId,
  profileExplicit,
  executorId,
  contextValue,
  initialPrompt,
  agentProfiles,
  groupId,
  onClose,
  toast,
  setActiveSession,
  activateSession,
  setIsCreating,
}: {
  promptRef: RefObject<TaskFormInputsHandle | null>;
  taskId: string;
  selectedProfileId: string;
  profileExplicit: boolean;
  executorId: string;
  contextValue: string;
  initialPrompt: string | null;
  agentProfiles: AgentProfileOption[];
  groupId?: string;
  onClose: () => void;
  toast: SummaryToastFn;
  setActiveSession: (taskId: string, sessionId: string) => void;
  activateSession: (
    sessionId: string,
    taskId: string,
    tabLabel: string,
    groupId: string | undefined,
    setActiveSession: (taskId: string, sessionId: string) => void,
  ) => void;
  setIsCreating: (creating: boolean) => void;
}) {
  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const typed = promptRef.current?.getValue().trim() ?? "";
      const prompt =
        contextValue === "copy_prompt" && !typed && initialPrompt ? initialPrompt : typed;
      if (!prompt) return;
      const selectedAttachments = promptRef.current?.getAttachments() ?? [];
      if (hasPendingAttachmentUploads(selectedAttachments)) return;
      setIsCreating(true);
      try {
        const { request } = buildStartRequest(taskId, selectedProfileId, {
          executorId,
          prompt,
          profileExplicit,
          attachments: toMessageAttachments(selectedAttachments),
        });
        const response = await launchSession(request);
        if (!response.session_id) {
          throw new Error("Session created but no session ID returned");
        }
        const effectiveProfileId = response.agent_profile_id ?? selectedProfileId;
        const profile = agentProfiles.find((p) => p.id === effectiveProfileId);
        activateSession(
          response.session_id,
          taskId,
          profile?.label ?? t("common:agent"),
          groupId,
          setActiveSession,
        );
        onClose();
      } catch (error) {
        toast({
          title: t("task:failedToCreateSession"),
          description: launchErrorDescription(error),
          variant: "error",
        });
      } finally {
        setIsCreating(false);
      }
    },
    [
      promptRef,
      taskId,
      selectedProfileId,
      profileExplicit,
      executorId,
      contextValue,
      initialPrompt,
      agentProfiles,
      groupId,
      onClose,
      toast,
      setActiveSession,
      activateSession,
      setIsCreating,
    ],
  );

  return handleSubmit;
}
