"use client";

import { useCallback } from "react";
import { type IDockviewHeaderActionsProps } from "dockview-react";
import {
  IconPlus,
  IconDeviceDesktop,
  IconTerminal2,
  IconFileText,
  IconFolder,
  IconGitBranch,
  IconPlayerPlay,
  IconLayoutSidebarRightCollapse,
  IconLayoutColumns,
  IconLayoutRows,
  IconX,
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
import { startProcess } from "@/lib/api";
import { createUserShell } from "@/lib/api/domains/user-shell-api";
import { useRepositoryScripts } from "@/hooks/domains/workspace/use-repository-scripts";
import { linkToSession } from "@/lib/links";
import type { Task, ProcessInfo } from "@/lib/types/http";
import type { ProcessStatusEntry } from "@/lib/state/slices";
import { NewTaskButton } from "./task-session-sidebar";

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

export function LeftHeaderActions(props: IDockviewHeaderActionsProps) {
  const { group, containerApi } = props;
  const sidebarGroupId = useDockviewStore((s) => s.sidebarGroupId);

  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const isPassthrough = useAppStore((state) => {
    if (!activeSessionId) return false;
    return state.taskSessions.items[activeSessionId]?.is_passthrough === true;
  });

  const addBrowserPanel = useDockviewStore((s) => s.addBrowserPanel);
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);
  const addPlanPanel = useDockviewStore((s) => s.addPlanPanel);
  const addFilesPanel = useDockviewStore((s) => s.addFilesPanel);
  const addChangesPanel = useDockviewStore((s) => s.addChangesPanel);

  const hasChanges = Boolean(
    containerApi.getPanel("changes") ?? containerApi.getPanel("diff-files"),
  );
  const hasFiles = Boolean(containerApi.getPanel("files") ?? containerApi.getPanel("all-files"));

  const isSidebarGroup = group.id === sidebarGroupId;

  const handleAddTerminal = useCallback(async () => {
    if (!activeSessionId) return;
    try {
      const result = await createUserShell(activeSessionId);
      addTerminalPanel(result.terminalId, group.id);
    } catch (error) {
      console.error("Failed to create terminal:", error);
    }
  }, [activeSessionId, addTerminalPanel, group.id]);

  if (isSidebarGroup) return null;

  return (
    <div className="flex items-center gap-1 pl-1">
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button size="sm" variant="ghost" className="h-6 w-6 p-0 cursor-pointer">
            <IconPlus className="h-3.5 w-3.5" />
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="start" className="w-44">
          <DropdownMenuItem onClick={handleAddTerminal} className="cursor-pointer text-xs">
            <IconTerminal2 className="h-3.5 w-3.5 mr-1.5" />
            Terminal
          </DropdownMenuItem>
          <DropdownMenuItem
            onClick={() => addBrowserPanel(undefined, group.id)}
            className="cursor-pointer text-xs"
          >
            <IconDeviceDesktop className="h-3.5 w-3.5 mr-1.5" />
            Browser
          </DropdownMenuItem>
          {!isPassthrough && (
            <DropdownMenuItem
              onClick={() => addPlanPanel(group.id)}
              className="cursor-pointer text-xs"
            >
              <IconFileText className="h-3.5 w-3.5 mr-1.5" />
              Plan
            </DropdownMenuItem>
          )}
          {!hasChanges && (
            <DropdownMenuItem
              onClick={() => addChangesPanel(group.id)}
              className="cursor-pointer text-xs"
            >
              <IconGitBranch className="h-3.5 w-3.5 mr-1.5" />
              Changes
            </DropdownMenuItem>
          )}
          {!hasFiles && (
            <DropdownMenuItem
              onClick={() => addFilesPanel(group.id)}
              className="cursor-pointer text-xs"
            >
              <IconFolder className="h-3.5 w-3.5 mr-1.5" />
              Files
            </DropdownMenuItem>
          )}
        </DropdownMenuContent>
      </DropdownMenu>
    </div>
  );
}

/** Faded split-vertical, split-horizontal, and close buttons for any non-sidebar group. */
function GroupSplitCloseActions({ group, containerApi }: IDockviewHeaderActionsProps) {
  const centerGroupId = useDockviewStore((s) => s.centerGroupId);
  const isChatGroup = group.id === centerGroupId;

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
    <>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="h-6 w-6 inline-flex items-center justify-center text-muted-foreground/50 hover:text-foreground transition-colors cursor-pointer"
            onClick={handleSplitRight}
          >
            <IconLayoutColumns className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Split right</TooltipContent>
      </Tooltip>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            type="button"
            className="h-6 w-6 inline-flex items-center justify-center text-muted-foreground/50 hover:text-foreground transition-colors cursor-pointer"
            onClick={handleSplitDown}
          >
            <IconLayoutRows className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent>Split down</TooltipContent>
      </Tooltip>
      {!isChatGroup && (
        <Tooltip>
          <TooltipTrigger asChild>
            <button
              type="button"
              className="h-6 w-6 inline-flex items-center justify-center text-muted-foreground/50 hover:text-foreground transition-colors cursor-pointer"
              onClick={handleCloseGroup}
            >
              <IconX className="h-3.5 w-3.5" />
            </button>
          </TooltipTrigger>
          <TooltipContent>Close group</TooltipContent>
        </Tooltip>
      )}
    </>
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
    <div className="flex items-center gap-0.5 pr-1">
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
      const oldSessionId = appStore.getState().tasks.activeSessionId;
      setActiveTask(task.id);
      if (meta?.taskSessionId) {
        setActiveSession(task.id, meta.taskSessionId);
        performLayoutSwitch(oldSessionId, meta.taskSessionId);
        window.history.replaceState({}, "", linkToSession(meta.taskSessionId));
      }
    },
    [setActiveTask, setActiveSession, appStore],
  );

  return (
    <div className="flex items-center gap-1 pr-2">
      <NewTaskButton
        workspaceId={workspaceId}
        workflowId={workflowId}
        steps={steps}
        onSuccess={handleTaskCreated}
      />
    </div>
  );
}

function RightTopGroupActions() {
  const toggleRightPanels = useDockviewStore((s) => s.toggleRightPanels);
  return (
    <Tooltip>
      <TooltipTrigger asChild>
        <Button
          size="sm"
          variant="ghost"
          className="h-6 w-6 p-0 cursor-pointer"
          onClick={toggleRightPanels}
        >
          <IconLayoutSidebarRightCollapse className="h-3.5 w-3.5" />
        </Button>
      </TooltipTrigger>
      <TooltipContent>Hide right panels</TooltipContent>
    </Tooltip>
  );
}

function SessionModeBadge() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const modeId = useAppStore((state) => {
    if (!activeSessionId) return null;
    return state.sessionMode.bySessionId[activeSessionId] ?? null;
  });

  if (!modeId) return null;

  return (
    <span className="text-[10px] font-medium text-muted-foreground bg-muted/60 px-1.5 py-0.5 rounded">
      {modeId}
    </span>
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
      <SessionModeBadge />
      {hasDevScript && (
        <Button
          size="sm"
          variant="ghost"
          className="h-6 w-6 p-0 cursor-pointer"
          onClick={handleStartBrowser}
          title="Open browser preview"
        >
          <IconDeviceDesktop className="h-3.5 w-3.5" />
        </Button>
      )}
    </div>
  );
}

function useHasActiveSessionDevScript() {
  return useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return false;
    const repoId = state.taskSessions.items[sessionId]?.repository_id;
    if (!repoId) return false;
    const allRepos = Object.values(state.repositories.itemsByWorkspaceId).flat();
    const repo = allRepos.find((r) => r.id === repoId);
    return Boolean(repo?.dev_script?.trim());
  });
}

function TerminalGroupRightActions() {
  const activeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const repositoryId = useAppStore((state) => {
    const sessionId = state.tasks.activeSessionId;
    if (!sessionId) return null;
    return state.taskSessions.items[sessionId]?.repository_id ?? null;
  });
  const hasDevScript = useHasActiveSessionDevScript();

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
        const result = await createUserShell(activeSessionId, scriptId);
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
              className="h-6 w-6 p-0 cursor-pointer"
              title="Run script"
            >
              <IconPlayerPlay className="h-3.5 w-3.5" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end" className="w-52">
            {scripts.map((script) => (
              <DropdownMenuItem
                key={script.id}
                onClick={() => handleRunScript(script.id)}
                className="cursor-pointer text-xs"
              >
                <IconTerminal2 className="h-3.5 w-3.5 mr-1.5 shrink-0" />
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
          className="h-6 w-6 p-0 cursor-pointer"
          onClick={handleStartPreview}
          title="Start dev server preview"
        >
          <IconDeviceDesktop className="h-3.5 w-3.5" />
        </Button>
      )}
    </>
  );
}
