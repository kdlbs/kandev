import { expect, type Page } from "@playwright/test";

type WireFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: unknown;
};

type LaunchTrace = {
  action: string;
  intent: string;
  requestAt: string;
  responseAt: string;
  response: Record<string, unknown>;
};

export type WorkspaceStatusEvent = {
  receivedAt: string;
  receivedAtMs: number;
  payload: Record<string, unknown>;
};

type HeldRequest = {
  frame: string;
  server: BridgeSocket;
};

type BridgeSocket = {
  send(message: string | Buffer): void;
  onMessage(handler: (message: string | Buffer) => void): void;
};

type PromotionTraceState = {
  taskId: string;
  sessionId: string;
  correlatedRequests: Map<string, { action: string; intent: string; requestAt: string }>;
  launches: LaunchTrace[];
  statusEvents: WorkspaceStatusEvent[];
  pendingGitReads: Map<string, string>;
  heldGitReads: HeldRequest[];
  holdGitReads: boolean;
  holdStartedAt: string | null;
  holdStartedAtMs: number | null;
  gitReadRequestsBeforeHold: number;
  gitReadResponsesBeforeHold: number;
  gitReadRequestsAfterHold: number;
  gitReadResponsesAfterHold: number;
};

export function recordValue(value: unknown): Record<string, unknown> | null {
  return typeof value === "object" && value !== null && !Array.isArray(value)
    ? (value as Record<string, unknown>)
    : null;
}

function parseFrame(value: string): WireFrame | null {
  try {
    return recordValue(JSON.parse(value)) as WireFrame | null;
  } catch {
    return null;
  }
}

function isGitReadAction(action: unknown): action is string {
  return (
    typeof action === "string" &&
    (/^session\.git\./.test(action) ||
      action === "session.cumulative_diff" ||
      action === "session.commit_diff")
  );
}

function trackLaunchRequest(
  frame: WireFrame,
  payload: Record<string, unknown> | null,
  state: PromotionTraceState,
) {
  if (
    typeof frame.id !== "string" ||
    (frame.action !== "session.launch" && frame.action !== "session.recover") ||
    payload?.task_id !== state.taskId ||
    payload.session_id !== state.sessionId
  ) {
    return;
  }
  const intent = frame.action === "session.recover" ? payload.action : payload.intent;
  state.correlatedRequests.set(frame.id, {
    action: frame.action,
    intent: typeof intent === "string" ? intent : "",
    requestAt: new Date().toISOString(),
  });
}

function trackGitReadRequest(
  frame: WireFrame,
  rawFrame: string,
  server: BridgeSocket,
  state: PromotionTraceState,
): boolean {
  if (typeof frame.id !== "string" || !isGitReadAction(frame.action)) return true;
  state.pendingGitReads.set(frame.id, frame.action);
  if (!state.holdGitReads) {
    state.gitReadRequestsBeforeHold += 1;
    return true;
  }
  state.gitReadRequestsAfterHold += 1;
  state.heldGitReads.push({ frame: rawFrame, server });
  return false;
}

function handleOutgoingFrame(
  rawFrame: string,
  frame: WireFrame | null,
  server: BridgeSocket,
  state: PromotionTraceState,
): boolean {
  if (frame?.type !== "request") return true;
  trackLaunchRequest(frame, recordValue(frame.payload), state);
  return trackGitReadRequest(frame, rawFrame, server, state);
}

function recordLaunchResponse(
  frame: WireFrame,
  payload: Record<string, unknown>,
  state: PromotionTraceState,
) {
  if (typeof frame.id !== "string") return;
  const request = state.correlatedRequests.get(frame.id);
  if (!request) return;
  state.launches.push({ ...request, responseAt: new Date().toISOString(), response: payload });
  state.correlatedRequests.delete(frame.id);
}

function recordGitReadResponse(frame: WireFrame, state: PromotionTraceState) {
  if (typeof frame.id !== "string" || !isGitReadAction(state.pendingGitReads.get(frame.id))) return;
  state.pendingGitReads.delete(frame.id);
  if (state.holdStartedAtMs === null) state.gitReadResponsesBeforeHold += 1;
  else state.gitReadResponsesAfterHold += 1;
}

function recordStatusEvent(payload: Record<string, unknown>, state: PromotionTraceState) {
  if (payload.session_id !== state.sessionId || payload.type !== "status_update") return;
  const receivedAtMs = Date.now();
  state.statusEvents.push({
    receivedAt: new Date(receivedAtMs).toISOString(),
    receivedAtMs,
    payload,
  });
}

function handleIncomingFrame(frame: WireFrame | null, state: PromotionTraceState) {
  if (!frame) return;
  const payload = recordValue(frame.payload);
  if (payload && (frame.type === "response" || frame.type === "error")) {
    recordLaunchResponse(frame, payload, state);
    recordGitReadResponse(frame, state);
  }
  if (frame.type === "notification" && frame.action === "session.git.event" && payload) {
    recordStatusEvent(payload, state);
  }
}

function forwardBridgeMessage(
  message: string | Buffer,
  target: BridgeSocket,
  onFrame: (rawFrame: string, frame: WireFrame | null) => boolean,
) {
  if (typeof message !== "string") {
    target.send(message);
    return;
  }
  const forwarded = message
    .split("\n")
    .filter((part) => part.trim())
    .filter((part) => onFrame(part.trim(), parseFrame(part.trim())));
  if (forwarded.length > 0) target.send(forwarded.join("\n"));
}

function createPromotionTrace(state: PromotionTraceState) {
  return {
    waitForActionResponse(intent: string, action: string): Promise<LaunchTrace> {
      return expect
        .poll(
          () =>
            state.launches.find((trace) => trace.intent === intent && trace.action === action) ??
            null,
          {
            timeout: 30_000,
            message: `Waiting for the correlated ${action} ${intent} response`,
          },
        )
        .not.toBeNull()
        .then(
          () => state.launches.find((trace) => trace.intent === intent && trace.action === action)!,
        );
    },
    waitForStatusEvent(
      description: string,
      predicate: (payload: Record<string, unknown>) => boolean,
      afterMs?: number,
    ): Promise<WorkspaceStatusEvent> {
      return expect
        .poll(
          () =>
            state.statusEvents.find(
              (event) =>
                (afterMs === undefined || event.receivedAtMs >= afterMs) &&
                predicate(event.payload),
            ) ?? null,
          {
            timeout: 30_000,
            message: `Waiting for the ${description} workspace stream event`,
          },
        )
        .not.toBeNull()
        .then(
          () =>
            state.statusEvents.find(
              (event) =>
                (afterMs === undefined || event.receivedAtMs >= afterMs) &&
                predicate(event.payload),
            )!,
        );
    },
    async holdGitReadsAfterDrain(): Promise<void> {
      await expect
        .poll(() => state.pendingGitReads.size, {
          timeout: 30_000,
          message: "Initial Git reads should settle before the post-mutation stream proof",
        })
        .toBe(0);
      state.holdStartedAtMs = Date.now();
      state.holdStartedAt = new Date(state.holdStartedAtMs).toISOString();
      state.holdGitReads = true;
    },
    releaseHeldGitReads(): void {
      state.holdGitReads = false;
      for (const request of state.heldGitReads.splice(0)) request.server.send(request.frame);
    },
    diagnostics() {
      return {
        holdStartedAt: state.holdStartedAt,
        gitReadRequestsBeforeHold: state.gitReadRequestsBeforeHold,
        gitReadResponsesBeforeHold: state.gitReadResponsesBeforeHold,
        gitReadRequestsAfterHold: state.gitReadRequestsAfterHold,
        gitReadResponsesAfterHold: state.gitReadResponsesAfterHold,
        heldGitReadActions: state.heldGitReads
          .map((request) => parseFrame(request.frame)?.action)
          .filter((action): action is string => typeof action === "string"),
        launches: state.launches.map((trace) => ({
          action: trace.action,
          intent: trace.intent,
          requestAt: trace.requestAt,
          responseAt: trace.responseAt,
          success: trace.response.success === true,
          agentExecutionId: trace.response.agent_execution_id,
          state: trace.response.state,
        })),
        statusEvents: state.statusEvents.map(({ receivedAt, payload }) => {
          const status = statusFromPayload(payload);
          return {
            receivedAt,
            taskId: payload.task_id,
            sessionId: payload.session_id,
            environmentId: payload.task_environment_id,
            agentExecutionId: payload.agent_id,
            statusState: status?.status_state,
            detailState: status?.detail_state,
            fileCount: Object.keys(statusFiles(status) ?? {}).length,
          };
        }),
      };
    },
  };
}

/** Observe the real browser gateway and keep later Git reads from masking a missing push. */
export async function routeWorkspaceStreamPromotion(page: Page, taskId: string, sessionId: string) {
  const state: PromotionTraceState = {
    taskId,
    sessionId,
    correlatedRequests: new Map(),
    launches: [],
    statusEvents: [],
    pendingGitReads: new Map(),
    heldGitReads: [],
    holdGitReads: false,
    holdStartedAt: null,
    holdStartedAtMs: null,
    gitReadRequestsBeforeHold: 0,
    gitReadResponsesBeforeHold: 0,
    gitReadRequestsAfterHold: 0,
    gitReadResponsesAfterHold: 0,
  };
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) =>
      forwardBridgeMessage(message, server, (rawFrame, frame) =>
        handleOutgoingFrame(rawFrame, frame, server, state),
      ),
    );
    server.onMessage((message) =>
      forwardBridgeMessage(message, socket, (_rawFrame, frame) => {
        handleIncomingFrame(frame, state);
        return true;
      }),
    );
  });
  return createPromotionTrace(state);
}

export function statusFromPayload(
  payload: Record<string, unknown>,
): Record<string, unknown> | null {
  return recordValue(payload.status);
}

export function statusFiles(
  status: Record<string, unknown> | null,
): Record<string, unknown> | null {
  return recordValue(status?.files);
}

export type PromotionTrace = Awaited<ReturnType<typeof routeWorkspaceStreamPromotion>>;
