import type { Page } from "@playwright/test";

type Frame = {
  id?: string;
  type?: string;
  action?: string;
  payload?: { session_id?: string; intent?: string };
};

function parseFrame(message: string): Frame | null {
  try {
    return JSON.parse(message) as Frame | null;
  } catch {
    return null;
  }
}

/** Let explicit recovery inspect a legacy workspace before background Git reads do. */
export async function holdLegacyRecoveryReads(page: Page, sessionId: string): Promise<void> {
  let held = true;
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    const waiting: string[] = [];
    const recoveryRequests = new Set<string>();
    socket.onMessage((message) => {
      if (typeof message !== "string") {
        server.send(message);
        return;
      }
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = parseFrame(part);
        if (frame?.type === "request" && frame.payload?.session_id === sessionId) {
          if (
            held &&
            (frame.action === "session.git.commits" || frame.action === "session.cumulative_diff")
          ) {
            waiting.push(part);
            continue;
          }
          if (
            frame.id &&
            (frame.action === "session.recover" ||
              (frame.action === "session.launch" && frame.payload.intent === "restore_workspace"))
          ) {
            recoveryRequests.add(frame.id);
          }
        }
        server.send(part);
      }
    });
    server.onMessage((message) => {
      socket.send(message);
      if (typeof message !== "string") return;
      for (const part of message.split("\n").filter(Boolean)) {
        const frame = parseFrame(part);
        if (
          frame?.id &&
          (frame.type === "response" || frame.type === "error") &&
          recoveryRequests.delete(frame.id)
        ) {
          held = false;
          for (const pending of waiting.splice(0)) server.send(pending);
        }
      }
    });
  });
}
