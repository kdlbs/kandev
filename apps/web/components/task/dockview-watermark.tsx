"use client";

import { useCallback, useMemo, useState } from "react";
import type { IWatermarkPanelProps } from "dockview-react";
import { IconPlus, IconLayoutSidebarRightCollapse } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { useAppStore } from "@/components/state-provider";
import { useEnvironmentId } from "@/hooks/use-environment-session-id";
import { useUserShells } from "@/hooks/domains/session/use-user-shells";
import { useTaskPR } from "@/hooks/domains/github/use-task-pr";
import { useTaskMRs } from "@/hooks/domains/gitlab/use-task-mr";
import { createUserShell } from "@/lib/api/domains/user-shell-api";
import { AddPanelMenuItems } from "./dockview-add-panel-items";
import { NewSessionDialog } from "./new-session-dialog";
import { useActiveSessionDevScript } from "./repository-scripts-menu";

/**
 * Watermark rendered by Dockview when a group becomes empty (e.g. after
 * the user splits a group and the new half has no panels yet). Mirrors
 * the header "+" menu so the user gets the same rich picker — existing
 * sessions and terminals are listed (clicking re-focuses an open tab or
 * re-opens a parked one) instead of always minting a fresh panel.
 *
 * Before this, the watermark's terminal button called createUserShell
 * without a taskId, so it produced a legacy passthrough shell that
 * never appeared in the user-shell list at all.
 */
export function DockviewWatermark({ containerApi, group }: IWatermarkPanelProps) {
  const groupId = group?.id;
  const environmentId = useEnvironmentId();
  const taskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const sidebarGroupId = useDockviewStore((s) => s.sidebarGroupId);
  const [showNewSessionDialog, setShowNewSessionDialog] = useState(false);

  // Eagerly populate the user-shell store so the Terminals submenu has
  // its list ready the moment the watermark opens.
  useUserShells(environmentId, taskID);

  const menuState = useWatermarkMenuState(containerApi, taskID);
  const handlers = useWatermarkHandlers(groupId, environmentId, taskID);

  if (group?.id === sidebarGroupId) return null;
  if (!groupId) return null;

  return (
    <div className="flex h-full w-full flex-col items-center justify-center gap-2 text-muted-foreground">
      <IconLayoutSidebarRightCollapse className="h-5 w-5 opacity-50" />
      <p className="text-xs">Empty group</p>
      <DropdownMenu>
        <DropdownMenuTrigger asChild>
          <Button
            variant="outline"
            size="sm"
            className="cursor-pointer gap-1.5"
            data-testid="watermark-add-panel-btn"
          >
            <IconPlus className="h-3.5 w-3.5" />
            Add panel
          </Button>
        </DropdownMenuTrigger>
        <DropdownMenuContent align="center" className="w-44">
          <AddPanelMenuItems
            groupId={groupId}
            state={menuState}
            onNewSession={() => setShowNewSessionDialog(true)}
            onAddTerminal={handlers.handleAddTerminal}
            onRunScript={handlers.handleRunScript}
            onRunDevScript={handlers.handleRunDevScript}
          />
        </DropdownMenuContent>
      </DropdownMenu>
      {taskID && (
        <NewSessionDialog
          open={showNewSessionDialog}
          onOpenChange={setShowNewSessionDialog}
          taskId={taskID}
          groupId={groupId}
        />
      )}
    </div>
  );
}

function useWatermarkMenuState(
  containerApi: IWatermarkPanelProps["containerApi"],
  taskID: string | null,
) {
  const activeSessionId = useAppStore((s) => s.tasks.activeSessionId);
  const isPassthrough = useAppStore((s) => {
    if (!activeSessionId) return false;
    return s.taskSessions.items[activeSessionId]?.is_passthrough === true;
  });
  const { prs } = useTaskPR(taskID);
  const mrs = useTaskMRs(taskID);
  return useMemo(
    () => ({
      taskId: taskID,
      isPassthrough,
      hasChanges: Boolean(containerApi.getPanel("changes") ?? containerApi.getPanel("diff-files")),
      hasFiles: Boolean(containerApi.getPanel("files") ?? containerApi.getPanel("all-files")),
      prs,
      mrs,
    }),
    [taskID, isPassthrough, containerApi, prs, mrs],
  );
}

function useWatermarkHandlers(
  groupId: string | undefined,
  environmentId: string | null,
  taskID: string | null,
) {
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);
  const addUserShell = useAppStore((s) => s.addUserShell);
  const devScript = useActiveSessionDevScript();

  const handleAddTerminal = useCallback(async () => {
    if (!environmentId || !groupId) return;
    try {
      const result = await createUserShell(environmentId, { taskId: taskID ?? undefined });
      addUserShell(environmentId, {
        terminalId: result.terminalId,
        kind: result.kind,
        seq: result.seq,
        displayName: result.displayName,
        customName: null,
        state: result.state ?? "open",
        ptyStatus: result.ptyStatus ?? "stopped",
        running: result.ptyStatus === "running",
        label: result.label,
        closable: result.closable ?? true,
        initialCommand: result.initialCommand,
      });
      addTerminalPanel(result.terminalId, groupId, environmentId, taskID ?? undefined, "Terminal");
    } catch (error) {
      console.error("Failed to create terminal:", error);
    }
  }, [environmentId, taskID, groupId, addTerminalPanel, addUserShell]);

  const handleRunScript = useCallback(
    async (scriptId: string) => {
      if (!environmentId || !groupId) return;
      try {
        const result = await createUserShell(environmentId, { scriptId });
        addTerminalPanel(
          result.terminalId,
          groupId,
          environmentId,
          undefined,
          result.label ?? "Script",
        );
      } catch (error) {
        console.error("Failed to run script:", error);
      }
    },
    [environmentId, groupId, addTerminalPanel],
  );

  const handleRunDevScript = useCallback(async () => {
    if (!environmentId || !devScript || !groupId) return;
    try {
      const result = await createUserShell(environmentId, {
        command: devScript,
        label: "Dev Server",
      });
      addTerminalPanel(result.terminalId, groupId, environmentId, undefined, "Dev Server");
    } catch (error) {
      console.error("Failed to start dev script:", error);
    }
  }, [environmentId, devScript, groupId, addTerminalPanel]);

  return { handleAddTerminal, handleRunScript, handleRunDevScript };
}
