import { expect, type Page, type WebSocketRoute } from "@playwright/test";

type PausedRequest = { message: string; server: WebSocketRoute };

function isUserShellCreateFrame(part: string): boolean {
  if (!part.trim()) return false;
  try {
    const frame = JSON.parse(part) as { type?: unknown; action?: unknown };
    return frame.type === "request" && frame.action === "user_shell.create";
  } catch {
    return false;
  }
}

/**
 * Holds the next `user_shell.create` request at the WebSocket boundary. The
 * caller can navigate the SPA without a reload while the create promise is
 * still pending, then release it to exercise late-completion guards.
 *
 * Install before the first navigation: `routeWebSocket` only sees sockets
 * opened after it is registered.
 */
export async function pauseNextUserShellCreate(page: Page) {
  let armed = false;
  let paused: PausedRequest | null = null;

  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (!armed || paused || typeof message !== "string") {
        server.send(message);
        return;
      }
      const frames = message.split("\n");
      const createIndex = frames.findIndex(isUserShellCreateFrame);
      if (createIndex === -1) {
        server.send(message);
        return;
      }
      armed = false;
      paused = { message: frames[createIndex]!, server };
      const passthrough = frames.filter((_, index) => index !== createIndex).join("\n");
      if (passthrough.trim()) server.send(passthrough);
    });
    server.onMessage((message) => socket.send(message));
  });

  return {
    arm() {
      armed = true;
      paused = null;
    },
    async waitForRequest() {
      await expect
        .poll(() => paused !== null, {
          message: "user_shell.create should reach the paused transport boundary",
        })
        .toBe(true);
    },
    release() {
      if (!paused) throw new Error("No user_shell.create request is paused");
      paused.server.send(paused.message);
      paused = null;
    },
  };
}
