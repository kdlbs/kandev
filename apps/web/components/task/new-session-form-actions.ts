import { useCallback, useEffect, useMemo, useRef, useState, type RefObject } from "react";
import { useTranslation } from "react-i18next";
import { launchSession } from "@/lib/services/session-launch-service";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import {
  hasPendingAttachmentUploads,
  toMessageAttachments,
} from "@/components/task-create-dialog-helpers";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { ExecutorProfile } from "@/lib/types/http";
import type { SummarizeSessionResult } from "@/hooks/use-summarize-session";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import { useCompatibleAgentProfiles } from "@/hooks/domains/session/use-compatible-agent-profiles";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { usePromptResultDelivery } from "@/hooks/use-prompt-result-delivery";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { applySummarizeSessionResult, type SummaryToastFn } from "./session-context-summary";
import { t } from "@/lib/i18n";
import { recordAgentProfileRecentUseBestEffort } from "@/lib/agent-profile-recent-use";
import type { AgentProfileRecentUseRecord } from "@/lib/agent-profile-recent-use";
import { resolveComposerWorkspaceId } from "./chat/composer-workspace";
import type { HandoffPreset } from "./handoff-types";
import { resolveNewSessionProfileSelection } from "./new-session-profile-selection";
import type { ConversationForkFormContext } from "./conversation-fork-types";

export function agentProfileDisplayLabel(profile: AgentProfileOption): string {
  const parts = profile.label.split(" \u2022 ");
  return parts.length > 1 ? parts.slice(1).join(" \u2022 ") : (parts[0] ?? profile.label);
}

export function useNewSessionDialogState(taskId: string) {
  const { t } = useTranslation();
  const resolvedWorkspaceId = useAppStore((state) =>
    resolveComposerWorkspaceId({
      sessionId: null,
      taskId,
      quickChatSessions: state.quickChat.sessions,
      activeWorkflowId: state.kanban.workflowId,
      activeTasks: state.kanban.tasks,
      snapshots: Object.values(state.kanbanMulti.snapshots),
      workflows: state.workflows.items,
    }),
  );
  const taskTitle = useAppStore((state) => {
    const task = state.kanban.tasks.find((entry: { id: string }) => entry.id === taskId);
    return task?.title ?? t("common:task");
  });
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const currentSession = useAppStore((state) => {
    return activeSessionId ? (state.taskSessions.items[activeSessionId] ?? null) : null;
  });
  const worktreeBranch = useAppStore((state) => {
    if (!activeSessionId) return null;
    const wtIds = state.sessionWorktreesBySessionId.itemsBySessionId[activeSessionId];
    if (wtIds?.length) {
      const wt = state.worktrees.items[wtIds[0]];
      if (wt?.branch) return wt.branch;
    }
    return currentSession?.worktree_branch ?? null;
  });
  const initialPrompt = useAppStore((state) => {
    if (!activeSessionId) return null;
    const msgs = state.messages.bySession[activeSessionId];
    if (!msgs?.length) return null;
    const first = msgs.find((message: { author_type?: string }) => message.author_type === "user");
    return first ? ((first as { content?: string }).content ?? null) : null;
  });
  const executorLabel = useAppStore((state) => {
    if (!currentSession?.executor_id) return null;
    const executor = state.executors.items.find(
      (item: { id: string }) => item.id === currentSession.executor_id,
    );
    return executor?.name ?? null;
  });

  return {
    resolvedWorkspaceId,
    taskTitle,
    agentProfiles,
    currentSession,
    worktreeBranch,
    initialPrompt,
    executorLabel,
    sessionProfileId: currentSession?.agent_profile_id ?? "",
  };
}

export function activateNewSession(
  sessionId: string,
  taskId: string,
  tabLabel: string,
  groupId: string | undefined,
  setActiveSession: (taskId: string, sessionId: string) => void,
) {
  setActiveSession(taskId, sessionId);
  const { api, centerGroupId } = useDockviewStore.getState();
  if (api) addSessionPanel(api, groupId ?? centerGroupId, sessionId, tabLabel);
}

export function useSessionOptions(taskId: string) {
  const { t } = useTranslation();
  const { sessions, loadSessions } = useTaskSessions(taskId);
  const agentProfiles = useAppStore((state) => state.agentProfiles.items);
  useEffect(() => {
    loadSessions(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return useMemo(() => {
    const sorted = [...sessions].sort(
      (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
    );
    return sorted.map((session, index) => {
      const profile = agentProfiles.find(
        (item: { id: string }) => item.id === session.agent_profile_id,
      );
      const name = profile ? agentProfileDisplayLabel(profile) : t("task:panelAgent");
      return { id: session.id, label: name, index: index + 1, agentName: profile?.agent_name };
    });
  }, [sessions, agentProfiles, t]);
}

export function useSessionPromptController(
  promptRef: RefObject<TaskFormInputsHandle | null>,
  taskId: string,
) {
  const { t } = useTranslation();
  const { toast } = useToast();
  const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({ sessionId: null });
  const latestPromptValueRef = useRef("");
  const promptResultDelivery = usePromptResultDelivery({
    scopeKey: `new-session:${taskId}`,
    getCurrent: () => promptRef.current?.getValue() ?? latestPromptValueRef.current,
    apply: (value) => {
      const promptInput = promptRef.current;
      if (!promptInput) return false;
      promptInput.setValue(value);
      return true;
    },
  });

  const handleEnhancePrompt = useCallback(async () => {
    const current = promptRef.current?.getValue() ?? "";
    if (!current.trim()) return;
    latestPromptValueRef.current = current;
    const generation = promptResultDelivery.captureScope();

    await enhancePrompt(current, (enhanced) => {
      const delivered = promptResultDelivery.deliver(current, enhanced, generation);
      if (delivered) {
        toast({ description: t("task:enhancedPromptApplied"), variant: "success" });
      }

      return delivered;
    });
  }, [enhancePrompt, promptRef, promptResultDelivery, toast]);

  return {
    handleEnhancePrompt,
    isEnhancingPrompt,
    pendingResult: promptResultDelivery.pendingResult,
    applyPending: promptResultDelivery.applyPending,
    copyPending: promptResultDelivery.copyPending,
  };
}

function isMissingCompatibleProfile(
  executorProfile: ExecutorProfile | null,
  totalAgentCount: number,
  hasCompatibleProfiles: boolean,
): boolean {
  return !!executorProfile && totalAgentCount > 0 && !hasCompatibleProfiles;
}

export function shouldDisableSubmit(isBusy: boolean, hasPrompt: boolean, hasProfiles: boolean) {
  const submitPenalty = Number(!hasPrompt) + Number(!hasProfiles) + Number(isBusy);
  return submitPenalty > 0;
}

export function useSessionProfileSelection({
  agentProfiles,
  executorProfile,
  currentProfileId,
  handoff,
}: {
  agentProfiles: AgentProfileOption[];
  executorProfile: ExecutorProfile | null;
  currentProfileId: string;
  handoff?: HandoffPreset;
}) {
  const compatibleAgentProfiles = useCompatibleAgentProfiles(
    agentProfiles,
    executorProfile,
    handoff,
  );
  const recentProfileIds = useAppStore(
    (state) => state.agentProfileRecentUse?.records.task_session?.profileIds,
  );
  const automaticSelection = useMemo(
    () =>
      resolveNewSessionProfileSelection({
        compatibleProfiles: compatibleAgentProfiles,
        recentProfileIds,
        currentProfileId,
        handoffProfileId: handoff?.targetProfileId,
      }),
    [compatibleAgentProfiles, currentProfileId, handoff?.targetProfileId, recentProfileIds],
  );
  const [selectedProfileId, setSelectedProfileId] = useState(automaticSelection.profileId);
  const [selectionSource, setSelectionSource] = useState(automaticSelection.source);
  useEffect(() => {
    const selectedIsCompatible = compatibleAgentProfiles.some(
      (profile) => profile.id === selectedProfileId,
    );
    if (selectionSource === "manual" && selectedIsCompatible) return;
    if (
      selectedProfileId === automaticSelection.profileId &&
      selectionSource === automaticSelection.source
    ) {
      return;
    }
    setSelectedProfileId(automaticSelection.profileId);
    setSelectionSource(automaticSelection.source);
  }, [automaticSelection, compatibleAgentProfiles, selectedProfileId, selectionSource]);
  const profileOptions = useAgentProfileOptions(compatibleAgentProfiles, "task_session");
  const hasProfiles = profileOptions.length > 0;
  const noCompatibleProfiles = isMissingCompatibleProfile(
    executorProfile,
    agentProfiles.length,
    hasProfiles,
  );
  const showAgentSelector =
    hasProfiles &&
    (profileOptions.length > 1 ||
      (!!currentProfileId && !profileOptions.find((option) => option.value === currentProfileId)));
  const onProfileChange = useCallback((value: string) => {
    setSelectedProfileId(value);
    setSelectionSource("manual");
  }, []);

  return {
    profileOptions,
    hasProfiles,
    noCompatibleProfiles,
    showAgentSelector,
    selectedProfileId,
    profileExplicit:
      selectionSource === "handoff" || selectionSource === "recent" || selectionSource === "manual",
    onProfileChange,
  };
}

export function useConversationForkModelEstimate(
  conversationFork: ConversationForkFormContext | undefined,
  selectedModelId: string,
) {
  const callbackRef = useRef(conversationFork?.onModelChange);
  const lastEstimatedModel = useRef("");
  const snapshotId = conversationFork?.snapshot.descriptor.id ?? "";

  useEffect(() => {
    callbackRef.current = conversationFork?.onModelChange;
  }, [conversationFork?.onModelChange]);
  useEffect(() => {
    const key = `${snapshotId}:${selectedModelId}`;
    if (!snapshotId || !selectedModelId || key === lastEstimatedModel.current) return;
    lastEstimatedModel.current = key;
    callbackRef.current?.(selectedModelId);
  }, [selectedModelId, snapshotId]);
}

export function handoffProfileLabel(
  agentProfiles: AgentProfileOption[],
  handoff: HandoffPreset | undefined,
): string | null {
  if (!handoff) return null;
  const profile = agentProfiles.find((item) => item.id === handoff.targetProfileId);
  return profile ? agentProfileDisplayLabel(profile) : null;
}

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

type SessionLaunchInput = {
  prompt: string;
  attachments: ReturnType<TaskFormInputsHandle["getAttachments"]>;
};

function readSessionLaunchInput(
  promptRef: RefObject<TaskFormInputsHandle | null>,
  contextValue: string,
  initialPrompt: string | null,
): SessionLaunchInput | null {
  const typed = promptRef.current?.getValue().trim() ?? "";
  const prompt = contextValue === "copy_prompt" && !typed && initialPrompt ? initialPrompt : typed;
  if (!prompt) return null;
  const attachments = promptRef.current?.getAttachments() ?? [];
  if (hasPendingAttachmentUploads(attachments)) return null;
  return { prompt, attachments };
}

async function launchAndActivateSession({
  prompt,
  attachments,
  taskId,
  selectedProfileId,
  profileExplicit,
  executorId,
  conversationForkId,
  creationRequestId,
  agentProfiles,
  groupId,
  activateSession,
  setActiveSession,
  applyAgentProfileRecentUse,
  onForkConsumed,
  onClose,
}: {
  prompt: string;
  attachments: SessionLaunchInput["attachments"];
  taskId: string;
  selectedProfileId: string;
  profileExplicit: boolean;
  executorId: string;
  conversationForkId?: string;
  creationRequestId?: string;
  agentProfiles: AgentProfileOption[];
  groupId?: string;
  activateSession: (
    sessionId: string,
    taskId: string,
    tabLabel: string,
    groupId: string | undefined,
    setActiveSession: (taskId: string, sessionId: string) => void,
  ) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
  applyAgentProfileRecentUse: (
    context: "task_session",
    record: AgentProfileRecentUseRecord,
  ) => void;
  onForkConsumed?: () => void;
  onClose: () => void;
}): Promise<void> {
  const { request } = buildStartRequest(taskId, selectedProfileId, {
    executorId,
    prompt,
    profileExplicit,
    attachments: toMessageAttachments(attachments),
    conversationForkId,
    creationRequestId,
  });
  const response = await launchSession(request);
  if (!response.session_id) {
    throw new Error("Session created but no session ID returned");
  }
  const effectiveProfileId = response.agent_profile_id ?? selectedProfileId;
  recordTaskSessionProfileUse(effectiveProfileId, applyAgentProfileRecentUse);
  const profile = agentProfiles.find((item) => item.id === effectiveProfileId);
  activateSession(
    response.session_id,
    taskId,
    profile?.label ?? t("common:agent"),
    groupId,
    setActiveSession,
  );
  onForkConsumed?.();
  onClose();
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
  conversationForkId,
  creationRequestId,
  onForkConsumed,
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
  conversationForkId?: string;
  creationRequestId?: string;
  onForkConsumed?: () => void;
}) {
  const applyAgentProfileRecentUse = useAppStore((state) => state.applyAgentProfileRecentUse);
  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const input = readSessionLaunchInput(promptRef, contextValue, initialPrompt);
      if (!input) return;
      setIsCreating(true);
      try {
        await launchAndActivateSession({
          ...input,
          executorId,
          taskId,
          selectedProfileId,
          profileExplicit,
          conversationForkId,
          creationRequestId,
          agentProfiles,
          groupId,
          activateSession,
          setActiveSession,
          applyAgentProfileRecentUse,
          onForkConsumed,
          onClose,
        });
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
      applyAgentProfileRecentUse,
      conversationForkId,
      creationRequestId,
      onForkConsumed,
    ],
  );

  return handleSubmit;
}
