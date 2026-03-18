"use client";

import { useCallback, useMemo, useState } from "react";
import {
  IconPlayerStop,
  IconFileTextSpark,
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
import type { CommandItem } from "@/lib/commands/types";

type SessionCommandsProps = {
  sessionId: string | null;
  baseBranch?: string;
  isAgentRunning?: boolean;
  hasWorktree?: boolean;
  isPassthrough?: boolean;
};

type GitRunFn = (
  op: () => Promise<{ success: boolean; output: string; error?: string }>,
  name: string,
) => Promise<void>;
type GitOps = ReturnType<typeof useGitOperations>;
type PanelActions = ReturnType<typeof usePanelActions>;

function buildSessionCommands(
  isAgentRunning: boolean | undefined,
  isPassthrough: boolean | undefined,
  cancelTurn: () => void,
  panels: PanelActions,
): CommandItem[] {
  const items: CommandItem[] = [];
  if (isAgentRunning)
    items.push({
      id: "session-cancel",
      label: "Cancel Turn",
      group: "Agent",
      icon: <IconPlayerStop className="size-3.5" />,
      keywords: ["cancel", "stop", "turn"],
      action: cancelTurn,
    });
  if (!isPassthrough)
    items.push({
      id: "session-plan-mode",
      label: "Toggle Plan Mode",
      group: "Agent",
      icon: <IconFileTextSpark className="size-3.5" />,
      keywords: ["plan", "mode", "toggle"],
      action: () => panels.addPlan(),
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
      keywords: ["commit", "git", "save"],
      action: openCommitDialog,
    },
    {
      id: "git-push",
      label: "Push",
      group: "Git",
      icon: <IconArrowUp className="size-3.5" />,
      keywords: ["push", "git", "upload"],
      action: () => runGitWithFeedback(() => git.push(), "Push"),
    },
    {
      id: "git-pull",
      label: "Pull",
      group: "Git",
      icon: <IconArrowDown className="size-3.5" />,
      keywords: ["pull", "git", "download", "fetch"],
      action: () => runGitWithFeedback(() => git.pull(), "Pull"),
    },
    {
      id: "git-create-pr",
      label: "Create PR",
      group: "Git",
      icon: <IconGitPullRequest className="size-3.5" />,
      keywords: ["pull request", "pr", "git"],
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

function buildTaskCommands(
  activeTaskId: string | null,
  openNewAgent: () => void,
  openSubtask: () => void,
): CommandItem[] {
  if (!activeTaskId) return [];
  return [
    {
      id: "agent-new",
      label: "New Agent",
      group: "Agent",
      icon: <IconMessagePlus className="size-3.5" />,
      keywords: ["new", "agent", "session", "start"],
      action: openNewAgent,
    },
    {
      id: "subtask-create",
      label: "Create Subtask",
      group: "Tasks",
      icon: <IconSubtask className="size-3.5" />,
      keywords: ["subtask", "create", "new", "child"],
      action: openSubtask,
    },
  ];
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
      keywords: ["browser", "preview", "web"],
      action: () => panels.addBrowser(),
    },
    {
      id: "panel-terminal",
      label: "Add Terminal Panel",
      group: "Panels",
      icon: <IconTerminal2 className="size-3.5" />,
      keywords: ["terminal", "shell", "console"],
      action: () => panels.addTerminal(),
    },
  ];
  if (!isPassthrough)
    items.push({
      id: "panel-plan",
      label: "Add Plan Panel",
      group: "Panels",
      icon: <IconFileText className="size-3.5" />,
      keywords: ["plan", "document"],
      action: () => panels.addPlan(),
    });
  items.push({
    id: "panel-changes",
    label: "Add Changes Panel",
    group: "Panels",
    icon: <IconFileDiff className="size-3.5" />,
    keywords: ["changes", "diff", "git"],
    action: () => panels.addChanges(),
  });
  return items;
}

export function SessionCommands({
  sessionId,
  baseBranch,
  isAgentRunning,
  hasWorktree,
  isPassthrough,
}: SessionCommandsProps) {
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

  const [showNewAgentDialog, setShowNewAgentDialog] = useState(false);
  const [showSubtaskDialog, setShowSubtaskDialog] = useState(false);
  const openNewAgent = useCallback(() => setShowNewAgentDialog(true), []);
  const openSubtask = useCallback(() => setShowSubtaskDialog(true), []);

  const cancelTurn = useCallback(async () => {
    if (!sessionId) return;
    const client = getWebSocketClient();
    if (!client) return;
    try {
      await client.request("agent.cancel", { session_id: sessionId }, 15000);
    } catch (error) {
      console.error("Failed to cancel agent turn:", error);
    }
  }, [sessionId]);

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

  const commands = useMemo<CommandItem[]>(() => {
    if (!sessionId) return [];
    const items = [
      ...buildSessionCommands(isAgentRunning, isPassthrough, cancelTurn, panels),
      ...(hasWorktree
        ? buildGitCommands(git, baseBranch, openCommitDialog, openPRDialog, runGitWithFeedback)
        : []),
      ...(hasWorktree ? buildWorkspaceCommands(sessionId) : []),
      ...buildPanelCommands(panels, isPassthrough),
      ...buildTaskCommands(activeTaskId, openNewAgent, openSubtask),
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
    openCommitDialog,
    openPRDialog,
    runGitWithFeedback,
    openNewAgent,
    openSubtask,
  ]);

  useRegisterCommands(commands);

  return (
    <>
      {activeTaskId && (
        <NewSessionDialog
          open={showNewAgentDialog}
          onOpenChange={setShowNewAgentDialog}
          taskId={activeTaskId}
        />
      )}
      {activeTaskId && (
        <NewSubtaskDialog
          open={showSubtaskDialog}
          onOpenChange={setShowSubtaskDialog}
          parentTaskId={activeTaskId}
          parentTaskTitle={activeTaskTitle}
        />
      )}
    </>
  );
}
