"use client";

import { useCallback, useState } from "react";
import { type IDockviewHeaderActionsProps } from "dockview-react";
import {
  IconPlus,
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
import { createUserShell } from "@/lib/api/domains/user-shell-api";
import { useRepositoryScripts } from "@/hooks/domains/workspace/use-repository-scripts";
import { replaceTaskUrl } from "@/lib/links";
import type { Task } from "@/lib/types/http";
import { AddPanelMenuItems, MENU_ITEM_CLASS } from "./dockview-add-panel-items";
import { NewSessionDialog } from "./new-session-dialog";
import { NewTaskDropdown } from "./new-task-dropdown";
import { useActiveSessionDevScript } from "./repository-scripts-menu";
import { GroupSplitCloseActionsView, useDockviewGroupWidth } from "./dockview-group-actions";

const HEADER_ACTION_BUTTON_CLASS =
  "h-6 w-6 p-0 cursor-pointer text-muted-foreground hover:bg-muted/70 hover:text-foreground focus-visible:ring-1 focus-visible:ring-ring";
const RAW_HEADER_ACTION_BUTTON_CLASS =
  "inline-flex h-6 w-6 items-center justify-center rounded-[5px] text-muted-foreground transition-colors hover:bg-muted/70 hover:text-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring cursor-pointer";
const HEADER_ICON_CLASS = "h-3.5 w-3.5";

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
  const sidebarGroupId = useDockviewStore((s) => s.sidebarGroupId);
  const rightTopGroupId = useDockviewStore((s) => s.rightTopGroupId);
  const rightBottomGroupId = useDockviewStore((s) => s.rightBottomGroupId);

  const isSidebarGroup = group.id === sidebarGroupId;
  if (isSidebarGroup) return <SidebarRightActions />;

  const isRightTopGroup = group.id === rightTopGroupId;
  const isTerminalGroup = group.id === rightBottomGroupId;

  return (
    <div className="flex items-center gap-0.5 pr-0.5">
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

function TerminalGroupRightActions() {
  const environmentId = useEnvironmentId();
  const repositoryId = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.repository_id ?? null;
  });
  const { scripts } = useRepositoryScripts(repositoryId);
  const rightBottomGroupId = useDockviewStore((s) => s.rightBottomGroupId);

  if (scripts.length === 0) return null;

  return (
    <TerminalScriptsDropdown
      scripts={scripts}
      environmentId={environmentId}
      rightBottomGroupId={rightBottomGroupId}
    />
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
      <DropdownMenuContent align="end" className="w-[min(18rem,calc(100vw-1rem))]">
        {scripts.map((script) => (
          <DropdownMenuItem
            key={script.id}
            onClick={() => handleRunScript(script.id)}
            className={`${MENU_ITEM_CLASS} grid grid-cols-[auto_minmax(0,1fr)] items-start gap-x-2 gap-y-0 py-1.5`}
          >
            <IconTerminal2 className="mt-0.5 h-3.5 w-3.5 shrink-0" />
            <span className="min-w-0 overflow-hidden">
              <span className="block truncate leading-4" title={script.name}>
                {script.name}
              </span>
              <span
                className="mt-0.5 block truncate font-mono text-[10px] leading-3 text-muted-foreground"
                title={script.command}
              >
                {script.command}
              </span>
            </span>
          </DropdownMenuItem>
        ))}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}
