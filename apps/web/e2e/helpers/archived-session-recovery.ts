import type { Page } from "@playwright/test";

export type GatewayRequest = {
  action?: string;
  payload: Record<string, unknown>;
};

type RecoveryFailureRouteOptions = {
  taskId: string;
  sessionId: string;
  resumeError: string;
  restoreError: string;
};

type GatewayFrame = {
  id?: string;
  type?: string;
  action?: string;
  payload?: unknown;
};

function parseGatewayFrame(value: string): GatewayFrame | null {
  try {
    const frame = JSON.parse(value) as unknown;
    return typeof frame === "object" && frame !== null ? (frame as GatewayFrame) : null;
  } catch {
    return null;
  }
}

function framePayload(frame: GatewayFrame | null): Record<string, unknown> | null {
  return typeof frame?.payload === "object" && frame.payload !== null
    ? (frame.payload as Record<string, unknown>)
    : null;
}

function recoveryErrorFrame(id: string, message: string): string {
  return JSON.stringify({
    id,
    type: "error",
    action: "session.launch",
    payload: { code: "INTERNAL_ERROR", message },
  });
}

function recoverySuccessFrame(id: string, taskId: string, sessionId: string): string {
  return JSON.stringify({
    id,
    type: "response",
    action: "session.launch",
    payload: {
      success: true,
      task_id: taskId,
      session_id: sessionId,
      state: "STARTING",
    },
  });
}

type RecoveryRouteState = {
  resumeFailurePending: boolean;
  restoreFailurePending: boolean;
  statusRequestIds: Set<string>;
};

type SocketMessage = string | Buffer;
type SocketBridge = {
  send: (message: SocketMessage) => void;
};
type ServerBridge = SocketBridge;

function isTargetStatusRequest(
  frame: GatewayFrame | null,
  payload: Record<string, unknown> | null,
  options: RecoveryFailureRouteOptions,
): frame is GatewayFrame & { id: string } {
  return (
    frame?.type === "request" &&
    typeof frame.id === "string" &&
    frame.action === "task.session.status" &&
    payload?.task_id === options.taskId &&
    payload.session_id === options.sessionId
  );
}

function isTargetLaunchRequest(
  frame: GatewayFrame | null,
  payload: Record<string, unknown> | null,
  options: RecoveryFailureRouteOptions,
): frame is GatewayFrame & { id: string } {
  return (
    frame?.type === "request" &&
    typeof frame.id === "string" &&
    frame.action === "session.launch" &&
    payload?.task_id === options.taskId &&
    payload.session_id === options.sessionId
  );
}

function interceptLaunchRequest(
  frame: GatewayFrame & { id: string },
  payload: Record<string, unknown>,
  socket: SocketBridge,
  options: RecoveryFailureRouteOptions,
  state: RecoveryRouteState,
): void {
  const intent = payload.intent;
  if (intent === "resume" && state.resumeFailurePending) {
    state.resumeFailurePending = false;
    socket.send(recoveryErrorFrame(frame.id, options.resumeError));
    return;
  }
  if (intent === "restore_workspace" && state.restoreFailurePending) {
    state.restoreFailurePending = false;
    socket.send(recoveryErrorFrame(frame.id, options.restoreError));
    return;
  }
  socket.send(recoverySuccessFrame(frame.id, options.taskId, options.sessionId));
}

function forwardClientMessage(
  message: SocketMessage,
  socket: SocketBridge,
  server: ServerBridge,
  options: RecoveryFailureRouteOptions,
  state: RecoveryRouteState,
): void {
  if (typeof message !== "string") {
    server.send(message);
    return;
  }
  const forwarded: string[] = [];
  for (const part of message.split("\n")) {
    const frame = parseGatewayFrame(part.trim());
    const payload = framePayload(frame);
    if (isTargetStatusRequest(frame, payload, options)) {
      state.statusRequestIds.add(frame.id);
      forwarded.push(part);
      continue;
    }
    if (isTargetLaunchRequest(frame, payload, options)) {
      interceptLaunchRequest(frame, payload, socket, options, state);
      continue;
    }
    if (part.trim()) forwarded.push(part);
  }
  if (forwarded.length > 0) server.send(forwarded.join("\n"));
}

function rewriteStatusResponse(
  part: string,
  options: RecoveryFailureRouteOptions,
  state: RecoveryRouteState,
): string {
  const frame = parseGatewayFrame(part.trim());
  if (
    frame?.type !== "response" ||
    frame.action !== "task.session.status" ||
    typeof frame.id !== "string" ||
    !state.statusRequestIds.delete(frame.id)
  ) {
    return part;
  }
  const payload = framePayload(frame);
  if (payload?.task_id !== options.taskId || payload.session_id !== options.sessionId) {
    return part;
  }
  return JSON.stringify({
    ...frame,
    payload: {
      ...payload,
      is_resumable: true,
      needs_resume: true,
      needs_workspace_restore: false,
    },
  });
}

function forwardServerMessage(
  message: SocketMessage,
  socket: SocketBridge,
  options: RecoveryFailureRouteOptions,
  state: RecoveryRouteState,
): void {
  if (typeof message !== "string") {
    socket.send(message);
    return;
  }
  socket.send(
    message
      .split("\n")
      .map((part) => rewriteStatusResponse(part, options, state))
      .join("\n"),
  );
}

/**
 * Bound both automatic recovery launches and make the next manual retry
 * succeed. Status responses are adjusted only for the targeted session so the
 * test exercises the real hook lifecycle without changing other gateway work.
 */
export async function routeRecoveryFailureAndRetry(
  page: Page,
  options: RecoveryFailureRouteOptions,
): Promise<void> {
  const state: RecoveryRouteState = {
    resumeFailurePending: true,
    restoreFailurePending: true,
    statusRequestIds: new Set<string>(),
  };

  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => forwardClientMessage(message, socket, server, options, state));
    server.onMessage((message) => forwardServerMessage(message, socket, options, state));
  });
}

/** Capture gateway requests emitted after the listener is attached. */
export function captureGatewayRequests(page: Page): GatewayRequest[] {
  const requests: GatewayRequest[] = [];
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", (event) => {
      if (typeof event.payload !== "string") return;
      try {
        const frame = JSON.parse(event.payload) as {
          type?: string;
          action?: string;
          payload?: Record<string, unknown>;
        };
        if (frame.type === "request") {
          requests.push({ action: frame.action, payload: frame.payload ?? {} });
        }
      } catch {
        // Ignore non-JSON gateway frames.
      }
    });
  });
  return requests;
}

export function sessionLaunchRequests(
  requests: GatewayRequest[],
  sessionId: string,
): GatewayRequest[] {
  return requests.filter(
    (request) => request.action === "session.launch" && request.payload.session_id === sessionId,
  );
}
