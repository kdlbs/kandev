import { useCallback, type RefObject } from "react";
import { launchSession } from "@/lib/services/session-launch-service";
import { useAppStore } from "@/components/state-provider";
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
import { recordAgentProfileRecentUseBestEffort } from "@/lib/agent-profile-recent-use";
import type { AgentProfileRecentUseRecord } from "@/lib/agent-profile-recent-use";

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

function recordTaskSessionProfileUse(
  profileId: string,
  applyAgentProfileRecentUse: (
    context: "task_session",
    record: AgentProfileRecentUseRecord,
  ) => void,
) {
  recordAgentProfileRecentUseBestEffort("task_session", profileId, (record) =>
    applyAgentProfileRecentUse("task_session", record),
  );
}

function resolveSessionPrompt(
  promptRef: RefObject<TaskFormInputsHandle | null>,
  contextValue: string,
  initialPrompt: string | null,
): string {
  const typed = promptRef.current?.getValue().trim() ?? "";
  return contextValue === "copy_prompt" && !typed && initialPrompt ? initialPrompt : typed;
}

function buildSessionStartOptions(
  executorId: string,
  prompt: string,
  profileExplicit: boolean,
  attachments: ReturnType<typeof toMessageAttachments>,
  mcpServerIds: string[] | undefined,
) {
  const options = { executorId, prompt, profileExplicit, attachments };
  return mcpServerIds?.length ? { ...options, mcpServerIds } : options;
}

function requireSessionId(sessionId: string | null | undefined): string {
  if (!sessionId) throw new Error("Session created but no session ID returned");
  return sessionId;
}

type SessionLaunchActionArgs = {
  taskId: string;
  selectedProfileId: string;
  profileExplicit: boolean;
  executorId: string;
  prompt: string;
  selectedAttachments: ReturnType<typeof toMessageAttachments>;
  mcpServerIds: string[] | undefined;
  agentProfiles: AgentProfileOption[];
  groupId: string | undefined;
  setActiveSession: (taskId: string, sessionId: string) => void;
  activateSession: (
    sessionId: string,
    taskId: string,
    tabLabel: string,
    groupId: string | undefined,
    setActiveSession: (taskId: string, sessionId: string) => void,
  ) => void;
  applyAgentProfileRecentUse: (
    context: "task_session",
    record: AgentProfileRecentUseRecord,
  ) => void;
};

async function launchAndActivateSession({
  taskId,
  selectedProfileId,
  profileExplicit,
  executorId,
  prompt,
  selectedAttachments,
  mcpServerIds,
  agentProfiles,
  groupId,
  setActiveSession,
  activateSession,
  applyAgentProfileRecentUse,
}: SessionLaunchActionArgs) {
  const { request } = buildStartRequest(
    taskId,
    selectedProfileId,
    buildSessionStartOptions(
      executorId,
      prompt,
      profileExplicit,
      selectedAttachments,
      mcpServerIds,
    ),
  );
  const response = await launchSession(request);
  const sessionId = requireSessionId(response.session_id);
  const effectiveProfileId = response.agent_profile_id ?? selectedProfileId;
  recordTaskSessionProfileUse(effectiveProfileId, applyAgentProfileRecentUse);
  const profile = agentProfiles.find((item) => item.id === effectiveProfileId);
  activateSession(
    sessionId,
    taskId,
    profile?.label ?? t("common:agent"),
    groupId,
    setActiveSession,
  );
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
  mcpServerIds,
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
  mcpServerIds?: string[];
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
  const applyAgentProfileRecentUse = useAppStore((state) => state.applyAgentProfileRecentUse);
  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const prompt = resolveSessionPrompt(promptRef, contextValue, initialPrompt);
      if (!prompt) return;
      const selectedAttachments = promptRef.current?.getAttachments() ?? [];
      if (hasPendingAttachmentUploads(selectedAttachments)) return;
      setIsCreating(true);
      try {
        await launchAndActivateSession({
          taskId,
          selectedProfileId,
          profileExplicit,
          executorId,
          prompt,
          selectedAttachments: toMessageAttachments(selectedAttachments),
          mcpServerIds,
          agentProfiles,
          groupId,
          setActiveSession,
          activateSession,
          applyAgentProfileRecentUse,
        });
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
      mcpServerIds,
      onClose,
      toast,
      setActiveSession,
      activateSession,
      setIsCreating,
      applyAgentProfileRecentUse,
    ],
  );

  return handleSubmit;
}
