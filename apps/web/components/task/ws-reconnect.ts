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
          if (stableOpenTimeout) {
            clearTimeout(stableOpenTimeout);
            stableOpenTimeout = null;
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
