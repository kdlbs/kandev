"use client";

import React, { useRef, useCallback, useMemo, useState } from "react";
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
import {
  log,
  MIN_WIDTH,
  MIN_HEIGHT,
  useTerminalInit,
  useWebSocketConnection,
  useSendResize,
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
export function PassthroughTerminal(props: PassthroughTerminalProps) {
  const { sessionId: propSessionId, mode, label, autoFocus, pendingCommand, onCommandSent } = props;
  const terminalId = mode === "shell" ? props.terminalId : undefined;
  const terminalRef = useRef<HTMLDivElement>(null);
  const xtermRef = useRef<Terminal | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const attachAddonRef = useRef<AttachAddon | null>(null);
  const isInitializedRef = useRef(false);
  const lastDimensionsRef = useRef({ cols: 0, rows: 0 });
  const resizeTimeoutRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const webglAddonRef = useRef<WebglAddon | null>(null);

  const storeSessionId = useAppStore((state) => state.tasks.activeSessionId);
  const sessionId = propSessionId ?? storeSessionId;

  const { session, isActive } = useSession(sessionId);
  useSessionAgentctl(sessionId);
  const taskId = session?.task_id ?? null;
  const canConnect = Boolean(sessionId && isActive);

  const wsBaseUrl = useMemo(() => {
    try {
      const url = new URL(getBackendConfig().apiBaseUrl);
      return `${url.protocol === "https:" ? "wss:" : "ws:"}//${url.host}`;
    } catch {
      return "ws://localhost:8080";
    }
  }, []);

  const [isTerminalReady, setIsTerminalReady] = useState(false);
  const onTerminalReady = useCallback(() => {
    log("Terminal ready — will trigger WebSocket connection");
    setIsTerminalReady(true);
  }, []);

  // Track which session has an active WebSocket connection.  The loading
  // overlay is shown whenever the current sessionId doesn't match the
  // connected one — this naturally resets on session switches without
  // needing a separate effect that calls setState.
  const [connectedSessionId, setConnectedSessionId] = useState<string | null>(null);
  const isConnected = sessionId != null && connectedSessionId === sessionId;
  const onConnected = useCallback(() => {
    setConnectedSessionId(sessionId ?? null);
    if (autoFocus) xtermRef.current?.textarea?.focus({ preventScroll: true });
  }, [sessionId, autoFocus]);

  const linkHandler = useTerminalLinkHandler();
  const sendResize = useSendResize(wsRef);
  const fitAndResize = useFitAndResize({
    xtermRef,
    fitAddonRef,
    terminalRef,
    lastDimensionsRef,
    sendResize,
  });

  const toggleBottomTerminal = useAppStore((s) => s.toggleBottomTerminal);
  useTerminalInit({
    terminalRef,
    xtermRef,
    fitAddonRef,
    isInitializedRef,
    lastDimensionsRef,
    resizeTimeoutRef,
    webglAddonRef,
    fitAndResize,
    onReady: onTerminalReady,
    linkHandler,
    onToggleBottomTerminal: toggleBottomTerminal,
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
      <div ref={terminalRef} className="h-full w-full p-2" />
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
