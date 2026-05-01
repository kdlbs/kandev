"use client";

import { useCallback, useState } from "react";
import { type IDockviewHeaderActionsProps } from "dockview-react";
import {
  IconPlus,
  IconMessagePlus,
  IconDeviceDesktop,
  IconTerminal2,
  IconFileText,
  IconFolder,
  IconGitBranch,
  IconGitPullRequest,
  IconPlayerPlay,
  IconLayoutSidebarRightCollapse,
  IconBrandVscode,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useDockviewStore, performLayoutSwitch } from "@/lib/state/dockview-store";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useActiveTaskPR } from "@/hooks/domains/github/use-task-pr";
import { prPanelLabel } from "@/components/github/pr-utils";
import { startProcess } from "@/lib/api";
import { createUserShell } from "@/lib/api/domains/user-shell-api";
import { useRepositoryScripts } from "@/hooks/domains/workspace/use-repository-scripts";
import { replaceTaskUrl } from "@/lib/links";
import type { Task, ProcessInfo } from "@/lib/types/http";
import type { ProcessStatusEntry } from "@/lib/state/slices";
import { NewSessionDialog } from "./new-session-dialog";
import { NewTaskDropdown } from "./new-task-dropdown";
import { RepositoryScriptsMenuItems, useActiveSessionDevScript } from "./repository-scripts-menu";
import { SessionReopenMenuItems } from "./session-reopen-menu";
import { GroupSplitCloseActionsView, useDockviewGroupWidth } from "./dockview-group-actions";

const HEADER_ACTION_BUTTON_CLASS =
  "h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring";
const RAW_HEADER_ACTION_BUTTON_CLASS =
  "inline-flex h-6 w-6 items-center justify-center rounded-[5px] text-muted-foreground transition-colors hover:bg-muted/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring cursor-pointer";
const HEADER_ICON_CLASS = "h-3.5 w-3.5";
const MENU_ICON_CLASS = "h-3.5 w-3.5 mr-1.5 shrink-0";
const MENU_ITEM_CLASS = "cursor-pointer text-xs";

/** Map a ProcessInfo response to a ProcessStatusEntry for the store. */
function mapProcessToStatus(process: ProcessInfo): ProcessStatusEntry {
  return {
    processId: process.id,
    sessionId: process.session_id,
    kind: process.kind,
    scriptName: process.script_name,
    status: process.status,
    command: process.command,
    workingDir: process.working_dir,
    exitCode: process.exit_code ?? null,
    startedAt: process.started_at,
    updatedAt: process.updated_at,
  };
}

function useLeftHeaderState(
  groupId: string,
  containerApi: IDockviewHeaderActionsProps["containerApi"],
) {
  const sidebarGroupId = useDockviewStore((s) => s.sidebarGroupId);
  const centerGroupId = useDockviewStore((s) => s.centerGroupId);
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const taskId = useAppStore((state) => state.tasks.activeTaskId);
  const isPassthrough = useAppStore((state) => {
    if (!activeSessionId) return false;
    return state.taskSessions.items[activeSessionId]?.is_passthrough === true;
  });
  const pr = useActiveTaskPR();
  const hasChanges = Boolean(
    containerApi.getPanel("changes") ?? containerApi.getPanel("diff-files"),
  );
  const hasFiles = Boolean(containerApi.getPanel("files") ?? containerApi.getPanel("all-files"));
  return {
    isSidebarGroup: groupId === sidebarGroupId,
    isCenterGroup: groupId === centerGroupId,
    activeSessionId,
    taskId,
    isPassthrough,
    pr,
    hasChanges,
    hasFiles,
  };
}

function AddPanelMenuItems({
  groupId,
  state,
  onNewSession,
  onAddTerminal,
  onRunScript,
  onRunDevScript,
}: {
  groupId: string;
  state: ReturnType<typeof useLeftHeaderState>;
  onNewSession: () => void;
  onAddTerminal: () => void;
  onRunScript: (scriptId: string) => void;
  onRunDevScript: () => void;
}) {
  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const addVscodePanel = useDockviewStore((s) => s.addVscodePanel);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);
  const addFilesPanel = useDockviewStore((s) => s.addFilesPanel);
  const addChangesPanel = useDockviewStore((s) => s.addChangesPanel);
  const addPRPanel = useDockviewStore((s) => s.addPRPanel);

  return (
    <>
      {state.taskId && (
        <>
          <DropdownMenuItem
            onClick={onNewSession}
            className={MENU_ITEM_CLASS}
            data-testid="new-session-button"
          >
            <IconMessagePlus className={MENU_ICON_CLASS} />
            New Agent
          </DropdownMenuItem>
          <DropdownMenuSeparator />
          <SessionReopenMenuItems taskId={state.taskId} groupId={groupId} />
        </>
      )}
      <DropdownMenuItem onClick={onAddTerminal} className={MENU_ITEM_CLASS}>
        <IconTerminal2 className={MENU_ICON_CLASS} />
        Terminal
      </DropdownMenuItem>
      <DropdownMenuItem
        onClick={() => addBrowserPanel(undefined, groupId)}
        className={MENU_ITEM_CLASS}
      >
        <IconDeviceDesktop className={MENU_ICON_CLASS} />
        Browser
      </DropdownMenuItem>
      <DropdownMenuItem onClick={() => addVscodePanel()} className={MENU_ITEM_CLASS}>
        <IconBrandVscode className={MENU_ICON_CLASS} />
        VS Code
      </DropdownMenuItem>
      {!state.isPassthrough && (
        <DropdownMenuItem onClick={() => addPlanPanel({ groupId })} className={MENU_ITEM_CLASS}>
          <IconFileText className={MENU_ICON_CLASS} />
          Plan
        </DropdownMenuItem>
      )}
      {!state.hasChanges && (
        <DropdownMenuItem onClick={() => addChangesPanel(groupId)} className={MENU_ITEM_CLASS}>
          <IconGitBranch className={MENU_ICON_CLASS} />
          Changes
        </DropdownMenuItem>
      )}
      {!state.hasFiles && (
        <DropdownMenuItem onClick={() => addFilesPanel(groupId)} className={MENU_ITEM_CLASS}>
          <IconFolder className={MENU_ICON_CLASS} />
          Files
        </DropdownMenuItem>
      )}
      {state.pr && (
        <DropdownMenuItem onClick={() => addPRPanel()} className={MENU_ITEM_CLASS}>
          <IconGitPullRequest className={MENU_ICON_CLASS} />
          {prPanelLabel(state.pr.pr_number)}
        </DropdownMenuItem>
      )}
      <RepositoryScriptsMenuItems onRunScript={onRunScript} onRunDevScript={onRunDevScript} />
    </>
  );
}

export function LeftHeaderActions(props: IDockviewHeaderActionsProps) {
  const { group, containerApi } = props;
  const state = useLeftHeaderState(group.id, containerApi);
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);
  const devScript = useActiveSessionDevScript();
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);

  const handleAddTerminal = useCallback(async () => {
    if (!state.activeSessionId) return;
    try {
      const result = await createUserShell(state.activeSessionId);
      addTerminalPanel(result.terminalId, group.id);
    } catch (error) {
      console.error("Failed to create terminal:", error);
    }
  }, [state.activeSessionId, addTerminalPanel, group.id]);

  const handleRunScript = useCallback(
    async (scriptId: string) => {
      if (!state.activeSessionId) return;
      try {
        const result = await createUserShell(state.activeSessionId, { scriptId });
        addTerminalPanel(result.terminalId, group.id);
      } catch (error) {
        console.error("Failed to run script:", error);
      }
    },
    [state.activeSessionId, addTerminalPanel, group.id],
  );

  const handleRunDevScript = useCallback(async () => {
    if (!state.activeSessionId || !devScript) return;
    try {
      const result = await createUserShell(state.activeSessionId, {
        command: devScript,
        label: "Dev Server",
      });
      addTerminalPanel(result.terminalId, group.id);
    } catch (error) {
      console.error("Failed to start dev script:", error);
    }
  }, [state.activeSessionId, devScript, addTerminalPanel, group.id]);

  if (state.isSidebarGroup) return null;

  return (
    <div className="flex items-center gap-0.5 pl-0.5">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            size="sm"
            variant="ghost"
            className={HEADER_ACTION_BUTTON_CLASS}
            data-testid="dockview-add-panel-btn"
            aria-label="Add panel"
            title="Add panel"
          >
            <IconPlus className={HEADER_ICON_CLASS} />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-44">
          <AddPanelMenuItems
            groupId={group.id}
            state={state}
            onNewSession={() => setShowNewSessionDialog(true)}
            onAddTerminal={handleAddTerminal}
            onRunScript={handleRunScript}
            onRunDevScript={handleRunDevScript}
          />
        </DropdownMenuContent>
      </DropdownMenu>
      {state.taskId && (
        <NewSessionDialog
          open={showNewSessionDialog}
          onOpenChange={setShowNewSessionDialog}
          taskId={state.taskId}
          groupId={group.id}
        />
      )}
    </div>
  );
}

/** Faded maximize, split, and close buttons for any non-sidebar group. */
function GroupSplitCloseActions({ group, containerApi }: IDockviewHeaderActionsProps) {
  const centerGroupId = useDockviewStore((s) => s.centerGroupId);
  const isChatGroup = group.id === centerGroupId;
  const isMaximized = useDockviewStore((s) => s.preMaximizeLayout !== null);
  const storeMaximize = useDockviewStore((s) => s.maximizeGroup);
  const storeExitMaximize = useDockviewStore((s) => s.exitMaximizedLayout);
  const width = useDockviewGroupWidth(group);

  const handleMaximize = useCallback(() => {
    if (isMaximized) {
      storeExitMaximize();
    } else {
      storeMaximize(group.id);
    }
  }, [group.id, isMaximized, storeMaximize, storeExitMaximize]);

  const handleSplitRight = useCallback(() => {
    containerApi.addGroup({ referenceGroup: group, direction: "right" });
  }, [group, containerApi]);

  const handleSplitDown = useCallback(() => {
    containerApi.addGroup({ referenceGroup: group, direction: "below" });
  }, [group, containerApi]);

  const handleCloseGroup = useCallback(() => {
    const panels = [...group.panels];
    if (panels.length === 0) {
      try {
        containerApi.removeGroup(group);
      } catch {
        /* already removed */
      }
      return;
    }
    for (const panel of panels) {
      try {
        containerApi.removePanel(panel);
      } catch {
        /* already removed */
      }
    }
  }, [group, containerApi]);

  return (
    <GroupSplitCloseActionsView
      width={width}
      isChatGroup={isChatGroup}
      isMaximized={isMaximized}
      onMaximize={handleMaximize}
      onSplitRight={handleSplitRight}
      onSplitDown={handleSplitDown}
      onCloseGroup={handleCloseGroup}
    />
  );
}

export function RightHeaderActions(props: IDockviewHeaderActionsProps) {
  const { group } = props;
  const centerGroupId = useDockviewStore((s) => s.centerGroupId);
  const sidebarGroupId = useDockviewStore((s) => s.sidebarGroupId);
  const rightTopGroupId = useDockviewStore((s) => s.rightTopGroupId);
  const rightBottomGroupId = useDockviewStore((s) => s.rightBottomGroupId);

  const isSidebarGroup = group.id === sidebarGroupId;
  if (isSidebarGroup) return <SidebarRightActions />;

  const isCenterGroup = group.id === centerGroupId;
  const isRightTopGroup = group.id === rightTopGroupId;
  const isTerminalGroup = group.id === rightBottomGroupId;

  return (
    <div className="flex items-center gap-0.5 pr-0.5">
      {isCenterGroup && <CenterRightActions />}
      {isRightTopGroup && <RightTopGroupActions />}
      {isTerminalGroup && <TerminalGroupRightActions />}
      <GroupSplitCloseActions {...props} />
    </div>
  );
}

function SidebarRightActions() {
  const workspaceId = useAppStore((state) => state.workspaces.activeId);
  const workflowId = useAppStore((state) => state.workflows.activeId);
  const kanban = useAppStore((state) => state.kanban);
  const activeTaskId = useAppStore((state) => state.tasks.activeTaskId);
  const activeTaskTitle = useAppStore((state) => {
    const id = state.tasks.activeTaskId;
    if (!id) return "";
    return state.kanban.tasks.find((t: { id: string }) => t.id === id)?.title ?? "";
  });
  const setActiveTask = useAppStore((state) => state.setActiveTask);
  const setActiveSession = useAppStore((state) => state.setActiveSession);
  const appStore = useAppStoreApi();
  const steps = (kanban?.steps ?? []).map(
    (s: {
      id: string;
      title: string;
      color?: string;
      events?: {
        on_enter?: Array<{ type: string; config?: Record<string, unknown> }>;
        on_turn_complete?: Array<{ type: string; config?: Record<string, unknown> }>;
      };
    }) => ({
      id: s.id,
      title: s.title,
      color: s.color,
      events: s.events,
    }),
  );

  const handleTaskCreated = useCallback(
    (task: Task, _mode: "create" | "edit", meta?: { taskSessionId?: string | null }) => {
      const state = appStore.getState();
      const oldSessionId = state.tasks.activeSessionId;
      const oldEnvId = oldSessionId ? (state.environmentIdBySessionId[oldSessionId] ?? null) : null;
      setActiveTask(task.id);
      if (meta?.taskSessionId) {
        setActiveSession(task.id, meta.taskSessionId);
        const newEnvId = appStore.getState().environmentIdBySessionId[meta.taskSessionId] ?? null;
        if (newEnvId) performLayoutSwitch(oldEnvId, newEnvId, meta.taskSessionId);
      }
      replaceTaskUrl(task.id);
    },
    [setActiveTask, setActiveSession, appStore],
  );

  return (
    <div className="flex items-center gap-1 pr-2">
      <NewTaskDropdown
        workspaceId={workspaceId}
        workflowId={workflowId}
        steps={steps}
        activeTaskId={activeTaskId}
        activeTaskTitle={activeTaskTitle}
        onTaskCreated={handleTaskCreated}
      />
    </div>
  );
}

function RightTopGroupActions() {
  const toggleRightPanels = useDockviewStore((s) => s.toggleRightPanels);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <button
          type="button"
          className={RAW_HEADER_ACTION_BUTTON_CLASS}
          onClick={toggleRightPanels}
          aria-label="Hide right panels"
        >
          <IconLayoutSidebarRightCollapse className={HEADER_ICON_CLASS} />
        </button>
      </TooltipTrigger>
      <TooltipContent>Hide right panels</TooltipContent>
    </Tooltip>
  );
}

function CenterRightActions() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const repository = useAppStore((state) => {
    if (!activeSessionId) return null;
    const session = state.taskSessions.items[activeSessionId];
    if (!session) return null;
    const repoId = session.repository_id;
    if (!repoId) return null;
    const allRepos = Object.values(state.repositories.itemsByWorkspaceId).flat();
    return allRepos.find((r) => r.id === repoId) ?? null;
  });
  const hasDevScript = Boolean(repository?.dev_script?.trim());

  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);
  const setActiveProcess = useAppStore((state) => state.setActiveProcess);

  const handleStartBrowser = useCallback(async () => {
    addBrowserPanel();
    if (hasDevScript && activeSessionId) {
      try {
        const resp = await startProcess(activeSessionId, { kind: "dev" });
        if (resp?.process) {
          upsertProcessStatus(mapProcessToStatus(resp.process));
          setActiveProcess(resp.process.session_id, resp.process.id);
        }
      } catch {
        // Process may already be running
      }
    }
  }, [addBrowserPanel, hasDevScript, activeSessionId, upsertProcessStatus, setActiveProcess]);

  return (
    <div className="flex items-center gap-1">
      {/* Mode is shown in the chat input ModeSelector instead */}
      {hasDevScript && (
        <Button
          size="sm"
          variant="ghost"
          className={HEADER_ACTION_BUTTON_CLASS}
          onClick={handleStartBrowser}
          aria-label="Open browser preview"
          title="Open browser preview"
        >
          <IconDeviceDesktop className={HEADER_ICON_CLASS} />
        </Button>
      )}
    </div>
  );
}

function TerminalGroupRightActions() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const repositoryId = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.repository_id ?? null;
  });
  const hasDevScript = Boolean(useActiveSessionDevScript());

  const { scripts } = useRepositoryScripts(repositoryId);
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);
  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);
  const setActiveProcess = useAppStore((state) => state.setActiveProcess);
  const rightBottomGroupId = useDockviewStore((s) => s.rightBottomGroupId);

  const handleRunScript = useCallback(
    async (scriptId: string) => {
      if (!activeSessionId) return;
      try {
        const result = await createUserShell(activeSessionId, { scriptId });
        addTerminalPanel(result.terminalId, rightBottomGroupId);
      } catch (error) {
        console.error("Failed to run script:", error);
      }
    },
    [activeSessionId, addTerminalPanel, rightBottomGroupId],
  );

  const handleStartPreview = useCallback(async () => {
    if (!activeSessionId) return;
    addBrowserPanel();
    try {
      const resp = await startProcess(activeSessionId, { kind: "dev" });
      if (resp?.process) {
        upsertProcessStatus(mapProcessToStatus(resp.process));
        setActiveProcess(resp.process.session_id, resp.process.id);
      }
    } catch {
      // Process may already be running
    }
    try {
      const shell = await createUserShell(activeSessionId);
      addTerminalPanel(shell.terminalId, rightBottomGroupId);
    } catch {
      // Terminal creation is best-effort
    }
  }, [
    activeSessionId,
    addBrowserPanel,
    upsertProcessStatus,
    setActiveProcess,
    addTerminalPanel,
    rightBottomGroupId,
  ]);

  if (scripts.length === 0 && !hasDevScript) return null;

  return (
    <>
      {scripts.length > 0 && (
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button
              size="sm"
              variant="ghost"
              className={HEADER_ACTION_BUTTON_CLASS}
              aria-label="Run script"
              title="Run script"
            >
              <IconPlayerPlay className={HEADER_ICON_CLASS} />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-52">
            {scripts.map((script) => (
              <DropdownMenuItem
                key={script.id}
                onClick={() => handleRunScript(script.id)}
                className={MENU_ITEM_CLASS}
              >
                <IconTerminal2 className={MENU_ICON_CLASS} />
                <span className="truncate">{script.name}</span>
                <span className="ml-auto text-muted-foreground font-mono text-[10px] truncate max-w-[120px]">
                  {script.command}
                </span>
              </DropdownMenuItem>
            ))}
          </DropdownMenuContent>
        </DropdownMenu>
      )}
      {hasDevScript && (
        <Button
          size="sm"
          variant="ghost"
          className={HEADER_ACTION_BUTTON_CLASS}
          onClick={handleStartPreview}
          aria-label="Start dev server preview"
          title="Start dev server preview"
        >
          <IconDeviceDesktop className={HEADER_ICON_CLASS} />
        </Button>
      )}
    </>
  );
}
