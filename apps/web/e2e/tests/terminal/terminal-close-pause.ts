import { expect, type Page, type WebSocketRoute } from "@playwright/test";

type PausedRequest = {
  message: string;
  server: WebSocketRoute;
};

function isTerminalDestroyFrame(part: string): boolean {
  if (!part.trim()) return false;
  try {
    const frame = JSON.parse(part) as { type?: unknown; action?: unknown };
    return frame.type === "request" && frame.action === "user_shell.destroy";
  } catch {
    return false;
  }
}

export function partitionTerminalDestroyRequest(
  message: string | Buffer,
): { destroyFrame: string; passthrough: string | null } | null {
  if (typeof message !== "string") return null;
  const frames = message.split("\n");
  const destroyIndex = frames.findIndex(isTerminalDestroyFrame);
  if (destroyIndex === -1) return null;
  const passthrough = frames.filter((_, index) => index !== destroyIndex).join("\n");
  return {
    destroyFrame: frames[destroyIndex]!,
    passthrough: passthrough.trim() ? passthrough : null,
  };
}

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
