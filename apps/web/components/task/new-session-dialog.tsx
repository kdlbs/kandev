"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Textarea } from "@kandev/ui/textarea";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { launchSession } from "@/lib/services/session-launch-service";
import { buildStartRequest } from "@/lib/services/session-launch-helpers";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { addSessionPanel } from "@/lib/state/dockview-panel-actions";

import { AgentSelector } from "@/components/task-create-dialog-selectors";
import { useAgentProfileOptions } from "@/components/task-create-dialog-options";
import { toMessageAttachments } from "@/components/task-create-dialog-helpers";
import { useSummarizeSession } from "@/hooks/use-summarize-session";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import type { AgentProfileOption } from "@/lib/state/slices";
import { IconLoader2 } from "@tabler/icons-react";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import {
  EnvironmentBadges,
  ContextSelect,
  useDialogAttachments,
  AttachButton,
  toContextItems,
} from "./session-dialog-shared";
import { ContextZone } from "./chat/context-items/context-zone";

type NewSessionDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  taskId: string;
  groupId?: string;
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

  return { taskTitle, agentProfiles, currentSession, worktreeBranch, initialPrompt, executorLabel };
}

function activateNewSession(
  sessionId: string,
  taskId: string,
  tabLabel: string,
  groupId: string | undefined,
  setActiveSession: (taskId: string, sessionId: string) => void,
) {
  useDockviewStore.setState({ _skipLayoutSwitchForSession: sessionId });
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

// eslint-disable-next-line max-lines-per-function
function NewSessionForm({
  taskId,
  defaultProfileId,
  executorId,
  executorLabel,
  worktreeBranch,
  initialPrompt,
  agentProfiles,
  groupId,
  onClose,
}: {
  taskId: string;
  defaultProfileId: string;
  executorId: string;
  executorLabel: string | null;
  worktreeBranch: string | null;
  initialPrompt: string | null;
  agentProfiles: AgentProfileOption[];
  groupId?: string;
  onClose: () => void;
}) {
  const { toast } = useToast();
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const { summarize, isSummarizing } = useSummarizeSession();
  const [isCreating, setIsCreating] = useState(false);
  const [contextValue, setContextValue] = useState("blank");
  const [selectedProfileId, setSelectedProfileId] = useState(defaultProfileId);
  const [hasPrompt, setHasPrompt] = useState(false);
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
  const profileOptions = useAgentProfileOptions(agentProfiles);
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

  const handleContextChange = useCallback(
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
        if (result && promptRef.current) {
          promptRef.current.value = result;
          setHasPrompt(true);
        } else {
          setContextValue("blank");
          toast({
            title: "Summarize failed",
            description:
              "Could not generate a summary. Check that the summarize utility agent is configured and enabled in settings.",
            variant: "error",
          });
        }
      }
    },
    [initialPrompt, summarize, toast],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const typed = promptRef.current?.value?.trim() ?? "";
      const prompt =
        contextValue === "copy_prompt" && !typed && initialPrompt ? initialPrompt : typed;
      if (!prompt) return;
      setIsCreating(true);
      try {
        const { request } = buildStartRequest(taskId, selectedProfileId || defaultProfileId, {
          executorId,
          prompt,
          attachments: toMessageAttachments(attachments),
        });
        const response = await launchSession(request);
        if (!response.session_id) {
          throw new Error("Session created but no session ID returned");
        }
        const profile = agentProfiles.find(
          (p: AgentProfileOption) => p.id === (selectedProfileId || defaultProfileId),
        );
        activateNewSession(
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
      taskId,
      selectedProfileId,
      defaultProfileId,
      executorId,
      contextValue,
      initialPrompt,
      agentProfiles,
      groupId,
      onClose,
      toast,
      setActiveSession,
      attachments,
    ],
  );

  const showSessions = sessionOptions;

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <EnvironmentBadges executorLabel={executorLabel} worktreeBranch={worktreeBranch} />
      {profileOptions.length > 1 && (
        <div className="space-y-1.5">
          <label className="text-xs font-medium text-muted-foreground">Agent Profile</label>
          <AgentSelector
            options={profileOptions}
            value={selectedProfileId || defaultProfileId}
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
        sessionOptions={showSessions}
        isSummarizing={isSummarizing}
      />
      <div
        className="relative"
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
      >
        <div className="rounded-md border border-input bg-transparent">
          <ContextZone items={contextItems} />
          <Textarea
            ref={promptRef}
            placeholder="What should the agent work on?"
            className="border-0 focus-visible:ring-0 focus-visible:ring-offset-0 min-h-[120px] max-h-[240px] resize-none overflow-auto text-[13px]"
            autoFocus
            disabled={isCreating || isSummarizing}
            onInput={(e) => setHasPrompt(!!e.currentTarget.value)}
            onPaste={handlePaste}
            onKeyDown={(e) => {
              if (e.key === "Enter" && (e.metaKey || e.ctrlKey)) {
                e.preventDefault();
                handleSubmit(e);
              }
            }}
          />
          <div className="flex items-center px-1 pb-1">
            <AttachButton onClick={handleAttachClick} disabled={isCreating || isSummarizing} />
            <EnhancePromptButton
              onClick={handleEnhancePrompt}
              isLoading={isEnhancingPrompt}
              isConfigured={isUtilityConfigured}
            />
          </div>
          <input
            ref={fileInputRef}
            type="file"
            multiple
            className="hidden"
            onChange={handleFileInputChange}
            tabIndex={-1}
          />
        </div>
        {isDragging && (
          <div className="absolute inset-0 flex items-center justify-center bg-primary/10 border-2 border-dashed border-primary rounded-md pointer-events-none">
            <span className="text-sm text-primary font-medium">Drop files here</span>
          </div>
        )}
        {isSummarizing && (
          <div className="absolute inset-0 flex items-center justify-center rounded-md bg-background/80">
            <div className="flex items-center gap-2 text-xs text-muted-foreground">
              <IconLoader2 className="h-4 w-4 animate-spin" />
              <span>Generating summary...</span>
            </div>
          </div>
        )}
      </div>
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
          disabled={isCreating || isSummarizing || !hasPrompt}
          className="cursor-pointer"
        >
          {isCreating ? "Creating..." : "Start Agent"}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function NewSessionDialog({ open, onOpenChange, taskId, groupId }: NewSessionDialogProps) {
  const { taskTitle, agentProfiles, currentSession, worktreeBranch, initialPrompt, executorLabel } =
    useNewSessionDialogState(taskId);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle className="text-sm font-medium">
            New agent in <span className="text-foreground">{taskTitle}</span>
          </DialogTitle>
        </DialogHeader>
        <NewSessionForm
          key={`${open}`}
          taskId={taskId}
          defaultProfileId={currentSession?.agent_profile_id ?? ""}
          executorId={currentSession?.executor_id ?? ""}
          executorLabel={executorLabel}
          worktreeBranch={worktreeBranch}
          initialPrompt={initialPrompt}
          agentProfiles={agentProfiles}
          groupId={groupId}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
