"use client";

import { useCallback, useMemo, useState } from "react";
import { useTranslation } from "react-i18next";
import type { TFunction } from "i18next";
import {
  IconArchive,
  IconPlayerStop,
  IconGitCommit,
  IconArrowUp,
  IconArrowDown,
  IconGitPullRequest,
  IconGitBranch,
  IconGitMerge,
  IconBrowser,
  IconTerminal2,
  IconFileText,
  IconFileDiff,
  IconFilePlus,
  IconMessagePlus,
  IconSubtask,
} from "@tabler/icons-react";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { useGitOperations } from "@/hooks/use-git-operations";
import { useGitWithFeedback } from "@/hooks/use-git-with-feedback";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { useVcsDialogs } from "@/components/vcs/vcs-dialogs";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createFile } from "@/lib/ws/workspace-files";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { NewSessionDialog } from "@/components/task/new-session-dialog";
import { NewSubtaskDialog } from "@/components/task/new-subtask-dialog";
import { TaskArchiveConfirmDialog } from "@/components/task/task-archive-confirm-dialog";
import { useTaskArchiveConfirm } from "@/hooks/use-task-archive-confirm";
import { searchKeywords } from "@/lib/commands/search-keywords";
import type { CommandItem } from "@/lib/commands/types";

type SessionCommandsProps = {
  sessionId: string | null;
  baseBranch?: string;
  isAgentRunning?: boolean;
  hasWorktree?: boolean;
  isPassthrough?: boolean;
  /** Archived tasks are read-only, so the palette hides the archive command. */
  isTaskArchived?: boolean;
};

type GitRunFn = (
  op: () => Promise<{ success: boolean; output: string; error?: string }>,
  name: string,
) => Promise<void>;
type GitOps = ReturnType<typeof useGitOperations>;
type PanelActions = ReturnType<typeof usePanelActions>;

function buildSessionCommands(
  isAgentRunning: boolean | undefined,
  cancelTurn: () => void,
): CommandItem[] {
  const items: CommandItem[] = [];
  if (isAgentRunning)
    items.push({
      id: "session-cancel",
      label: "Cancel Turn",
      group: "Agent",
      icon: <IconPlayerStop className="size-3.5" />,
      keywords: ["cancel", "stop", "turn", "cancel agent", "stop agent", "interrupt agent"],
      action: cancelTurn,
    });
  return items;
}

function buildGitCommands(
  git: GitOps,
  baseBranch: string | undefined,
  openCommitDialog: () => void,
  openPRDialog: () => void,
  runGitWithFeedback: GitRunFn,
): CommandItem[] {
  return [
    {
      id: "git-commit",
      label: "Commit Changes",
      group: "Git",
      icon: <IconGitCommit className="size-3.5" />,
      keywords: ["commit", "git", "save changes", "git commit"],
      action: openCommitDialog,
    },
    {
      id: "git-push",
      label: "Push",
      group: "Git",
      icon: <IconArrowUp className="size-3.5" />,
      keywords: ["push", "git", "push changes", "push to remote", "upload changes"],
      action: () => runGitWithFeedback(() => git.push(), "Push"),
    },
    {
      id: "git-pull",
      label: "Pull",
      group: "Git",
      icon: <IconArrowDown className="size-3.5" />,
      keywords: ["pull", "git", "pull changes", "download changes"],
      action: () => runGitWithFeedback(() => git.pull(), "Pull"),
    },
    {
      id: "git-create-pr",
      label: "Create PR",
      group: "Git",
      icon: <IconGitPullRequest className="size-3.5" />,
      keywords: ["pull request", "pr", "open pull request", "submit pull request", "git"],
      action: openPRDialog,
    },
    {
      id: "git-rebase",
      label: "Rebase",
      group: "Git",
      icon: <IconGitBranch className="size-3.5" />,
      keywords: ["rebase", "git", "branch"],
      action: () => {
        const t = baseBranch?.replace(/^origin\//, "") || "main";
        return runGitWithFeedback(() => git.rebase(t), "Rebase");
      },
    },
    {
      id: "git-merge",
      label: "Merge",
      group: "Git",
      icon: <IconGitMerge className="size-3.5" />,
      keywords: ["merge", "git", "branch"],
      action: () => {
        const t = baseBranch?.replace(/^origin\//, "") || "main";
        return runGitWithFeedback(() => git.merge(t), "Merge");
      },
    },
  ];
}

function buildWorkspaceCommands(sessionId: string): CommandItem[] {
  return [
    {
      id: "workspace-create-file",
      label: "Create File",
      group: "Workspace",
      icon: <IconFilePlus className="size-3.5" />,
      keywords: ["create", "new", "file", "add"],
      enterMode: "input",
      inputPlaceholder: "File path relative to workspace root...",
      onInputSubmit: async (path) => {
        const client = getWebSocketClient();
        if (!client) return;
        try {
          const response = await createFile(client, sessionId, path);
          if (response.success) {
            const name = path.split("/").pop() || path;
            useDockviewStore.getState().addFileEditorPanel(path, name);
          }
        } catch (error) {
          console.error("Failed to create file:", error);
        }
      },
    },
  ];
}

type TaskCommandOptions = {
  activeTaskId: string | null;
  isTaskArchived: boolean;
  t: TFunction;
  openNewAgent: () => void;
  openSubtask: () => void;
  requestArchive: () => void;
};

export function buildTaskCommands({
  activeTaskId,
  isTaskArchived,
  t,
  openNewAgent,
  openSubtask,
  requestArchive,
}: TaskCommandOptions): CommandItem[] {
  if (!activeTaskId) return [];
  const items: CommandItem[] = [
    {
      id: "agent-new",
      label: "New Agent",
      group: "Agent",
      icon: <IconMessagePlus className="size-3.5" />,
      keywords: ["new", "agent", "session", "start agent", "new session"],
      action: openNewAgent,
    },
    {
      id: "subtask-create",
      label: "Create Subtask",
      group: t("common:commandGroupTasks"),
      icon: <IconSubtask className="size-3.5" />,
      keywords: ["subtask", "create", "new subtask", "new sub-task", "child task"],
      action: openSubtask,
    },
  ];
  // An archived task cannot be archived again; the palette offers no archive
  // entry for it rather than surfacing a command that always fails.
  if (!isTaskArchived) {
    items.push({
      id: "task-archive",
      label: t("tasks:archiveTask"),
      group: t("common:commandGroupTasks"),
      icon: <IconArchive className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandArchiveTaskKeywords"),
      action: requestArchive,
    });
  }
  return items;
}

function buildPanelCommands(
  panels: PanelActions,
  isPassthrough: boolean | undefined,
): CommandItem[] {
  const items: CommandItem[] = [
    {
      id: "panel-browser",
      label: "Add Browser Panel",
      group: "Panels",
      icon: <IconBrowser className="size-3.5" />,
      keywords: ["browser", "preview", "web", "open browser preview", "web preview", "app preview"],
      action: () => panels.addBrowser(),
    },
    {
      id: "panel-terminal",
      label: "Add Terminal Panel",
      group: "Panels",
      icon: <IconTerminal2 className="size-3.5" />,
      keywords: ["terminal", "shell", "console", "new terminal", "open terminal", "command line"],
      action: () => panels.addTerminal(),
    },
  ];
  if (!isPassthrough)
    items.push({
      id: "panel-plan",
      label: "Add Plan Panel",
      group: "Panels",
      icon: <IconFileText className="size-3.5" />,
      keywords: ["plan", "document", "task plan", "implementation plan", "plan details"],
      action: () => panels.addPlan(),
    });
  items.push({
    id: "panel-changes",
    label: "Add Changes Panel",
    group: "Panels",
    icon: <IconFileDiff className="size-3.5" />,
    keywords: [
      "changes",
      "diff",
      "git changes",
      "changed files",
      "source control",
      "git diff",
      "review changes",
    ],
    action: () => panels.addChanges(),
  });
  return items;
}

function useCancelTurn(sessionId: string | null) {
  return useCallback(async () => {
    if (!sessionId) return;
    const client = getWebSocketClient();
    if (!client) return;
    try {
      await client.request("agent.cancel", { session_id: sessionId }, 15000);
    } catch (error) {
      console.error("Failed to cancel agent turn:", error);
    }
  }, [sessionId]);
}

function useCommandDialogState() {
  const [newAgentOpen, setNewAgentOpen] = useState(false);
  const [subtaskOpen, setSubtaskOpen] = useState(false);
  const openNewAgent = useCallback(() => setNewAgentOpen(true), []);
  const openSubtask = useCallback(() => setSubtaskOpen(true), []);
  return { newAgentOpen, setNewAgentOpen, subtaskOpen, setSubtaskOpen, openNewAgent, openSubtask };
}

type SessionCommandDialogsProps = {
  activeTaskId: string;
  activeTaskTitle: string;
  dialogs: ReturnType<typeof useCommandDialogState>;
  archive: ReturnType<typeof useTaskArchiveConfirm>;
};

/** Dialogs the task-scoped palette commands open. */
function SessionCommandDialogs({
  activeTaskId,
  activeTaskTitle,
  dialogs,
  archive,
}: SessionCommandDialogsProps) {
  return (
    <>
      <NewSessionDialog
        open={dialogs.newAgentOpen}
        onOpenChange={dialogs.setNewAgentOpen}
        taskId={activeTaskId}
      />
      <NewSubtaskDialog
        open={dialogs.subtaskOpen}
        onOpenChange={dialogs.setSubtaskOpen}
        parentTaskId={activeTaskId}
        parentTaskTitle={activeTaskTitle}
      />
      <TaskArchiveConfirmDialog
        open={archive.target !== null}
        onOpenChange={(open) => {
          if (!open) archive.closeConfirm();
        }}
        taskTitle={archive.target?.title ?? ""}
        taskId={activeTaskId}
        executorType={archive.target?.executorType}
        isArchiving={archive.isPending}
        onConfirm={archive.confirmArchive}
        confirmTestId="palette-archive-confirm"
      />
    </>
  );
}

export function SessionCommands({
  sessionId,
  baseBranch,
  isAgentRunning,
  hasWorktree,
  isPassthrough,
  isTaskArchived,
}: SessionCommandsProps) {
  const { t } = useTranslation();
  const git = useGitOperations(sessionId);
  const panels = usePanelActions();
  const { openCommitDialog, openPRDialog } = useVcsDialogs();
  const gitWithFeedback = useGitWithFeedback();

  const activeTaskId = useAppStore((s) => s.tasks.activeTaskId);
  const activeTaskTitle = useAppStore((s) => {
    const id = s.tasks.activeTaskId;
    if (!id) return "";
    return s.kanban.tasks.find((t: { id: string }) => t.id === id)?.title ?? "";
  });

  const dialogs = useCommandDialogState();
  const { openNewAgent, openSubtask } = dialogs;
  const cancelTurn = useCancelTurn(sessionId);

  const runGitWithFeedback = useCallback(
    async (
      operation: () => Promise<{ success: boolean; output: string; error?: string }>,
      operationName: string,
    ) => {
      panels.addChanges();
      await gitWithFeedback(operation, operationName);
    },
    [panels, gitWithFeedback],
  );

  const archive = useTaskArchiveConfirm(activeTaskId);
  const { requestArchive } = archive;

  // Session-scoped commands need a live session; task-scoped ones only need the
  // task, so they stay available while a session is still being ensured.
  const commands = useMemo<CommandItem[]>(() => {
    const sessionScoped = sessionId
      ? [
          ...buildSessionCommands(isAgentRunning, cancelTurn),
          ...(hasWorktree
            ? buildGitCommands(git, baseBranch, openCommitDialog, openPRDialog, runGitWithFeedback)
            : []),
          ...(hasWorktree ? buildWorkspaceCommands(sessionId) : []),
          ...buildPanelCommands(panels, isPassthrough),
        ]
      : [];
    const items = [
      ...sessionScoped,
      ...buildTaskCommands({
        activeTaskId,
        isTaskArchived: Boolean(isTaskArchived),
        t,
        openNewAgent,
        openSubtask,
        requestArchive,
      }),
    ];
    return items.map((cmd) => ({ ...cmd, priority: 0 }));
  }, [
    sessionId,
    activeTaskId,
    git,
    panels,
    cancelTurn,
    baseBranch,
    isAgentRunning,
    hasWorktree,
    isPassthrough,
    isTaskArchived,
    t,
    openCommitDialog,
    openPRDialog,
    runGitWithFeedback,
    openNewAgent,
    openSubtask,
    requestArchive,
  ]);

  useRegisterCommands(commands);

  if (!activeTaskId) return null;

  return (
    <SessionCommandDialogs
      activeTaskId={activeTaskId}
      activeTaskTitle={activeTaskTitle}
      dialogs={dialogs}
      archive={archive}
    />
  );
}
