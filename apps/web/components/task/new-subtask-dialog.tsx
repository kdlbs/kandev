"use client";

import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "@kandev/ui/dialog";
import { useAppStore } from "@/components/state-provider";
import { useToast } from "@/components/toast-provider";

import {
  useAgentProfileOptions,
  useExecutorProfileOptions,
} from "@/components/task-create-dialog-options";
import { useDialogHandlers } from "@/components/task-create-dialog-handlers";
import {
  useDiscoverReposEffect,
  useGitHubUrlErrorEffect,
} from "@/components/task-create-dialog-effects";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useSessionWorktrees } from "@/hooks/domains/session/use-session-worktrees";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useSummarizeSession, type SummarizeSessionResult } from "@/hooks/use-summarize-session";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { useSubtaskCount } from "@/hooks/use-subtask-count";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { taskQueryOptions } from "@/lib/query/query-options";
import type { ExecutorProfile, ExecutorType, Repository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import {
  defaultSubtaskWorkspaceMode,
  type SubtaskWorkspaceMode,
  useSubtaskFormState,
} from "./new-subtask-form-state";
import { PromptZone, SubtaskFormBody } from "./new-subtask-form-parts";
import { applySummarizeSessionResult, type SummaryToastFn } from "./session-context-summary";
import { useSubtaskPromptZone, useSubtaskSubmit } from "./use-subtask-submit";

type NewSubtaskDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  parentTaskId: string;
  parentTaskTitle: string;
};

function useSubtaskDialogState(parentTaskId: string) {
  const { agentProfiles, executors } = useSettingsData(true);
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const activeWorkspaceId = useAppStore((s) => s.workspaces.activeId);
  const parentTaskQuery = useQuery({
    ...taskQueryOptions(parentTaskId),
    enabled: Boolean(parentTaskId),
  });
  const workspaceId = parentTaskQuery.data?.workspace_id ?? activeWorkspaceId;
  const workflowId = parentTaskQuery.data?.workflow_id ?? null;

  const currentSession = useAppStore((s) =>
    activeSessionId ? (s.taskSessions.items[activeSessionId] ?? null) : null,
  );

  const worktrees = useSessionWorktrees(activeSessionId);
  const worktreeBranch = worktrees[0]?.branch ?? currentSession?.worktree_branch ?? null;

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
  const { agentProfiles } = useSettingsData(true);
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
  executors: Array<{ id: string; type: ExecutorType; name: string; profiles?: ExecutorProfile[] }>,
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

function useSyncedDefaultTitle(defaultTitle: string): [string, (value: string) => void] {
  const [title, setTitle] = useState(defaultTitle);
  const previousDefaultTitleRef = useRef(defaultTitle);

  useEffect(() => {
    const previousDefaultTitle = previousDefaultTitleRef.current;
    if (previousDefaultTitle === defaultTitle) return;
    previousDefaultTitleRef.current = defaultTitle;
    setTitle((current) => (current === previousDefaultTitle ? defaultTitle : current));
  }, [defaultTitle]);

  return [title, setTitle];
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
  summarize: (sessionId: string) => Promise<SummarizeSessionResult>;
  toast: SummaryToastFn;
}) {
  const { setContextValue, setHasPrompt, promptRef, initialPrompt, summarize, toast } = opts;
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
        applySummarizeSessionResult({ result, promptRef, setContextValue, setHasPrompt, toast });
      }
    },
    [setContextValue, setHasPrompt, promptRef, initialPrompt, summarize, toast],
  );
}

type SubtaskFormProps = {
  parentTaskId: string;
  defaultTitle: string;
  defaultProfileId: string;
  worktreeBranch: string | null;
  initialPrompt: string | null;
  agentProfiles: AgentProfileOption[];
  executors: Array<{ id: string; type: ExecutorType; name: string; profiles?: ExecutorProfile[] }>;
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
  const isUtilityConfigured = useIsUtilityConfigured();
  const { summarize, isSummarizing } = useSummarizeSession();
  const [isCreating, setIsCreating] = useState(false);
  const [title, setTitle] = useSyncedDefaultTitle(defaultTitle);
  const [hasPrompt, setHasPrompt] = useState(false);
  const [contextValue, setContextValue] = useState("blank");
  const [workspaceMode, setWorkspaceMode] = useState<SubtaskWorkspaceMode>(() =>
    defaultSubtaskWorkspaceMode(worktreeBranch),
  );
  // Shim DialogFormState shared with the create-task dialog.
  const fs = useSubtaskFormState();
  useSeedParentRepository(fs, parentRepositoryId, baseBranch);
  useSeedAgentProfileId(fs, defaultProfileId);
  const handlers = useDialogHandlers(fs, availableRepositories);
  useGitHubUrlErrorEffect(fs, isOpen);
  useDiscoverReposEffect(fs, isOpen, workspaceId, false, toast);
  const profileOptions = useAgentProfileOptions(agentProfiles);
  const sessionOptions = useSessionOptions(parentTaskId);
  const allExecutorProfiles = useExecutorProfiles(executors);
  const executorProfileOptions = useExecutorProfileOptions(allExecutorProfiles);
  useAutoSelectExecutorProfile(allExecutorProfiles, fs.executorProfileId, fs.setExecutorProfileId);
  const promptZone = useSubtaskPromptZone({
    taskTitle: title,
    inputDisabled: isCreating || isSummarizing,
    contextValue,
    initialPrompt,
    setHasPrompt,
  });
  const handleContextChange = useContextChangeHandler({
    setContextValue,
    setHasPrompt,
    promptRef: promptZone.promptRef,
    initialPrompt,
    summarize,
    toast,
  });
  const { handleSubmit } = useSubtaskSubmit({
    fs,
    parentTaskId,
    defaultProfileId,
    workspaceId,
    workflowId,
    availableRepositories,
    attachments: promptZone.attachments.attachments,
    resolvePrompt: promptZone.resolvePrompt,
    title,
    setIsCreating,
    onClose,
    workspaceMode,
  });
  return renderSubtaskFormBody({
    fs,
    handlers,
    title,
    setTitle,
    workspaceId,
    availableRepositories,
    parentRepositoryId,
    worktreeBranch,
    profileOptions,
    executorProfileOptions,
    agentProfileId: fs.agentProfileId || defaultProfileId,
    workspaceMode,
    onWorkspaceModeChange: setWorkspaceMode,
    contextValue,
    onContextChange: handleContextChange,
    hasInitialPrompt: !!initialPrompt,
    sessionOptions: isUtilityConfigured ? sessionOptions : [],
    promptZoneProps: {
      ...promptZone,
      isCreating,
      isSummarizing,
      isUtilityConfigured,
      setHasPrompt,
      onSubmitShortcut: handleSubmit,
    },
    isCreating,
    isSummarizing,
    hasPrompt,
    onClose,
    onSubmit: handleSubmit,
  });
}

type RenderArgs = Omit<React.ComponentProps<typeof SubtaskFormBody>, "promptZone"> & {
  promptZoneProps: React.ComponentProps<typeof PromptZone>;
};

function renderSubtaskFormBody(args: RenderArgs) {
  const { promptZoneProps, ...rest } = args;
  return <SubtaskFormBody {...rest} promptZone={<PromptZone {...promptZoneProps} />} />;
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
  } = useSubtaskDialogState(parentTaskId);

  // Ensure executor/agent data is loaded when dialog opens
  useSettingsData(open);
  // Load workspace repositories so the subtask repo override picker has options.
  const { repositories: availableRepositories } = useRepositories(workspaceId, open);

  const siblingCount = useSubtaskCount(open, parentTaskId);

  const defaultTitle = useMemo(
    () => `${parentTaskTitle} / Subtask ${siblingCount + 1}`,
    [parentTaskTitle, siblingCount],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="new-subtask-dialog"
        className="w-full h-full min-w-0 max-w-full max-h-full overflow-hidden rounded-none sm:w-[800px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col"
      >
        <DialogHeader>
          <DialogTitle className="min-w-0 wrap-break-word pr-6 text-sm font-medium">
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
