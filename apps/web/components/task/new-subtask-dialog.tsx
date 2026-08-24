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
import type { KanbanState } from "@/lib/state/slices/kanban/types";
import type { Message, TaskSession } from "@/lib/types/http";
import { useTranslation } from "react-i18next";

type NewSubtaskDialogProps = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  parentTaskId: string;
  parentTaskTitle: string;
};

type ParentTaskContext = {
  task: KanbanState["tasks"][number] | null;
  session: TaskSession | null;
  worktreeBranch: string | null;
  initialPrompt: string | null;
  parentRepositoryId: string | null;
  baseBranch: string | null;
};

export function resolveSubtaskParentContext({
  task,
  sessionsById,
  sessionsByTaskId,
  worktreeIdsBySessionId,
  worktrees,
  messagesBySession,
}: {
  task: KanbanState["tasks"][number] | null;
  sessionsById: Record<string, TaskSession>;
  sessionsByTaskId: Record<string, TaskSession[]>;
  worktreeIdsBySessionId: Record<string, string[]>;
  worktrees: Record<string, { branch?: string }>;
  messagesBySession: Record<string, Message[]>;
}): ParentTaskContext {
  const session = resolveParentSession(task, sessionsById, sessionsByTaskId);
  const primaryRepository = resolvePrimaryRepository(task);

  return {
    task,
    session,
    worktreeBranch: resolveWorktreeBranch(session, worktreeIdsBySessionId, worktrees),
    initialPrompt: resolveInitialPrompt(session, messagesBySession),
    parentRepositoryId: primaryRepository?.repository_id ?? session?.repository_id ?? null,
    baseBranch: primaryRepository?.base_branch ?? session?.base_branch ?? null,
  };
}

function resolveParentSession(
  task: KanbanState["tasks"][number] | null,
  sessionsById: Record<string, TaskSession>,
  sessionsByTaskId: Record<string, TaskSession[]>,
) {
  const taskSessions = task ? (sessionsByTaskId[task.id] ?? []) : [];
  const sessionId = task?.primarySessionId ?? taskSessions.find((s) => s.is_primary)?.id;
  return sessionId
    ? (sessionsById[sessionId] ?? taskSessions.find((s) => s.id === sessionId) ?? null)
    : null;
}

function resolvePrimaryRepository(task: KanbanState["tasks"][number] | null) {
  const repositories = task?.repositories ?? [];
  if (repositories.length === 0) return undefined;
  return repositories.reduce((current, repository) =>
    !current || repository.position < current.position ? repository : current,
  );
}

function resolveWorktreeBranch(
  session: TaskSession | null,
  worktreeIdsBySessionId: Record<string, string[]>,
  worktrees: Record<string, { branch?: string }>,
) {
  if (!session) return null;
  const worktreeBranch = (worktreeIdsBySessionId[session.id] ?? [])
    .map((id) => worktrees[id]?.branch)
    .find((branch): branch is string => Boolean(branch));
  return worktreeBranch ?? session.worktree_branch ?? null;
}

function resolveInitialPrompt(
  session: TaskSession | null,
  messagesBySession: Record<string, Message[]>,
) {
  if (!session) return null;
  return (
    messagesBySession[session.id]?.find((message) => message.author_type === "user")?.content ??
    null
  );
}

function useSubtaskDialogState(parentTaskId: string, parentSessions: TaskSession[]) {
  const agentProfiles = useAppStore((s) => s.agentProfiles.items);
  const workspaceId = useAppStore((s) => s.workspaces.activeId);
  const workflowId = useAppStore((s) => s.kanban.workflowId);
  const executors = useAppStore((s) => s.executors.items);
  const parentTask = useAppStore(
    (s) => s.kanban.tasks.find((task) => task.id === parentTaskId) ?? null,
  );
  const sessionsById = useAppStore((s) => s.taskSessions.items);
  const sessionsByTaskId = useAppStore((s) => s.taskSessionsByTask.itemsByTaskId);
  const worktreeIdsBySessionId = useAppStore((s) => s.sessionWorktreesBySessionId.itemsBySessionId);
  const worktrees = useAppStore((s) => s.worktrees.items);
  const messagesBySession = useAppStore((s) => s.messages.bySession);
  const parentContext = useMemo(
    () =>
      resolveSubtaskParentContext({
        task: parentTask,
        sessionsById,
        sessionsByTaskId: {
          ...sessionsByTaskId,
          [parentTaskId]: parentSessions.length
            ? parentSessions
            : (sessionsByTaskId[parentTaskId] ?? []),
        },
        worktreeIdsBySessionId,
        worktrees,
        messagesBySession,
      }),
    [
      messagesBySession,
      parentTask,
      parentSessions,
      sessionsById,
      sessionsByTaskId,
      worktreeIdsBySessionId,
      worktrees,
    ],
  );

  return {
    agentProfiles,
    workspaceId,
    workflowId,
    executors,
    ...parentContext,
  };
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
      const parts = profile?.label.split(" \u2022 ");
      const name = parts?.[1] || parts?.[0] || t("task:panelAgent");
      return { id: s.id, label: name, index: idx + 1, agentName: profile?.agent_name };
    });
  }, [sessions, agentProfiles, t]);
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
  autoTitle: boolean;
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
  autoTitle,
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
  const fs = useSubtaskFormState(workspaceId);
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
  const selectedExecutorProfile = executorProfileOptions.find(
    (profile) => profile.value === fs.executorProfileId,
  );
  const isLocalExecutor = Boolean(
    selectedExecutorProfile?.executorType === "local" ||
    selectedExecutorProfile?.executorType === "local_pc",
  );
  const freshBranchAvailable =
    workspaceMode === "new_workspace" &&
    !fs.useRemote &&
    isLocalExecutor &&
    fs.repositories.length === 1;
  const promptZone = useSubtaskPromptZone({
    parentTaskId,
    workspaceId,
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
    autoTitle,
    autopilot: fs.autopilot,
    isLocalExecutor,
    setIsCreating,
    onClose,
    workspaceMode,
  });
  return renderSubtaskFormBody({
    fs,
    handlers,
    title,
    setTitle,
    autoTitle,
    autopilot: fs.autopilot,
    workspaceId,
    availableRepositories,
    worktreeBranch,
    profileOptions,
    executorProfileOptions,
    agentProfileId: fs.agentProfileId || defaultProfileId,
    workspaceMode,
    onWorkspaceModeChange: (mode: SubtaskWorkspaceMode) => {
      if (mode !== "new_workspace") fs.setFreshBranchEnabled(false);
      setWorkspaceMode(mode);
    },
    isLocalExecutor,
    freshBranchAvailable,
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
      onApplyPending: promptZone.applyPending,
      onCopyPending: promptZone.copyPending,
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
  const { t } = useTranslation();
  const { sessions: parentSessions } = useTaskSessions(parentTaskId);
  const {
    agentProfiles,
    workspaceId,
    workflowId,
    executors,
    session: parentSession,
    worktreeBranch,
    initialPrompt,
    parentRepositoryId,
    baseBranch,
  } = useSubtaskDialogState(parentTaskId, parentSessions);

  // Ensure executor/agent data is loaded when dialog opens
  useSettingsData(open);
  // Load workspace repositories so the subtask repo override picker has options.
  const { repositories: availableRepositories } = useRepositories(workspaceId, open);

  const siblingCount = useAppStore(
    (s) => s.kanban.tasks.filter((t) => t.parentTaskId === parentTaskId).length,
  );
  const autoTitle = useAppStore((s) => s.userSettings.agentGeneratedTaskTitles);

  const defaultTitle = useMemo(
    () => t("task:defaultSubtaskTitle", { parent: parentTaskTitle, index: siblingCount + 1 }),
    [parentTaskTitle, siblingCount],
  );

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        data-testid="new-subtask-dialog"
        onOpenAutoFocus={(event) => {
          if (!autoTitle) return;
          const content = event.currentTarget as HTMLElement | null;
          const prompt = content?.querySelector<HTMLTextAreaElement>(
            '[data-testid="subtask-prompt-input"]',
          );
          if (!prompt) return;
          event.preventDefault();
          prompt.focus();
        }}
        className="w-full h-full min-w-0 max-w-full max-h-full overflow-hidden rounded-none sm:w-[800px] sm:h-auto sm:max-w-none sm:max-h-[85vh] sm:rounded-lg flex flex-col"
      >
        <DialogHeader>
          <DialogTitle className="min-w-0 wrap-break-word pr-6 text-sm font-medium">
            {t("task:newSubtaskFor")} <span className="text-foreground">{parentTaskTitle}</span>
          </DialogTitle>
        </DialogHeader>
        <NewSubtaskForm
          key={`${parentTaskId}-${open}`}
          parentTaskId={parentTaskId}
          defaultTitle={defaultTitle}
          defaultProfileId={parentSession?.agent_profile_id ?? ""}
          worktreeBranch={worktreeBranch}
          initialPrompt={initialPrompt}
          agentProfiles={agentProfiles}
          executors={executors}
          workspaceId={workspaceId}
          workflowId={workflowId}
          parentRepositoryId={parentRepositoryId}
          baseBranch={baseBranch}
          availableRepositories={availableRepositories}
          isOpen={open}
          onClose={() => onOpenChange(false)}
          autoTitle={autoTitle}
        />
      </DialogContent>
    </Dialog>
  );
}
