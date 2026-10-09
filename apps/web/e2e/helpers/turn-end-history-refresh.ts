import { expect, type Page } from "@playwright/test";

export const LOADING_ROWS =
  "[data-testid='session-history-loading'], [data-testid='conversation-loading-state']";

export type IdleConversationGap = {
  armed: boolean;
  completed: boolean;
  delivered: number;
  gapChecked: boolean;
  recoveryResponseReceived: boolean;
  recovered: boolean;
  recoveredMessageIds: string[];
  holdRecoveryResponse: boolean;
  releaseRecoveryResponse: () => void;
};

type SocketFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: unknown;
};

function parseFrame(message: unknown): SocketFrame | null {
  try {
    const parsed: unknown = JSON.parse(String(message));
    return parsed && typeof parsed === "object" ? (parsed as SocketFrame) : null;
  } catch {
    return null;
  }
}

function readSuccessfulMessageListResponse(
  frame: SocketFrame | null,
  pendingRequestIds: Set<string>,
): string[] | null {
  if (
    frame?.type !== "response" ||
    typeof frame.id !== "string" ||
    !pendingRequestIds.has(frame.id)
  ) {
    return null;
  }
  pendingRequestIds.delete(frame.id);
  const payload = frame.payload as { messages?: unknown } | undefined;
  return Array.isArray(payload?.messages)
    ? payload.messages.flatMap((item) => {
        if (!item || typeof item !== "object") return [];
        const id = (item as { id?: unknown }).id;
        return typeof id === "string" ? [id] : [];
      })
    : [];
}

/** Records whether either transcript loading row is ever inserted. */
export async function watchLoadingRows(page: Page) {
  // Initial history hydration is allowed to load; observe recovery only once it settles.
  await expect(page.locator(LOADING_ROWS)).toHaveCount(0);
  await page.evaluate((selector) => {
    const w = window as unknown as { __loadingRowSeen?: boolean };
    w.__loadingRowSeen = false;
    new MutationObserver(() => {
      if (document.querySelector(selector)) w.__loadingRowSeen = true;
    }).observe(document.body, { childList: true, subtree: true });
  }, LOADING_ROWS);
  return () =>
    page.evaluate(() => (window as unknown as { __loadingRowSeen?: boolean }).__loadingRowSeen);
}

function revisionOf(text: string): number {
  const match = /"revision":"(\d+)"/.exec(text);
  return match ? Number(match[1]) : 0;
}

/**
 * Drops the first conversation change after turn completion. The client must
 * find the gap with its revision check, then fetch and apply the transcript.
 */
export async function forceIdleConversationGap(page: Page): Promise<IdleConversationGap> {
  const pendingMessageListRequests = new Set<string>();
  let forwardHeldResponse: (() => void) | undefined;
  const state: IdleConversationGap = {
    armed: false,
    completed: false,
    delivered: 0,
    gapChecked: false,
    recoveryResponseReceived: false,
    recovered: false,
    recoveredMessageIds: [],
    holdRecoveryResponse: false,
    releaseRecoveryResponse: () => forwardHeldResponse?.(),
  };

  await page.routeWebSocket(/\/ws(\?|$)/, (ws) => {
    const server = ws.connectToServer();
    ws.onMessage((message) => {
      const frame = parseFrame(message);
      if (
        state.gapChecked &&
        frame?.type === "request" &&
        frame.action === "message.list" &&
        typeof frame.id === "string"
      ) {
        pendingMessageListRequests.add(frame.id);
      }
      server.send(message);
    });
    server.onMessage((message) => {
      const frame = parseFrame(message);
      const text = String(message);
      const messageIds = readSuccessfulMessageListResponse(frame, pendingMessageListRequests);
      if (messageIds !== null) {
        state.recoveredMessageIds = messageIds;
        state.recoveryResponseReceived = true;
        const forwardResponse = () => {
          state.recovered = true;
          forwardHeldResponse = undefined;
          ws.send(message);
        };
        if (state.holdRecoveryResponse) {
          forwardHeldResponse = forwardResponse;
          return;
        }
        forwardResponse();
        return;
      } else if (!text.includes('"session.conversation.changed"')) {
        ws.send(message);
        return;
      }

      const revision = revisionOf(text);
      if (text.includes('"check":true')) {
        if (state.completed && revision > state.delivered) state.gapChecked = true;
        ws.send(message);
        return;
      }
      if (!text.includes('"operations":[{')) {
        ws.send(message);
        return;
      }
      if (state.armed && state.completed && !state.gapChecked) return;
      if (state.armed && /"entity":"turn".*"completed_at":"[^"]/.test(text)) {
        state.completed = true;
      }
      state.delivered = Math.max(state.delivered, revision);
      ws.send(message);
    });
  });
  return state;
}

export async function waitForRecoveredMessagesApplied(page: Page, gap: IdleConversationGap) {
  if (gap.recoveredMessageIds.length === 0) {
    throw new Error("The successful message.list response did not contain message IDs");
  }
  await page.waitForFunction(
    ({ ids }) => {
      const store = (
        window as unknown as {
          __KANDEV_E2E_STORE__?: {
            getState: () => {
              tasks: { activeSessionId: string | null };
              messages: { bySession: Record<string, { id: string }[] | undefined> };
            };
          };
        }
      ).__KANDEV_E2E_STORE__;
      const state = store?.getState();
      const sessionId = state?.tasks.activeSessionId;
      if (!state || !sessionId) return false;
      const messages = state.messages.bySession[sessionId] ?? [];
      return ids.every((id) => messages.some((message) => message.id === id));
    },
    { ids: gap.recoveredMessageIds },
    { timeout: 10_000, message: "the recovered message.list snapshot should reach the store" },
  );
  await page.evaluate(
    () =>
      new Promise<void>((resolve) => {
        requestAnimationFrame(() => requestAnimationFrame(() => resolve()));
      }),
  );
}
