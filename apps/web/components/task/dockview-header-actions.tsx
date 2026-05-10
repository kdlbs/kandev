"use client";

import { useCallback, useState } from "react";
import { type IDockviewHeaderActionsProps } from "dockview-react";
import {
  IconPlus,
  IconDeviceDesktop,
  IconTerminal2,
  IconPlayerPlay,
  IconLayoutSidebarRightCollapse,
} from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Tooltip, TooltipContent, TooltipTrigger } from "@kandev/ui/tooltip";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@kandev/ui/dropdown-menu";
import { useDockviewStore, performLayoutSwitch } from "@/lib/state/dockview-store";
import { useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useEnvironmentId } from "@/hooks/use-environment-session-id";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { startProcess } from "@/lib/api";
import { createUserShell } from "@/lib/api/domains/user-shell-api";
import { useRepositoryScripts } from "@/hooks/domains/workspace/use-repository-scripts";
import { replaceTaskUrl } from "@/lib/links";
import type { Task, ProcessInfo } from "@/lib/types/http";
import type { ProcessStatusEntry } from "@/lib/state/slices";
import { AddPanelMenuItems, MENU_ICON_CLASS, MENU_ITEM_CLASS } from "./dockview-add-panel-items";
import { NewSessionDialog } from "./new-session-dialog";
import { NewTaskDropdown } from "./new-task-dropdown";
import { useActiveSessionDevScript } from "./repository-scripts-menu";
import { GroupSplitCloseActionsView, useDockviewGroupWidth } from "./dockview-group-actions";

const HEADER_ACTION_BUTTON_CLASS =
  "h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring";
const RAW_HEADER_ACTION_BUTTON_CLASS =
  "inline-flex h-6 w-6 items-center justify-center rounded-[5px] text-muted-foreground transition-colors hover:bg-muted/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring cursor-pointer";
const HEADER_ICON_CLASS = "h-3.5 w-3.5";

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
  const { prs } = useTaskPR(taskId);
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
    prs,
    hasChanges,
    hasFiles,
  };
}

export function LeftHeaderActions(props: IDockviewHeaderActionsProps) {
  const { group, containerApi } = props;
  const state = useLeftHeaderState(group.id, containerApi);
  const environmentId = useEnvironmentId();
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);
  const devScript = useActiveSessionDevScript();
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);

  const handleAddTerminal = useCallback(async () => {
    if (!environmentId) return;
    try {
      const result = await createUserShell(environmentId);
      addTerminalPanel(result.terminalId, group.id, environmentId);
    } catch (error) {
      console.error("Failed to create terminal:", error);
    }
  }, [environmentId, addTerminalPanel, group.id]);

  const handleRunScript = useCallback(
    async (scriptId: string) => {
      if (!environmentId) return;
      try {
        const result = await createUserShell(environmentId, { scriptId });
        addTerminalPanel(result.terminalId, group.id, environmentId);
      } catch (error) {
        console.error("Failed to run script:", error);
      }
    },
    [environmentId, addTerminalPanel, group.id],
  );

  const handleRunDevScript = useCallback(async () => {
    if (!environmentId || !devScript) return;
    try {
      const result = await createUserShell(environmentId, {
        command: devScript,
        label: "Dev Server",
      });
      addTerminalPanel(result.terminalId, group.id, environmentId);
    } catch (error) {
      console.error("Failed to start dev script:", error);
    }
  }, [environmentId, devScript, addTerminalPanel, group.id]);

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
  const kanban = useAppStore((state) => state.kanban);
  // Use kanban.workflowId (task context) not workflows.activeId so "All Workflows" isn't clobbered when viewing a task.
  const workflowId = kanban.workflowId;
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
  const environmentId = useEnvironmentId();
  const repositoryId = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.repository_id ?? null;
  });
  const hasDevScript = Boolean(useActiveSessionDevScript());
  const { scripts } = useRepositoryScripts(repositoryId);
  const rightBottomGroupId = useDockviewStore((s) => s.rightBottomGroupId);

  if (scripts.length === 0 && !hasDevScript) return null;

  return (
    <>
      <TerminalScriptsDropdown
        scripts={scripts}
        environmentId={environmentId}
        rightBottomGroupId={rightBottomGroupId}
      />
      <TerminalDevPreviewButton
        environmentId={environmentId}
        rightBottomGroupId={rightBottomGroupId}
        visible={hasDevScript}
      />
    </>
  );
}

type TerminalScriptsDropdownProps = {
  scripts: ReturnType<typeof useRepositoryScripts>["scripts"];
  environmentId: string | null;
  rightBottomGroupId: string | null;
};

function TerminalScriptsDropdown({
  scripts,
  environmentId,
  rightBottomGroupId,
}: TerminalScriptsDropdownProps) {
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);

  const handleRunScript = useCallback(
    async (scriptId: string) => {
      if (!environmentId) return;
      try {
        const result = await createUserShell(environmentId, { scriptId });
        addTerminalPanel(result.terminalId, rightBottomGroupId ?? undefined, environmentId);
      } catch (error) {
        console.error("Failed to run script:", error);
      }
    },
    [environmentId, addTerminalPanel, rightBottomGroupId],
  );

  if (scripts.length === 0) return null;

  return (
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
  );
}

type TerminalDevPreviewButtonProps = {
  environmentId: string | null;
  rightBottomGroupId: string | null;
  visible: boolean;
};

function TerminalDevPreviewButton({
  environmentId,
  rightBottomGroupId,
  visible,
}: TerminalDevPreviewButtonProps) {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);
  const upsertProcessStatus = useAppStore((state) => state.upsertProcessStatus);
  const setActiveProcess = useAppStore((state) => state.setActiveProcess);

  const handleStartPreview = useCallback(async () => {
    if (!activeSessionId || !environmentId) return;
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
      const shell = await createUserShell(environmentId);
      addTerminalPanel(shell.terminalId, rightBottomGroupId ?? undefined, environmentId);
    } catch {
      // Terminal creation is best-effort
    }
  }, [
    activeSessionId,
    environmentId,
    addBrowserPanel,
    upsertProcessStatus,
    setActiveProcess,
    addTerminalPanel,
    rightBottomGroupId,
  ]);

  if (!visible) return null;

  return (
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
  );
}
