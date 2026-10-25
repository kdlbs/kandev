import type { Page, WebSocketRoute } from "@playwright/test";

export type ShellInputFrame = {
  sessionId: string;
  data: string;
};

export type AvailableCommandsFrame = {
  sessionId: string;
  count: number;
};

export type MessageAddFrame = {
  taskId: string;
  sessionId: string;
  content: string;
};

type ParsedFrame = {
  type?: string;
  action?: string;
  payload?: {
    task_id?: string;
    session_id?: string;
    content?: string;
    data?: string;
    available_commands?: unknown[];
  };
};

type MessageAddRequest = {
  id?: string;
  payload?: ParsedFrame["payload"];
};

function parseMessageAddRequest(part: string): MessageAddRequest | null {
  try {
    const frame = JSON.parse(part) as MessageAddRequest & {
      type?: string;
      action?: string;
    };
    return frame.type === "request" && frame.action === "message.add" ? frame : null;
  } catch {
    return null;
  }
}

function captureMessageAdd(messageAdds: MessageAddFrame[], frame: MessageAddRequest): void {
  const taskId = frame.payload?.task_id;
  const sessionId = frame.payload?.session_id;
  const content = frame.payload?.content;
  if (typeof taskId !== "string" || typeof sessionId !== "string" || typeof content !== "string") {
    return;
  }
  messageAdds.push({ taskId, sessionId, content });
}

/**
 * PassthroughTerminal's resize frames start with a 0x01 tag byte followed by
 * a JSON `{cols, rows}` body. A naive "first byte === 0x01" check would
 * misclassify Ctrl+A (also 0x01) as a resize, so confirm the JSON shape
 * before discarding.
 */
const RESIZE_FRAME_TAG = 0x01;

function isResizeFrame(payload: Buffer | Uint8Array): boolean {
  if (payload.length < 2 || payload[0] !== RESIZE_FRAME_TAG) return false;
  try {
    const tail = new TextDecoder("utf-8", { fatal: false }).decode(
      (payload as Uint8Array).slice(1),
    );
    const parsed = JSON.parse(tail) as { cols?: unknown; rows?: unknown };
    return typeof parsed?.cols === "number" && typeof parsed?.rows === "number";
  } catch {
    return false;
  }
}

function decodeBinaryFrame(payload: Buffer | Uint8Array): string | null {
  if (!payload || payload.length === 0) return null;
  if (isResizeFrame(payload)) return null;
  try {
    return new TextDecoder("utf-8", { fatal: false }).decode(payload as Uint8Array);
  } catch {
    return null;
  }
}

/**
 * Subscribe to outgoing WS frames on the given page and collect every shell
 * input frame, regardless of which transport carried it:
 *
 *  - JSON `{action: "shell.input", payload: {session_id, data}}` over the
 *    kandev gateway WS — the per-session default shell.
 *  - Raw text or binary frames over a PassthroughTerminal's dedicated WS.
 *    AttachAddon sends ordinary xterm input as text; manual mobile routing
 *    sends binary. The session ID is unknown for these frames, so an empty
 *    string is reported.
 *
 * Tests assert on the `data` field, which works the same either way.
 *
 * Returns a live array that tests can poll via `expect.poll`. Must be called
 * before the page navigates, since `framesent` events fire before your next
 * tick.
 */
export function attachShellInputCapture(page: Page): { frames: ShellInputFrame[] } {
  const frames: ShellInputFrame[] = [];
  page.on("websocket", (ws) => {
    const isTerminalSocket = new URL(ws.url()).pathname.startsWith("/terminal/");
    ws.on("framesent", (event) => {
      const payload = event.payload;
      if (typeof payload === "string") {
        if (isTerminalSocket) {
          if (payload) frames.push({ sessionId: "", data: payload });
          return;
        }
        if (!payload.includes('"shell.input"')) return;
        try {
          const msg = JSON.parse(payload) as ParsedFrame;
          if (msg.action !== "shell.input") return;
          const sessionId = msg.payload?.session_id;
          const data = msg.payload?.data;
          if (typeof sessionId === "string" && typeof data === "string") {
            frames.push({ sessionId, data });
          }
        } catch {
          /* non-JSON string frames — ignore */
        }
        return;
      }
      const decoded = decodeBinaryFrame(payload);
      if (decoded) frames.push({ sessionId: "", data: decoded });
    });
  });
  return { frames };
}

/**
 * Subscribe to incoming WS frames and collect every session.available_commands
 * update. Call before navigation so tests do not miss eager agent-init frames.
 */
export function attachAvailableCommandsCapture(page: Page): {
  frames: AvailableCommandsFrame[];
} {
  const frames: AvailableCommandsFrame[] = [];
  page.on("websocket", (ws) => {
    ws.on("framereceived", (event) => {
      const payload = event.payload;
      if (typeof payload !== "string" || !payload.includes('"session.available_commands"')) return;
      try {
        const msg = JSON.parse(payload) as ParsedFrame;
        if (msg.action !== "session.available_commands") return;
        const sessionId = msg.payload?.session_id;
        const commands = msg.payload?.available_commands;
        if (typeof sessionId === "string" && Array.isArray(commands)) {
          frames.push({ sessionId, count: commands.length });
        }
      } catch {
        /* non-JSON string frames — ignore */
      }
    });
  });
  return { frames };
}

/**
 * Capture submitted chat text from outgoing gateway message.add requests.
 * Attach before navigation so the page's gateway socket is observed.
 */
export function attachMessageAddCapture(page: Page): { frames: MessageAddFrame[] } {
  const frames: MessageAddFrame[] = [];
  page.on("websocket", (ws) => {
    if (new URL(ws.url()).pathname !== "/ws") return;
    ws.on("framesent", (event) => {
      const payload = event.payload;
      if (typeof payload !== "string" || !payload.includes('"message.add"')) return;
      try {
        const msg = JSON.parse(payload) as ParsedFrame;
        const taskId = msg.payload?.task_id;
        const sessionId = msg.payload?.session_id;
        const content = msg.payload?.content;
        if (
          msg.type === "request" &&
          msg.action === "message.add" &&
          typeof taskId === "string" &&
          typeof sessionId === "string" &&
          typeof content === "string"
        ) {
          frames.push({ taskId, sessionId, content });
        }
      } catch {
        /* non-JSON string frames — ignore */
      }
    });
  });
  return { frames };
}

/** Keep the real gateway connection live while allowing tests to deliver server notifications. */
export async function routeGatewayNotifications(page: Page) {
  const sockets: WebSocketRoute[] = [];
  const messageAdds: MessageAddFrame[] = [];
  let nextId = 1;
  let acknowledgeMessageAdds = false;
  await page.routeWebSocket(/\/ws(?:\?.*)?$/, (socket) => {
    sockets.push(socket);
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message !== "string" || !acknowledgeMessageAdds) {
        server.send(message);
        return;
      }
      const forwarded: string[] = [];
      for (const part of message.split("\n")) {
        const frame = parseMessageAddRequest(part);
        if (frame) {
          captureMessageAdd(messageAdds, frame);
          if (frame.id) {
            socket.send(
              JSON.stringify({
                id: frame.id,
                type: "response",
                action: "message.add",
                payload: { accepted: true },
                timestamp: new Date().toISOString(),
              }),
            );
            continue;
          }
        }
        if (part) forwarded.push(part);
      }
      if (forwarded.length > 0) server.send(forwarded.join("\n"));
    });
    server.onMessage((message) => socket.send(message));
  });

  return {
    acknowledgeMessageAdds() {
      acknowledgeMessageAdds = true;
    },
    send(action: string, payload: Record<string, unknown>) {
      if (sockets.length === 0) throw new Error("The gateway WebSocket is not connected");
      const message = JSON.stringify({
        id: `e2e-notification-${nextId++}`,
        type: "notification",
        action,
        payload,
        timestamp: new Date().toISOString(),
      });
      for (const socket of sockets) socket.send(message);
    },
    messageAdds,
  };
}
