"use client";

import { useCallback, useRef, useState } from "react";
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
import { resumeUserShell } from "@/lib/api/domains/user-shell-api";
import { useTerminalDestroy } from "@/hooks/domains/session/use-terminal-destroy";
import { useEnvironmentId } from "@/hooks/use-environment-session-id";
import { markTerminalPanelTerminateClose } from "./dockview-layout-setup";
import { CloseTerminalConfirmPopover } from "./close-terminal-confirm-popover";
import { useTranslation } from "react-i18next";

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
  const { t } = useTranslation();
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
  const [pendingDestroyId, setPendingDestroyId] = useState<string | null>(null);
  const destroyAnchorRef = useRef<HTMLElement>(null);
  const destroyMenuRef = useRef<HTMLElement>(null);
  const pendingDestroy = ordinary.find((shell) => shell.terminalId === pendingDestroyId) ?? null;
  const destroyTerminal = useDestroyTerminalRow({
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
      <DropdownMenuLabel className="text-xs text-muted-foreground">
        {t("task:terminals")}
      </DropdownMenuLabel>
      {onNewTerminal && (
        <DropdownMenuItem
          onClick={onNewTerminal}
          className="cursor-pointer text-xs gap-1.5"
          data-testid="new-terminal-button"
        >
          <IconTerminal2 className="h-3.5 w-3.5 shrink-0" />
          <span className="flex-1 truncate">{t("task:newTerminal2")}</span>
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
            onAskDestroy={(anchor) => {
              destroyAnchorRef.current = anchor;
              destroyMenuRef.current = anchor.closest('[data-slot="dropdown-menu-content"]');
              setPendingDestroyId(shell.terminalId);
            }}
          />
        ))}
      <DropdownMenuSeparator />
      <CloseTerminalConfirmPopover
        open={pendingDestroy !== null}
        terminalName={terminalDisplayName(pendingDestroy, t("task:terminal"))}
        anchorRef={destroyAnchorRef}
        focusBoundaryRef={destroyMenuRef}
        onOpenChange={(open) => {
          if (!open) setPendingDestroyId(null);
        }}
        onConfirm={() => {
          if (pendingDestroy) void destroyTerminal(pendingDestroy.terminalId);
        }}
      />
    </>
  );
}

function terminalDisplayName(shell: UserShellInfo | null, fallback: string): string {
  return shell?.customName && shell.customName !== "" ? shell.customName : fallback;
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
  const handleDestroyed = useCallback(
    (terminalId: string) => {
      if (!environmentId) return;
      removeUserShellStore(environmentId, terminalId);
      const existing = api ? findExistingTerminalPanel(api, terminalId) : null;
      if (existing) {
        markTerminalPanelTerminateClose(existing.api.id);
        existing.api.close();
      }
    },
    [api, environmentId, removeUserShellStore],
  );
  return useTerminalDestroy({
    environmentId,
    taskId: taskID,
    onDestroyed: handleDestroyed,
  }).destroyTerminal;
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
  onAskDestroy,
}: {
  shell: ShellRow;
  isOpen: boolean;
  onClick: (terminalId: string, state: string | undefined, label: string) => void;
  onAskDestroy: (anchor: HTMLElement) => void;
}) {
  const { t } = useTranslation();
  // Ordinary terminals don't need the backend "Terminal {seq}" suffix
  // in the label — the seq lives in the adjacent badge, so the row
  // reads "[N] Terminal" (or "[N] custom name") with no duplication.
  const label = shell.customName && shell.customName !== "" ? shell.customName : t("task:terminal");
  return (
    <DropdownMenuItem
      onClick={() => onClick(shell.terminalId, shell.state, label)}
      className={`cursor-pointer gap-1.5 pr-10 text-xs [@media(pointer:coarse)]:min-h-11 [@media(pointer:coarse)]:pr-14 ${isOpen ? "opacity-50" : ""}`}
      data-testid={`reopen-terminal-${shell.terminalId}`}
    >
      {shell.seq != null && (
        <span
          data-testid={`reopen-terminal-seq-${shell.seq}`}
          className="shrink-0 text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5"
        >
          #{shell.seq}
        </span>
      )}
      <IconTerminal2 className="h-3.5 w-3.5 shrink-0" />
      <span className="flex-1 truncate">{label}</span>
      <button
        type="button"
        aria-label={t("task:terminateTerminal", { seq: shell.seq ?? "" })}
        title={t("task:terminate")}
        className="absolute inset-y-0 right-1 flex w-7 cursor-pointer items-center justify-center rounded text-muted-foreground transition-[color,background-color,transform] duration-100 hover:bg-destructive/15 hover:text-destructive active:scale-[0.96] [@media(pointer:coarse)]:w-11"
        data-testid="destroy-terminal-row"
        onClick={(event) => {
          event.preventDefault();
          event.stopPropagation();
          onAskDestroy(event.currentTarget);
        }}
      >
        <IconX className="h-3 w-3" />
      </button>
    </DropdownMenuItem>
  );
}
