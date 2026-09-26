import type { Page } from "@playwright/test";

type WireFrame = {
  id?: unknown;
  type?: unknown;
  action?: unknown;
  payload?: unknown;
};

export type MessageListResponseHold = {
  holdNextLatestWindow: (sessionId: string) => void;
  heldCount: () => number;
  releaseHeldResponse: () => void;
};

function parseFrame(value: string): WireFrame | null {
  try {
    const parsed = JSON.parse(value) as unknown;
    return typeof parsed === "object" && parsed !== null ? (parsed as WireFrame) : null;
  } catch {
    return null;
  }
}

function framePayload(frame: WireFrame | null): Record<string, unknown> | null {
  return typeof frame?.payload === "object" && frame.payload !== null
    ? (frame.payload as Record<string, unknown>)
    : null;
}

/**
 * Holds one correlated latest-window `message.list` response while keeping the
 * gateway socket and every unrelated frame live. Install before navigation,
 * then arm immediately before the task switch under test.
 */
export async function routeMainWebSocketWithMessageListResponseHold(
  page: Page,
): Promise<MessageListResponseHold> {
  let armedSessionId: string | null = null;
  const requestIds = new Set<string>();
  const heldFrames: string[] = [];
  let releaseToClient: ((frames: readonly string[]) => void) | null = null;

  await page.routeWebSocket(/\/ws$/, (ws) => {
    const server = ws.connectToServer();
    releaseToClient = (frames) => {
      for (const frame of frames) ws.send(frame);
    };
    ws.onMessage((message) => {
      if (typeof message === "string" && armedSessionId) {
        for (const part of message.split("\n")) {
          const frame = parseFrame(part.trim());
          const payload = framePayload(frame);
          if (
            frame?.type === "request" &&
            frame.action === "message.list" &&
            typeof frame.id === "string" &&
            payload?.session_id === armedSessionId &&
            payload.before === undefined
          ) {
            requestIds.add(frame.id);
          }
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      if (typeof message !== "string" || requestIds.size === 0) {
        ws.send(message);
        return;
      }
      const kept: string[] = [];
      for (const part of message.split("\n")) {
        const trimmed = part.trim();
        const frame = parseFrame(trimmed);
        if (
          frame?.type === "response" &&
          frame.action === "message.list" &&
          typeof frame.id === "string" &&
          requestIds.delete(frame.id)
        ) {
          heldFrames.push(trimmed);
        } else if (trimmed) {
          kept.push(part);
        }
      }
      if (kept.length > 0) ws.send(kept.join("\n"));
    });
  });

  return {
    holdNextLatestWindow: (sessionId) => {
      armedSessionId = sessionId;
      requestIds.clear();
      heldFrames.length = 0;
    },
    heldCount: () => heldFrames.length,
    releaseHeldResponse: () => {
      const frames = heldFrames.splice(0);
      armedSessionId = null;
      requestIds.clear();
      releaseToClient?.(frames);
    },
  };
}
