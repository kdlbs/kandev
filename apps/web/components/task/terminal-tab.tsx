"use client";

import { useCallback, useEffect, useRef, useState, type RefObject } from "react";
import { DockviewDefaultTab, type IDockviewPanelHeaderProps } from "dockview-react";
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@kandev/ui/context-menu";
import { useAppStore } from "@/components/state-provider";
import { renameUserShell } from "@/lib/api/domains/user-shell-api";
import { useTerminalDestroy } from "@/hooks/domains/session/use-terminal-destroy";
import { CloseTerminalConfirmPopover } from "./close-terminal-confirm-popover";
import { TabRenameInput } from "./tab-rename-input";
import { markTerminalPanelTerminateClose } from "./dockview-layout-setup";
import { useTranslation } from "react-i18next";

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
  ordinaryLabel: string,
): string {
  if (shell?.customName && shell.customName !== "") return shell.customName;
  if (shell?.kind === "ordinary") return ordinaryLabel;
  if (shell?.label) return shell.label;
  return fallback;
}

function useTerminalTabState(
  stampedEnv: string | undefined,
  terminalId: string,
  apiTitle: string,
  ordinaryLabel: string,
) {
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
  const displayName = pickDisplayName(shell, apiTitle, ordinaryLabel);
  const closable = shell?.closable ?? true;
  return { shell, isOrdinary, seq, showBadge, displayName, closable };
}

function useTerminalTabClose({
  terminalId,
  taskID,
  stampedEnv,
  closable,
  panelId,
  closePanel,
}: {
  terminalId: string;
  taskID: string | null;
  stampedEnv: string | undefined;
  closable: boolean;
  panelId: string;
  closePanel: () => void;
}) {
  const removeUserShellStore = useAppStore((s) => s.removeUserShell);
  const [confirmClose, setConfirmClose] = useState(false);
  const closeAnchorRef = useRef<HTMLElement>(null);

  const handleDestroyed = useCallback(() => {
    if (stampedEnv) {
      removeUserShellStore(stampedEnv, terminalId);
    }
    markTerminalPanelTerminateClose(panelId);
    closePanel();
  }, [stampedEnv, terminalId, removeUserShellStore, panelId, closePanel]);
  const { destroyTerminal } = useTerminalDestroy({
    environmentId: stampedEnv,
    taskId: taskID,
    onDestroyed: handleDestroyed,
  });

  const destroyAndClosePanel = useCallback(async () => {
    if (!stampedEnv) {
      closePanel();
      return true;
    }
    return destroyTerminal(terminalId);
  }, [stampedEnv, terminalId, destroyTerminal, closePanel]);

  const handleCloseTab = useCallback(() => {
    if (!closable) return;
    setConfirmClose(true);
  }, [closable]);

  return {
    confirmClose,
    setConfirmClose,
    closeAnchorRef,
    handleCloseTab,
    destroyAndClosePanel,
  };
}

export function TerminalTab(props: IDockviewPanelHeaderProps) {
  const { t } = useTranslation();
  const { terminalId, taskID: stampedTaskID, environmentId: stampedEnv } = extractParams(props);
  const activeTaskID = useAppStore((s) => s.tasks?.activeTaskId ?? null);
  const taskID = stampedTaskID ?? activeTaskID ?? null;
  const { shell, isOrdinary, seq, showBadge, displayName, closable } = useTerminalTabState(
    stampedEnv,
    terminalId,
    props.api.title ?? t("common:terminal"),
    t("task:panelTerminal"),
  );

  // DockviewDefaultTab reads the title from `api.title` directly and
  // ignores prop overrides — push the corrected title onto the api so
  // the default-tab body re-renders the right text.
  useEffect(() => {
    if (props.api.title !== displayName) props.api.setTitle(displayName);
  }, [props.api, displayName]);

  const [isRenaming, setIsRenaming] = useState(false);
  const { confirmClose, setConfirmClose, closeAnchorRef, handleCloseTab, destroyAndClosePanel } =
    useTerminalTabClose({
      terminalId,
      taskID,
      stampedEnv,
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
              terminalId={terminalId}
              closeLabel={t("task:close2", { label: displayName })}
              closeAnchorRef={closeAnchorRef}
              onCloseTab={handleCloseTab}
            />
          )}
        </ContextMenuTrigger>
        <TerminalTabMenu
          canMutate={isOrdinary}
          onStartRename={() => setIsRenaming(true)}
          onClosePanel={() => props.api.close()}
          onTerminatePanel={handleCloseTab}
        />
      </ContextMenu>
      <CloseTerminalConfirmPopover
        open={confirmClose}
        terminalName={displayName}
        anchorRef={closeAnchorRef}
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
  terminalId,
  closeLabel,
  closeAnchorRef,
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
  onCloseTab: () => void;
  terminalId: string;
  closeLabel: string;
  closeAnchorRef: RefObject<HTMLElement | null>;
}) {
  void _displayName;
  const tabContentRef = useRef<HTMLDivElement>(null);

  useEffect(() => {
    if (!closable) return;
    const closeAction = tabContentRef.current?.querySelector(".dv-default-tab-action");
    if (!(closeAction instanceof HTMLElement)) return;
    closeAnchorRef.current = closeAction;
    closeAction.setAttribute("data-testid", `terminal-tab-close-${terminalId}`);
    closeAction.setAttribute("aria-label", closeLabel);

    const needsKeyboardShim = closeAction.tagName !== "BUTTON";
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key !== "Enter" && event.key !== " ") return;
      event.preventDefault();
      event.stopPropagation();
      onCloseTab();
    };
    if (needsKeyboardShim) {
      closeAction.setAttribute("role", "button");
      closeAction.setAttribute("tabindex", "0");
      closeAction.addEventListener("keydown", handleKeyDown);
    }

    return () => {
      if (closeAnchorRef.current === closeAction) closeAnchorRef.current = null;
      closeAction.removeAttribute("data-testid");
      closeAction.removeAttribute("aria-label");
      if (needsKeyboardShim) {
        closeAction.removeAttribute("role");
        closeAction.removeAttribute("tabindex");
        closeAction.removeEventListener("keydown", handleKeyDown);
      }
    };
  }, [closable, terminalId, props.api.id, closeLabel, closeAnchorRef, onCloseTab]);

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
        hideClose={!closable}
        closeActionOverride={closable ? onCloseTab : undefined}
      />
    </div>
  );
}

export function TerminalTabMenu({
  canMutate,
  onStartRename,
  onClosePanel,
  onTerminatePanel,
}: {
  canMutate: boolean;
  onStartRename: () => void;
  onClosePanel: () => void;
  onTerminatePanel: () => void;
}) {
  const { t } = useTranslation();
  const pendingActionAfterCloseRef = useRef<"rename" | "terminate" | null>(null);

  const handleRename = useCallback(() => {
    pendingActionAfterCloseRef.current = "rename";
  }, []);

  const handleTerminate = useCallback(() => {
    if (!canMutate) {
      onClosePanel();
      return;
    }
    pendingActionAfterCloseRef.current = "terminate";
  }, [canMutate, onClosePanel]);

  return (
    <ContextMenuContent
      onCloseAutoFocus={(event) => {
        const pendingAction = pendingActionAfterCloseRef.current;
        if (!pendingAction) return;
        pendingActionAfterCloseRef.current = null;
        event.preventDefault();
        if (pendingAction === "rename") {
          queueMicrotask(onStartRename);
          return;
        }
        onTerminatePanel();
      }}
    >
      {canMutate && (
        <>
          <ContextMenuItem onClick={handleRename}>{t("task:rename2")}</ContextMenuItem>
          <ContextMenuSeparator />
        </>
      )}
      <ContextMenuItem
        onClick={handleTerminate}
        className="text-destructive focus:text-destructive"
      >
        {t("task:terminate")}
      </ContextMenuItem>
    </ContextMenuContent>
  );
}
