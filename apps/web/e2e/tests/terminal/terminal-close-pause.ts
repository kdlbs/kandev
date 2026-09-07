import { expect, type Page, type WebSocketRoute } from "@playwright/test";
import { partitionTerminalDestroyRequest } from "./terminal-close-pause-parser";

type PausedRequest = {
  message: string;
  server: WebSocketRoute;
};

export { partitionTerminalDestroyRequest } from "./terminal-close-pause-parser";

export async function pauseNextTerminalDestroy(page: Page) {
  let armed = false;
  let paused: PausedRequest | null = null;

  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      const partition = armed && !paused ? partitionTerminalDestroyRequest(message) : null;
      if (partition) {
        armed = false;
        paused = { message: partition.destroyFrame, server };
        if (partition.passthrough) server.send(partition.passthrough);
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
