"use client";

import { useCallback, useEffect, useMemo, useState } from "react";
import type { RefObject } from "react";
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
import { useRepositories } from "@/hooks/domains/workspace/use-repositories";
import { useIsUtilityConfigured } from "@/hooks/use-is-utility-configured";
import { useSummarizeSession, type SummarizeSessionResult } from "@/hooks/use-summarize-session";
import { useTaskSessions } from "@/hooks/use-task-sessions";
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

function useExecutorDefault(
  allProfiles: ExecutorProfile[],
  executorProfileId: string,
  setExecutorProfileId: (value: string) => void,
) {
  const lastUsedExecutorProfileId = useAppStore(
    (s) => s.userSettings.taskCreateLastUsed?.executorProfileId ?? null,
  );
  useEffect(() => {
    if (executorProfileId || allProfiles.length === 0) return;
    const pick =
      lastUsedExecutorProfileId && allProfiles.some((p) => p.id === lastUsedExecutorProfileId)
        ? lastUsedExecutorProfileId
        : allProfiles[0].id;
    setExecutorProfileId(pick);
  }, [allProfiles, executorProfileId, lastUsedExecutorProfileId, setExecutorProfileId]);
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
  promptValue: string;
  setPromptValue: (value: string) => void;
  initialPrompt: string | null;
  summarize: (sessionId: string) => Promise<SummarizeSessionResult>;
  toast: SummaryToastFn;
}) {
  const {
    setContextValue,
    setHasPrompt,
    promptRef,
    promptValue,
    setPromptValue,
    initialPrompt,
    summarize,
    toast,
  } = opts;
  return useCallback(
    async (value: string) => {
      if (!value) return;
      setContextValue(value);
      if (!promptRef.current) return;
      if (value === "copy_prompt" && initialPrompt) {
        setPromptValue(initialPrompt);
        setHasPrompt(true);
        return;
      }
      if (value === "blank") {
        setPromptValue("");
        setHasPrompt(false);
        return;
      }
      if (value.startsWith("summarize:")) {
        const controlledPromptRef: RefObject<HTMLTextAreaElement | null> = {
          current: promptRef.current
            ? ({
                get value() {
                  return promptValue;
                },
                set value(value: string) {
                  setPromptValue(value);
                },
              } as HTMLTextAreaElement)
            : null,
        };
        const result = await summarize(value.slice("summarize:".length));
        applySummarizeSessionResult({
          result,
          promptRef: controlledPromptRef,
          setContextValue,
          setHasPrompt,
          toast,
        });
      }
    },
    [
      initialPrompt,
      promptRef,
      promptValue,
      setContextValue,
      setHasPrompt,
      setPromptValue,
      summarize,
      toast,
    ],
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
  const isUtilityConfigured = useIsUtilityConfigured();
  const { summarize, isSummarizing } = useSummarizeSession();
  const [isCreating, setIsCreating] = useState(false);
  const [title, setTitle] = useState(defaultTitle);
  const [hasPrompt, setHasPrompt] = useState(false);
  const [promptValue, setPromptValue] = useState("");
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
  useExecutorDefault(allExecutorProfiles, fs.executorProfileId, fs.setExecutorProfileId);
  const promptZone = useSubtaskPromptZone({
    taskTitle: title,
    inputDisabled: isCreating || isSummarizing,
    contextValue,
    initialPrompt,
    promptValue,
    setPromptValue,
    setHasPrompt,
  });
  const handleContextChange = useContextChangeHandler({
    setContextValue,
    setHasPrompt,
    promptRef: promptZone.promptRef,
    promptValue,
    setPromptValue,
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
      promptValue,
      isCreating,
      isSummarizing,
      isUtilityConfigured,
      onPromptChange: (value: string) => {
        setPromptValue(value);
        setHasPrompt(value.trim().length > 0);
      },
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
