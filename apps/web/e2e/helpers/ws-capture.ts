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
 * Subscribe to outgoing WS frames on the given page and collect every
 * `shell.input` request with its `{ session_id, data }` payload. Useful for
 * asserting that UI actions translate to the correct escape sequences without
 * depending on xterm paint timing.
 *
 * Returns a live array that tests can poll via `expect.poll`. Must be called
 * before the page navigates, since `framesent` events fire before your next
 * tick.
 */
export function attachShellInputCapture(page: Page): { frames: ShellInputFrame[] } {
  const frames: ShellInputFrame[] = [];
  page.on("websocket", (ws) => {
    ws.on("framesent", (event) => {
      const raw =
        typeof event.payload === "string" ? event.payload : (event.payload?.toString("utf8") ?? "");
      if (!raw || !raw.includes('"shell.input"')) return;
      try {
        const msg = JSON.parse(raw) as ParsedFrame;
        if (msg.action !== "shell.input") return;
        const sessionId = msg.payload?.session_id;
        const data = msg.payload?.data;
        if (typeof sessionId === "string" && typeof data === "string") {
          frames.push({ sessionId, data });
        }
      } catch {
        /* non-JSON frames (binary, etc.) — ignore */
      }
    });
  });
  return { frames };
}
