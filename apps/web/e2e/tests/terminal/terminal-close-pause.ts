import { expect, type Page, type WebSocketRoute } from "@playwright/test";

type PausedRequest = {
  message: string | Buffer;
  server: WebSocketRoute;
};

function isTerminalDestroyRequest(message: string | Buffer): boolean {
  if (typeof message !== "string") return false;
  return message.split("\n").some((part) => {
    if (!part.trim()) return false;
    try {
      const frame = JSON.parse(part) as { type?: unknown; action?: unknown };
      return frame.type === "request" && frame.action === "user_shell.destroy";
    } catch {
      return false;
    }
  });
}

export async function pauseNextTerminalDestroy(page: Page) {
  let armed = false;
  let paused: PausedRequest | null = null;

  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (armed && !paused && isTerminalDestroyRequest(message)) {
        armed = false;
        paused = { message, server };
        return;
      }
      server.send(message);
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
          message: "terminal destroy request should reach the paused transport boundary",
        })
        .toBe(true);
    },
    release() {
      if (!paused) throw new Error("No terminal destroy request is paused");
      paused.server.send(paused.message);
      paused = null;
    },
  };
}
