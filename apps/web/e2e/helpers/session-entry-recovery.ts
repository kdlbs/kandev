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
  requestCount: (action: string) => number;
  delayedResponseCount: (action: string) => number;
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

/**
 * Delay selected gateway responses while forwarding every other frame. Rules
 * correlate replies by request id, so the test never relies on action-only or
 * payload timing and does not inspect message contents.
 */
export async function routeSessionEntryRecovery(page: Page): Promise<SessionEntryRecoveryProxy> {
  const requestActions = new Map<string, string>();
  const requestCounts = new Map<string, number>();
  const delayedCounts = new Map<string, number>();
  const rules = new Map<string, DelayRule>();

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
        const action = responseAction(frame, requestActions);
        if (typeof frame?.id === "string") requestActions.delete(frame.id);
        const rule = isResponseFrame(frame) && action ? rules.get(action) : null;

        if (rule && rule.remaining > 0 && action) {
          rule.remaining -= 1;
          delayedCounts.set(action, (delayedCounts.get(action) ?? 0) + 1);
          void (async () => {
            await injectLatency(rule.delayMs, rule.reason);
            ws.send(trimmed);
          })();
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
    requestCount: (action) => requestCounts.get(action) ?? 0,
    delayedResponseCount: (action) => delayedCounts.get(action) ?? 0,
  };
}
