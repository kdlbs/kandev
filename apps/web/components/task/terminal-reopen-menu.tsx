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
import { destroyUserShell, resumeUserShell } from "@/lib/api/domains/user-shell-api";
import { useEnvironmentId } from "@/hooks/use-environment-session-id";
import { useUserShells } from "@/hooks/domains/session/use-user-shells";

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
  const { shells } = useUserShells(environmentId, taskID);
  const updateUserShell = useAppStore((s) => s.updateUserShell);
  const removeUserShellStore = useAppStore((s) => s.removeUserShell);
  const api = useDockviewStore((s) => s.api);
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);

  const handleDestroyRow = useCallback(
    async (event: React.MouseEvent, terminalId: string, seq: number | undefined) => {
      // Stop the parent DropdownMenuItem from firing its "reopen" onClick
      // and from closing the menu. preventDefault also blocks Radix's
      // default item-select behavior.
      event.preventDefault();
      event.stopPropagation();
      if (!environmentId) return;
      const label = seq != null ? `terminal #${seq}` : "this terminal";
      if (!window.confirm(`Terminate ${label}? This kills the running PTY.`)) return;
      try {
        await destroyUserShell(environmentId, terminalId, taskID ?? undefined);
        removeUserShellStore(environmentId, terminalId);
      } catch (error) {
        console.error("terminate terminal from reopen menu:", error);
      }
    },
    [environmentId, taskID, removeUserShellStore],
  );

  const ordinary = shells.filter((s) => s.kind === "ordinary");

  const handleClick = useCallback(
    async (terminalId: string, state: string | undefined, displayName: string | undefined) => {
      if (!api) return;
      // The default panel keeps its registry id `terminal-default` even
      // after migration to a `shell-<uuid>` PTY, so a direct id lookup
      // misses it. Fall back to scanning `params.terminalId` so we focus
      // the existing tab instead of duplicating it.
      const existing = findExistingTerminalPanel(api, terminalId);
      if (existing) {
        existing.api.setActive();
        return;
      }
      // Parked → resume to bring the row back to "open" before mounting
      // the dockview panel. The PTY survives parking, so this just
      // toggles the metadata flag and re-attaches the UI.
      if (state === "parked" && environmentId) {
        try {
          await resumeUserShell(terminalId, taskID ?? undefined);
          updateUserShell(environmentId, terminalId, { state: "open" });
        } catch (error) {
          console.error("resume terminal:", error);
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
          <span className="w-5 shrink-0" aria-hidden="true" />
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

type ShellRow = {
  terminalId: string;
  seq?: number;
  customName?: string | null;
  displayName?: string;
  state?: string;
};

function TerminalReopenRow({
  shell,
  isOpen,
  onClick,
  onDestroy,
}: {
  shell: ShellRow;
  isOpen: boolean;
  onClick: (terminalId: string, state: string | undefined, label: string) => void;
  onDestroy: (event: React.MouseEvent, terminalId: string, seq: number | undefined) => void;
}) {
  const label =
    shell.customName && shell.customName !== ""
      ? shell.customName
      : (shell.displayName ?? `Terminal ${shell.seq ?? ""}`);
  return (
    <DropdownMenuItem
      onClick={() => onClick(shell.terminalId, shell.state, label)}
      className={`cursor-pointer text-xs gap-1.5 ${isOpen ? "opacity-50" : ""}`}
      data-testid={`reopen-terminal-${shell.terminalId}`}
    >
      {shell.seq != null && (
        <span className="w-5 shrink-0 text-muted-foreground text-right">#{shell.seq}</span>
      )}
      <IconTerminal2 className="h-3.5 w-3.5 shrink-0" />
      <span className="flex-1 truncate">{label}</span>
      <button
        type="button"
        aria-label={`Terminate terminal #${shell.seq ?? ""}`}
        title="Terminate"
        className="shrink-0 ml-1 rounded p-0.5 text-muted-foreground hover:bg-destructive/15 hover:text-destructive cursor-pointer"
        data-testid="destroy-terminal-row"
        onClick={(e) => onDestroy(e, shell.terminalId, shell.seq)}
      >
        <IconX className="h-3 w-3" />
      </button>
    </DropdownMenuItem>
  );
}
