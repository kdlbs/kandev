"use client";

import { useCallback } from "react";
import type { DockviewApi } from "dockview-react";
import { IconTerminal2, IconX } from "@tabler/icons-react";
import {
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import type { UserShellInfo } from "@/lib/state/slices";
import { destroyUserShell, resumeUserShell } from "@/lib/api/domains/user-shell-api";
import { useEnvironmentId } from "@/hooks/use-environment-session-id";
import { markTerminalPanelTerminateClose } from "./dockview-layout-setup";

const EMPTY_SHELLS: UserShellInfo[] = [];

/**
 * Lists ordinary user terminals (open and parked alike) inside the
 * dockview "+" menu so a user can jump to, re-open, or terminate a
 * terminal that isn't already a panel.
 *
 * - Already-mounted terminals are dimmed; clicking re-focuses the
 *   existing panel. Parked terminals are visually identical — the row
 *   doesn't advertise parked state, since clicking just brings the
 *   terminal back into view either way.
 * - The trailing × terminates (destroys) the terminal permanently —
 *   PTY killed, DB row deleted, no return after reload.
 */
export function TerminalReopenMenuItems({
  groupId,
  onNewTerminal,
}: {
  groupId: string;
  /**
   * Click handler for the leading "New Terminal" item rendered as the
   * first row under the section label. Omit to hide the row.
   */
  onNewTerminal?: () => void;
}) {
  const environmentId = useEnvironmentId();
  const taskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const shells = useAppStore((s) => {
    if (!environmentId) return EMPTY_SHELLS;
    return s.userShells.byEnvironmentId[environmentId] ?? EMPTY_SHELLS;
  });
  const updateUserShell = useAppStore((s) => s.updateUserShell);
  const removeUserShellStore = useAppStore((s) => s.removeUserShell);
  const api = useDockviewStore((s) => s.api);
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);
  const ordinary = shells.filter((s) => s.kind === "ordinary");
  const handleDestroyRow = useDestroyTerminalRow({
    api,
    environmentId,
    taskID,
    removeUserShellStore,
  });
  const handleClick = useOpenTerminalPanel({
    api,
    addTerminalPanel,
    environmentId,
    taskID,
    groupId,
    updateUserShell,
  });

  // Show the section header + the New Terminal row whenever onNewTerminal
  // is supplied, even if no ordinary terminals exist yet. This puts the
  // create action under the section label (matching the Agents pattern).
  if (ordinary.length === 0 && !onNewTerminal) return null;

  return (
    <>
      <DropdownMenuLabel className="text-xs text-muted-foreground">Terminals</DropdownMenuLabel>
      {onNewTerminal && (
        <DropdownMenuItem
          onClick={onNewTerminal}
          className="cursor-pointer text-xs gap-1.5"
          data-testid="new-terminal-button"
        >
          <IconTerminal2 className="h-3.5 w-3.5 shrink-0" />
          <span className="flex-1 truncate">New Terminal</span>
        </DropdownMenuItem>
      )}
      {ordinary
        .sort((a, b) => (a.seq ?? 0) - (b.seq ?? 0))
        .map((shell) => (
          <TerminalReopenRow
            key={shell.terminalId}
            shell={shell}
            isOpen={Boolean(api && findExistingTerminalPanel(api, shell.terminalId))}
            onClick={handleClick}
            onDestroy={handleDestroyRow}
          />
        ))}
      <DropdownMenuSeparator />
    </>
  );
}

type DestroyTerminalRowOptions = {
  api: DockviewApi | null;
  environmentId: string | null;
  taskID: string | null;
  removeUserShellStore: (environmentId: string, terminalId: string) => void;
};

function useDestroyTerminalRow({
  api,
  environmentId,
  taskID,
  removeUserShellStore,
}: DestroyTerminalRowOptions) {
  return useCallback(
    async (event: React.MouseEvent, shell: UserShellInfo) => {
      event.preventDefault();
      event.stopPropagation();
      if (!environmentId) return;
      const label = shell.seq != null ? `terminal #${shell.seq}` : "this terminal";
      if (!window.confirm(`Terminate ${label}? This kills the running PTY.`)) return;

      try {
        await destroyUserShell(environmentId, shell.terminalId, taskID ?? undefined);
      } catch (error) {
        console.error("terminate terminal from reopen menu:", error);
        return;
      }

      removeUserShellStore(environmentId, shell.terminalId);
      const existing = api ? findExistingTerminalPanel(api, shell.terminalId) : null;
      if (existing) {
        markTerminalPanelTerminateClose(existing.api.id);
        existing.api.close();
      }
    },
    [api, environmentId, taskID, removeUserShellStore],
  );
}

type OpenTerminalPanelOptions = {
  api: DockviewApi | null;
  addTerminalPanel: (
    terminalId?: string,
    groupId?: string,
    environmentId?: string,
    taskID?: string,
    title?: string,
  ) => void;
  environmentId: string | null;
  taskID: string | null;
  groupId: string;
  updateUserShell: (environmentId: string, terminalId: string, patch: { state: "open" }) => void;
};

function useOpenTerminalPanel({
  api,
  addTerminalPanel,
  environmentId,
  taskID,
  groupId,
  updateUserShell,
}: OpenTerminalPanelOptions) {
  return useCallback(
    async (terminalId: string, state: string | undefined, displayName: string | undefined) => {
      if (!api) return;
      const existing = findExistingTerminalPanel(api, terminalId);
      if (existing) {
        existing.api.setActive();
        return;
      }
      if (state === "parked" && environmentId) {
        try {
          await resumeUserShell(terminalId, taskID ?? undefined);
          updateUserShell(environmentId, terminalId, { state: "open" });
        } catch (error) {
          console.error("resume terminal:", error);
          return;
        }
      }
      addTerminalPanel(
        terminalId,
        groupId,
        environmentId ?? undefined,
        taskID ?? undefined,
        displayName,
      );
    },
    [api, addTerminalPanel, environmentId, taskID, updateUserShell, groupId],
  );
}

/**
 * Find a dockview panel that represents the given terminalId. Falls back
 * to matching panels whose `params.terminalId` equals the requested id,
 * so the default-migrated `terminal-default` panel (whose id never
 * changes but whose params.terminalId is a `shell-<uuid>`) resolves
 * correctly.
 */
function findExistingTerminalPanel(api: DockviewApi, terminalId: string) {
  const direct = api.getPanel(terminalId);
  if (direct) return direct;
  return (
    api.panels.find((p) => {
      const params = (p.params ?? null) as Record<string, unknown> | null;
      return typeof params?.terminalId === "string" && params.terminalId === terminalId;
    }) ?? null
  );
}

type ShellRow = UserShellInfo;

function TerminalReopenRow({
  shell,
  isOpen,
  onClick,
  onDestroy,
}: {
  shell: ShellRow;
  isOpen: boolean;
  onClick: (terminalId: string, state: string | undefined, label: string) => void;
  onDestroy: (event: React.MouseEvent, shell: ShellRow) => void;
}) {
  // Ordinary terminals don't need the backend "Terminal {seq}" suffix
  // in the label — the seq lives in the adjacent badge, so the row
  // reads "[N] Terminal" (or "[N] custom name") with no duplication.
  const label = shell.customName && shell.customName !== "" ? shell.customName : "Terminal";
  return (
    <DropdownMenuItem
      onClick={() => onClick(shell.terminalId, shell.state, label)}
      className={`cursor-pointer text-xs gap-1.5 ${isOpen ? "opacity-50" : ""}`}
      data-testid={`reopen-terminal-${shell.terminalId}`}
    >
      {shell.seq != null && (
        <span
          data-testid={`reopen-terminal-seq-${shell.seq}`}
          className="shrink-0 text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5"
        >
          {shell.seq}
        </span>
      )}
      <IconTerminal2 className="h-3.5 w-3.5 shrink-0" />
      <span className="flex-1 truncate">{label}</span>
      <button
        type="button"
        aria-label={`Terminate terminal #${shell.seq ?? ""}`}
        title="Terminate"
        className="shrink-0 ml-1 rounded p-0.5 text-muted-foreground hover:bg-destructive/15 hover:text-destructive cursor-pointer"
        data-testid="destroy-terminal-row"
        onClick={(e) => onDestroy(e, shell)}
      >
        <IconX className="h-3 w-3" />
      </button>
    </DropdownMenuItem>
  );
}
