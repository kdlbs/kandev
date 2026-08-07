import type { Page } from "@playwright/test";

export type GatewayTrafficFrame = {
  direction: "sent" | "received";
  action?: string;
  type?: string;
  sessionId?: string;
  taskId?: string;
  bytes: number;
  timestamp: number;
};

export type GatewayTrafficSummary = {
  frameCount: number;
  byteCount: number;
  sentFrameCount: number;
  sentByteCount: number;
  receivedFrameCount: number;
  receivedByteCount: number;
  byAction: Record<
    string,
    {
      sent: number;
      received: number;
      bytes: number;
    }
  >;
};

type WireMessage = {
  action?: unknown;
  type?: unknown;
  payload?: unknown;
};

function decodePayload(payload: string | Buffer | Uint8Array): string | null {
  if (typeof payload === "string") return payload;
  try {
    return new TextDecoder("utf-8", { fatal: false }).decode(payload);
  } catch {
    return null;
  }
}

function payloadBytes(payload: string | Buffer | Uint8Array): number {
  return typeof payload === "string" ? Buffer.byteLength(payload, "utf8") : payload.byteLength;
}

function parseMessage(payload: string | Buffer | Uint8Array): WireMessage | null {
  const decoded = decodePayload(payload);
  if (!decoded) return null;
  try {
    const parsed = JSON.parse(decoded) as WireMessage;
    return parsed && typeof parsed === "object" ? parsed : null;
  } catch {
    return null;
  }
}

function stringField(value: unknown): string | undefined {
  return typeof value === "string" && value.length > 0 ? value : undefined;
}

function frameFromPayload(
  direction: GatewayTrafficFrame["direction"],
  payload: string | Buffer | Uint8Array,
): GatewayTrafficFrame {
  const message = parseMessage(payload);
  const messagePayload =
    message?.payload && typeof message.payload === "object"
      ? (message.payload as Record<string, unknown>)
      : undefined;

  return {
    direction,
    action: stringField(message?.action),
    type: stringField(message?.type),
    sessionId:
      stringField(messagePayload?.session_id) ??
      stringField((message as Record<string, unknown> | null)?.session_id),
    taskId:
      stringField(messagePayload?.task_id) ??
      stringField((message as Record<string, unknown> | null)?.task_id),
    bytes: payloadBytes(payload),
    timestamp: Date.now(),
  };
}

/**
 * Capture only the gateway WebSocket. Terminal/preview sockets are deliberately
 * excluded so the result measures task/session traffic rather than transport
 * noise from other panels.
 */
export function attachGatewayTrafficCapture(page: Page): { frames: GatewayTrafficFrame[] } {
  const frames: GatewayTrafficFrame[] = [];
  page.on("websocket", (webSocket) => {
    if (!webSocket.url().endsWith("/ws")) return;
    webSocket.on("framesent", (event) => {
      frames.push(frameFromPayload("sent", event.payload));
    });
    webSocket.on("framereceived", (event) => {
      frames.push(frameFromPayload("received", event.payload));
    });
  });
  return { frames };
}

export function summarizeGatewayTraffic(
  frames: readonly GatewayTrafficFrame[],
): GatewayTrafficSummary {
  const summary: GatewayTrafficSummary = {
    frameCount: frames.length,
    byteCount: 0,
    sentFrameCount: 0,
    sentByteCount: 0,
    receivedFrameCount: 0,
    receivedByteCount: 0,
    byAction: {},
  };

  for (const frame of frames) {
    summary.byteCount += frame.bytes;
    if (frame.direction === "sent") {
      summary.sentFrameCount += 1;
      summary.sentByteCount += frame.bytes;
    } else {
      summary.receivedFrameCount += 1;
      summary.receivedByteCount += frame.bytes;
    }

    const action = frame.action ?? "<non-json>";
    const actionSummary = (summary.byAction[action] ??= {
      sent: 0,
      received: 0,
      bytes: 0,
    });
    actionSummary[frame.direction] += 1;
    actionSummary.bytes += frame.bytes;
  }

  return summary;
}
