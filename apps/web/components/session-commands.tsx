"use client";

import { useCallback, useMemo, useRef, useState, type ReactNode } from "react";
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
import { gitOperationLabel, useGitWithFeedback } from "@/hooks/use-git-with-feedback";
import { usePanelActions } from "@/hooks/use-panel-actions";
import { useVcsDialogs } from "@/components/vcs/vcs-dialogs";
import { useAppStore } from "@/components/state-provider";
import { getWebSocketClient } from "@/lib/ws/connection";
import { createFile } from "@/lib/ws/workspace-files";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { NewSessionDialog } from "@/components/task/new-session-dialog";
import { NewSubtaskDialog } from "@/components/task/new-subtask-dialog";
import { TaskArchiveConfirmation } from "@/components/task/task-archive-confirmation";
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

// Catalog keys, not copy — safe at module scope because nothing calls `t()`
// here. The palette groups by the RESOLVED value, so every producer of a group
// must go through these constants; `global-commands.tsx` does the same.
const GROUP_AGENT = "common:commandGroupAgent";
const GROUP_GIT = "common:commandGroupGit";
const GROUP_WORKSPACE = "common:commandGroupWorkspace";
const GROUP_PANELS = "common:commandGroupPanels";

export function buildSessionCommands(
  isAgentRunning: boolean | undefined,
  cancelTurn: () => void,
  t: TFunction,
): CommandItem[] {
  const items: CommandItem[] = [];
  if (isAgentRunning)
    items.push({
      id: "session-cancel",
      label: t("common:commandCancelTurn"),
      group: t(GROUP_AGENT),
      icon: <IconPlayerStop className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandCancelTurnKeywords"),
      action: cancelTurn,
    });
  return items;
}

type GitCommandOptions = {
  git: GitOps;
  baseBranch: string | undefined;
  openCommitDialog: () => void;
  openPRDialog: () => void;
  runGitWithFeedback: GitRunFn;
  t: TFunction;
};

export function buildGitCommands({
  git,
  baseBranch,
  openCommitDialog,
  openPRDialog,
  runGitWithFeedback,
  t,
}: GitCommandOptions): CommandItem[] {
  const group = t(GROUP_GIT);
  return [
    {
      id: "git-commit",
      label: t("common:commandCommitChanges"),
      group,
      icon: <IconGitCommit className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandCommitChangesKeywords"),
      action: openCommitDialog,
    },
    {
      id: "git-push",
      label: t("common:gitOpPush"),
      group,
      icon: <IconArrowUp className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandPushKeywords"),
      action: () => runGitWithFeedback(() => git.push(), gitOperationLabel(t, "common:gitOpPush")),
    },
    {
      id: "git-pull",
      label: t("common:gitOpPull"),
      group,
      icon: <IconArrowDown className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandPullKeywords"),
      action: () => runGitWithFeedback(() => git.pull(), gitOperationLabel(t, "common:gitOpPull")),
    },
    {
      id: "git-create-pr",
      label: t("common:commandCreatePr"),
      group,
      icon: <IconGitPullRequest className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandCreatePrKeywords"),
      action: openPRDialog,
    },
    {
      id: "git-rebase",
      label: t("common:gitOpRebase"),
      group,
      icon: <IconGitBranch className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandRebaseKeywords"),
      action: () => {
        const target = baseBranch?.replace(/^origin\//, "") || "main";
        return runGitWithFeedback(
          () => git.rebase(target),
          gitOperationLabel(t, "common:gitOpRebase"),
        );
      },
    },
    {
      id: "git-merge",
      label: t("common:gitOpMerge"),
      group,
      icon: <IconGitMerge className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandMergeKeywords"),
      action: () => {
        const target = baseBranch?.replace(/^origin\//, "") || "main";
        return runGitWithFeedback(
          () => git.merge(target),
          gitOperationLabel(t, "common:gitOpMerge"),
        );
      },
    },
  ];
}

export function buildWorkspaceCommands(sessionId: string, t: TFunction): CommandItem[] {
  return [
    {
      id: "workspace-create-file",
      label: t("common:commandCreateFile"),
      group: t(GROUP_WORKSPACE),
      icon: <IconFilePlus className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandCreateFileKeywords"),
      enterMode: "input",
      inputPlaceholder: t("common:commandCreateFilePlaceholder"),
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
  archiveConfirmation?: ReactNode;
  onArchiveConfirmationDismiss?: () => void;
};

export function buildTaskCommands({
  activeTaskId,
  isTaskArchived,
  t,
  openNewAgent,
  openSubtask,
  requestArchive,
  archiveConfirmation,
  onArchiveConfirmationDismiss,
}: TaskCommandOptions): CommandItem[] {
  if (!activeTaskId) return [];
  const items: CommandItem[] = [
    {
      id: "agent-new",
      label: t("common:commandNewAgent"),
      group: t(GROUP_AGENT),
      icon: <IconMessagePlus className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandNewAgentKeywords"),
      action: openNewAgent,
    },
    {
      id: "subtask-create",
      label: t("common:commandCreateSubtask"),
      group: t("common:commandGroupTasks"),
      icon: <IconSubtask className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandCreateSubtaskKeywords"),
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
      keepOpen: true,
      confirmation: archiveConfirmation,
      onConfirmationDismiss: onArchiveConfirmationDismiss,
    });
  }
  return items;
}

export function buildPanelCommands(
  panels: PanelActions,
  isPassthrough: boolean | undefined,
  t: TFunction,
): CommandItem[] {
  const group = t(GROUP_PANELS);
  const items: CommandItem[] = [
    {
      id: "panel-browser",
      label: t("common:commandAddBrowserPanel"),
      group,
      icon: <IconBrowser className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandAddBrowserPanelKeywords"),
      action: () => panels.addBrowser(),
    },
    {
      id: "panel-terminal",
      label: t("common:commandAddTerminalPanel"),
      group,
      icon: <IconTerminal2 className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandAddTerminalPanelKeywords"),
      action: () => panels.addTerminal(),
    },
  ];
  if (!isPassthrough)
    items.push({
      id: "panel-plan",
      label: t("common:commandAddPlanPanel"),
      group,
      icon: <IconFileText className="size-3.5" />,
      keywords: searchKeywords(t, "common:commandAddPlanPanelKeywords"),
      action: () => panels.addPlan(),
    });
  items.push({
    id: "panel-changes",
    label: t("common:commandAddChangesPanel"),
    group,
    icon: <IconFileDiff className="size-3.5" />,
    keywords: searchKeywords(t, "common:commandAddChangesPanelKeywords"),
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
};

/** Dialogs the task-scoped palette commands open. */
function SessionCommandDialogs({
  activeTaskId,
  activeTaskTitle,
  dialogs,
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
    </>
  );
}

function useArchiveCommandConfirmation(archive: ReturnType<typeof useTaskArchiveConfirm>) {
  const archiveAnchorRef = useRef<HTMLElement>(null);
  if (archive.target === null) return undefined;

  return (
    <TaskArchiveConfirmation
      open
      inline
      anchorRef={archiveAnchorRef}
      taskId={archive.target.id}
      taskTitle={archive.target.title}
      executorType={archive.target.executorType}
      isArchiving={archive.isPending}
      onOpenChange={(open) => {
        if (!open) archive.closeConfirm();
      }}
      onConfirm={archive.confirmArchive}
      confirmTestId="palette-archive-confirm"
    />
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
  const archiveConfirmation = useArchiveCommandConfirmation(archive);

  // Session-scoped commands need a live session; task-scoped ones only need the
  // task, so they stay available while a session is still being ensured.
  const commands = useMemo<CommandItem[]>(() => {
    const sessionScoped = sessionId
      ? [
          ...buildSessionCommands(isAgentRunning, cancelTurn, t),
          ...(hasWorktree
            ? buildGitCommands({
                git,
                baseBranch,
                openCommitDialog,
                openPRDialog,
                runGitWithFeedback,
                t,
              })
            : []),
          ...(hasWorktree ? buildWorkspaceCommands(sessionId, t) : []),
          ...buildPanelCommands(panels, isPassthrough, t),
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
        archiveConfirmation,
        onArchiveConfirmationDismiss: archive.closeConfirm,
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
    archiveConfirmation,
    archive.closeConfirm,
  ]);

  useRegisterCommands(commands);

  if (!activeTaskId) return null;

  return (
    <SessionCommandDialogs
      activeTaskId={activeTaskId}
      activeTaskTitle={activeTaskTitle}
      dialogs={dialogs}
    />
  );
}
