"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";

import { AgentSelector } from "@/components/task-create-dialog-selectors";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import { useSummarizeSession } from "@/hooks/use-summarize-session";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useRemoteAuthSpecs } from "@/hooks/domains/settings/use-remote-auth-specs";
import { useTaskExecutorProfile } from "@/hooks/domains/session/use-task-executor-profile";
import { isAgentConfiguredOnExecutor } from "@/lib/agent-executor-compat";
import type { AgentProfileOption } from "@/lib/state/slices";
import type { ExecutorProfile } from "@/lib/types/http";
import { buildHandoffInitialState, type HandoffPreset } from "./handoff-types";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import {
  EnvironmentBadges,
  ContextSelect,
  useDialogAttachments,
  toContextItems,
} from "./session-dialog-shared";
import { SessionPromptField } from "./new-session-form-prompt";
import { useSessionContextChange, useSessionLaunchSubmit } from "./new-session-form-actions";

export type { HandoffPreset } from "./handoff-types";

type NewSessionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  groupId?: string;
  handoff?: HandoffPreset;
};

function useNewSessionDialogState(taskId: string) {
  const taskTitle = useAppStore((state) => {
    const task = state.kanban.tasks.find((t: { id: string }) => t.id === taskId);
    return task?.title ?? "Task";
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
  const profileIsValid = agentProfiles.some((p: { id: string }) => p.id === sessionProfileId);
  const effectiveDefaultProfileId: string = profileIsValid
    ? sessionProfileId
    : (agentProfiles[0]?.id ?? "");

  return {
    taskTitle,
    agentProfiles,
    currentSession,
    worktreeBranch,
    initialPrompt,
    executorLabel,
    sessionProfileId,
    effectiveDefaultProfileId,
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
      const parts = profile?.label.split(" \u2022 ");
      const name = parts?.[1] || parts?.[0] || "Agent";
      return { id: s.id, label: name, index: idx + 1, agentName: profile?.agent_name };
    });
  }, [sessions, agentProfiles]);
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

function useCompatibleAgentProfiles(
  agentProfiles: AgentProfileOption[],
  executorProfile: ExecutorProfile | null,
): AgentProfileOption[] {
  const { specs: authSpecs, loaded: authLoaded } = useRemoteAuthSpecs();
  return useMemo(() => {
    if (!executorProfile || !authLoaded) return agentProfiles;
    return agentProfiles.filter((ap) =>
      isAgentConfiguredOnExecutor(ap, executorProfile, authSpecs),
    );
  }, [agentProfiles, executorProfile, authSpecs, authLoaded]);
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

function useEnforceCompatibleProfile(
  hasExecutorProfile: boolean,
  compatible: AgentProfileOption[],
  selectedId: string,
  setSelected: (id: string) => void,
) {
  useEffect(() => {
    if (!hasExecutorProfile) return;
    if (compatible.some((p) => p.id === selectedId)) return;
    if (compatible.length > 0) setSelected(compatible[0].id);
  }, [hasExecutorProfile, compatible, selectedId, setSelected]);
}

// eslint-disable-next-line max-lines-per-function, complexity
function NewSessionForm({
  taskId,
  defaultProfileId,
  initialProfileId,
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
  defaultProfileId: string;
  initialProfileId?: string;
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
  const handoffInitial = handoff ? buildHandoffInitialState(handoff) : null;
  const { toast } = useToast();
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const { summarize, isSummarizing } = useSummarizeSession();
  const [contextValue, setContextValue] = useState(handoffInitial?.contextValue ?? "blank");
  const [selectedProfileId, setSelectedProfileId] = useState(
    handoffInitial?.selectedProfileId ?? initialProfileId ?? defaultProfileId,
  );
  const [hasPrompt, setHasPrompt] = useState(false);
  const [isCreating, setIsCreating] = useState(false);
  const promptRef = useRef<HTMLTextAreaElement>(null);
  const {
    attachments,
    isDragging,
    fileInputRef,
    handleRemoveAttachment,
    handlePaste,
    handleDragOver,
    handleDragLeave,
    handleDrop,
    handleAttachClick,
    handleFileInputChange,
  } = useDialogAttachments(isCreating || isSummarizing);
  const contextItems = useMemo(
    () => toContextItems(attachments, handleRemoveAttachment),
    [attachments, handleRemoveAttachment],
  );
  const compatibleAgentProfiles = useCompatibleAgentProfiles(agentProfiles, executorProfile);
  useEnforceCompatibleProfile(
    Boolean(executorProfile),
    compatibleAgentProfiles,
    selectedProfileId,
    setSelectedProfileId,
  );
  const profileOptions = useAgentProfileOptions(compatibleAgentProfiles);
  const sessionOptions = useSessionOptions(taskId);
  const isUtilityConfigured = useIsUtilityConfigured();
  const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({ sessionId: null });
  const handleEnhancePrompt = useCallback(() => {
    const current = promptRef.current?.value?.trim();
    if (!current) return;
    enhancePrompt(current, (enhanced) => {
      if (promptRef.current) {
        promptRef.current.value = enhanced;
        setHasPrompt(true);
      }
    });
  }, [enhancePrompt]);
  const hasProfiles = profileOptions.length > 0;
  const noCompatibleProfiles = isMissingCompatibleProfile(
    executorProfile,
    agentProfiles.length,
    hasProfiles,
  );
  const showAgentSelector =
    hasProfiles &&
    (profileOptions.length > 1 ||
      (!!defaultProfileId && !profileOptions.find((o) => o.value === defaultProfileId)));
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
    activateSession: activateNewSession,
    setIsCreating,
  });
  const isBusy = isCreating || isSummarizing;

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <EnvironmentBadges executorLabel={executorLabel} worktreeBranch={worktreeBranch} />
      <NoAgentBanner
        noCompatibleProfiles={noCompatibleProfiles}
        hasProfiles={hasProfiles}
        executorProfileName={executorProfile?.name ?? null}
      />
      {showAgentSelector && (
        <div className="space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">Agent Profile</label>
          <AgentSelector
            options={profileOptions}
            value={selectedProfileId}
            onValueChange={setSelectedProfileId}
            disabled={isCreating}
            placeholder="Select agent profile"
          />
        </div>
      )}
      <ContextSelect
        value={contextValue}
        onValueChange={handleContextChange}
        hasInitialPrompt={!!initialPrompt}
        sessionOptions={sessionOptions}
        isSummarizing={isSummarizing}
      />
      <SessionPromptField
        promptRef={promptRef}
        contextItems={contextItems}
        isBusy={isBusy}
        isDragging={isDragging}
        isSummarizing={isSummarizing}
        hasPrompt={hasPrompt}
        hasProfiles={hasProfiles}
        isUtilityConfigured={isUtilityConfigured}
        isEnhancingPrompt={isEnhancingPrompt}
        fileInputRef={fileInputRef}
        onPromptInput={() => setHasPrompt(!!promptRef.current?.value?.trim())}
        onPaste={handlePaste}
        onSubmit={handleSubmit}
        onAttachClick={handleAttachClick}
        onEnhancePrompt={handleEnhancePrompt}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onFileInputChange={handleFileInputChange}
      />
      <DialogFooter>
        <Button
          type="button"
          variant="ghost"
          onClick={onClose}
          disabled={isCreating}
          className="cursor-pointer"
        >
          Cancel
        </Button>
        <Button
          type="submit"
          disabled={isBusy || !hasPrompt || !hasProfiles}
          className="cursor-pointer"
        >
          {isCreating ? "Creating..." : "Start Agent"}
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
  if (noCompatibleProfiles) {
    return (
      <p className="text-xs text-center text-muted-foreground">
        No agent profile is configured for{" "}
        <span className="text-foreground">“{executorProfileName}”</span>. Configure credentials in
        Settings → Executors.
      </p>
    );
  }
  if (!hasProfiles) {
    return (
      <p className="text-xs text-center text-muted-foreground">
        No agent profiles configured. Add one in Settings → Agents first.
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
  const parts = profile.label.split(" \u2022 ");
  return parts[1] || parts[0] || profile.label;
}

export function NewSessionDialog({
  open,
  onOpenChange,
  taskId,
  groupId,
  handoff,
}: NewSessionDialogProps) {
  const {
    taskTitle,
    agentProfiles,
    currentSession,
    worktreeBranch,
    initialPrompt,
    executorLabel,
    sessionProfileId,
    effectiveDefaultProfileId,
  } = useNewSessionDialogState(taskId);
  const executorProfile = useTaskExecutorProfile(taskId, open);
  const handoffLabel = handoffProfileLabel(agentProfiles, handoff);
  const formKey = handoff
    ? `${open}-${handoff.sourceSessionId}-${handoff.targetProfileId}`
    : `${open}`;

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle className="text-sm font-medium">
            {handoffLabel ? (
              <>
                Hand off to <span className="text-foreground">{handoffLabel}</span>
              </>
            ) : (
              <>
                New agent in <span className="text-foreground">{taskTitle}</span>
              </>
            )}
          </DialogTitle>
        </DialogHeader>
        <NewSessionForm
          key={formKey}
          taskId={taskId}
          defaultProfileId={sessionProfileId}
          initialProfileId={effectiveDefaultProfileId}
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
