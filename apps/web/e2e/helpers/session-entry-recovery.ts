import type { Page } from "@playwright/test";
import { injectLatency } from "./causal-waits";

type WireFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
};

type DelayRule = {
  remaining: number;
  delayMs: number;
  reason: string;
};

export type SessionEntryRecoveryProxy = {
  delayNextResponses: (action: string, count: number, delayMs: number, reason: string) => void;
  dropNextResponses: (action: string, count: number) => void;
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
  requestActions: Map<string, string>,
): string | undefined {
  if (typeof frame?.action === "string") return frame.action;
  if (typeof frame?.id === "string") return requestActions.get(frame.id);
  return undefined;
}

function takeResponseAction(
  frame: WireFrame | null,
  requestActions: Map<string, string>,
): string | undefined {
  const action = responseAction(frame, requestActions);
  if (typeof frame?.id === "string") requestActions.delete(frame.id);
  return action;
}

function consumeDropRule(
  action: string | undefined,
  dropRules: Map<string, number>,
  droppedCounts: Map<string, number>,
): boolean {
  if (!action) return false;
  const remaining = dropRules.get(action) ?? 0;
  if (remaining < 1) return false;
  dropRules.set(action, remaining - 1);
  droppedCounts.set(action, (droppedCounts.get(action) ?? 0) + 1);
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
  const requestActions = new Map<string, string>();
  const requestCounts = new Map<string, number>();
  const delayedCounts = new Map<string, number>();
  const droppedCounts = new Map<string, number>();
  const rules = new Map<string, DelayRule>();
  const dropRules = new Map<string, number>();

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
            requestActions.set(frame.id, frame.action);
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
        const action = takeResponseAction(frame, requestActions);
        if (isResponseFrame(frame)) {
          if (consumeDropRule(action, dropRules, droppedCounts)) continue;
          if (consumeDelayRule(action, trimmed, rules, delayedCounts, ws.send.bind(ws))) continue;
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
    dropNextResponses: (action, count) => {
      if (count < 1) throw new Error("dropNextResponses requires a positive response count");
      dropRules.set(action, count);
    },
    requestCount: (action) => requestCounts.get(action) ?? 0,
    delayedResponseCount: (action) => delayedCounts.get(action) ?? 0,
    droppedResponseCount: (action) => droppedCounts.get(action) ?? 0,
  };
}
