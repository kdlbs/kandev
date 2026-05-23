"use client";

import { useCallback, useEffect } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { useAppStore } from "@/components/state-provider";
import { destroyUserShell, parkUserShell, renameUserShell } from "@/lib/api/domains/user-shell-api";

/**
 * Custom dockview tab for terminal panels.
 *
 * Mirrors the session-tab badge behaviour: the `#N` pill only renders when
 * there's more than one ordinary terminal in the active task — a single
 * terminal needs no disambiguation.
 *
 * The tab also exposes a context menu (right-click) for rename / park /
 * destroy on ordinary terminals, since the dockview default close button
 * is the only direct affordance dockview provides.
 */
type StampedParams = {
  terminalId: string;
  taskID: string | undefined;
  environmentId: string | undefined;
};

function extractParams(props: IDockviewPanelHeaderProps): StampedParams {
  const panelParams = (props.params ?? {}) as Record<string, unknown>;
  return {
    terminalId: (panelParams.terminalId as string | undefined) ?? props.api.id,
    taskID: panelParams.taskID as string | undefined,
    environmentId: panelParams.environmentId as string | undefined,
  };
}

/**
 * Tab title text — intentionally drops the backend's "Terminal {seq}"
 * suffix so the title reads "Terminal" and the seq lives only in the
 * sibling badge (mirroring session-tab's pattern where the agent name is
 * the title and the seq is a separate pill before it).
 *
 * Custom names override the default; legacy passthrough shells keep
 * their server-supplied label (e.g. "Script", "Dev Server").
 */
function pickDisplayName(
  shell: { kind?: string; customName?: string | null; label?: string } | null,
  fallback: string,
): string {
  if (shell?.customName && shell.customName !== "") return shell.customName;
  if (shell?.kind === "ordinary") return "Terminal";
  if (shell?.label) return shell.label;
  return fallback;
}

export function TerminalTab(props: IDockviewPanelHeaderProps) {
  const { terminalId, taskID: stampedTaskID, environmentId: stampedEnv } = extractParams(props);

  const activeTaskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const taskID = stampedTaskID ?? activeTaskID ?? null;

  // Pull this terminal's metadata from the store (shells are env-scoped).
  const shell = useAppStore((s) => {
    if (!stampedEnv) return null;
    const list = s.userShells.byEnvironmentId[stampedEnv] ?? [];
    return list.find((it) => it.terminalId === terminalId) ?? null;
  });

  // Count ordinary terminals for the badge "only when >1" rule.
  const ordinaryCount = useAppStore((s) => {
    if (!stampedEnv) return 0;
    const list = s.userShells.byEnvironmentId[stampedEnv] ?? [];
    return list.filter((it) => it.kind === "ordinary").length;
  });

  const isOrdinary = shell?.kind === "ordinary";
  const seq = shell?.seq;
  const showBadge = isOrdinary && ordinaryCount > 1 && typeof seq === "number";
  const displayName = pickDisplayName(shell, props.api.title ?? "Terminal");

  // DockviewDefaultTab reads the title directly from `api.title` and
  // ignores any prop overrides. So a panel created with title="Terminal 2"
  // keeps rendering "Terminal 2" even after we recompute displayName to
  // just "Terminal". Push the corrected title onto the api so the
  // default-tab body re-renders the right text. Mirrors session-tab's
  // api.setTitle(agentLabel) pattern.
  useEffect(() => {
    if (props.api.title !== displayName) props.api.setTitle(displayName);
  }, [props.api, displayName]);

  return (
    <ContextMenu>
      <ContextMenuTrigger
        className="flex h-full items-center cursor-pointer select-none"
        data-testid={`terminal-tab-${terminalId}`}
      >
        <TerminalTabBody {...props} showBadge={showBadge} seq={seq} displayName={displayName} />
      </ContextMenuTrigger>
      <TerminalTabMenu
        terminalId={terminalId}
        taskID={taskID}
        environmentId={stampedEnv ?? null}
        canMutate={isOrdinary}
        currentName={shell?.customName ?? null}
        defaultName={shell?.displayName ?? `Terminal ${seq ?? ""}`}
      />
    </ContextMenu>
  );
}

function TerminalTabBody({
  showBadge,
  seq,
  ...props
}: IDockviewPanelHeaderProps & {
  showBadge: boolean;
  seq: number | undefined;
  /** Unused after the api.setTitle migration; kept so callers don't break. */
  displayName: string;
}) {
  return (
    <div className="flex h-full items-center">
      {showBadge && (
        <span
          data-testid={`terminal-tab-seq-${seq}`}
          className="ml-2 text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5"
        >
          {seq}
        </span>
      )}
      <DockviewDefaultTab {...props} />
    </div>
  );
}

function TerminalTabMenu({
  terminalId,
  taskID,
  environmentId,
  canMutate,
  currentName,
  defaultName,
}: {
  terminalId: string;
  taskID: string | null;
  environmentId: string | null;
  canMutate: boolean;
  currentName: string | null;
  defaultName: string;
}) {
  const updateUserShell = useAppStore((s) => s.updateUserShell);
  const removeUserShellStore = useAppStore((s) => s.removeUserShell);

  const handleRename = useCallback(async () => {
    if (!canMutate) return;
    const current = currentName ?? defaultName;
    const next = window.prompt("Rename terminal (leave empty to reset to default)", current);
    if (next === null) return;
    const trimmed = next.trim();
    const normalized = trimmed === "" ? null : trimmed;
    try {
      await renameUserShell(terminalId, normalized, taskID ?? undefined);
      if (environmentId) updateUserShell(environmentId, terminalId, { customName: normalized });
    } catch (error) {
      console.error("rename terminal:", error);
    }
  }, [canMutate, currentName, defaultName, terminalId, taskID, environmentId, updateUserShell]);

  const handlePark = useCallback(async () => {
    if (!canMutate) return;
    try {
      await parkUserShell(terminalId, taskID ?? undefined);
      if (environmentId) updateUserShell(environmentId, terminalId, { state: "parked" });
    } catch (error) {
      console.error("park terminal:", error);
    }
  }, [canMutate, terminalId, taskID, environmentId, updateUserShell]);

  const handleDestroy = useCallback(async () => {
    if (!environmentId) return;
    try {
      await destroyUserShell(environmentId, terminalId, taskID ?? undefined);
      removeUserShellStore(environmentId, terminalId);
    } catch (error) {
      console.error("destroy terminal:", error);
    }
  }, [environmentId, terminalId, taskID, removeUserShellStore]);

  return (
    <ContextMenuContent>
      {canMutate && (
        <>
          <ContextMenuItem onClick={handleRename}>Rename…</ContextMenuItem>
          <ContextMenuItem onClick={handlePark}>Park (hide, keep PTY)</ContextMenuItem>
          <ContextMenuSeparator />
        </>
      )}
      <ContextMenuItem onClick={handleDestroy} className="text-destructive focus:text-destructive">
        Destroy
      </ContextMenuItem>
    </ContextMenuContent>
  );
}
