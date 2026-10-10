import type { Page } from "@playwright/test";

type WireFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: unknown;
};

type LaunchPayload = {
  task_id?: unknown;
  session_id?: unknown;
  intent?: unknown;
};

export type SessionOpenInspectionContentionProxy = {
  resumeRequestCount: () => number;
  interceptedResumeCount: () => number;
  forwardedResumeResponses: () => string[];
  restoreRequestCount: () => number;
  scopedActions: () => string[];
  allowResumeRetries: () => void;
};

function parseFrame(value: string): WireFrame | null {
  try {
    const frame = JSON.parse(value) as unknown;
    return typeof frame === "object" && frame !== null ? (frame as WireFrame) : null;
  } catch {
    return null;
  }
}

function launchPayload(frame: WireFrame | null): LaunchPayload | null {
  if (!frame || typeof frame.payload !== "object" || frame.payload === null) return null;
  return frame.payload as LaunchPayload;
}

type InspectionProxyState = {
  resumeRequestCount: number;
  interceptedResumeCount: number;
  forwardedResumeRequestIds: Set<string>;
  forwardedResumeResponses: string[];
  restoreRequestCount: number;
  retriesAllowed: boolean;
  scopedActions: string[];
};

function isResumeFrame(frame: WireFrame | null, payload: LaunchPayload | null): boolean {
  return (
    (frame?.action === "session.recover" && payload?.action === "resume") ||
    (frame?.action === "session.launch" && payload?.intent === "resume")
  );
}

function isRestoreFrame(frame: WireFrame | null, payload: LaunchPayload | null): boolean {
  return (
    (frame?.action === "session.recover" && payload?.action === "restore") ||
    (frame?.action === "session.launch" && payload?.intent === "restore_workspace")
  );
}

function isScopedRecoveryFrame(
  frame: WireFrame | null,
  payload: LaunchPayload | null,
  scope: { taskId: string; sessionId: string },
): frame is WireFrame & { id: string; action: string } {
  return (
    typeof frame?.id === "string" &&
    frame.type === "request" &&
    (frame.action === "session.recover" || frame.action === "session.launch") &&
    payload?.task_id === scope.taskId &&
    payload.session_id === scope.sessionId
  );
}

function sendBusyResponse(
  frame: WireFrame & { id: string; action: string },
  state: InspectionProxyState,
  pendingDelayMs: number,
  send: (message: string) => void,
): void {
  state.interceptedResumeCount += 1;
  const response = JSON.stringify({
    id: frame.id,
    action: frame.action,
    type: "error",
    payload: {
      code: "CONFLICT",
      message: "workspace recovery inspection is busy",
      details: { kind: "recovery_inspection_busy" },
    },
  });
  const delay = state.interceptedResumeCount === 1 ? pendingDelayMs : 0;
  if (delay > 0) setTimeout(() => send(response), delay);
  else send(response);
}

function handleClientFrame(
  part: string,
  ws: import("@playwright/test").WebSocketRoute,
  state: InspectionProxyState,
  scope: { taskId: string; sessionId: string; pendingDelayMs?: number },
): "forward" | "intercept" {
  const frame = parseFrame(part.trim());
  const payload = launchPayload(frame);
  if (!isScopedRecoveryFrame(frame, payload, scope)) return "forward";

  state.scopedActions.push(
    `${frame.action}:${String(payload?.intent ?? payload?.action ?? "unknown")}`,
  );
  if (isRestoreFrame(frame, payload)) state.restoreRequestCount += 1;
  if (!isResumeFrame(frame, payload)) return "forward";

  state.resumeRequestCount += 1;
  if (state.retriesAllowed) {
    state.forwardedResumeRequestIds.add(frame.id);
    return "forward";
  }

  sendBusyResponse(frame, state, scope.pendingDelayMs ?? 1000, (response) => ws.send(response));
  return "intercept";
}

function forwardResumeResponses(message: unknown, state: InspectionProxyState): void {
  if (typeof message !== "string") return;
  for (const part of message.split("\n")) {
    const frame = parseFrame(part.trim());
    if (
      typeof frame?.id === "string" &&
      state.forwardedResumeRequestIds.has(frame.id) &&
      (frame.type === "response" || frame.type === "error")
    ) {
      state.forwardedResumeResponses.push(part.trim());
    }
  }
}

function forwardServerMessage(
  message: unknown,
  ws: import("@playwright/test").WebSocketRoute,
  state: InspectionProxyState,
): void {
  forwardResumeResponses(message, state);
  ws.send(message);
}

function routeClientMessage(
  message: unknown,
  server: import("@playwright/test").WebSocketRoute,
  ws: import("@playwright/test").WebSocketRoute,
  state: InspectionProxyState,
  scope: { taskId: string; sessionId: string; pendingDelayMs?: number },
): void {
  if (typeof message !== "string") {
    server.send(message);
    return;
  }

  let intercepted = false;
  const forwarded = message.split("\n").filter((part) => {
    const disposition = handleClientFrame(part, ws, state, scope);
    if (disposition === "intercept") intercepted = true;
    return disposition === "forward";
  });
  if (!intercepted) server.send(message);
  else if (forwarded.some((part) => part.trim() !== "")) server.send(forwarded.join("\n"));
}

function connectInspectionWebSocket(
  ws: import("@playwright/test").WebSocketRoute,
  state: InspectionProxyState,
  scope: { taskId: string; sessionId: string; pendingDelayMs?: number },
): void {
  const server = ws.connectToServer();
  ws.onMessage((message) => routeClientMessage(message, server, ws, state, scope));
  server.onMessage((message) => forwardServerMessage(message, ws, state));
}

/** Refuse matching resumes before the backend sees them, until the test releases a user retry. */
export async function routeSessionOpenInspectionContention(
  page: Page,
  scope: { taskId: string; sessionId: string; pendingDelayMs?: number },
): Promise<SessionOpenInspectionContentionProxy> {
  const state: InspectionProxyState = {
    resumeRequestCount: 0,
    interceptedResumeCount: 0,
    forwardedResumeRequestIds: new Set(),
    forwardedResumeResponses: [],
    restoreRequestCount: 0,
    retriesAllowed: false,
    scopedActions: [],
  };

  await page.routeWebSocket(/\/ws$/, (ws) => connectInspectionWebSocket(ws, state, scope));

  return {
    resumeRequestCount: () => state.resumeRequestCount,
    interceptedResumeCount: () => state.interceptedResumeCount,
    forwardedResumeResponses: () => [...state.forwardedResumeResponses],
    restoreRequestCount: () => state.restoreRequestCount,
    scopedActions: () => [...state.scopedActions],
    allowResumeRetries: () => {
      state.retriesAllowed = true;
    },
  };
}
