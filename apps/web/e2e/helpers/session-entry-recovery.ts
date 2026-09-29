import type { Page, WebSocketRoute } from "@playwright/test";
import { injectLatency } from "./causal-waits";
import type { SeedData } from "../fixtures/test-base";
import type { CreateTaskResponse } from "../../lib/types/http";
import type { ApiClient } from "./api-client";
import { waitForSessionDone } from "./session";

type WireFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: Record<string, unknown>;
};

type RequestContext = {
  action: string;
  sessionId?: string;
  rejectionMessage?: string;
};

type DropRule = {
  remaining: number;
  sessionId?: string;
};

type HoldRule = {
  sessionId?: string;
};

type RejectRule = {
  message: string;
  sessionId?: string;
};

type DelayRule = {
  remaining: number;
  delayMs: number;
  reason: string;
};

type ResponseHandlingState = {
  requestContexts: Map<string, RequestContext>;
  rejectRules: Map<string, RejectRule>;
  rejectedCounts: Map<string, number>;
  holdRules: Map<string, HoldRule>;
  heldCounts: Map<string, number>;
  heldMessages: Map<string, string[]>;
  failureMessages: Map<string, string>;
  failedCounts: Map<string, number>;
  dropRules: Map<string, DropRule>;
  droppedCounts: Map<string, number>;
  delayRules: Map<string, DelayRule>;
  delayedCounts: Map<string, number>;
};

export type SessionEntryRecoveryProxy = {
  delayNextResponses: (action: string, count: number, delayMs: number, reason: string) => void;
  dropNextResponses: (action: string, count: number, scope?: { sessionId?: string }) => void;
  rejectResponsesUntilReleased: (
    action: string,
    message: string,
    scope?: { sessionId?: string },
  ) => void;
  releaseRejectedResponses: (action: string) => void;
  requestCount: (action: string) => number;
  delayedResponseCount: (action: string) => number;
  droppedResponseCount: (action: string) => number;
  rejectedResponseCount: (action: string) => number;
  failResponses: (action: string, message: string) => void;
  allowResponses: (action: string) => void;
  holdResponses: (action: string, scope?: { sessionId?: string }) => void;
  releaseHeldResponses: (action: string) => void;
  pendingRequestCount: (action: string) => number;
  failedResponseCount: (action: string) => number;
  heldResponseCount: (action: string) => number;
};

export async function createSettledHistoryTask(
  apiClient: ApiClient,
  seedData: SeedData,
  title: string,
): Promise<CreateTaskResponse> {
  const task = await apiClient.createTaskWithAgent(
    seedData.workspaceId,
    title,
    seedData.agentProfileId,
    {
      description: "/e2e:simple-message",
      workflow_id: seedData.workflowId,
      workflow_step_id: seedData.startStepId,
      repository_ids: [seedData.repositoryId],
    },
  );
  if (!task.session_id) throw new Error("history recovery task has no session_id");

  await waitForSessionDone(
    apiClient,
    task.id,
    task.session_id,
    "Waiting for the history recovery task's initial prompt to finish",
    60_000,
  );
  return task;
}

function parseFrame(value: string): WireFrame | null {
  try {
    const parsed = JSON.parse(value) as unknown;
    return typeof parsed === "object" && parsed !== null ? (parsed as WireFrame) : null;
  } catch {
    return null;
  }
}

function isResponseFrame(frame: WireFrame | null): boolean {
  return frame?.type === "response" || frame?.type === "error";
}

function responseAction(
  frame: WireFrame | null,
  requestContexts: Map<string, RequestContext>,
): RequestContext | undefined {
  const request = typeof frame?.id === "string" ? requestContexts.get(frame.id) : undefined;
  const action = typeof frame?.action === "string" ? frame.action : request?.action;
  return action
    ? {
        action,
        sessionId: request?.sessionId,
        rejectionMessage: request?.rejectionMessage,
      }
    : undefined;
}

function takeResponseContext(
  frame: WireFrame | null,
  requestContexts: Map<string, RequestContext>,
): RequestContext | undefined {
  const context = responseAction(frame, requestContexts);
  if (typeof frame?.id === "string") requestContexts.delete(frame.id);
  return context;
}

function consumeDropRule(
  context: RequestContext | undefined,
  dropRules: Map<string, DropRule>,
  droppedCounts: Map<string, number>,
): boolean {
  if (!context) return false;
  const rule = dropRules.get(context.action);
  if (!rule || rule.remaining < 1) return false;
  if (rule.sessionId && context.sessionId !== rule.sessionId) return false;
  rule.remaining -= 1;
  droppedCounts.set(context.action, (droppedCounts.get(context.action) ?? 0) + 1);
  return true;
}

function consumeRejectRule(
  context: RequestContext | undefined,
  rejectedCounts: Map<string, number>,
): string | undefined {
  if (!context?.rejectionMessage) return undefined;
  rejectedCounts.set(context.action, (rejectedCounts.get(context.action) ?? 0) + 1);
  return context.rejectionMessage;
}

function consumeHoldRule(
  context: RequestContext | undefined,
  holdRules: Map<string, HoldRule>,
  heldCounts: Map<string, number>,
): boolean {
  if (!context) return false;
  const rule = holdRules.get(context.action);
  if (!rule || (rule.sessionId && context.sessionId !== rule.sessionId)) return false;
  heldCounts.set(context.action, (heldCounts.get(context.action) ?? 0) + 1);
  return true;
}

function consumeDelayRule(
  action: string | undefined,
  message: string,
  rules: Map<string, DelayRule>,
  delayedCounts: Map<string, number>,
  send: (message: string) => void,
): boolean {
  if (!action) return false;
  const rule = rules.get(action);
  if (!rule || rule.remaining < 1) return false;
  rule.remaining -= 1;
  delayedCounts.set(action, (delayedCounts.get(action) ?? 0) + 1);
  void (async () => {
    await injectLatency(rule.delayMs, rule.reason);
    send(message);
  })();
  return true;
}

function interceptServerResponse(
  message: string,
  frame: WireFrame | null,
  context: RequestContext | undefined,
  socket: WebSocketRoute,
  state: ResponseHandlingState,
): boolean {
  const rejection = consumeRejectRule(context, state.rejectRules, state.rejectedCounts);
  if (rejection && frame) {
    socket.send(
      JSON.stringify({
        ...frame,
        type: "error",
        payload: { code: "E2E_SIMULATED_ERROR", message: rejection },
      }),
    );
    return true;
  }

  if (context && consumeHoldRule(context, state.holdRules, state.heldCounts)) {
    const messages = state.heldMessages.get(context.action) ?? [];
    messages.push(message);
    state.heldMessages.set(context.action, messages);
    return true;
  }

  if (frame && context && state.failureMessages.has(context.action)) {
    state.failedCounts.set(context.action, (state.failedCounts.get(context.action) ?? 0) + 1);
    socket.send(
      JSON.stringify({
        ...frame,
        type: "error",
        payload: {
          code: "INTERNAL_ERROR",
          message: state.failureMessages.get(context.action),
        },
      }),
    );
    return true;
  }

  if (consumeDropRule(context, state.dropRules, state.droppedCounts)) return true;
  return consumeDelayRule(
    context?.action,
    message,
    state.delayRules,
    state.delayedCounts,
    (delayedMessage) => socket.send(delayedMessage),
  );
}

function forwardServerMessage(
  message: string | Buffer,
  socket: WebSocketRoute,
  state: ResponseHandlingState,
) {
  if (typeof message !== "string") {
    socket.send(message);
    return;
  }

  for (const part of message.split("\n")) {
    const trimmed = part.trim();
    if (!trimmed) continue;
    const frame = parseFrame(trimmed);
    const context = takeResponseContext(frame, state.requestContexts);
    if (
      !isResponseFrame(frame) ||
      !interceptServerResponse(trimmed, frame, context, socket, state)
    ) {
      socket.send(trimmed);
    }
  }
}

/**
 * Fail, delay, drop, or hold selected gateway responses while forwarding other frames.
 * Rules correlate replies by request id, so the test never relies on
 * action-only or payload timing and does not inspect message contents.
 */
export async function routeSessionEntryRecovery(page: Page): Promise<SessionEntryRecoveryProxy> {
  const requestContexts = new Map<string, RequestContext>();
  const requestCounts = new Map<string, number>();
  const delayedCounts = new Map<string, number>();
  const droppedCounts = new Map<string, number>();
  const rejectedCounts = new Map<string, number>();
  const failedCounts = new Map<string, number>();
  const heldCounts = new Map<string, number>();
  const rules = new Map<string, DelayRule>();
  const dropRules = new Map<string, DropRule>();
  const rejectRules = new Map<string, RejectRule>();
  const holdRules = new Map<string, HoldRule>();
  const heldMessages = new Map<string, string[]>();
  const failureMessages = new Map<string, string>();
  let sendHeldMessage: ((message: string) => void) | undefined;
  const responseState: ResponseHandlingState = {
    requestContexts,
    rejectRules,
    rejectedCounts,
    holdRules,
    heldCounts,
    heldMessages,
    failureMessages,
    failedCounts,
    dropRules,
    droppedCounts,
    delayRules: rules,
    delayedCounts,
  };

  await page.routeWebSocket(/\/ws$/, (ws) => {
    sendHeldMessage = (message) => ws.send(message);
    const server = ws.connectToServer();

    ws.onMessage((message) => {
      if (typeof message === "string") {
        for (const part of message.split("\n")) {
          const frame = parseFrame(part.trim());
          if (
            frame?.type === "request" &&
            typeof frame.id === "string" &&
            typeof frame.action === "string"
          ) {
            const context: RequestContext = {
              action: frame.action,
              sessionId:
                typeof frame.payload?.session_id === "string"
                  ? frame.payload.session_id
                  : undefined,
            };
            const rejectRule = rejectRules.get(context.action);
            // Keep fault injection stable for requests that are already in flight.
            if (
              !rejectRule ||
              (rejectRule.sessionId && rejectRule.sessionId !== context.sessionId)
            ) {
              requestContexts.set(frame.id, context);
            } else {
              requestContexts.set(frame.id, { ...context, rejectionMessage: rejectRule.message });
            }
            requestCounts.set(frame.action, (requestCounts.get(frame.action) ?? 0) + 1);
          }
        }
      }
      server.send(message);
    });

    server.onMessage((message) => forwardServerMessage(message, ws, responseState));
  });

  return {
    delayNextResponses: (action, count, delayMs, reason) => {
      if (count < 1) throw new Error("delayNextResponses requires a positive response count");
      if (delayMs < 0) throw new Error("delayNextResponses requires a non-negative delay");
      rules.set(action, { remaining: count, delayMs, reason });
    },
    dropNextResponses: (action, count, scope) => {
      if (count < 1) throw new Error("dropNextResponses requires a positive response count");
      dropRules.set(action, { remaining: count, sessionId: scope?.sessionId });
    },
    rejectResponsesUntilReleased: (action, message, scope) => {
      if (!message) throw new Error("rejectResponsesUntilReleased requires an error message");
      rejectRules.set(action, { message, sessionId: scope?.sessionId });
    },
    releaseRejectedResponses: (action) => rejectRules.delete(action),
    rejectedResponseCount: (action) => rejectedCounts.get(action) ?? 0,
    failResponses: (action, message) => {
      if (!message) throw new Error("failResponses requires an error message");
      failureMessages.set(action, message);
    },
    allowResponses: (action) => {
      failureMessages.delete(action);
    },
    holdResponses: (action, scope) => {
      holdRules.set(action, { sessionId: scope?.sessionId });
    },
    releaseHeldResponses: (action) => {
      holdRules.delete(action);
      for (const message of heldMessages.get(action) ?? []) sendHeldMessage?.(message);
      heldMessages.delete(action);
    },
    pendingRequestCount: (action) =>
      [...requestContexts.values()].filter((context) => context.action === action).length,
    requestCount: (action) => requestCounts.get(action) ?? 0,
    delayedResponseCount: (action) => delayedCounts.get(action) ?? 0,
    droppedResponseCount: (action) => droppedCounts.get(action) ?? 0,
    failedResponseCount: (action) => failedCounts.get(action) ?? 0,
    heldResponseCount: (action) => heldCounts.get(action) ?? 0,
  };
}

export async function rejectSessionEnsureRequests(
  page: Page,
  message: string,
): Promise<{ requestCount: () => Promise<number> }> {
  await page.addInitScript((failureMessage) => {
    const testWindow = window as Window & { __e2eSessionEnsureRequestCount?: number };
    testWindow.__e2eSessionEnsureRequestCount = 0;
    const nativeSend = WebSocket.prototype.send;
    WebSocket.prototype.send = function (data) {
      if (typeof data !== "string") {
        nativeSend.call(this, data);
        return;
      }
      const forwarded: string[] = [];
      let intercepted = false;
      for (const part of data.split("\n")) {
        let frame: { id?: unknown; type?: unknown; action?: unknown } | null = null;
        try {
          frame = JSON.parse(part) as { id?: unknown; type?: unknown; action?: unknown };
        } catch {
          forwarded.push(part);
          continue;
        }
        if (
          frame?.type === "request" &&
          frame.action === "session.ensure" &&
          typeof frame.id === "string"
        ) {
          intercepted = true;
          testWindow.__e2eSessionEnsureRequestCount =
            (testWindow.__e2eSessionEnsureRequestCount ?? 0) + 1;
          const response = JSON.stringify({
            id: frame.id,
            type: "error",
            action: "session.ensure",
            payload: { code: "E2E_SIMULATED_ERROR", message: failureMessage },
            timestamp: new Date().toISOString(),
          });
          queueMicrotask(() => this.dispatchEvent(new MessageEvent("message", { data: response })));
          continue;
        }
        forwarded.push(part);
      }
      if (!intercepted) {
        nativeSend.call(this, data);
      } else if (forwarded.length > 0) {
        nativeSend.call(this, forwarded.join("\n"));
      }
    };
  }, message);

  return {
    requestCount: () =>
      page.evaluate(
        () =>
          (window as Window & { __e2eSessionEnsureRequestCount?: number })
            .__e2eSessionEnsureRequestCount ?? 0,
      ),
  };
}
