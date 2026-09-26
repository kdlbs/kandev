import type { Page } from "@playwright/test";
import { injectLatency } from "./causal-waits";

type WireFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: Record<string, unknown>;
};

type RequestContext = {
  action: string;
  sessionId?: string;
};

type DropRule = {
  remaining: number;
  sessionId?: string;
};

type DelayRule = {
  remaining: number;
  delayMs: number;
  reason: string;
};

export type SessionEntryRecoveryProxy = {
  delayNextResponses: (action: string, count: number, delayMs: number, reason: string) => void;
  dropNextResponses: (action: string, count: number, scope?: { sessionId?: string }) => void;
  requestCount: (action: string) => number;
  delayedResponseCount: (action: string) => number;
  droppedResponseCount: (action: string) => number;
};

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
  return action ? { action, sessionId: request?.sessionId } : undefined;
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

/**
 * Delay or drop selected gateway responses while forwarding every other frame.
 * Rules correlate replies by request id, so the test never relies on
 * action-only or payload timing and does not inspect message contents.
 */
export async function routeSessionEntryRecovery(page: Page): Promise<SessionEntryRecoveryProxy> {
  const requestContexts = new Map<string, RequestContext>();
  const requestCounts = new Map<string, number>();
  const delayedCounts = new Map<string, number>();
  const droppedCounts = new Map<string, number>();
  const rules = new Map<string, DelayRule>();
  const dropRules = new Map<string, DropRule>();

  await page.routeWebSocket(/\/ws$/, (ws) => {
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
            requestContexts.set(frame.id, {
              action: frame.action,
              sessionId:
                typeof frame.payload?.session_id === "string"
                  ? frame.payload.session_id
                  : undefined,
            });
            requestCounts.set(frame.action, (requestCounts.get(frame.action) ?? 0) + 1);
          }
        }
      }
      server.send(message);
    });

    server.onMessage((message) => {
      if (typeof message !== "string") {
        ws.send(message);
        return;
      }

      for (const part of message.split("\n")) {
        const trimmed = part.trim();
        if (!trimmed) continue;
        const frame = parseFrame(trimmed);
        const context = takeResponseContext(frame, requestContexts);
        if (isResponseFrame(frame)) {
          if (consumeDropRule(context, dropRules, droppedCounts)) continue;
          if (consumeDelayRule(context?.action, trimmed, rules, delayedCounts, ws.send.bind(ws)))
            continue;
        }

        ws.send(trimmed);
      }
    });
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
    requestCount: (action) => requestCounts.get(action) ?? 0,
    delayedResponseCount: (action) => delayedCounts.get(action) ?? 0,
    droppedResponseCount: (action) => droppedCounts.get(action) ?? 0,
  };
}
