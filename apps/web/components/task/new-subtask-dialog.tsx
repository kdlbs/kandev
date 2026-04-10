"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Badge } from "@kandev/ui/badge";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { Textarea } from "@kandev/ui/textarea";
import { IconGitBranch } from "@tabler/icons-react";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { createTask } from "@/lib/api/domains/kanban-api";
import { performLayoutSwitch } from "@/lib/state/dockview-store";
import { replaceTaskUrl } from "@/lib/links";
import { AgentSelector, ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import {
  useAgentProfileOptions,
  useExecutorProfileOptions,
} from "@/components/task-create-dialog-options";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { EnhancePromptButton } from "@/components/enhance-prompt-button";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { useSummarizeSession } from "@/hooks/use-summarize-session";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import type { ExecutorProfile } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import { IconLoader2 } from "@tabler/icons-react";
import { toMessageAttachments } from "@/components/task-create-dialog-helpers";
import {
  ContextSelect,
  useDialogAttachments,
  AttachButton,
  toContextItems,
} from "./session-dialog-shared";
import { ContextZone } from "./chat/context-items/context-zone";

type NewSubtaskDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  parentTaskId: string;
  parentTaskTitle: string;
};

function useSubtaskDialogState() {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.kanban.workflowId);
  const executors = useAppStore((s) => s.executors.items);

  const currentSession = useAppStore((s) =>
    activeSessionId ? (s.taskSessions.items[activeSessionId] ?? null) : null,
  );

  const worktreeBranch = useAppStore((s) => {
    if (!activeSessionId) return null;
    const wtIds = s.sessionWorktreesBySessionId.itemsBySessionId[activeSessionId];
    if (wtIds?.length) {
      const wt = s.worktrees.items[wtIds[0]];
      if (wt?.branch) return wt.branch;
    }
    return currentSession?.worktree_branch ?? null;
  });

  const initialPrompt = useAppStore((s) => {
    if (!activeSessionId) return null;
    const msgs = s.messages.bySession[activeSessionId];
    if (!msgs?.length) return null;
    const first = msgs.find((m: { author_type?: string }) => m.author_type === "user");
    return first ? ((first as { content?: string }).content ?? null) : null;
  });

  return {
    agentProfiles,
    workspaceId,
    workflowId,
    executors,
    currentSession,
    worktreeBranch,
    initialPrompt,
  };
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

function useExecutorProfiles(
  executors: Array<{ id: string; type: string; name: string; profiles?: ExecutorProfile[] }>,
) {
  return useMemo<ExecutorProfile[]>(() => {
    return executors.flatMap((executor) =>
      (executor.profiles ?? []).map((p) => ({
        ...p,
        executor_type: p.executor_type ?? executor.type,
        executor_name: p.executor_name ?? executor.name,
      })),
    );
  }, [executors]);
}

function useAutoSelectExecutorProfile(
  allProfiles: ExecutorProfile[],
  executorProfileId: string,
  setExecutorProfileId: (v: string) => void,
) {
  useEffect(() => {
    if (executorProfileId || allProfiles.length === 0) return;
    const lastId = getLocalStorage<string | null>(STORAGE_KEYS.LAST_EXECUTOR_PROFILE_ID, null);
    const pick = lastId && allProfiles.some((p) => p.id === lastId) ? lastId : allProfiles[0].id;
    setExecutorProfileId(pick);
  }, [allProfiles, executorProfileId, setExecutorProfileId]);
}

function activateSubtaskSession(opts: {
  sessionId: string;
  oldSessionId: string | null;
  taskId: string;
  setActiveTask: (taskId: string) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
}) {
  opts.setActiveTask(opts.taskId);
  opts.setActiveSession(opts.taskId, opts.sessionId);
  performLayoutSwitch(opts.oldSessionId, opts.sessionId);
  replaceTaskUrl(opts.taskId);
}

type SubtaskFormProps = {
  parentTaskId: string;
  defaultTitle: string;
  defaultProfileId: string;
  worktreeBranch: string | null;
  initialPrompt: string | null;
  agentProfiles: AgentProfileOption[];
  executors: Array<{ id: string; type: string; name: string; profiles?: ExecutorProfile[] }>;
  workspaceId: string | null;
  workflowId: string | null;
  repositoryId: string | null;
  baseBranch: string | null;
  onClose: () => void;
};

// eslint-disable-next-line max-lines-per-function
function NewSubtaskForm({
  parentTaskId,
  defaultTitle,
  defaultProfileId,
  worktreeBranch,
  initialPrompt,
  agentProfiles,
  executors,
  workspaceId,
  workflowId,
  repositoryId,
  baseBranch,
  onClose,
}: SubtaskFormProps) {
  const { toast } = useToast();
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const setActiveTask = useAppStore((s) => s.setActiveTask);
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const isUtilityConfigured = useIsUtilityConfigured();
  const { summarize, isSummarizing } = useSummarizeSession();
  const [isCreating, setIsCreating] = useState(false);
  const [title, setTitle] = useState(defaultTitle);
  const [hasPrompt, setHasPrompt] = useState(false);
  const [contextValue, setContextValue] = useState("blank");
  const [selectedProfileId, setSelectedProfileId] = useState(defaultProfileId);
  const [executorProfileId, setExecutorProfileId] = useState("");
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
  const sessionOptions = useSessionOptions(parentTaskId);
  const { enhancePrompt, isEnhancingPrompt } = useUtilityAgentGenerator({
    sessionId: null,
    taskTitle: title,
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
  }, [enhancePrompt]);

  const allExecutorProfiles = useExecutorProfiles(executors);
  const executorProfileOptions = useExecutorProfileOptions(allExecutorProfiles);
  useAutoSelectExecutorProfile(allExecutorProfiles, executorProfileId, setExecutorProfileId);

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
        }
      }
    },
    [initialPrompt, summarize],
  );

  const resolvePrompt = useCallback(() => {
    const typed = promptRef.current?.value?.trim() ?? "";
    if (contextValue === "copy_prompt" && !typed && initialPrompt) return initialPrompt;
    return typed;
  }, [contextValue, initialPrompt]);

  const performCreate = useCallback(
    async (trimmedTitle: string, prompt: string) => {
      const repositories = repositoryId
        ? [{ repository_id: repositoryId, base_branch: baseBranch || undefined }]
        : [];
      const profileId = selectedProfileId || defaultProfileId || undefined;

      const response = await createTask({
        workspace_id: workspaceId!,
        workflow_id: workflowId!,
        title: trimmedTitle,
        description: prompt,
        repositories,
        start_agent: true,
        agent_profile_id: profileId,
        executor_profile_id: executorProfileId || undefined,
        parent_id: parentTaskId,
        attachments: toMessageAttachments(attachments),
      });

      const newSessionId = response.session_id ?? response.primary_session_id ?? null;
      if (newSessionId) {
        activateSubtaskSession({
          sessionId: newSessionId,
          oldSessionId: activeSessionId ?? null,
          taskId: response.id,
          setActiveTask,
          setActiveSession,
        });
      }
    },
    [
      repositoryId,
      baseBranch,
      selectedProfileId,
      defaultProfileId,
      workspaceId,
      workflowId,
      executorProfileId,
      parentTaskId,
      attachments,
      activeSessionId,
      setActiveTask,
      setActiveSession,
    ],
  );

  const handleSubmit = useCallback(
    async (e: React.FormEvent) => {
      e.preventDefault();
      const trimmedTitle = title.trim();
      if (!trimmedTitle || !workspaceId || !workflowId) return;
      const prompt = resolvePrompt();
      if (!prompt) return;

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
        setIsCreating(false);
      }
    },
    [title, workspaceId, workflowId, resolvePrompt, performCreate, onClose, toast],
  );

  const showSessions = isUtilityConfigured ? sessionOptions : [];

  return (
    <form onSubmit={handleSubmit} className="space-y-4">
      <div className="space-y-1.5">
        <label className="text-xs font-medium text-muted-foreground">Title</label>
        <Input
          value={title}
          onChange={(e) => setTitle(e.target.value)}
          placeholder="Subtask title"
          className="text-sm"
          data-testid="subtask-title-input"
          disabled={isCreating}
        />
      </div>
      {worktreeBranch && (
        <div className="flex items-center gap-2 text-xs text-muted-foreground">
          <Badge variant="outline" className="text-xs font-normal gap-1">
            <IconGitBranch className="h-3 w-3" />
            {worktreeBranch}
          </Badge>
          <span>Same branch as current session</span>
        </div>
      )}
      <div className="space-y-1.5">
        <label className="text-xs font-medium text-muted-foreground">Executor</label>
        <ExecutorProfileSelector
          options={executorProfileOptions}
          value={executorProfileId}
          onValueChange={setExecutorProfileId}
          disabled={isCreating}
          placeholder="Select executor profile"
        />
      </div>
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
            data-testid="subtask-prompt-input"
            onInput={(e) => setHasPrompt((e.target as HTMLTextAreaElement).value.trim().length > 0)}
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
          {isCreating ? "Creating..." : "Create Subtask"}
        </Button>
      </DialogFooter>
    </form>
  );
}

export function NewSubtaskDialog({
  open,
  onOpenChange,
  parentTaskId,
  parentTaskTitle,
}: NewSubtaskDialogProps) {
  const {
    agentProfiles,
    workspaceId,
    workflowId,
    executors,
    currentSession,
    worktreeBranch,
    initialPrompt,
  } = useSubtaskDialogState();

  // Ensure executor/agent data is loaded when dialog opens
  useSettingsData(open);

  const siblingCount = useAppStore(
    (s) => s.kanban.tasks.filter((t) => t.parentTaskId === parentTaskId).length,
  );

  const defaultTitle = useMemo(
    () => `${parentTaskTitle} / Subtask ${siblingCount + 1}`,
    [parentTaskTitle, siblingCount],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="sm:max-w-[520px]">
        <DialogHeader>
          <DialogTitle className="text-sm font-medium">
            New subtask for <span className="text-foreground">{parentTaskTitle}</span>
          </DialogTitle>
        </DialogHeader>
        <NewSubtaskForm
          key={`${open}`}
          parentTaskId={parentTaskId}
          defaultTitle={defaultTitle}
          defaultProfileId={currentSession?.agent_profile_id ?? ""}
          worktreeBranch={worktreeBranch}
          initialPrompt={initialPrompt}
          agentProfiles={agentProfiles}
          executors={executors}
          workspaceId={workspaceId}
          workflowId={workflowId}
          repositoryId={currentSession?.repository_id ?? null}
          baseBranch={currentSession?.base_branch ?? null}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
