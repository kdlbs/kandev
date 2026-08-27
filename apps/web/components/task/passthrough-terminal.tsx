"use client";

import React, { useRef, useCallback, useEffect, useMemo, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { AttachAddon } from "@xterm/addon-attach";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import { PanelLoadingState } from "@/components/panel-loading-state";
import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import { useTheme } from "@/components/theme/app-theme";
import type { TaskSessionState } from "@/app/office/tasks/[id]/types";
import { getBackendConfig } from "@/lib/config";
import { useTerminalLinkHandler } from "@/hooks/use-terminal-link-handler";
import { buildTerminalFontFamily } from "@/lib/terminal/terminal-font";
import {
  MIN_WIDTH,
  MIN_HEIGHT,
  useTerminalInit,
  useWebSocketConnection,
  useSendResize,
  useSendInput,
  useFitAndResize,
} from "./use-passthrough-terminal";
import { useTouchScroll } from "./use-touch-scroll";
import { useEnvironmentSessionId } from "@/hooks/use-environment-session-id";
import { useTerminalSearch } from "./use-terminal-search";
import { TerminalSearchBar } from "./terminal-search-bar";
import { usePanelSearch } from "@/hooks/use-panel-search";
import { useTerminalBusyTracking } from "./use-terminal-busy-tracking";
import { useTerminalTheme } from "./use-terminal-theme";
import { useTranslation } from "react-i18next";

type BaseProps = {
  autoFocus?: boolean;
  pendingCommand?: string | null;
  onCommandSent?: () => void;
  /** Called once the xterm instance is created. Mobile uses this to register
   * the active key-bar input target. Desktop ignores. */
  onXtermReady?: (xterm: Terminal) => void;
  /** Skip the WebGL renderer addon and use xterm's canvas renderer instead.
   * WebGL miscomputes glyph atlas scaling on mobile (notably after a
   * desktop→mobile responsive switch) and renders text at multiples of the
   * intended font size. Mobile callers should pass true. */
  disableWebgl?: boolean;
  /** When true, the AttachAddon is configured receive-only — incoming PTY
   * data still flows to the terminal display, but the consumer is responsible
   * for forwarding xterm.onData to the WebSocket. Mobile sets this so the
   * key-bar's modifier transforms can run before bytes go on the wire. */
  manualInputRouting?: boolean;
  /** Fires when the dedicated terminal WebSocket reaches the OPEN state.
   * Mobile uses this to register a key-bar sender that writes raw bytes
   * directly to this terminal's socket. */
  onWsReady?: (ws: WebSocket) => void;
  /** Translate single-finger touch swipes on the terminal area into xterm
   * scrollback navigation. Mobile callers set this so the xterm canvas no
   * longer silently absorbs touch gestures. */
  enableTouchScroll?: boolean;
};
type AgentTerminalProps = BaseProps & { mode: "agent"; sessionId?: string | null; label?: string };
type ShellTerminalProps = BaseProps & {
  mode: "shell";
  environmentId: string | null | undefined;
  terminalId: string;
  label?: string;
};
type PassthroughTerminalProps = AgentTerminalProps | ShellTerminalProps;

/**
 * PassthroughTerminal provides direct terminal interaction with an agent CLI.
 *
 * Design: Dedicated Binary WebSocket + AttachAddon
 * - Uses dedicated WebSocket routes for session-scoped agent terminals and env-scoped shells
 * - Raw binary frames bypass JSON encoding/decoding latency
 * - AttachAddon (official xterm.js addon) handles the bridging
 * - Unicode11Addon enables proper unicode character support
 * - Resize commands sent via binary protocol: [0x01][JSON {cols, rows}]
 */
function useTerminalRefs() {
  return {
    terminalRef: useRef<HTMLDivElement>(null),
    xtermRef: useRef<Terminal | null>(null),
    fitAddonRef: useRef<FitAddon | null>(null),
    wsRef: useRef<WebSocket | null>(null),
    attachAddonRef: useRef<AttachAddon | null>(null),
    isInitializedRef: useRef(false),
    lastDimensionsRef: useRef({ cols: 0, rows: 0 }),
    resizeTimeoutRef: useRef<ReturnType<typeof setTimeout> | null>(null),
    webglAddonRef: useRef<WebglAddon | null>(null),
  };
}

function useXtermSearchIntegration(
  xtermRef: React.RefObject<Terminal | null>,
  isTerminalReady: boolean,
  containerRef: React.RefObject<HTMLDivElement | null>,
) {
  const search = useTerminalSearch({ xtermRef, isTerminalReady });
  const onFindInPanelRef = useRef<(() => void) | undefined>(search.open);
  useEffect(() => {
    onFindInPanelRef.current = search.open;
  }, [search.open]);
  usePanelSearch({
    containerRef,
    isOpen: search.isOpen,
    onOpen: search.open,
    onClose: search.close,
  });
  return { search, onFindInPanelRef };
}

const WS_BASE_URL_FALLBACK = "ws://localhost:38429";
function useWsBaseUrl() {
  return useMemo(() => {
    try {
      const url = new URL(getBackendConfig().apiBaseUrl);
      return `${url.protocol === "https:" ? "wss:" : "ws:"}//${url.host}`;
    } catch {
      return WS_BASE_URL_FALLBACK;
    }
  }, []);
}

/**
 * Decide whether the WS effect should attempt to open a terminal connection.
 *
 * Agent terminals connect as soon as the session identity is known. The
 * backend owns workspace readiness and passthrough process recovery; gating
 * on the client's one-shot agentctl status can strand cold subscribers.
 *
 * Shell terminals route by env on the backend; the env handler lazy-creates
 * the execution and waits for remote readiness server-side, so the client
 * only needs the env id. No session involvement.
 */
export function computeCanConnect(
  mode: "agent" | "shell",
  connectionID: string | null | undefined,
  sessionId: string | null | undefined,
  environmentEnded = false,
): boolean {
  if (!connectionID) return false;
  if (mode === "agent") return Boolean(sessionId);
  // The env handler refuses a shell for a dead environment, and refuses it
  // identically on every attempt. Connecting anyway only restarts the retry
  // timer, so stop before opening the socket at all.
  return !environmentEnded;
}

const SESSION_TERMINAL_STATES = new Set<TaskSessionState>(["COMPLETED", "FAILED", "CANCELLED"]);

export function isSessionEnded(state: TaskSessionState | null | undefined): boolean {
  return state != null && SESSION_TERMINAL_STATES.has(state);
}

/**
 * Whether every session sharing this environment has ended.
 *
 * Shell terminals are environment-scoped, but the session handle this component
 * holds is not a proxy for one: `useEnvironmentSessionId` deliberately caches
 * the first session seen for an environment, so once session A ends and session
 * B reuses that environment, the handle still resolves to A. Gating on A's
 * state alone would refuse a shell on a live environment — worse than the loop
 * this exists to stop.
 *
 * So refuse only on positive knowledge: sessions for this environment are
 * known, and every one of them is terminal. Anything unknown connects.
 */
export function isEnvironmentEnded(
  environmentId: string | null | undefined,
  sessionStateById: Record<string, TaskSessionState | undefined>,
  environmentIdBySessionId: Record<string, string>,
): boolean {
  if (!environmentId) return false;
  const sessionIds = Object.keys(environmentIdBySessionId).filter(
    (id) => environmentIdBySessionId[id] === environmentId,
  );
  if (sessionIds.length === 0) return false;
  return sessionIds.every((id) => isSessionEnded(sessionStateById[id]));
}

/**
 * What the pane should render. Exported so every state can be asserted without
 * mounting xterm.
 *
 * This is a named function rather than two inline conditions because the
 * loading overlay is opaque and full-bleed: any state that leaves
 * `isConnected` false while the terminal will never connect renders a
 * "connecting…" spinner that is actively false. Deciding it in one place makes
 * "ended outranks connecting" something a test can pin down.
 */
export function computeTerminalPaneState(
  mode: "agent" | "shell",
  environmentEnded: boolean,
  isConnected: boolean,
): "connected" | "connecting" | "ended" {
  if (mode === "shell" && environmentEnded) return "ended";
  return isConnected ? "connected" : "connecting";
}

/** Reads the two store slices `isEnvironmentEnded` needs. */
function useEnvironmentEnded(environmentId: string | null | undefined): boolean {
  return useAppStore((state) =>
    isEnvironmentEnded(
      environmentId,
      Object.fromEntries(Object.entries(state.taskSessions.items).map(([id, s]) => [id, s?.state])),
      state.environmentIdBySessionId,
    ),
  );
}

// eslint-disable-next-line max-lines-per-function -- wires many hooks + refs; each block is already its own hook
export function PassthroughTerminal(props: PassthroughTerminalProps) {
  const {
    mode,
    label,
    autoFocus,
    pendingCommand,
    onCommandSent,
    onXtermReady,
    disableWebgl,
    manualInputRouting,
    onWsReady,
    enableTouchScroll,
  } = props;
  const { resolvedTheme } = useTheme();
  const terminalId = mode === "shell" ? props.terminalId : undefined;
  const environmentId = mode === "shell" ? props.environmentId : undefined;
  const refs = useTerminalRefs();
  const { terminalRef, xtermRef, fitAddonRef, wsRef, attachAddonRef } = refs;

  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const environmentSessionId = useEnvironmentSessionId();
  const sessionId = mode === "agent" ? (props.sessionId ?? storeSessionId) : environmentSessionId;

  const { session } = useSession(sessionId);
  const taskId = session?.task_id ?? null;
  const connectionID = mode === "agent" ? sessionId : environmentId;
  const environmentEnded = useEnvironmentEnded(environmentId);
  const canConnect = computeCanConnect(mode, connectionID, sessionId, environmentEnded);
  const wsBaseUrl = useWsBaseUrl();

  const [isTerminalReady, setIsTerminalReady] = useState(false);
  const onTerminalReady = useCallback(() => {
    setIsTerminalReady(true);
    if (xtermRef.current) onXtermReady?.(xtermRef.current);
  }, [onXtermReady, xtermRef]);

  // Track which terminal target has an active WebSocket connection. The loading
  // overlay resets on target switches without needing a separate setState effect.
  const [connectedTargetId, setConnectedTargetId] = useState<string | null>(null);
  const isConnected = connectionID != null && connectedTargetId === connectionID;
  const paneState = computeTerminalPaneState(mode, environmentEnded, isConnected);
  const onConnected = useCallback(() => {
    setConnectedTargetId(connectionID ?? null);
    if (autoFocus) refs.xtermRef.current?.textarea?.focus({ preventScroll: true });
  }, [connectionID, autoFocus, refs.xtermRef]);
  const onDisconnected = useCallback(() => {
    const disconnectedTargetId = connectionID ?? null;
    setConnectedTargetId((current) => (current === disconnectedTargetId ? null : current));
  }, [connectionID]);

  const linkHandler = useTerminalLinkHandler();
  const terminalFontFamily = useAppStore((s) => s.userSettings.terminalFontFamily);
  const terminalFontSize = useAppStore((s) => s.userSettings.terminalFontSize);
  const sendResize = useSendResize(wsRef);
  const fitAndResize = useFitAndResize({
    xtermRef: refs.xtermRef,
    fitAddonRef: refs.fitAddonRef,
    terminalRef: refs.terminalRef,
    lastDimensionsRef: refs.lastDimensionsRef,
    sendResize,
  });

  const sendInput = useSendInput(wsRef);
  const toggleBottomTerminal = useAppStore((s) => s.toggleBottomTerminal);
  const keyboardShortcuts = useAppStore((s) => s.userSettings.keyboardShortcuts);
  const keyboardShortcutsRef = useRef(keyboardShortcuts);
  useEffect(() => {
    keyboardShortcutsRef.current = keyboardShortcuts;
  }, [keyboardShortcuts]);
  const containerRef = useRef<HTMLDivElement>(null);
  const { search, onFindInPanelRef } = useXtermSearchIntegration(
    xtermRef,
    isTerminalReady,
    containerRef,
  );
  useTerminalInit({
    terminalRef: refs.terminalRef,
    xtermRef: refs.xtermRef,
    fitAddonRef: refs.fitAddonRef,
    isInitializedRef: refs.isInitializedRef,
    lastDimensionsRef: refs.lastDimensionsRef,
    resizeTimeoutRef: refs.resizeTimeoutRef,
    webglAddonRef: refs.webglAddonRef,
    fitAndResize,
    onReady: onTerminalReady,
    linkHandler,
    fontFamily: buildTerminalFontFamily(terminalFontFamily),
    fontSize: terminalFontSize ?? undefined,
    disableWebgl,
    resolvedTheme,
    onToggleBottomTerminal: toggleBottomTerminal,
    sendInput,
    keyboardShortcutsRef,
    onFindInPanelRef,
  });
  useTerminalTheme({
    terminalRef: xtermRef,
    containerRef: terminalRef,
    isTerminalReady,
    resolvedTheme,
  });
  useTerminalBusyTracking(terminalId, xtermRef, mode === "shell", isTerminalReady);

  useTouchScroll({
    terminalRef: refs.terminalRef,
    xtermRef: refs.xtermRef,
    enabled: !!enableTouchScroll,
    isTerminalReady,
  });

  useWebSocketConnection({
    taskId,
    sessionId,
    environmentId,
    canConnect,
    isTerminalReady,
    fitAndResize,
    wsBaseUrl,
    mode,
    terminalId,
    label,
    xtermRef,
    fitAddonRef,
    wsRef,
    attachAddonRef,
    onConnected,
    onDisconnected,
    manualInputRouting,
    onWsReady,
  });

  usePendingCommand(pendingCommand, isConnected, wsRef, onCommandSent);

  return (
    <div
      ref={containerRef}
      tabIndex={-1}
      data-testid={mode === "agent" ? "passthrough-terminal" : undefined}
      data-panel-kind="terminal"
      className="relative h-full w-full overflow-hidden bg-background outline-none"
      style={{ minWidth: MIN_WIDTH, minHeight: MIN_HEIGHT }}
    >
      <div className="h-full w-full p-2 pb-3">
        <div ref={terminalRef} data-testid="terminal-xterm-host" className="h-full w-full" />
      </div>
      <TerminalSearchBar search={search} />
      <TerminalPaneOverlay paneState={paneState} mode={mode} />
    </div>
  );
}

/**
 * Covers the pane while it is not usable. Both branches are opaque and
 * full-bleed, so which one renders is the only thing the user ever sees here.
 */
function TerminalPaneOverlay({
  paneState,
  mode,
}: {
  paneState: ReturnType<typeof computeTerminalPaneState>;
  mode: "agent" | "shell";
}) {
  const { t } = useTranslation();
  if (paneState === "connected") return null;
  if (paneState === "ended") {
    return (
      <div
        data-testid="passthrough-session-ended"
        className="absolute inset-0 bg-background flex items-center justify-center p-4 text-center text-xs text-muted-foreground"
      >
        {t("task:terminalSessionEnded")}
      </div>
    );
  }
  return (
    <PanelLoadingState
      testId="passthrough-loading"
      className="absolute inset-0 bg-background"
      label={mode === "agent" ? t("task:preparingWorkspace") : t("task:connectingTerminal")}
    />
  );
}

/** Sends a pending command to the terminal WS once connected. */
function usePendingCommand(
  pendingCommand: string | null | undefined,
  isConnected: boolean,
  wsRef: React.RefObject<WebSocket | null>,
  onCommandSent?: () => void,
) {
  React.useEffect(() => {
    if (!pendingCommand || !isConnected) return;
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    // Small delay to ensure the shell prompt is ready after WS connect.
    const timer = setTimeout(() => {
      ws.send(new TextEncoder().encode(pendingCommand));
      onCommandSent?.();
    }, 300);
    return () => clearTimeout(timer);
  }, [pendingCommand, isConnected, wsRef, onCommandSent]);
}
