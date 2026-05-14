import { useEffect, useCallback, useRef } from "react";
import { Terminal } from "@xterm/xterm";
import { FitAddon } from "@xterm/addon-fit";
import { AttachAddon } from "@xterm/addon-attach";
import { Unicode11Addon } from "@xterm/addon-unicode11";
import { WebLinksAddon } from "@xterm/addon-web-links";
import { WebglAddon } from "@xterm/addon-webgl";
import { getTerminalTheme } from "@/lib/theme/terminal-theme";
import { startReconnectLoop, teardownWebSocket } from "./ws-reconnect";
import { matchesShortcut } from "@/lib/keyboard/utils";
import { SHORTCUTS } from "@/lib/keyboard/constants";
import { getShortcut, type StoredShortcutOverrides } from "@/lib/keyboard/shortcut-overrides";
import { exposeBufferReader, clearBufferReader } from "./terminal-buffer-reader";

// Debug flag - set to true to see detailed logs
const DEBUG = false;
export const log = (...args: unknown[]) => {
  if (DEBUG) console.log("[PassthroughTerminal]", ...args);
};

// Minimum dimensions to prevent zero-size issues
export const MIN_WIDTH = 100;
export const MIN_HEIGHT = 100;

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

type TerminalKeyHandlerOptions = {
  onToggleBottomTerminal?: () => void;
  sendInput?: (data: string) => void;
  /** Ref to keyboard shortcut overrides so the handler always reads the latest value. */
  keyboardShortcutsRef?: React.MutableRefObject<StoredShortcutOverrides | undefined>;
  /** Ref to handler called when Ctrl/Cmd+F is pressed inside the terminal. */
  onFindInPanelRef?: React.MutableRefObject<(() => void) | undefined>;
};

function isCmdArrowLine(event: KeyboardEvent): boolean {
  return (
    event.type === "keydown" &&
    event.metaKey &&
    !event.ctrlKey &&
    !event.altKey &&
    (event.key === "ArrowLeft" || event.key === "ArrowRight")
  );
}

function handleAppShortcuts(event: KeyboardEvent, options: TerminalKeyHandlerOptions): boolean {
  if (matchesShortcut(event, SHORTCUTS.FIND_IN_PANEL)) {
    event.preventDefault();
    if (event.type === "keydown") options.onFindInPanelRef?.current?.();
    return true;
  }
  if (
    matchesShortcut(event, getShortcut("BOTTOM_TERMINAL", options.keyboardShortcutsRef?.current))
  ) {
    event.preventDefault();
    if (event.type === "keydown") options.onToggleBottomTerminal?.();
    return true;
  }
  return false;
}

/** Handles app-level shortcuts and Cmd+Arrow→Home/End mapping for macOS. */
function createKeyEventHandler(options: TerminalKeyHandlerOptions) {
  return (event: KeyboardEvent): boolean => {
    if (handleAppShortcuts(event, options)) return false;
    if (isCmdArrowLine(event)) {
      event.preventDefault();
      // Ctrl+A (0x01) = beginning-of-line, Ctrl+E (0x05) = end-of-line
      // Works universally in bash/zsh emacs mode (the default)
      const seq = event.key === "ArrowLeft" ? "\x01" : "\x05";
      options.sendInput?.(seq);
      return false;
    }
    return true;
  };
}

/**
 * Defer WebGL addon loading to the next animation frame so the initial
 * synchronous work (Terminal + FitAddon + open) stays within the browser's
 * frame budget.
 *
 * Skips WebGL on Firefox: its canvas fingerprinting protection silently
 * poisons readback data, corrupting the glyph texture atlas.
 */
function deferWebGLAddon(refs: Pick<TerminalInitOptions, "xtermRef" | "webglAddonRef">) {
  const isFirefox = typeof navigator !== "undefined" && /firefox/i.test(navigator.userAgent);
  if (isFirefox) {
    log("Skipping WebGL addon on Firefox (canvas fingerprinting protection)");
    return;
  }
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
}

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
  options: {
    linkHandler?: (event: MouseEvent, uri: string) => void;
    fontFamily?: string;
    fontSize?: number;
    disableWebgl?: boolean;
  } & TerminalKeyHandlerOptions,
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
    fontSize: options.fontSize || 13,
    fontFamily: options.fontFamily || 'Menlo, Monaco, "Courier New", monospace',
    macOptionIsMeta: true,
    theme: getTerminalTheme(termContainer),
  });
  const fitAddon = new FitAddon();
  terminal.loadAddon(fitAddon);
  const unicode11Addon = new Unicode11Addon();
  terminal.loadAddon(unicode11Addon);
  terminal.unicode.activeVersion = "11";
  const webLinksAddon = new WebLinksAddon(options.linkHandler);
  terminal.loadAddon(webLinksAddon);
  terminal.open(termContainer);
  terminal.attachCustomKeyEventHandler(createKeyEventHandler(options));
  try {
    fitAddon.fit();
    refs.lastDimensionsRef.current = { cols: terminal.cols, rows: terminal.rows };
  } catch {
    /* fit failed */
  }
  refs.xtermRef.current = terminal;
  refs.fitAddonRef.current = fitAddon;
  if (!options.disableWebgl) deferWebGLAddon(refs);
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
    clearBufferReader(termContainer);
    refs.xtermRef.current = null;
    refs.fitAddonRef.current = null;
    refs.isInitializedRef.current = false;
    refs.lastDimensionsRef.current = { cols: 0, rows: 0 };
  };
}

type TerminalInitHookOptions = TerminalInitOptions &
  TerminalKeyHandlerOptions & {
    linkHandler?: (event: MouseEvent, uri: string) => void;
    fontFamily?: string;
    fontSize?: number;
    disableWebgl?: boolean;
  };

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
  fontFamily,
  fontSize,
  disableWebgl,
  onToggleBottomTerminal,
  sendInput,
  keyboardShortcutsRef,
  onFindInPanelRef,
}: TerminalInitHookOptions) {
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
        initTerminalInstance(container, refs, fitAndResize, {
          linkHandler,
          fontFamily,
          fontSize,
          disableWebgl,
          onToggleBottomTerminal,
          sendInput,
          keyboardShortcutsRef,
          onFindInPanelRef,
        });
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
  environmentId?: string | null | undefined;
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
  onDisconnected?: () => void;
  /** When true, AttachAddon is created in receive-only mode so callers can
   * intercept onData themselves (used by mobile to route input through the
   * key-bar's modifier transform). Defaults to bidirectional. */
  manualInputRouting?: boolean;
  /** Fires when the WebSocket reaches the OPEN state. Use to register a sender
   * that bypasses xterm.onData (mobile key-bar registry). */
  onWsReady?: (ws: WebSocket) => void;
};

export function buildTerminalWsUrl(
  wsBaseUrl: string,
  params: {
    mode: "agent" | "shell";
    sessionId?: string;
    environmentId?: string;
    terminalId?: string;
    label?: string;
  },
): string {
  const { mode, sessionId, environmentId, terminalId, label } = params;
  let wsUrl: string;
  if (mode === "agent") {
    if (!sessionId) throw new Error("sessionId is required for agent terminal");
    wsUrl = `${wsBaseUrl}/terminal/session/${encodeURIComponent(sessionId)}?mode=agent`;
  } else {
    if (!environmentId) throw new Error("environmentId is required for shell terminal");
    if (!terminalId) throw new Error("terminalId is required for shell terminal");
    wsUrl = `${wsBaseUrl}/terminal/environment/${encodeURIComponent(environmentId)}?terminalId=${encodeURIComponent(terminalId)}`;
  }
  if (label) wsUrl += `&label=${encodeURIComponent(label)}`;
  return wsUrl;
}

type ConnectWebSocketOptions = {
  sessionId?: string;
  environmentId?: string;
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
  manualInputRouting?: boolean;
  onWsReady?: (ws: WebSocket) => void;
};

function connectWebSocket({
  sessionId,
  environmentId,
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
  manualInputRouting,
  onWsReady,
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
  const wsUrl = buildTerminalWsUrl(wsBaseUrl, {
    mode,
    sessionId,
    environmentId,
    terminalId,
    label,
  });
  log("Connecting to", wsUrl, { mode, sessionId, environmentId, terminalId, label });
  const ws = new WebSocket(wsUrl);
  ws.binaryType = "arraybuffer";
  wsRef.current = ws;
  ws.onopen = () => {
    if (!isMountedCheck()) {
      ws.close();
      return;
    }
    log("WebSocket connected");
    // Mobile passes manualInputRouting=true so the consumer can intercept
    // onData and apply key-bar modifier transforms before the bytes go on the
    // wire — AttachAddon's auto-send would otherwise bypass that path.
    const attachAddon = new AttachAddon(ws, { bidirectional: !manualInputRouting });
    terminal.loadAddon(attachAddon);
    attachAddonRef.current = attachAddon;
    onWsReady?.(ws);
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
}: WebSocketConnectionOptions) {
  const taskIdRef = useRef(taskId);

  useEffect(() => {
    taskIdRef.current = taskId;
  }, [taskId]);

  useEffect(() => {
    const connectionKey = mode === "agent" ? sessionId : environmentId;
    const taskIdForLog = taskIdRef.current;
    log("WebSocket effect:", {
      taskId: taskIdForLog,
      sessionId,
      environmentId,
      mode,
      terminalId,
      canConnect,
      isTerminalReady,
      hasTerminal: !!xtermRef.current,
    });
    // Agent mode routes by sessionId (carried via connectionKey); shell mode
    // routes by env id (also carried via connectionKey). taskId is only used
    // for logging — gating on it would force the WS to wait for useSession()
    // hydration after refresh, which doesn't guard any real precondition.
    if (!connectionKey || !canConnect || !isTerminalReady) {
      log("WebSocket effect: early return", {
        taskId: taskIdForLog,
        connectionKey,
        canConnect,
        isTerminalReady,
      });
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
    log("Resetting terminal buffer for terminal target", connectionKey);
    terminal.reset();

    const stopReconnectLoop = startReconnectLoop({
      sessionId: sessionId ?? undefined,
      environmentId: environmentId ?? undefined,
      wsBaseUrl,
      mode,
      terminalId,
      label,
      terminal,
      fitAndResize,
      wsRef,
      attachAddonRef,
      onConnected,
      onDisconnected,
      connectWebSocket,
      manualInputRouting,
      onWsReady,
    });
    return () => teardownWebSocket(stopReconnectLoop, attachAddonRef, wsRef);
    // eslint-disable-next-line react-hooks/exhaustive-deps -- manualInputRouting/onWsReady should not retrigger reconnect
  }, [
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

/** Returns a stable ref whose `.current` sends raw string data to the PTY via WebSocket. */
export function useSendInput(wsRef: React.MutableRefObject<WebSocket | null>) {
  return useCallback(
    (data: string) => {
      const ws = wsRef.current;
      if (ws && ws.readyState === WebSocket.OPEN) {
        ws.send(new TextEncoder().encode(data));
      }
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
