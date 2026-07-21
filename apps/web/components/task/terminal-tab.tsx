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
import { shouldConfirmTerminalClose } from "@/lib/terminal/terminal-busy-registry";
import { CloseTerminalConfirmDialog } from "./close-terminal-confirm-dialog";
import { TabRenameInput } from "./tab-rename-input";
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
  const closable = shell?.closable ?? true;
  return { shell, isOrdinary, seq, showBadge, displayName, closable };
}

function useTerminalTabClose({
  terminalId,
  taskID,
  stampedEnv,
  shell,
  closable,
  panelId,
  closePanel,
}: {
  terminalId: string;
  taskID: string | null;
  stampedEnv: string | undefined;
  shell: { kind?: string } | null;
  closable: boolean;
  panelId: string;
  closePanel: () => void;
}) {
  const removeUserShellStore = useAppStore((s) => s.removeUserShell);
  const [confirmClose, setConfirmClose] = useState(false);
  const [isClosing, setIsClosing] = useState(false);

  const destroyAndClosePanel = useCallback(async () => {
    if (isClosing) return;
    if (!stampedEnv) {
      closePanel();
      return;
    }
    setIsClosing(true);
    try {
      await destroyUserShell(stampedEnv, terminalId, taskID ?? undefined);
      removeUserShellStore(stampedEnv, terminalId);
      markTerminalPanelTerminateClose(panelId);
      setConfirmClose(false);
      closePanel();
    } catch (error) {
      setIsClosing(false);
      console.error("close terminal:", error);
    }
  }, [isClosing, stampedEnv, terminalId, taskID, removeUserShellStore, panelId, closePanel]);

  const handleCloseTab = useCallback(() => {
    if (isClosing) return;
    if (!closable) return;
    if (
      shouldConfirmTerminalClose(terminalId, {
        kind: shell?.kind,
      })
    ) {
      setConfirmClose(true);
      return;
    }
    void destroyAndClosePanel();
  }, [isClosing, closable, terminalId, shell, destroyAndClosePanel]);

  return { confirmClose, setConfirmClose, isClosing, handleCloseTab, destroyAndClosePanel };
}

export function TerminalTab(props: IDockviewPanelHeaderProps) {
  const { terminalId, taskID: stampedTaskID, environmentId: stampedEnv } = extractParams(props);
  const activeTaskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const taskID = stampedTaskID ?? activeTaskID ?? null;
  const { shell, isOrdinary, seq, showBadge, displayName, closable } = useTerminalTabState(
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
  const { confirmClose, setConfirmClose, isClosing, handleCloseTab, destroyAndClosePanel } =
    useTerminalTabClose({
      terminalId,
      taskID,
      stampedEnv,
      shell,
      closable,
      panelId: props.api.id,
      closePanel: () => props.api.close(),
    });
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
    <>
      <ContextMenu>
        <ContextMenuTrigger
          className="flex h-full items-center cursor-pointer select-none"
          data-testid={`terminal-tab-${terminalId}`}
        >
          {isRenaming ? (
            <TabRenameInput
              initial={renameInitial}
              seqBadge={seqBadgeForInput}
              onCommit={handleCommitRename}
              onCancel={() => setIsRenaming(false)}
              testId="terminal-tab-rename-input"
            />
          ) : (
            <TerminalTabBody
              {...props}
              showBadge={showBadge}
              seq={seq}
              displayName={displayName}
              closable={closable}
              isClosing={isClosing}
              terminalId={terminalId}
              onCloseTab={handleCloseTab}
            />
          )}
        </ContextMenuTrigger>
        <TerminalTabMenu
          terminalId={terminalId}
          taskID={taskID}
          environmentId={stampedEnv ?? null}
          canMutate={isOrdinary}
          isClosing={isClosing}
          onStartRename={() => setIsRenaming(true)}
          onClosePanel={() => props.api.close()}
          onTerminatePanel={() => {
            markTerminalPanelTerminateClose(props.api.id);
            props.api.close();
          }}
        />
      </ContextMenu>
      <CloseTerminalConfirmDialog
        open={confirmClose}
        terminalName={displayName}
        onOpenChange={(open) => {
          if (!open) setConfirmClose(false);
        }}
        onConfirm={() => void destroyAndClosePanel()}
      />
    </>
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
  closable,
  isClosing,
  terminalId,
  onCloseTab,
  // `displayName` is computed in the parent but consumed via the
  // api.setTitle effect — drop it here so it doesn't leak into the
  // DOM via the {...props} spread below (React warning otherwise).
  displayName: _displayName,
  ...props
}: IDockviewPanelHeaderProps & {
  showBadge: boolean;
  seq: number | undefined;
  displayName: string;
  closable: boolean;
  isClosing: boolean;
  onCloseTab: () => void;
  terminalId: string;
}) {
  void _displayName;
  const tabContentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!closable || isClosing) return;
    const closeAction = tabContentRef.current?.querySelector(".dv-default-tab-action");
    if (!closeAction) return;
    closeAction.setAttribute("data-testid", `terminal-tab-close-${terminalId}`);
    return () => closeAction.removeAttribute("data-testid");
  }, [closable, isClosing, terminalId, props.api.id]);

  return (
    <div ref={tabContentRef} className="flex h-full items-center">
      {showBadge && (
        <span
          data-testid={`terminal-tab-seq-${seq}`}
          className="ml-2 text-[11px] font-medium leading-none text-muted-foreground bg-foreground/10 rounded px-1.5 py-0.5"
        >
          {seq}
        </span>
      )}
      <DockviewDefaultTab
        {...props}
        hideClose={!closable || isClosing}
        closeActionOverride={closable && !isClosing ? onCloseTab : undefined}
      />
      {closable && isClosing && <TerminalTabClosingSpinner terminalId={terminalId} />}
    </div>
  );
}

function TerminalTabClosingSpinner({ terminalId }: { terminalId: string }) {
  return (
    <span
      aria-label="Closing terminal"
      data-testid={`terminal-tab-closing-${terminalId}`}
      className="dv-default-tab-action inline-flex h-4 w-4 shrink-0 items-center justify-center"
    >
      <span className="h-3 w-3 animate-spin rounded-full border-2 border-muted-foreground/30 border-t-muted-foreground" />
    </span>
  );
}

function TerminalTabMenu({
  terminalId,
  taskID,
  environmentId,
  canMutate,
  isClosing,
  onStartRename,
  onClosePanel,
  onTerminatePanel,
}: {
  terminalId: string;
  taskID: string | null;
  environmentId: string | null;
  canMutate: boolean;
  isClosing: boolean;
  onStartRename: () => void;
  onClosePanel: () => void;
  onTerminatePanel: () => void;
}) {
  const removeUserShellStore = useAppStore((s) => s.removeUserShell);

  const handleTerminate = useCallback(async () => {
    if (isClosing) return;
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
    isClosing,
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
        disabled={isClosing}
        className="text-destructive focus:text-destructive"
      >
        Terminate
      </ContextMenuItem>
    </ContextMenuContent>
  );
}
