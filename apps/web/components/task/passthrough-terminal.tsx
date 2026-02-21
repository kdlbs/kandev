"use client";

import React, { useEffect, useRef, useCallback, useMemo } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { AttachAddon } from "@xterm/addon-attach";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import { WebglAddon } from "@xterm/addon-webgl";
import "@xterm/xterm/css/xterm.css";
import { useAppStore } from "@/components/state-provider";
import { useSession } from "@/hooks/domains/session/use-session";
import { useSessionAgentctl } from "@/hooks/domains/session/use-session-agentctl";
import { getBackendConfig } from "@/lib/config";
import { getTerminalTheme } from "@/lib/theme/terminal-theme";

type BaseProps = {
  sessionId?: string | null;
};
type AgentTerminalProps = BaseProps & { mode: "agent"; label?: string };
type ShellTerminalProps = BaseProps & { mode: "shell"; terminalId: string; label?: string };
type PassthroughTerminalProps = AgentTerminalProps | ShellTerminalProps;

// Debug flag - set to true to see detailed logs
const DEBUG = false;
const log = (...args: unknown[]) => {
  if (DEBUG) console.log("[PassthroughTerminal]", ...args);
};

// Minimum dimensions to prevent zero-size issues
const MIN_WIDTH = 100;
const MIN_HEIGHT = 100;

type TerminalInitOptions = {
  terminalRef: React.RefObject<HTMLDivElement | null>;
  xtermRef: React.MutableRefObject<Terminal | null>;
  fitAddonRef: React.MutableRefObject<FitAddon | null>;
  isInitializedRef: React.MutableRefObject<boolean>;
  lastDimensionsRef: React.MutableRefObject<{ cols: number; rows: number }>;
  resizeTimeoutRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>;
  webglAddonRef: React.MutableRefObject<WebglAddon | null>;
  fitAndResize: (force?: boolean) => void;
};

function initTerminalInstance(
  termContainer: HTMLDivElement,
  refs: Pick<
    TerminalInitOptions,
    | "xtermRef"
    | "fitAddonRef"
    | "isInitializedRef"
    | "lastDimensionsRef"
    | "webglAddonRef"
    | "resizeTimeoutRef"
  >,
  fitAndResize: (force?: boolean) => void,
) {
  if (refs.isInitializedRef.current || refs.xtermRef.current) return undefined;
  refs.isInitializedRef.current = true;
  log("Creating terminal");
  const terminal = new Terminal({
    allowProposedApi: true,
    cursorBlink: true,
    disableStdin: false,
    convertEol: false,
    scrollOnUserInput: true,
    scrollback: 5000,
    fontSize: 13,
    fontFamily: 'Menlo, Monaco, "Courier New", monospace',
    theme: getTerminalTheme(termContainer),
  });
  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  const unicode11Addon = new Unicode11Addon();
  terminal.loadAddon(unicode11Addon);
  terminal.unicode.activeVersion = "11";
  log("Opening terminal in container");
  terminal.open(termContainer);
  try {
    fitAddon.fit();
    refs.lastDimensionsRef.current = { cols: terminal.cols, rows: terminal.rows };
    log("Initial fit:", terminal.cols, "x", terminal.rows);
  } catch (e) {
    log("Initial fit failed:", e);
  }
  try {
    const webglAddon = new WebglAddon();
    webglAddon.onContextLoss(() => {
      log("WebGL context lost");
      webglAddon.dispose();
      refs.webglAddonRef.current = null;
    });
    terminal.loadAddon(webglAddon);
    refs.webglAddonRef.current = webglAddon;
    log("WebGL addon loaded");
  } catch (e) {
    log("WebGL failed, using canvas:", e);
  }
  refs.xtermRef.current = terminal;
  refs.fitAddonRef.current = fitAddon;
  const handleResize = () => {
    const rect = termContainer.getBoundingClientRect();
    if (rect.width < MIN_WIDTH || rect.height < MIN_HEIGHT) {
      log("Skipping resize - too small");
      return;
    }
    if (refs.resizeTimeoutRef.current) clearTimeout(refs.resizeTimeoutRef.current);
    refs.resizeTimeoutRef.current = setTimeout(() => {
      fitAndResize();
    }, 100);
  };
  const resizeObserver = new ResizeObserver(handleResize);
  resizeObserver.observe(termContainer);
  return () => {
    log("Terminal cleanup");
    if (refs.resizeTimeoutRef.current) clearTimeout(refs.resizeTimeoutRef.current);
    resizeObserver.disconnect();
    if (refs.webglAddonRef.current) {
      refs.webglAddonRef.current.dispose();
      refs.webglAddonRef.current = null;
    }
    terminal.dispose();
    refs.xtermRef.current = null;
    refs.fitAddonRef.current = null;
    refs.isInitializedRef.current = false;
    refs.lastDimensionsRef.current = { cols: 0, rows: 0 };
  };
}

function useTerminalInit({
  terminalRef,
  xtermRef,
  fitAddonRef,
  isInitializedRef,
  lastDimensionsRef,
  resizeTimeoutRef,
  webglAddonRef,
  fitAndResize,
}: TerminalInitOptions) {
  const refs = {
    xtermRef,
    fitAddonRef,
    isInitializedRef,
    lastDimensionsRef,
    resizeTimeoutRef,
    webglAddonRef,
  };
  useEffect(() => {
    log("Terminal init effect");
    const container = terminalRef.current;
    if (!container) {
      log("No container ref");
      return;
    }
    if (isInitializedRef.current) {
      log("Already initialized");
      return;
    }
    const initWhenReady = () => {
      const rect = container.getBoundingClientRect();
      log("Init check: dimensions", rect.width, "x", rect.height);
      if (rect.width >= MIN_WIDTH && rect.height >= MIN_HEIGHT) {
        initTerminalInstance(container, refs, fitAndResize);
        return true;
      }
      return false;
    };
    if (!initWhenReady()) {
      let retryCount = 0;
      const maxRetries = 30;
      const retry = () => {
        if (isInitializedRef.current) return;
        retryCount++;
        if (initWhenReady() || retryCount >= maxRetries) {
          if (retryCount >= maxRetries) {
            log("Max retries, forcing init");
            initTerminalInstance(container, refs, fitAndResize);
          }
          return;
        }
        requestAnimationFrame(retry);
      };
      requestAnimationFrame(retry);
    }
    const resizeTimeout = resizeTimeoutRef;
    const webgl = webglAddonRef;
    const xterm = xtermRef;
    const fitAddon = fitAddonRef;
    return () => {
      log("Effect cleanup");
      if (resizeTimeout.current) clearTimeout(resizeTimeout.current);
      if (webgl.current) {
        webgl.current.dispose();
        webgl.current = null;
      }
      if (xterm.current) {
        xterm.current.dispose();
        xterm.current = null;
      }
      fitAddon.current = null;
      isInitializedRef.current = false;
      lastDimensionsRef.current = { cols: 0, rows: 0 };
    };
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [fitAndResize]);
}

type WebSocketConnectionOptions = {
  taskId: string | null;
  sessionId: string | null | undefined;
  canConnect: boolean;
  fitAndResize: (force?: boolean) => void;
  wsBaseUrl: string;
  mode: "agent" | "shell";
  terminalId: string | undefined;
  label?: string;
  xtermRef: React.MutableRefObject<Terminal | null>;
  fitAddonRef: React.MutableRefObject<FitAddon | null>;
  wsRef: React.MutableRefObject<WebSocket | null>;
  attachAddonRef: React.MutableRefObject<AttachAddon | null>;
};

function buildWsUrl(
  wsBaseUrl: string,
  sessionId: string,
  mode: "agent" | "shell",
  terminalId: string | undefined,
  label?: string,
): string {
  let wsUrl =
    mode === "agent"
      ? `${wsBaseUrl}/terminal/${sessionId}?mode=agent`
      : `${wsBaseUrl}/terminal/${sessionId}?mode=shell&terminalId=${encodeURIComponent(terminalId!)}`;
  if (label) wsUrl += `&label=${encodeURIComponent(label)}`;
  return wsUrl;
}

type ConnectWebSocketOptions = {
  sessionId: string;
  wsBaseUrl: string;
  mode: "agent" | "shell";
  terminalId: string | undefined;
  label: string | undefined;
  terminal: Terminal;
  fitAndResize: (force?: boolean) => void;
  wsRef: React.MutableRefObject<WebSocket | null>;
  attachAddonRef: React.MutableRefObject<AttachAddon | null>;
  isMountedCheck: () => boolean;
  onInterval: (id: ReturnType<typeof setInterval>) => void;
};

function connectWebSocket({
  sessionId,
  wsBaseUrl,
  mode,
  terminalId,
  label,
  terminal,
  fitAndResize,
  wsRef,
  attachAddonRef,
  isMountedCheck,
  onInterval,
}: ConnectWebSocketOptions) {
  if (attachAddonRef.current) {
    attachAddonRef.current.dispose();
    attachAddonRef.current = null;
  }
  if (wsRef.current) {
    wsRef.current.close();
    wsRef.current = null;
  }
  const wsUrl = buildWsUrl(wsBaseUrl, sessionId, mode, terminalId, label);
  log("Connecting to", wsUrl, { mode, terminalId, label });
  const ws = new WebSocket(wsUrl);
  ws.binaryType = "arraybuffer";
  wsRef.current = ws;
  ws.onopen = () => {
    if (!isMountedCheck()) {
      ws.close();
      return;
    }
    log("WebSocket connected");
    const attachAddon = new AttachAddon(ws, { bidirectional: true });
    terminal.loadAddon(attachAddon);
    attachAddonRef.current = attachAddon;
    const triggerRefresh = () => {
      if (isMountedCheck() && ws.readyState === WebSocket.OPEN) fitAndResize(true);
    };
    requestAnimationFrame(triggerRefresh);
    setTimeout(triggerRefresh, 100);
    setTimeout(triggerRefresh, 300);
    setTimeout(triggerRefresh, 500);
    let refreshCount = 0;
    const intervalId = setInterval(() => {
      refreshCount++;
      if (refreshCount > 5 || !isMountedCheck()) {
        clearInterval(intervalId);
        return;
      }
      triggerRefresh();
    }, 1000);
    onInterval(intervalId);
  };
  ws.onclose = (event) => {
    log("WebSocket closed:", event.code, event.reason);
    if (attachAddonRef.current) {
      attachAddonRef.current.dispose();
      attachAddonRef.current = null;
    }
  };
  ws.onerror = (error) => {
    log("WebSocket error:", error);
  };
}

function useWebSocketConnection({
  taskId,
  sessionId,
  canConnect,
  fitAndResize,
  wsBaseUrl,
  mode,
  terminalId,
  label,
  xtermRef,
  fitAddonRef,
  wsRef,
  attachAddonRef,
}: WebSocketConnectionOptions) {
  useEffect(() => {
    log("WebSocket effect:", {
      taskId,
      sessionId,
      mode,
      terminalId,
      canConnect,
      hasTerminal: !!xtermRef.current,
    });
    if (!taskId || !sessionId || !canConnect) {
      log("WebSocket effect: early return", { taskId, sessionId, canConnect });
      return;
    }
    if (!xtermRef.current || !fitAddonRef.current) {
      log("Terminal not ready for WebSocket");
      return;
    }
    const terminal = xtermRef.current;
    let isMounted = true;
    let connectTimeout: ReturnType<typeof setTimeout> | null = null;
    let refreshInterval: ReturnType<typeof setInterval> | null = null;
    connectTimeout = setTimeout(() => {
      if (!isMounted) return;
      connectWebSocket({
        sessionId,
        wsBaseUrl,
        mode,
        terminalId,
        label,
        terminal,
        fitAndResize,
        wsRef,
        attachAddonRef,
        isMountedCheck: () => isMounted,
        onInterval: (id) => {
          refreshInterval = id;
        },
      });
    }, 150);
    return () => {
      log("WebSocket cleanup");
      isMounted = false;
      if (connectTimeout) clearTimeout(connectTimeout);
      if (refreshInterval) clearInterval(refreshInterval);
      if (attachAddonRef.current) {
        attachAddonRef.current.dispose();
        attachAddonRef.current = null;
      }
      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [
    taskId,
    sessionId,
    canConnect,
    fitAndResize,
    wsBaseUrl,
    mode,
    terminalId,
    label,
    xtermRef,
    fitAddonRef,
    wsRef,
    attachAddonRef,
  ]);
}

function buildResizeBuffer(cols: number, rows: number): Uint8Array {
  const json = JSON.stringify({ cols, rows });
  const encoder = new TextEncoder();
  const jsonBytes = encoder.encode(json);
  const buffer = new Uint8Array(1 + jsonBytes.length);
  buffer[0] = 0x01;
  buffer.set(jsonBytes, 1);
  return buffer;
}

function useSendResize(wsRef: React.MutableRefObject<WebSocket | null>) {
  return useCallback(
    (cols: number, rows: number) => {
      const ws = wsRef.current;
      if (!ws || ws.readyState !== WebSocket.OPEN) {
        log("sendResize: WebSocket not ready", ws?.readyState);
        return;
      }
      if (cols <= 0 || rows <= 0) {
        log("sendResize: invalid dimensions", cols, rows);
        return;
      }
      log("sendResize:", cols, "x", rows);
      ws.send(buildResizeBuffer(cols, rows));
    },
    [wsRef],
  );
}

type FitAndResizeOptions = {
  xtermRef: React.MutableRefObject<Terminal | null>;
  fitAddonRef: React.MutableRefObject<FitAddon | null>;
  terminalRef: React.RefObject<HTMLDivElement | null>;
  lastDimensionsRef: React.MutableRefObject<{ cols: number; rows: number }>;
  sendResize: (cols: number, rows: number) => void;
};

function useFitAndResize({
  xtermRef,
  fitAddonRef,
  terminalRef,
  lastDimensionsRef,
  sendResize,
}: FitAndResizeOptions) {
  return useCallback(
    (force = false) => {
      const terminal = xtermRef.current;
      const fitAddon = fitAddonRef.current;
      const container = terminalRef.current;
      if (!terminal || !fitAddon || !container) {
        log("fitAndResize: missing refs");
        return;
      }
      const rect = container.getBoundingClientRect();
      if (rect.width < MIN_WIDTH || rect.height < MIN_HEIGHT) {
        log("fitAndResize: container too small, skipping");
        return;
      }
      try {
        fitAddon.fit();
        log("fitAndResize: fit done", terminal.cols, "x", terminal.rows);
      } catch (e) {
        log("fitAndResize: fit failed", e);
        return;
      }
      const { cols, rows } = terminal;
      const last = lastDimensionsRef.current;
      if (force || cols !== last.cols || rows !== last.rows) {
        lastDimensionsRef.current = { cols, rows };
        sendResize(cols, rows);
      }
    },
    [xtermRef, fitAddonRef, terminalRef, lastDimensionsRef, sendResize],
  );
}

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
  const { sessionId: propSessionId, mode, label } = props;
  const terminalId = mode === "shell" ? props.terminalId : undefined;
  log("Render - props:", { propSessionId, mode, terminalId, label });

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
      const backendUrl = getBackendConfig().apiBaseUrl;
      const url = new URL(backendUrl);
      const protocol = url.protocol === "https:" ? "wss:" : "ws:";
      return `${protocol}//${url.host}`;
    } catch {
      return "ws://localhost:8080";
    }
  }, []);

  const sendResize = useSendResize(wsRef);
  const fitAndResize = useFitAndResize({
    xtermRef,
    fitAddonRef,
    terminalRef,
    lastDimensionsRef,
    sendResize,
  });

  useTerminalInit({
    terminalRef,
    xtermRef,
    fitAddonRef,
    isInitializedRef,
    lastDimensionsRef,
    resizeTimeoutRef,
    webglAddonRef,
    fitAndResize,
  });
  useWebSocketConnection({
    taskId,
    sessionId,
    canConnect,
    fitAndResize,
    wsBaseUrl,
    mode,
    terminalId,
    label,
    xtermRef,
    fitAddonRef,
    wsRef,
    attachAddonRef,
  });

  return (
    <div
      ref={terminalRef}
      className="h-full w-full overflow-hidden p-1"
      style={{ minWidth: MIN_WIDTH, minHeight: MIN_HEIGHT }}
    />
  );
}
