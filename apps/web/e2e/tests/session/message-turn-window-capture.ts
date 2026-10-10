import type { Page } from "@playwright/test";

type WireFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: unknown;
};

type MessageRow = { id: string; turn_id?: string | null };
type TurnRow = { id: string };
type MessageWindowPayload = {
  messages?: MessageRow[];
  turns?: TurnRow[];
  turn_coverage?: { message_ids?: string[]; active_turn_id?: string | null };
};

export type MessageWindowCapture = {
  requests: WireFrame[];
  responses: WireFrame[];
};

function parseFrame(payload: string | Buffer | Uint8Array): WireFrame | null {
  try {
    const frame: unknown = JSON.parse(
      typeof payload === "string" ? payload : Buffer.from(payload).toString(),
    );
    return frame && typeof frame === "object" ? (frame as WireFrame) : null;
  } catch {
    return null;
  }
}

export function captureMessageWindows(page: Page): MessageWindowCapture {
  const capture: MessageWindowCapture = { requests: [], responses: [] };
  page.on("websocket", (socket) => {
    if (!socket.url().endsWith("/ws")) return;
    socket.on("framesent", (event) => {
      const frame = parseFrame(event.payload);
      if (frame?.type === "request" && frame.action === "message.list") {
        capture.requests.push(frame);
      }
    });
    socket.on("framereceived", (event) => {
      const frame = parseFrame(event.payload);
      if (frame?.type === "response") capture.responses.push(frame);
    });
  });
  page.on("response", async (response) => {
    const url = new URL(response.url());
    const match = /\/task-sessions\/([^/]+)\/messages$/.exec(url.pathname);
    if (!match || response.request().method() !== "GET" || !response.ok()) return;
    const payload = {
      session_id: decodeURIComponent(match[1]),
      before: url.searchParams.get("before") ?? undefined,
      include_turns: url.searchParams.get("include_turns") === "true",
    };
    capture.requests.push({ id: response.url(), type: "request", action: "message.list", payload });
    try {
      capture.responses.push({
        id: response.url(),
        type: "response",
        payload: await response.json(),
      });
    } catch {
      // A cancelled route response cannot provide window coverage.
    }
  });
  return capture;
}

export function readMessageWindow(
  capture: MessageWindowCapture,
  sessionId: string,
  requireBeforeCursor = false,
): { request: WireFrame; response: MessageWindowPayload } | null {
  const request = [...capture.requests].reverse().find((candidate) => {
    if (!candidate.payload || typeof candidate.payload !== "object") return false;
    const payload = candidate.payload as Record<string, unknown>;
    return (
      payload.session_id === sessionId &&
      payload.include_turns === true &&
      (!requireBeforeCursor || typeof payload.before === "string")
    );
  });
  if (!request || typeof request.id !== "string") return null;
  const response = capture.responses.find((candidate) => candidate.id === request.id);
  if (!response?.payload || typeof response.payload !== "object") return null;
  return { request, response: response.payload as MessageWindowPayload };
}

export async function waitForInitialMessageWindow(page: Page, sessionId: string) {
  const snapshot = await page.evaluate((id) => {
    const state = (
      window as unknown as { __KANDEV_E2E_STORE__?: { getState(): Record<string, unknown> } }
    ).__KANDEV_E2E_STORE__?.getState();
    return { id, messages: state?.messages, turns: state?.turns };
  }, sessionId);
  const { test } = await import("@playwright/test");
  await test.info().attach("message-window-initial-state.json", {
    body: JSON.stringify(snapshot, null, 2),
    contentType: "application/json",
  });
  await page.waitForFunction(
    (id) => {
      const store = (
        window as unknown as {
          __KANDEV_E2E_STORE__?: {
            getState: () => {
              messages: {
                metaBySession: Record<string, { historyInitialized?: boolean }>;
                bySession: Record<string, unknown[] | undefined>;
              };
              turns: {
                bySession: Record<string, unknown[] | undefined>;
                loadedBySession: Record<string, boolean | undefined>;
                windowCoverageBySession?:
                  | Record<string, { messageIds?: string[]; activeTurnObserved?: boolean }>
                  | undefined;
              };
            };
          };
        }
      ).__KANDEV_E2E_STORE__;
      const state = store?.getState();
      return (
        state?.messages.metaBySession[id]?.historyInitialized === true &&
        (state.messages.bySession[id]?.length ?? 0) === 100 &&
        (state.turns.bySession[id]?.length ?? 0) === 1 &&
        state.turns.windowCoverageBySession?.[id]?.activeTurnObserved === true &&
        state.turns.loadedBySession[id] !== true
      );
    },
    sessionId,
    { timeout: 30_000, message: "boot should hydrate a bounded message and turn window" },
  );
}

export async function bootMessageCount(page: Page, sessionId: string): Promise<number> {
  const { readBootCapture } = await import("../task/journey-boot-loading-helpers");
  const boot = await readBootCapture(page);
  const state = boot.routeData?.taskDetail?.initialState as unknown as {
    messages?: { bySession?: Record<string, unknown[]> };
  };
  return state.messages?.bySession?.[sessionId]?.length ?? 0;
}
