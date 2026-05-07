import type { Page } from "@playwright/test";

export type ShellInputFrame = {
  sessionId: string;
  data: string;
};

type ParsedFrame = {
  type?: string;
  action?: string;
  payload?: { session_id?: string; data?: string };
};

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
 *  - Raw binary frames over a PassthroughTerminal's dedicated WS — used by
 *    mobile multi-terminal where the on-screen terminal owns its own
 *    AttachAddon connection. The session ID is unknown for these frames
 *    (the WS is env+terminalId scoped) so an empty string is reported.
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
    ws.on("framesent", (event) => {
      const payload = event.payload;
      if (typeof payload === "string") {
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
