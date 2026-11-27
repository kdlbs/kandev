import type { Page } from "@playwright/test";

export type NavigationRequest = {
  id: string;
  action: string;
  payload: Record<string, unknown>;
  at: number;
  receivedAt?: number;
  deliveredAt?: number;
};
type Frame = { id?: string; type?: string; action?: string; payload?: Record<string, unknown> };

function parseFrame(value: string): Frame | null {
  try {
    const parsed = JSON.parse(value);
    return parsed && typeof parsed === "object" ? parsed : null;
  } catch {
    return null;
  }
}

/** Correlates replies by request ID while keeping unrelated gateway traffic live. */
export class NavigationResponseGate {
  readonly requests: NavigationRequest[] = [];
  private pending = new Map<string, NavigationRequest>();
  private predicate: ((request: NavigationRequest) => boolean) | null = null;
  private heldIds = new Set<string>();
  private held: Array<{
    raw: string;
    send: (message: string) => void;
    request: NavigationRequest;
  }> = [];

  hold(predicate: (request: NavigationRequest) => boolean) {
    this.predicate = predicate;
  }
  heldCount() {
    return this.held.length;
  }

  recordRequests(message: string) {
    for (const part of message.split("\n")) {
      const frame = parseFrame(part);
      if (frame?.type !== "request" || !frame.id || !frame.action) continue;
      const request = {
        id: frame.id,
        action: frame.action,
        payload: frame.payload ?? {},
        at: performance.now(),
      };
      this.requests.push(request);
      this.pending.set(frame.id, request);
      if (this.predicate?.(request)) this.heldIds.add(frame.id);
    }
  }

  forwardResponses(message: string, send: (message: string) => void) {
    let intercepted = false;
    const kept: string[] = [];
    for (const part of message.split("\n")) {
      const frame = parseFrame(part);
      const request = frame?.id ? this.pending.get(frame.id) : undefined;
      if (request && (frame?.type === "response" || frame?.type === "error")) {
        request.receivedAt = performance.now();
        this.pending.delete(request.id);
        if (this.heldIds.delete(request.id)) {
          this.held.push({ raw: part, send, request });
          intercepted = true;
          continue;
        }
        request.deliveredAt = request.receivedAt;
      }
      kept.push(part);
    }
    if (!intercepted) send(message);
    else if (kept.some((part) => part.trim())) send(kept.join("\n"));
  }

  release(error?: string) {
    this.predicate = null;
    this.heldIds.clear();
    for (const held of this.held.splice(0)) {
      held.request.deliveredAt = performance.now();
      held.send(
        error
          ? JSON.stringify({ ...parseFrame(held.raw), type: "error", payload: { message: error } })
          : held.raw,
      );
    }
  }
}

export async function routeNavigationResponses(page: Page): Promise<NavigationResponseGate> {
  const gate = new NavigationResponseGate();
  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    ws.onMessage((message) => {
      if (typeof message === "string") gate.recordRequests(message);
      server.send(message);
    });
    server.onMessage((message) => {
      if (typeof message === "string") gate.forwardResponses(message, (frame) => ws.send(frame));
      else ws.send(message);
    });
  });
  return gate;
}
