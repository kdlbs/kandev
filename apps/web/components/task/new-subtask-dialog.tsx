"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
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
  useGitHubUrlBranchesEffect,
} from "@/components/task-create-dialog-effects";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useSummarizeSession } from "@/hooks/use-summarize-session";
import { useTaskSessions } from "@/hooks/use-task-sessions";
import { getLocalStorage } from "@/lib/local-storage";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import type { ExecutorProfile, Repository } from "@/lib/types/http";
import type { AgentProfileOption } from "@/lib/state/slices";
import {
  defaultSubtaskWorkspaceMode,
  type SubtaskWorkspaceMode,
  useSubtaskFormState,
} from "./new-subtask-form-state";
import { PromptZone, SubtaskFormBody } from "./new-subtask-form-parts";
import { useSubtaskPromptZone, useSubtaskSubmit } from "./use-subtask-submit";

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
  const [title, setTitle] = useState(defaultTitle);
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
  useGitHubUrlBranchesEffect(fs, isOpen);
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
