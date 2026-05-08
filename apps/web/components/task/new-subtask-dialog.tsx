"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { Dialog, DialogContent, DialogFooter, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { Button } from "@kandev/ui/button";
import { Input } from "@kandev/ui/input";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";
import { createTask } from "@/lib/api/domains/kanban-api";
import { replaceTaskUrl } from "@/lib/links";


import {
  useAgentProfileOptions,
  useExecutorProfileOptions,
} from "@/components/task-create-dialog-options";
import { RepoChipsRow } from "@/components/task-create-dialog-repo-chips";
import { useDialogHandlers } from "@/components/task-create-dialog-handlers";
import {
  useDiscoverReposEffect,
  useGitHubUrlBranchesEffect,
} from "@/components/task-create-dialog-effects";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useUtilityAgentGenerator } from "@/hooks/use-utility-agent-generator";
import { useSummarizeSession } from "@/hooks/use-summarize-session";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import type { ExecutorProfile, Repository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import {
  buildRepositoriesPayload,
  toMessageAttachments,
} from "@/components/task-create-dialog-helpers";
import { useSubtaskFormState } from "./new-subtask-form-state";
import { WorktreeBadge, SelectorsRow, PromptZone } from "./new-subtask-form-parts";
import {
  ContextSelect,
  useDialogAttachments,
  toContextItems,
} from "./session-dialog-shared";

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
  taskId: string;
  setActiveTask: (taskId: string) => void;
  setActiveSession: (taskId: string, sessionId: string) => void;
}) {
  opts.setActiveTask(opts.taskId);
  opts.setActiveSession(opts.taskId, opts.sessionId);
  // Layout switch is handled by useEnvSwitchCleanup when the new session's
  // task_environment_id is present; the hook subscribes to env-id updates.
  replaceTaskUrl(opts.taskId);
}

/**
 * Seeds the chip row with the parent task's repo + branch when the form
 * mounts. Form parent passes `key={open}` so each dialog open remounts the
 * form, which is when this fires. The user can still change or add repos
 * after seeding.
 */
function useSeedParentRepository(
  fs: ReturnType<typeof useSubtaskFormState>,
  parentRepositoryId: string | null,
  baseBranch: string | null,
) {
  useEffect(() => {
    if (!parentRepositoryId) return;
    fs.setRepositories([
      { key: "subtask-row-1", repositoryId: parentRepositoryId, branch: baseBranch ?? "" },
    ]);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}

/** Pre-fills the agent profile with the parent session's profile on mount. */
function useSeedAgentProfileId(
  fs: ReturnType<typeof useSubtaskFormState>,
  defaultProfileId: string,
) {
  useEffect(() => {
    if (defaultProfileId) fs.setAgentProfileId(defaultProfileId);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);
}

/**
 * Centralizes the Context selector's branching logic (Blank / Copy parent
 * prompt / Summarize session N) so the form component stays under the
 * complexity cap.
 */
function useContextChangeHandler(opts: {
  setContextValue: (v: string) => void;
  setHasPrompt: (v: boolean) => void;
  promptRef: React.RefObject<HTMLTextAreaElement | null>;
  initialPrompt: string | null;
  summarize: (sessionId: string) => Promise<string | null>;
}) {
  const { setContextValue, setHasPrompt, promptRef, initialPrompt, summarize } = opts;
  return useCallback(
    async (value: string) => {
      if (!value) return;
      setContextValue(value);
      const ta = promptRef.current;
      if (!ta) return;
      if (value === "copy_prompt" && initialPrompt) {
        ta.value = initialPrompt;
        setHasPrompt(true);
        return;
      }
      if (value === "blank") {
        ta.value = "";
        setHasPrompt(false);
        return;
      }
      if (value.startsWith("summarize:")) {
        const result = await summarize(value.slice("summarize:".length));
        if (result && promptRef.current) {
          promptRef.current.value = result;
          setHasPrompt(true);
        }
      }
    },
    [setContextValue, setHasPrompt, promptRef, initialPrompt, summarize],
  );
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
  /** The parent task's repository — used as the default for the subtask. */
  parentRepositoryId: string | null;
  baseBranch: string | null;
  /** Workspace repositories the user can pick from to override the default. */
  availableRepositories: Repository[];
  /** Whether the parent dialog is open — required for the GitHub URL effect. */
  isOpen: boolean;
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
  parentRepositoryId,
  baseBranch,
  availableRepositories,
  isOpen,
  onClose,
}: SubtaskFormProps) {
  const { toast } = useToast();
  const setActiveTask = useAppStore((s) => s.setActiveTask);
  const setActiveSession = useAppStore((s) => s.setActiveSession);
  const isUtilityConfigured = useIsUtilityConfigured();
  const { summarize, isSummarizing } = useSummarizeSession();
  const [isCreating, setIsCreating] = useState(false);
  const [title, setTitle] = useState(defaultTitle);
  const [hasPrompt, setHasPrompt] = useState(false);
  const [contextValue, setContextValue] = useState("blank");
  // Shim DialogFormState shared with the create-task dialog so RepoChipsRow,
  // useDialogHandlers, and the GitHub URL branches effect work unchanged.
  const fs = useSubtaskFormState();
  useSeedParentRepository(fs, parentRepositoryId, baseBranch);
  useSeedAgentProfileId(fs, defaultProfileId);
  const handlers = useDialogHandlers(fs, availableRepositories);
  useGitHubUrlBranchesEffect(fs, isOpen);
  // Fetch on-disk repos so the chip dropdown shows "on disk" entries
  // (matches the create-task dialog's RepoChipsRow behavior).
  useDiscoverReposEffect(fs, isOpen, workspaceId, false, toast);
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
  useAutoSelectExecutorProfile(allExecutorProfiles, fs.executorProfileId, fs.setExecutorProfileId);

  const handleContextChange = useContextChangeHandler({
    setContextValue,
    setHasPrompt,
    promptRef,
    initialPrompt,
    summarize,
  });

  const resolvePrompt = useCallback(() => {
    const typed = promptRef.current?.value?.trim() ?? "";
    if (contextValue === "copy_prompt" && !typed && initialPrompt) return initialPrompt;
    return typed;
  }, [contextValue, initialPrompt]);

  const performCreate = useCallback(
    async (trimmedTitle: string, prompt: string) => {
      const repositories = buildRepositoriesPayload({
        useGitHubUrl: fs.useGitHubUrl,
        githubUrl: fs.githubUrl,
        githubBranch: fs.githubBranch,
        githubPrHeadBranch: fs.githubPrHeadBranch,
        repositories: fs.repositories,
        discoveredRepositories: fs.discoveredRepositories,
        workspaceRepositories: availableRepositories,
      });
      const profileId = fs.agentProfileId || defaultProfileId || undefined;

      const response = await createTask({
        workspace_id: workspaceId!,
        workflow_id: workflowId!,
        title: trimmedTitle,
        description: prompt,
        repositories,
        start_agent: true,
        agent_profile_id: profileId,
        executor_profile_id: fs.executorProfileId || undefined,
        parent_id: parentTaskId,
        attachments: toMessageAttachments(attachments),
      });

      const newSessionId = response.session_id ?? response.primary_session_id ?? null;
      if (newSessionId) {
        activateSubtaskSession({
          sessionId: newSessionId,
          taskId: response.id,
          setActiveTask,
          setActiveSession,
        });
      }
    },
    [
      fs.useGitHubUrl,
      fs.githubUrl,
      fs.githubBranch,
      fs.githubPrHeadBranch,
      fs.repositories,
      fs.discoveredRepositories,
      fs.agentProfileId,
      fs.executorProfileId,
      availableRepositories,
      defaultProfileId,
      workspaceId,
      workflowId,
      parentTaskId,
      attachments,
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
  // Worktree badge is only meaningful when the subtask still targets the
  // parent's repo (single chip, same id). Adding repos or pasting a URL makes
  // it ambiguous, so hide.
  const onlyChipIsParent =
    fs.repositories.length === 1 &&
    fs.repositories[0]?.repositoryId === parentRepositoryId &&
    !fs.useGitHubUrl;
  const showWorktreeBadge = !!worktreeBranch && onlyChipIsParent;

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
      <RepoChipsRow
        fs={fs}
        repositories={availableRepositories}
        isTaskStarted={false}
        workspaceId={workspaceId}
        onRowRepositoryChange={handlers.handleRowRepositoryChange}
        onRowBranchChange={handlers.handleRowBranchChange}
        onToggleGitHubUrl={handlers.handleToggleGitHubUrl}
        onGitHubUrlChange={handlers.handleGitHubUrlChange}
      />
      <WorktreeBadge show={showWorktreeBadge} branch={worktreeBranch} />
      <SelectorsRow
        profileOptions={profileOptions}
        executorProfileOptions={executorProfileOptions}
        agentProfileId={fs.agentProfileId || defaultProfileId}
        executorProfileId={fs.executorProfileId}
        onAgentProfileChange={handlers.handleAgentProfileChange}
        onExecutorProfileChange={handlers.handleExecutorProfileChange}
        disabled={isCreating}
      />
      <ContextSelect
        value={contextValue}
        onValueChange={handleContextChange}
        hasInitialPrompt={!!initialPrompt}
        sessionOptions={showSessions}
        isSummarizing={isSummarizing}
      />
      <PromptZone
        promptRef={promptRef}
        contextItems={contextItems}
        fileInputRef={fileInputRef}
        isDragging={isDragging}
        isCreating={isCreating}
        isSummarizing={isSummarizing}
        isEnhancingPrompt={isEnhancingPrompt}
        isUtilityConfigured={isUtilityConfigured}
        onDragOver={handleDragOver}
        onDragLeave={handleDragLeave}
        onDrop={handleDrop}
        onPaste={handlePaste}
        onFileInputChange={handleFileInputChange}
        onAttachClick={handleAttachClick}
        onEnhancePrompt={handleEnhancePrompt}
        setHasPrompt={setHasPrompt}
        onSubmitShortcut={handleSubmit}
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
  // Load workspace repositories so the subtask repo override picker has options.
  const { repositories: availableRepositories } = useRepositories(workspaceId, open);

  const siblingCount = useAppStore(
    (s) => s.kanban.tasks.filter((t) => t.parentTaskId === parentTaskId).length,
  );

  const defaultTitle = useMemo(
    () => `${parentTaskTitle} / Subtask ${siblingCount + 1}`,
    [parentTaskTitle, siblingCount],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="new-subtask-dialog"
        className="w-full h-full max-w-full max-h-full rounded-none sm:w-[800px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col"
      >
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
          parentRepositoryId={currentSession?.repository_id ?? null}
          baseBranch={currentSession?.base_branch ?? null}
          availableRepositories={availableRepositories}
          isOpen={open}
          onClose={() => onOpenChange(false)}
        />
      </DialogContent>
    </Dialog>
  );
}
