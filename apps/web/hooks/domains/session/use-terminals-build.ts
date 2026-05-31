import type { UserShellInfo } from "@/lib/state/slices/session-runtime/types";

export const TERMINAL_TYPE_DEV_SERVER = "dev-server";
export const BOTTOM_PANEL_TERMINAL_ID = "bottom-panel";

import type { Terminal, TerminalType } from "./use-terminals-types";

export function deriveLabel(shell: UserShellInfo): string {
  if (shell.customName && shell.customName !== "") return shell.customName;
  if (shell.displayName) return shell.displayName;
  if (shell.label) return shell.label;
  if (shell.kind === "ordinary" && shell.seq) return `Terminal ${shell.seq}`;
  return shell.terminalId.startsWith("script-") ? "Script" : "Terminal";
}

export function isOrdinary(shell: UserShellInfo): boolean {
  if (shell.kind) return shell.kind === "ordinary";
  // Legacy fallback — ids without script-/bottom-panel prefix are treated
  // as ordinary if they have ordinary-shaped metadata.
  return (
    shell.terminalId !== BOTTOM_PANEL_TERMINAL_ID &&
    !shell.terminalId.startsWith("script-") &&
    shell.seq !== undefined
  );
}

export function shellToTerminal(shell: UserShellInfo): Terminal {
  const ordinary = isOrdinary(shell);
  const isScript = shell.terminalId.startsWith("script-");
  return {
    id: shell.terminalId,
    type: isScript ? "script" : "shell",
    label: deriveLabel(shell),
    closable: shell.closable ?? true,
    kind: ordinary ? "ordinary" : shell.kind,
    seq: shell.seq,
    customName: shell.customName,
    state: shell.state,
    ptyStatus: shell.ptyStatus,
    initialCommand: shell.initialCommand,
  };
}

/**
 * Compute the effective active tab value, preferring store, then
 * sessionStorage, then fallback.
 *
 * `activeTab` is only honoured when it points at a tab that is still
 * selectable — i.e. present in `terminals` or the synthetic "commands"
 * tab. A stale id (e.g. left over from a terminal that was parked or
 * destroyed in another tab) would otherwise stay active and block the
 * fallback shift to a real shell. The "commands" id is always accepted
 * because it has no corresponding entry in `terminals`.
 */
export function computeTerminalTabValue(
  activeTab: string | undefined,
  sessionJustChanged: boolean,
  savedTabFromStorage: string | null,
  terminals: Terminal[],
  savedTabExists: boolean,
): string {
  const activeTabSelectable =
    !!activeTab && (activeTab === "commands" || terminals.some((t) => t.id === activeTab));
  const effectiveActiveTab =
    !sessionJustChanged && activeTab && activeTab !== "" && activeTabSelectable ? activeTab : null;
  return (
    effectiveActiveTab ??
    (savedTabFromStorage && (terminals.length === 0 || savedTabExists)
      ? savedTabFromStorage
      : null) ??
    terminals.find((t) => t.type === "shell")?.id ??
    "commands"
  );
}

/** Build terminal list from user shells, preserving dev-server terminal if present. */
export function buildTerminalsFromShells(
  prev: Terminal[],
  userShells: UserShellInfo[],
): Terminal[] {
  const devTerminal = prev.find((t) => t.type === TERMINAL_TYPE_DEV_SERVER);
  const visibleShells = userShells.filter((s) => {
    // Parked terminals live in their own submenu, not the main strip.
    if (s.state === "parked") return false;
    // The bottom-panel terminal renders inside its own dedicated component
    // (bottom-terminal-panel.tsx) — exclude it from the right-panel strip.
    if (s.terminalId === BOTTOM_PANEL_TERMINAL_ID) return false;
    return true;
  });
  const userTerminals = visibleShells.map(shellToTerminal);
  const result: Terminal[] = [];
  if (devTerminal) result.push(devTerminal);
  result.push(...userTerminals);
  return result;
}

export function buildParkedTerminals(
  userShells: UserShellInfo[],
  userShellsLoaded: boolean,
): Terminal[] {
  if (!userShellsLoaded) return [];
  return userShells.filter((s) => s.state === "parked").map(shellToTerminal);
}

/** Sync the dev-server terminal with preview open state. */
export function syncDevTerminal(prev: Terminal[], previewOpen: boolean): Terminal[] {
  const hasDevTerminal = prev.some((t) => t.type === TERMINAL_TYPE_DEV_SERVER);
  if (previewOpen && !hasDevTerminal) {
    return [
      {
        id: TERMINAL_TYPE_DEV_SERVER,
        type: TERMINAL_TYPE_DEV_SERVER as TerminalType,
        label: "Dev Server",
        closable: true,
      },
      ...prev,
    ];
  }
  if (!previewOpen && hasDevTerminal) {
    return prev.filter((t) => t.type !== TERMINAL_TYPE_DEV_SERVER);
  }
  return prev;
}
