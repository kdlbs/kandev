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
import type { UserShellInfo } from "@/lib/state/slices/session-runtime/types";

const EMPTY_SHELLS: UserShellInfo[] = [];

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
export function TerminalReopenMenuItems({ groupId }: { groupId: string }) {
  const environmentId = useEnvironmentId();
  const taskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const shells = useAppStore((s) => {
    if (!environmentId) return EMPTY_SHELLS;
    // Return the stored array directly so Zustand's referential equality
    // check short-circuits — returning `?? []` on every call would create
    // a new array reference per render and cause an infinite-render loop.
    return s.userShells.byEnvironmentId[environmentId] ?? EMPTY_SHELLS;
  });
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

  if (ordinary.length === 0) return null;

  return (
    <>
      <DropdownMenuLabel className="text-xs text-muted-foreground">Terminals</DropdownMenuLabel>
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
