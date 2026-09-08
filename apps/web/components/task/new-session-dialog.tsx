"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import type { FormEvent, RefObject } from "react";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";

import { AgentSelector, TaskFormInputs } from "@/components/task-create-dialog-selectors";
import type { TaskFormInputsHandle } from "@/components/task-create-dialog-types";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import { useSummarizeSession } from "@/hooks/use-summarize-session";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useTaskExecutorProfile } from "@/hooks/domains/session/use-task-executor-profile";
import { useCompatibleAgentProfiles } from "@/hooks/domains/session/use-compatible-agent-profiles";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { ExecutorProfile } from "@/lib/types/http";
import { usePromptResultDelivery } from "@/hooks/use-prompt-result-delivery";
import { buildHandoffInitialState, type HandoffPreset } from "./handoff-types";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { PromptResultRecovery } from "@/components/prompt-result-recovery";
import { EnvironmentBadges, ContextSelect } from "./session-dialog-shared";
import { useSessionContextChange, useSessionLaunchSubmit } from "./new-session-form-actions";
import { resolveNewSessionProfileSelection } from "./new-session-profile-selection";
import { resolveComposerWorkspaceId } from "./chat/composer-workspace";
import { Trans, useTranslation } from "react-i18next";

export type { HandoffPreset } from "./handoff-types";

const PROGRAMMATIC_SUBMIT_EVENT = { preventDefault: () => {} } as unknown as FormEvent;

type NewSessionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  workspaceId?: string | null;
  groupId?: string;
  handoff?: HandoffPreset;
};

function agentProfileDisplayLabel(profile: AgentProfileOption): string {
  const parts = profile.label.split(" \u2022 ");
  return parts.length > 1 ? parts.slice(1).join(" \u2022 ") : (parts[0] ?? profile.label);
}

function useNewSessionDialogState(taskId: string) {
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
    const first = msgs.find((m: { author_type?: string }) => m.author_type === "user");
    return first ? ((first as { content?: string }).content ?? null) : null;
  });
  const executorLabel = useAppStore((state) => {
    if (!currentSession?.executor_id) return null;
    const executor = state.executors.items.find(
      (e: { id: string }) => e.id === currentSession.executor_id,
    );
    return executor?.name ?? null;
  });

  const sessionProfileId = currentSession?.agent_profile_id ?? "";

  return {
    resolvedWorkspaceId,
    taskTitle,
    agentProfiles,
    currentSession,
    worktreeBranch,
    initialPrompt,
    executorLabel,
    sessionProfileId,
  };
}

function activateNewSession(
  sessionId: string,
  taskId: string,
  tabLabel: string,
  groupId: string | undefined,
  setActiveSession: (taskId: string, sessionId: string) => void,
) {
  // New session within the same task = same env, so the env switch action
  // no-ops naturally. We just create the chat panel + activate.
  setActiveSession(taskId, sessionId);
  const { api, centerGroupId } = useDockviewStore.getState();
  if (api) addSessionPanel(api, groupId ?? centerGroupId, sessionId, tabLabel);
}

function useSessionOptions(taskId: string) {
  const { t } = useTranslation();
  const { sessions, loadSessions } = useTaskSessions(taskId);
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  useEffect(() => {
    loadSessions(true);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
  return useMemo(() => {
    const sorted = [...sessions].sort(
      (a, b) => new Date(a.started_at).getTime() - new Date(b.started_at).getTime(),
    );
    return sorted.map((s, idx) => {
      const profile = agentProfiles.find((p: { id: string }) => p.id === s.agent_profile_id);
      const name = profile ? agentProfileDisplayLabel(profile) : t("task:panelAgent");
      return { id: s.id, label: name, index: idx + 1, agentName: profile?.agent_name };
    });
  }, [sessions, agentProfiles, t]);
}

function isMissingCompatibleProfile(
  executorProfile: ExecutorProfile | null,
  totalAgentCount: number,
  hasCompatibleProfiles: boolean,
): boolean {
  if (!executorProfile) return false;
  if (totalAgentCount === 0) return false;
  return !hasCompatibleProfiles;
}

function useHandoffAutoSummarize(
  handoff: HandoffPreset | undefined,
  contextValue: string,
  onContextChange: (value: string) => void,
) {
  const started = useRef(false);
  useEffect(() => {
    if (!handoff || started.current) return;
    started.current = true;
    void onContextChange(contextValue);
  }, [handoff, contextValue, onContextChange]);
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

function shouldDisableSubmit(isBusy: boolean, hasPrompt: boolean, hasProfiles: boolean) {
  const submitPenalty =
    Number(hasPrompt === false) + Number(hasProfiles === false) + Number(isBusy);
  return submitPenalty > 0;
}

function useSessionProfileSelection({
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
      (!!currentProfileId && !profileOptions.find((o) => o.value === currentProfileId)));
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

function SessionFormHeader({
  executorLabel,
  worktreeBranch,
  noCompatibleProfiles,
  hasProfiles,
  executorProfileName,
  showAgentSelector,
  profileOptions,
  selectedProfileId,
  isCreating,
  onProfileChange,
}: {
  executorLabel: string | null;
  worktreeBranch: string | null;
  noCompatibleProfiles: boolean;
  hasProfiles: boolean;
  executorProfileName: string | null;
  showAgentSelector: boolean;
  profileOptions: ReturnType<typeof useAgentProfileOptions>;
  selectedProfileId: string;
  isCreating: boolean;
  onProfileChange: (value: string) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <EnvironmentBadges executorLabel={executorLabel} worktreeBranch={worktreeBranch} />
      <NoAgentBanner
        noCompatibleProfiles={noCompatibleProfiles}
        hasProfiles={hasProfiles}
        executorProfileName={executorProfileName}
      />
      {showAgentSelector && (
        <div className="min-w-0 space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">
            {t("task:agentProfile")}
          </label>
          <AgentSelector
            options={profileOptions}
            value={selectedProfileId}
            onValueChange={onProfileChange}
            disabled={isCreating}
            placeholder={t("task:selectAgentProfile")}
            popoverPortal
          />
        </div>
      )}
    </>
  );
}

// eslint-disable-next-line max-lines-per-function
function NewSessionForm({
  taskId,
  workspaceId,
  currentProfileId,
  executorId,
  executorLabel,
  executorProfile,
  worktreeBranch,
  initialPrompt,
  agentProfiles,
  groupId,
  handoff,
  onClose,
}: {
  taskId: string;
  workspaceId?: string | null;
  currentProfileId: string;
  executorId: string;
  executorLabel: string | null;
  executorProfile: ExecutorProfile | null;
  worktreeBranch: string | null;
  initialPrompt: string | null;
  agentProfiles: AgentProfileOption[];
  groupId?: string;
  handoff?: HandoffPreset;
  onClose: () => void;
}) {
  const { t } = useTranslation();
  const handoffInitial = handoff ? buildHandoffInitialState(handoff) : null;
  const { toast } = useToast();
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const { summarize, isSummarizing } = useSummarizeSession();
  const [contextValue, setContextValue] = useState(handoffInitial?.contextValue ?? "blank");
  const [hasPrompt, setHasPrompt] = useState(false);
  const [hasPendingAttachmentUploads, setHasPendingAttachmentUploads] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const promptRef = useRef<TaskFormInputsHandle | null>(null);
  const busySignal = Number(isCreating) + Number(isSummarizing);
  const isBusyState = busySignal > 0;
  const sessionOptions = useSessionOptions(taskId);
  const isUtilityConfigured = useIsUtilityConfigured();
  const profileSelection = useSessionProfileSelection({
    agentProfiles,
    executorProfile,
    currentProfileId,
    handoff,
  });
  const { handleEnhancePrompt, isEnhancingPrompt, pendingResult, applyPending, copyPending } =
    useSessionPromptController(promptRef, taskId);
  const handleContextChange = useSessionContextChange({
    promptRef,
    initialPrompt,
    summarize,
    toast,
    setContextValue,
    setHasPrompt,
  });
  useHandoffAutoSummarize(handoff, handoffInitial?.contextValue ?? "blank", handleContextChange);

  const handleSubmit = useSessionLaunchSubmit({
    promptRef,
    taskId,
    selectedProfileId: profileSelection.selectedProfileId,
    profileExplicit: profileSelection.profileExplicit,
    executorId,
    contextValue,
    initialPrompt,
    agentProfiles,
    groupId,
    onClose,
    toast,
    setActiveSession,
    activateSession: activateNewSession,
    setIsCreating,
  });
  const isSubmitDisabled =
    shouldDisableSubmit(isBusyState, hasPrompt, profileSelection.hasProfiles) ||
    hasPendingAttachmentUploads;

  return (
    <form onSubmit={handleSubmit} className="min-w-0 space-y-4">
      <SessionFormHeader
        executorLabel={executorLabel}
        worktreeBranch={worktreeBranch}
        noCompatibleProfiles={profileSelection.noCompatibleProfiles}
        hasProfiles={profileSelection.hasProfiles}
        executorProfileName={executorProfile?.name ?? null}
        showAgentSelector={profileSelection.showAgentSelector}
        profileOptions={profileSelection.profileOptions}
        selectedProfileId={profileSelection.selectedProfileId}
        isCreating={isCreating}
        onProfileChange={profileSelection.onProfileChange}
      />
      <ContextSelect
        value={contextValue}
        onValueChange={handleContextChange}
        hasInitialPrompt={!!initialPrompt}
        sessionOptions={sessionOptions}
        isSummarizing={isSummarizing}
      />
      <TaskFormInputs
        isSessionMode
        taskId={taskId}
        workspaceId={workspaceId}
        autoFocus
        initialDescription=""
        onDescriptionChange={setHasPrompt}
        onPendingAttachmentUploadsChange={setHasPendingAttachmentUploads}
        onKeyDown={(event) => {
          if (
            event.key === "Enter" &&
            (event.metaKey || event.ctrlKey) &&
            !isBusyState &&
            !hasPendingAttachmentUploads &&
            hasPrompt &&
            profileSelection.hasProfiles
          ) {
            event.preventDefault();
            void handleSubmit(event as unknown as FormEvent);
          }
        }}
        descriptionValueRef={promptRef}
        disabled={isBusyState}
        onEnhancePrompt={handleEnhancePrompt}
        isEnhancingPrompt={isEnhancingPrompt}
        isUtilityConfigured={isUtilityConfigured}
        onComposerSubmit={() => {
          if (isBusyState || hasPendingAttachmentUploads || !profileSelection.hasProfiles) {
            return false;
          }
          void handleSubmit(PROGRAMMATIC_SUBMIT_EVENT);
          return true;
        }}
      />
      <PromptResultRecovery
        pendingResult={pendingResult}
        onApply={applyPending}
        onCopy={copyPending}
      />
      <DialogFooter>
        <Button
          type="button"
          variant="ghost"
          onClick={onClose}
          disabled={isCreating}
          className="cursor-pointer"
        >
          {t("common:cancel")}
        </Button>
        <Button type="submit" disabled={isSubmitDisabled} className="cursor-pointer">
          {isCreating ? t("task:creatingEllipsis") : t("task:startAgent2")}
        </Button>
      </DialogFooter>
    </form>
  );
}

function NoAgentBanner({
  noCompatibleProfiles,
  hasProfiles,
  executorProfileName,
}: {
  noCompatibleProfiles: boolean;
  hasProfiles: boolean;
  executorProfileName: string | null;
}) {
  const { t } = useTranslation();
  if (noCompatibleProfiles) {
    return (
      <p className="text-xs text-center text-muted-foreground">
        <Trans i18nKey="task:noAgentProfileConfiguredFor" values={{ name: executorProfileName }}>
          No agent profile is configured for{" "}
          <span className="text-foreground">“{executorProfileName}”</span>. Configure credentials in
          Settings → Executors.
        </Trans>
      </p>
    );
  }
  if (!hasProfiles) {
    return (
      <p className="text-xs text-center text-muted-foreground">
        {t("task:noAgentProfilesConfiguredAddOne")}
      </p>
    );
  }
  return null;
}

function handoffProfileLabel(
  agentProfiles: AgentProfileOption[],
  handoff: HandoffPreset | undefined,
): string | null {
  if (!handoff) return null;
  const profile = agentProfiles.find((p) => p.id === handoff.targetProfileId);
  if (!profile) return null;
  return agentProfileDisplayLabel(profile);
}

export function NewSessionDialog({
  open,
  onOpenChange,
  taskId,
  workspaceId,
  groupId,
  handoff,
}: NewSessionDialogProps) {
  const {
    taskTitle,
    resolvedWorkspaceId,
    agentProfiles,
    currentSession,
    worktreeBranch,
    initialPrompt,
    executorLabel,
    sessionProfileId,
  } = useNewSessionDialogState(taskId);
  const executorProfile = useTaskExecutorProfile(taskId, open);
  const handoffLabel = handoffProfileLabel(agentProfiles, handoff);
  const formKey = handoff
    ? `${taskId}-${open}-${handoff.sourceSessionId}-${handoff.targetProfileId}`
    : `${taskId}-${open}`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="min-w-0 overflow-hidden sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle className="min-w-0 wrap-break-word pr-6 text-sm font-medium">
            {handoffLabel ? (
              <>
                <Trans i18nKey="task:handOffToTarget" values={{ label: handoffLabel }}>
                  Hand off to <span className="text-foreground">{handoffLabel}</span>
                </Trans>
              </>
            ) : (
              <>
                <Trans i18nKey="task:newAgentInTask" values={{ title: taskTitle }}>
                  New agent in <span className="text-foreground">{taskTitle}</span>
                </Trans>
              </>
            )}
          </DialogTitle>
        </DialogHeader>
        <NewSessionForm
          key={formKey}
          taskId={taskId}
          workspaceId={workspaceId ?? resolvedWorkspaceId}
          currentProfileId={sessionProfileId}
          executorId={currentSession?.executor_id ?? ""}
          executorLabel={executorLabel}
          executorProfile={executorProfile}
          worktreeBranch={worktreeBranch}
          initialPrompt={initialPrompt}
          agentProfiles={agentProfiles}
          groupId={groupId}
          handoff={handoff}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
