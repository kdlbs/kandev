"use client";

import { useCallback } from "react";
import { IconTerminal2 } from "@tabler/icons-react";
import {
  DropdownMenuItem,
  DropdownMenuLabel,
  DropdownMenuSeparator,
} from "@kandev/ui/dropdown-menu";
import { useAppStore } from "@/components/state-provider";
import { useDockviewStore } from "@/lib/state/dockview-store";
import { resumeUserShell } from "@/lib/api/domains/user-shell-api";
import { useEnvironmentId } from "@/hooks/use-environment-session-id";
import { useUserShells } from "@/hooks/domains/session/use-user-shells";

/**
 * Lists ordinary user terminals (both open and parked) inside the dockview
 * "+" menu so a user can jump to or re-open a terminal that isn't already
 * a panel.
 *
 * - Open terminals that already have a dockview panel are dimmed and
 *   re-focus the existing panel on click.
 * - Parked terminals appear with a "parked" pill; clicking resumes them
 *   (sets state=open) and opens a new dockview panel for the PTY.
 * - Hidden when there are no managed terminals for the env.
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
  // useUserShells fetches the list into the Zustand store the first time
  // the menu mounts. On desktop dockview, no other code path triggers
  // this — the mobile/tablet right-panel hook would, but it never runs
  // here — so without this call the section stays empty until the user
  // creates a terminal manually. The fetch is idempotent per env+task
  // (see use-user-shells.ts), so it doesn't refetch on every render.
  const { shells } = useUserShells(environmentId, taskID);
  const updateUserShell = useAppStore((s) => s.updateUserShell);
  const api = useDockviewStore((s) => s.api);
  const addTerminalPanel = useDockviewStore((s) => s.addTerminalPanel);

  const ordinary = shells.filter((s) => s.kind === "ordinary");

  const handleClick = useCallback(
    async (terminalId: string, state: string | undefined, displayName: string | undefined) => {
      if (!api) return;
      const existing = api.getPanel(terminalId);
      if (existing) {
        existing.api.setActive();
        return;
      }
      // Parked → resume to bring the row back to "open" before adding the
      // dockview panel. The PTY survives parking, so this just toggles the
      // metadata flag and re-attaches the UI.
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
        .map((shell) => {
          const isOpen = Boolean(api?.getPanel(shell.terminalId));
          const isParked = shell.state === "parked";
          const label =
            shell.customName && shell.customName !== ""
              ? shell.customName
              : (shell.displayName ?? `Terminal ${shell.seq ?? ""}`);
          return (
            <DropdownMenuItem
              key={shell.terminalId}
              onClick={() => handleClick(shell.terminalId, shell.state, label)}
              className={`cursor-pointer text-xs gap-1.5 ${isOpen ? "opacity-50" : ""}`}
              data-testid={`reopen-terminal-${shell.terminalId}`}
            >
              {shell.seq != null && (
                <span className="w-5 shrink-0 text-muted-foreground text-right">#{shell.seq}</span>
              )}
              <IconTerminal2 className="h-3.5 w-3.5 shrink-0" />
              <span className="flex-1 truncate">{label}</span>
              {isParked && (
                <span className="shrink-0 text-[10px] font-mono px-1 py-0.5 rounded-sm bg-muted text-muted-foreground">
                  parked
                </span>
              )}
            </DropdownMenuItem>
          );
        })}
      <DropdownMenuSeparator />
    </>
  );
}
