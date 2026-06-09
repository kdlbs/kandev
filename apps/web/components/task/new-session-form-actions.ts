import { useCallback, type RefObject } from "react";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { toMessageAttachments } from "@/components/task-create-dialog-helpers";
import type { FileAttachment } from "./chat/file-attachment";
import type { AgentProfileOption } from "@/lib/state/slices";

type ToastFn = (opts: {
  title: string;
  description?: string;
  variant?: "error" | "default";
}) => void;

type SessionContextChangeOpts = {
  promptRef: RefObject<HTMLTextAreaElement | null>;
  initialPrompt: string | null;
  summarize: (sessionId: string) => Promise<string | null>;
  toast: ToastFn;
  setContextValue: (v: string) => void;
  setHasPrompt: (v: boolean) => void;
};

function sanitizePromptText(value: string): string {
  return value.replace(/\r/g, "").replace(/[<>]/g, " ");
}

export function useSessionContextChange(opts: SessionContextChangeOpts) {
  const { promptRef, initialPrompt, summarize, toast, setContextValue, setHasPrompt } = opts;
  return useCallback(
    async (value: string) => {
      if (!value) return;
      setContextValue(value);
      if (value === "copy_prompt" && initialPrompt && promptRef.current) {
        promptRef.current.value = initialPrompt;
        setHasPrompt(true);
      } else if (value === "blank" && promptRef.current) {
        promptRef.current.value = "";
        setHasPrompt(false);
      } else if (value.startsWith("summarize:")) {
        const sessionId = value.slice("summarize:".length);
        const result = await summarize(sessionId);
        if (result === null) {
          setContextValue("blank");
          toast({
            title: "Summarize failed",
            description:
              "Could not generate a summary. Check that the summarize utility agent is configured and enabled in settings.",
            variant: "error",
          });
        } else if (promptRef.current) {
          promptRef.current.value = sanitizePromptText(result);
          setHasPrompt(true);
        }
      }
    },
    [initialPrompt, promptRef, summarize, setContextValue, setHasPrompt, toast],
  );
}

export function useSessionLaunchSubmit({
  promptRef,
  taskId,
  selectedProfileId,
  executorId,
  contextValue,
  initialPrompt,
  agentProfiles,
  groupId,
  attachments,
  onClose,
  toast,
  setActiveSession,
  activateSession,
  setIsCreating,
}: {
  promptRef: RefObject<HTMLTextAreaElement | null>;
  taskId: string;
  selectedProfileId: string;
  executorId: string;
  contextValue: string;
  initialPrompt: string | null;
  agentProfiles: AgentProfileOption[];
  groupId?: string;
  attachments: FileAttachment[];
  onClose: () => void;
  toast: ToastFn;
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
      const typed = promptRef.current?.value?.trim() ?? "";
      const prompt =
        contextValue === "copy_prompt" && !typed && initialPrompt ? initialPrompt : typed;
      if (!prompt) return;
      setIsCreating(true);
      try {
        const { request } = buildStartRequest(taskId, selectedProfileId, {
          executorId,
          prompt,
          attachments: toMessageAttachments(attachments),
        });
        const response = await launchSession(request);
        if (!response.session_id) {
          throw new Error("Session created but no session ID returned");
        }
        const profile = agentProfiles.find((p) => p.id === selectedProfileId);
        activateSession(
          response.session_id,
          taskId,
          profile?.label ?? "Agent",
          groupId,
          setActiveSession,
        );
        onClose();
      } catch (error) {
        toast({
          title: "Failed to create session",
          description: error instanceof Error ? error.message : "Unknown error",
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
      executorId,
      contextValue,
      initialPrompt,
      agentProfiles,
      groupId,
      onClose,
      toast,
      setActiveSession,
      attachments,
      activateSession,
      setIsCreating,
    ],
  );

  return handleSubmit;
}
