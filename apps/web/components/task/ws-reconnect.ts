import { Terminal } from "@xterm/xterm";
import { AttachAddon } from "@xterm/addon-attach";
import { log } from "./use-passthrough-terminal";

export function teardownWebSocket(
  stopReconnectLoop: () => void,
  attachAddonRef: React.MutableRefObject<AttachAddon | null>,
  wsRef: React.MutableRefObject<WebSocket | null>,
): void {
  log("WebSocket cleanup");
  stopReconnectLoop();
  if (attachAddonRef.current) {
    attachAddonRef.current.dispose();
    attachAddonRef.current = null;
  }
  // Only close if the connection is actually open or has completed opening.
  // Closing a CONNECTING WebSocket triggers a browser warning ("WebSocket is
  // closed before the connection is established").
  const ws = wsRef.current;
  if (ws && (ws.readyState === WebSocket.OPEN || ws.readyState === WebSocket.CLOSING)) {
    ws.close();
  }
  wsRef.current = null;
}

const STABLE_CONNECTION_MS = 500;

/**
 * Sent by the server when the session behind a terminal has ended. Nothing
 * about that recovers, so the loop must stop rather than back off. Kept in
 * sync with `closeCodeSessionTerminal` in the gateway's terminal_handler.go.
 */
export const CLOSE_CODE_SESSION_TERMINAL = 4001;

/** Close codes that mean "do not reconnect", as opposed to "not yet". */
const PERMANENT_CLOSE_CODES = new Set<number>([CLOSE_CODE_SESSION_TERMINAL]);

export function isPermanentCloseCode(code: number): boolean {
  return PERMANENT_CLOSE_CODES.has(code);
}

export function reconnectDelayMs(attempt: number): number {
  const cappedAttempt = Math.min(attempt, 5);
  return Math.min(5000, 300 * 2 ** cappedAttempt);
}

type ConnectWebSocketFn = (opts: {
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
}) => void;

export type ReconnectLoopOptions = {
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
  onConnected: () => void;
  onDisconnected?: () => void;
  /** Called instead of scheduling a retry when the server says the terminal
   * will never be available again. Receives the close reason so the caller can
   * tell the user why the terminal stopped, rather than leaving it blank. */
  onPermanentClose?: (reason: string) => void;
  connectWebSocket: ConnectWebSocketFn;
  manualInputRouting?: boolean;
  onWsReady?: (ws: WebSocket) => void;
};

export function startReconnectLoop({
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
  onConnected,
  onDisconnected,
  onPermanentClose,
  connectWebSocket,
  manualInputRouting,
  onWsReady,
}: ReconnectLoopOptions): () => void {
  let isMounted = true;
  let connectTimeout: ReturnType<typeof setTimeout> | null = null;
  let settleTimeout: ReturnType<typeof setTimeout> | null = null;
  let stableOpenTimeout: ReturnType<typeof setTimeout> | null = null;
  let retryAttempt = 0;

  const scheduleConnect = (delayMs: number) => {
    if (!isMounted) return;
    if (connectTimeout) clearTimeout(connectTimeout);
    connectTimeout = setTimeout(() => {
      if (!isMounted) return;
      // The backend replays the PTY buffer whenever a terminal WebSocket
      // reconnects. Clear the existing xterm contents first so replay replaces
      // the visible terminal instead of appending duplicate prompt lines.
      terminal.reset();
      connectWebSocket({
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
        manualInputRouting,
        onWsReady,
        isMountedCheck: () => isMounted,
        onTimeout: (id) => {
          settleTimeout = id;
        },
        onConnected: () => {
          if (stableOpenTimeout) clearTimeout(stableOpenTimeout);
          stableOpenTimeout = setTimeout(() => {
            retryAttempt = 0;
            stableOpenTimeout = null;
          }, STABLE_CONNECTION_MS);
          onConnected();
        },
        onSocketClose: (event) => {
          if (!isMounted) return;
          onDisconnected?.();
          if (stableOpenTimeout) {
            clearTimeout(stableOpenTimeout);
            stableOpenTimeout = null;
          }
          // Retrying a permanent failure is what turned a dead terminal tab
          // into an endless reconnect loop. Stop, and hand the reason up so
          // the pane can say something instead of sitting blank.
          if (isPermanentCloseCode(event.code)) {
            log("Permanent close, not reconnecting", {
              code: event.code,
              reason: event.reason,
            });
            isMounted = false;
            onPermanentClose?.(event.reason);
            return;
          }
          const nextDelay = reconnectDelayMs(retryAttempt);
          retryAttempt += 1;
          log("Scheduling reconnect", {
            attempt: retryAttempt,
            delayMs: nextDelay,
            code: event.code,
          });
          scheduleConnect(nextDelay);
        },
      });
    }, delayMs);
  };

  scheduleConnect(150);

  return () => {
    isMounted = false;
    if (connectTimeout) clearTimeout(connectTimeout);
    if (settleTimeout) clearTimeout(settleTimeout);
    if (stableOpenTimeout) clearTimeout(stableOpenTimeout);
  };
}
