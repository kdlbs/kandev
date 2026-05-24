"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { useAppStore } from "@/components/state-provider";
import { destroyUserShell, renameUserShell } from "@/lib/api/domains/user-shell-api";
import { markTerminalPanelTerminateClose } from "./dockview-layout-setup";

/**
 * Custom dockview tab for terminal panels.
 *
 * Mirrors the session-tab badge behaviour: the `#N` pill only renders when
 * there's more than one ordinary terminal in the active task — a single
 * terminal needs no disambiguation.
 *
 * The tab exposes a right-click context menu with Rename / Terminate;
 * choosing Rename swaps the title in place for an editable input.
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

function useTerminalTabState(stampedEnv: string | undefined, terminalId: string, apiTitle: string) {
  const shell = useAppStore((s) => {
    if (!stampedEnv) return null;
    const list = s.userShells.byEnvironmentId[stampedEnv] ?? [];
    return list.find((it) => it.terminalId === terminalId) ?? null;
  });
  const ordinaryCount = useAppStore((s) => {
    if (!stampedEnv) return 0;
    const list = s.userShells.byEnvironmentId[stampedEnv] ?? [];
    return list.filter((it) => it.kind === "ordinary").length;
  });
  const isOrdinary = shell?.kind === "ordinary";
  const seq = shell?.seq;
  const showBadge = isOrdinary && ordinaryCount > 1 && typeof seq === "number";
  const displayName = pickDisplayName(shell, apiTitle);
  return { shell, isOrdinary, seq, showBadge, displayName };
}

export function TerminalTab(props: IDockviewPanelHeaderProps) {
  const { terminalId, taskID: stampedTaskID, environmentId: stampedEnv } = extractParams(props);
  const activeTaskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const taskID = stampedTaskID ?? activeTaskID ?? null;
  const { shell, isOrdinary, seq, showBadge, displayName } = useTerminalTabState(
    stampedEnv,
    terminalId,
    props.api.title ?? "Terminal",
  );

  // DockviewDefaultTab reads the title from `api.title` directly and
  // ignores prop overrides — push the corrected title onto the api so
  // the default-tab body re-renders the right text.
  useEffect(() => {
    if (props.api.title !== displayName) props.api.setTitle(displayName);
  }, [props.api, displayName]);

  const [isRenaming, setIsRenaming] = useState(false);
  const handleCommitRename = useRenameCommitter({
    isOrdinary,
    stampedEnv,
    terminalId,
    taskID,
    currentCustomName: shell?.customName ?? null,
    onDone: () => setIsRenaming(false),
  });

  const renameInitial =
    shell?.customName && shell.customName !== "" ? shell.customName : displayName;
  const seqBadgeForInput = showBadge && typeof seq === "number" ? seq : null;

  return (
    <ContextMenu>
      <ContextMenuTrigger
        className="flex h-full items-center cursor-pointer select-none"
        data-testid={`terminal-tab-${terminalId}`}
      >
        {isRenaming ? (
          <TerminalTabRenameInput
            initial={renameInitial}
            seqBadge={seqBadgeForInput}
            onCommit={handleCommitRename}
            onCancel={() => setIsRenaming(false)}
          />
        ) : (
          <TerminalTabBody {...props} showBadge={showBadge} seq={seq} displayName={displayName} />
        )}
      </ContextMenuTrigger>
      <TerminalTabMenu
        terminalId={terminalId}
        taskID={taskID}
        environmentId={stampedEnv ?? null}
        canMutate={isOrdinary}
        onStartRename={() => setIsRenaming(true)}
        onClosePanel={() => props.api.close()}
        onTerminatePanel={() => {
          markTerminalPanelTerminateClose(props.api.id);
          props.api.close();
        }}
      />
    </ContextMenu>
  );
}

function useRenameCommitter({
  isOrdinary,
  stampedEnv,
  terminalId,
  taskID,
  currentCustomName,
  onDone,
}: {
  isOrdinary: boolean;
  stampedEnv: string | undefined;
  terminalId: string;
  taskID: string | null;
  currentCustomName: string | null;
  onDone: () => void;
}) {
  const updateUserShell = useAppStore((s) => s.updateUserShell);
  return useCallback(
    async (next: string) => {
      onDone();
      if (!isOrdinary || !stampedEnv) return;
      const normalized = next.trim() === "" ? null : next.trim();
      if (currentCustomName === normalized) return;
      try {
        await renameUserShell(terminalId, normalized, taskID ?? undefined);
        updateUserShell(stampedEnv, terminalId, { customName: normalized });
      } catch (error) {
        console.error("rename terminal:", error);
      }
    },
    [isOrdinary, stampedEnv, terminalId, taskID, currentCustomName, updateUserShell, onDone],
  );
}

function TerminalTabBody({
  showBadge,
  seq,
  // `displayName` is computed in the parent but consumed via the
  // api.setTitle effect — drop it here so it doesn't leak into the
  // DOM via the {...props} spread below (React warning otherwise).
  displayName: _displayName,
  ...props
}: IDockviewPanelHeaderProps & {
  showBadge: boolean;
  seq: number | undefined;
  displayName: string;
}) {
  void _displayName;
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

function TerminalTabRenameInput({
  initial,
  seqBadge,
  onCommit,
  onCancel,
}: {
  initial: string;
  seqBadge: number | null;
  onCommit: (next: string) => void;
  onCancel: () => void;
}) {
  const [value, setValue] = useState(initial);
  const inputRef = useRef<HTMLInputElement>(null);

  useEffect(() => {
    inputRef.current?.focus();
    inputRef.current?.select();
  }, []);

  return (
    <div
      className="flex h-full items-center gap-1 px-2"
      // Stop the click from selecting the tab while we type.
      onClick={(e) => e.stopPropagation()}
      onMouseDown={(e) => e.stopPropagation()}
    >
      {seqBadge != null && (
        <span className="text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5">
          {seqBadge}
        </span>
      )}
      <input
        ref={inputRef}
        value={value}
        onChange={(e) => setValue(e.target.value)}
        onKeyDown={(e) => {
          if (e.key === "Enter") {
            e.preventDefault();
            onCommit(value);
          } else if (e.key === "Escape") {
            e.preventDefault();
            onCancel();
          }
          // Don't let dockview see typed keys as shortcuts.
          e.stopPropagation();
        }}
        onBlur={() => onCommit(value)}
        data-testid="terminal-tab-rename-input"
        className="h-5 min-w-[6rem] max-w-[14rem] rounded border border-input bg-background px-1 text-xs outline-none focus:ring-1 focus:ring-ring"
      />
    </div>
  );
}

function TerminalTabMenu({
  terminalId,
  taskID,
  environmentId,
  canMutate,
  onStartRename,
  onClosePanel,
  onTerminatePanel,
}: {
  terminalId: string;
  taskID: string | null;
  environmentId: string | null;
  canMutate: boolean;
  onStartRename: () => void;
  onClosePanel: () => void;
  onTerminatePanel: () => void;
}) {
  const removeUserShellStore = useAppStore((s) => s.removeUserShell);

  const handleTerminate = useCallback(async () => {
    if (!environmentId) return;
    if (!canMutate) {
      onClosePanel();
      return;
    }
    try {
      await destroyUserShell(environmentId, terminalId, taskID ?? undefined);
      removeUserShellStore(environmentId, terminalId);
      onTerminatePanel();
    } catch (error) {
      console.error("terminate terminal:", error);
    }
  }, [
    canMutate,
    environmentId,
    terminalId,
    taskID,
    removeUserShellStore,
    onClosePanel,
    onTerminatePanel,
  ]);

  return (
    <ContextMenuContent>
      {canMutate && (
        <>
          <ContextMenuItem onClick={onStartRename}>Rename…</ContextMenuItem>
          <ContextMenuSeparator />
        </>
      )}
      <ContextMenuItem
        onClick={handleTerminate}
        className="text-destructive focus:text-destructive"
      >
        Terminate
      </ContextMenuItem>
    </ContextMenuContent>
  );
}
