import { useEffect, useCallback } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { AttachAddon } from "@xterm/addon-attach";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { getTerminalTheme } from "@/lib/theme/terminal-theme";
import { startReconnectLoop } from "./ws-reconnect";

// Debug flag - set to true to see detailed logs
const DEBUG = false;
export const log = (...args: unknown[]) => {
  if (DEBUG) console.log("[PassthroughTerminal]", ...args);
};

// Minimum dimensions to prevent zero-size issues
export const MIN_WIDTH = 100;
export const MIN_HEIGHT = 100;

type TerminalContainerWithBuffer = HTMLDivElement & { __xtermReadBuffer?: () => string };

/** Expose buffer reader on the container for e2e tests (xterm renders to canvas). */
function exposeBufferReader(container: HTMLDivElement, terminal: Terminal) {
  (container as TerminalContainerWithBuffer).__xtermReadBuffer = () => {
    const buf = terminal.buffer.active;
    const lines: string[] = [];
    for (let i = 0; i <= buf.baseY + buf.cursorY; i++) {
      lines.push(buf.getLine(i)?.translateToString(true) ?? "");
    }
    return lines.join("\n");
  };
}

export type TerminalInitOptions = {
  terminalRef: React.RefObject<HTMLDivElement | null>;
  xtermRef: React.MutableRefObject<Terminal | null>;
  fitAddonRef: React.MutableRefObject<FitAddon | null>;
  isInitializedRef: React.MutableRefObject<boolean>;
  lastDimensionsRef: React.MutableRefObject<{ cols: number; rows: number }>;
  resizeTimeoutRef: React.MutableRefObject<ReturnType<typeof setTimeout> | null>;
  webglAddonRef: React.MutableRefObject<WebglAddon | null>;
  fitAndResize: (force?: boolean) => void;
  onReady: () => void;
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
  linkHandler?: (event: MouseEvent, uri: string) => void,
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
  const webLinksAddon = new WebLinksAddon(linkHandler);
  terminal.loadAddon(webLinksAddon);
  log("Opening terminal in container");
  terminal.open(termContainer);
  try {
    fitAddon.fit();
    refs.lastDimensionsRef.current = { cols: terminal.cols, rows: terminal.rows };
    log("Initial fit:", terminal.cols, "x", terminal.rows);
  } catch (e) {
    log("Initial fit failed:", e);
  }
  refs.xtermRef.current = terminal;
  refs.fitAddonRef.current = fitAddon;
  // Defer WebGL addon loading to the next animation frame so the initial
  // synchronous work (Terminal + FitAddon + open) stays within the browser's
  // frame budget and avoids "Violation: 'setTimeout' handler took Xms" warnings.
  requestAnimationFrame(() => {
    const term = refs.xtermRef.current;
    if (!term || refs.webglAddonRef.current) return;
    try {
      const webglAddon = new WebglAddon();
      webglAddon.onContextLoss(() => {
        log("WebGL context lost");
        webglAddon.dispose();
        refs.webglAddonRef.current = null;
      });
      term.loadAddon(webglAddon);
      refs.webglAddonRef.current = webglAddon;
      log("WebGL addon loaded");
    } catch (e) {
      log("WebGL failed, using canvas:", e);
    }
  });
  exposeBufferReader(termContainer, terminal);
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
    (termContainer as TerminalContainerWithBuffer).__xtermReadBuffer = undefined;
    refs.xtermRef.current = null;
    refs.fitAddonRef.current = null;
    refs.isInitializedRef.current = false;
    refs.lastDimensionsRef.current = { cols: 0, rows: 0 };
  };
}

export function useTerminalInit({
  terminalRef,
  xtermRef,
  fitAddonRef,
  isInitializedRef,
  lastDimensionsRef,
  resizeTimeoutRef,
  webglAddonRef,
  fitAndResize,
  onReady,
  linkHandler,
}: TerminalInitOptions & { linkHandler?: (event: MouseEvent, uri: string) => void }) {
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

    const tryInit = () => {
      if (isInitializedRef.current) return true;
      const rect = container.getBoundingClientRect();
      log("Init check: dimensions", rect.width, "x", rect.height);
      if (rect.width >= MIN_WIDTH && rect.height >= MIN_HEIGHT) {
        initTerminalInstance(container, refs, fitAndResize, linkHandler);
        onReady();
        return true;
      }
      return false;
    };

    // If the container already has dimensions, init immediately.
    if (tryInit()) {
      // Already initialized — cleanup handled by initTerminalInstance's return
    } else {
      // Container is 0×0 (e.g. terminal is in a background dockview tab).
      // Use a ResizeObserver to wait until the container becomes visible
      // instead of polling with rAF and force-initializing at 0×0
      // (which causes black backgrounds since CSS vars can't be read).
      log("Container not visible, waiting via ResizeObserver");
      const observer = new ResizeObserver(() => {
        if (tryInit()) {
          observer.disconnect();
        }
      });
      observer.observe(container);
      return () => {
        observer.disconnect();
      };
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

export type WebSocketConnectionOptions = {
  taskId: string | null;
  sessionId: string | null | undefined;
  canConnect: boolean;
  isTerminalReady: boolean;
  fitAndResize: (force?: boolean) => void;
  wsBaseUrl: string;
  mode: "agent" | "shell";
  terminalId: string | undefined;
  label?: string;
  xtermRef: React.MutableRefObject<Terminal | null>;
  fitAddonRef: React.MutableRefObject<FitAddon | null>;
  wsRef: React.MutableRefObject<WebSocket | null>;
  attachAddonRef: React.MutableRefObject<AttachAddon | null>;
  onConnected: () => void;
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
  onTimeout: (id: ReturnType<typeof setTimeout>) => void;
  onConnected: () => void;
  onSocketClose: (event: CloseEvent) => void;
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
  onTimeout,
  onConnected,
  onSocketClose,
}: ConnectWebSocketOptions) {
  if (attachAddonRef.current) {
    attachAddonRef.current.dispose();
    attachAddonRef.current = null;
  }
  if (wsRef.current) {
    if (
      wsRef.current.readyState === WebSocket.OPEN ||
      wsRef.current.readyState === WebSocket.CLOSING
    ) {
      wsRef.current.close();
    }
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
    onConnected();
    // Send initial resize (forced) so the backend knows our terminal dimensions,
    // then one deferred resize to catch layout settling + force a full redraw.
    // The WebGL renderer can become stale when the container transitions through
    // 0×0 (portal system detach/reattach), so we must explicitly refresh.
    requestAnimationFrame(() => {
      if (isMountedCheck() && ws.readyState === WebSocket.OPEN) {
        fitAndResize(true);
      }
    });
    const settleTimeout = setTimeout(() => {
      if (!isMountedCheck()) return;
      if (ws.readyState === WebSocket.OPEN) {
        fitAndResize(true);
      }
      // Force full redraw — WebGL canvas may be stale after portal 0×0 transition
      terminal.refresh(0, terminal.rows - 1);
    }, 500);
    onTimeout(settleTimeout);
  };
  ws.onclose = (event) => {
    log("WebSocket closed:", event.code, event.reason);
    if (attachAddonRef.current) {
      attachAddonRef.current.dispose();
      attachAddonRef.current = null;
    }
    onSocketClose(event);
  };
  ws.onerror = (error) => {
    log("WebSocket error:", error);
  };
}

// reconnectDelayMs and startReconnectLoop are in ws-reconnect.ts
// Re-export reconnectDelayMs for tests that import from this module.
export { reconnectDelayMs } from "./ws-reconnect";

export function useWebSocketConnection({
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
}: WebSocketConnectionOptions) {
  useEffect(() => {
    log("WebSocket effect:", {
      taskId,
      sessionId,
      mode,
      terminalId,
      canConnect,
      isTerminalReady,
      hasTerminal: !!xtermRef.current,
    });
    if (!taskId || !sessionId || !canConnect || !isTerminalReady) {
      log("WebSocket effect: early return", { taskId, sessionId, canConnect, isTerminalReady });
      return;
    }
    if (!xtermRef.current || !fitAddonRef.current) {
      log("Terminal not ready for WebSocket (refs missing despite isTerminalReady)");
      return;
    }
    const terminal = xtermRef.current;

    // Clear the terminal buffer before reconnecting.  The chat panel is global
    // (not session-scoped), so the same xterm instance persists across session
    // switches.  Without this reset, the previous session's PTY output leaks
    // into the new session's scrollback.
    log("Resetting terminal buffer for session", sessionId);
    terminal.reset();

    const stopReconnectLoop = startReconnectLoop({
      sessionId,
      wsBaseUrl,
      mode,
      terminalId,
      label,
      terminal,
      fitAndResize,
      wsRef,
      attachAddonRef,
      onConnected,
      connectWebSocket,
    });
    return () => {
      log("WebSocket cleanup");
      stopReconnectLoop();
      if (attachAddonRef.current) {
        attachAddonRef.current.dispose();
        attachAddonRef.current = null;
      }
      if (wsRef.current) {
        // Only close if the connection is actually open or has completed
        // opening. Closing a CONNECTING WebSocket triggers a browser warning
        // ("WebSocket is closed before the connection is established").
        if (
          wsRef.current.readyState === WebSocket.OPEN ||
          wsRef.current.readyState === WebSocket.CLOSING
        ) {
          wsRef.current.close();
        }
        wsRef.current = null;
      }
    };
  }, [
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

export function useSendResize(wsRef: React.MutableRefObject<WebSocket | null>) {
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

export type FitAndResizeOptions = {
  xtermRef: React.MutableRefObject<Terminal | null>;
  fitAddonRef: React.MutableRefObject<FitAddon | null>;
  terminalRef: React.RefObject<HTMLDivElement | null>;
  lastDimensionsRef: React.MutableRefObject<{ cols: number; rows: number }>;
  sendResize: (cols: number, rows: number) => void;
};

export function useFitAndResize({
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
      const changed = cols !== last.cols || rows !== last.rows;
      const wasZero = last.cols === 0 && last.rows === 0;
      if (force || changed) {
        lastDimensionsRef.current = { cols, rows };
        sendResize(cols, rows);
      }
      // Force full redraw when transitioning from uninitialized/zero dimensions —
      // the WebGL canvas may be stale after the container was at 0×0 (portal moves).
      if (wasZero || changed) {
        terminal.refresh(0, terminal.rows - 1);
      }
    },
    [xtermRef, fitAddonRef, terminalRef, lastDimensionsRef, sendResize],
  );
}
