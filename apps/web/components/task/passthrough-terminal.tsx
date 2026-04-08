"use client";

import React, { useRef, useCallback, useEffect, useMemo, useState } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { AttachAddon } from "@xterm/addon-attach";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import { GridSpinner } from "@/components/grid-spinner";
import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
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

type BaseProps = {
  sessionId?: string | null;
  autoFocus?: boolean;
  pendingCommand?: string | null;
  onCommandSent?: () => void;
};
type AgentTerminalProps = BaseProps & { mode: "agent"; label?: string };
type ShellTerminalProps = BaseProps & { mode: "shell"; terminalId: string; label?: string };
type PassthroughTerminalProps = AgentTerminalProps | ShellTerminalProps;

/**
 * PassthroughTerminal provides direct terminal interaction with an agent CLI.
 *
 * Design: Dedicated Binary WebSocket + AttachAddon
 * - Uses a dedicated WebSocket connection to /terminal/:sessionId
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

const WS_BASE_URL_FALLBACK = "ws://localhost:8080";
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

export function PassthroughTerminal(props: PassthroughTerminalProps) {
  const { sessionId: propSessionId, mode, label, autoFocus, pendingCommand, onCommandSent } = props;
  const terminalId = mode === "shell" ? props.terminalId : undefined;
  const refs = useTerminalRefs();
  const { terminalRef, xtermRef, fitAddonRef, wsRef, attachAddonRef } = refs;

  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = propSessionId ?? storeSessionId;

  const { session, isActive } = useSession(sessionId);
  const agentctlStatus = useSessionAgentctl(sessionId);
  const taskId = session?.task_id ?? null;
  // Gate WS connection on agentctl readiness. During a long prepare script
  // the backend terminal endpoint isn't accepting connections yet, and
  // spamming reconnects burns cycles and confuses the loading state.
  const canConnect = Boolean(sessionId && isActive && agentctlStatus.isReady);
  const wsBaseUrl = useWsBaseUrl();

  const [isTerminalReady, setIsTerminalReady] = useState(false);
  const onTerminalReady = useCallback(() => setIsTerminalReady(true), []);

  // Track which session has an active WebSocket connection. The loading overlay
  // resets on session switches without needing a separate setState effect.
  const [connectedSessionId, setConnectedSessionId] = useState<string | null>(null);
  const isConnected = sessionId != null && connectedSessionId === sessionId;
  const onConnected = useCallback(() => {
    setConnectedSessionId(sessionId ?? null);
    if (autoFocus) refs.xtermRef.current?.textarea?.focus({ preventScroll: true });
  }, [sessionId, autoFocus, refs.xtermRef]);

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
    onToggleBottomTerminal: toggleBottomTerminal,
    sendInput,
    keyboardShortcutsRef,
  });

  useWebSocketConnection({
    taskId,
    sessionId,
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
  });

  usePendingCommand(pendingCommand, isConnected, wsRef, onCommandSent);

  return (
    <div
      data-testid={mode === "agent" ? "passthrough-terminal" : undefined}
      className="relative h-full w-full overflow-hidden bg-background"
      style={{ minWidth: MIN_WIDTH, minHeight: MIN_HEIGHT }}
    >
      <div className="h-full w-full p-2 pb-3">
        <div ref={terminalRef} className="h-full w-full" />
      </div>
      {!isConnected && (
        <div
          data-testid="passthrough-loading"
          className="absolute inset-0 flex items-start justify-center pt-12 bg-background"
        >
          <div className="flex flex-col items-center gap-3 text-muted-foreground">
            <GridSpinner />
            <span className="text-sm">
              {mode === "agent" ? "Preparing workspace..." : "Connecting terminal..."}
            </span>
          </div>
        </div>
      )}
    </div>
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
