import type { Page } from "@playwright/test";
import type { SessionCommit } from "../../../lib/state/slices/session-runtime/types";

type Frame = {
  id?: string;
  type?: string;
  action?: string;
  payload?: { session_id?: string };
};

function parseFrame(message: string): Frame | null {
  try {
    return JSON.parse(message) as Frame;
  } catch {
    return null;
  }
}

export type CommitSpacingSource = {
  use: (sessionId: string, commits: SessionCommit[]) => void;
  responseCount: () => number;
};

/** Install before navigation. Keep synthetic spacing data consistent across
 * initial reads, late replies and refetches, forwarding all other gateway data.
 */
export async function routeCommitSpacingSource(page: Page): Promise<CommitSpacingSource> {
  const requests = new Map<string, string>();
  let fixture: { sessionId: string; commits: SessionCommit[] } | null = null;
  let responses = 0;
  await page.routeWebSocket(/\/ws$/, (socket) => {
    const server = socket.connectToServer();
    socket.onMessage((message) => {
      if (typeof message === "string") {
        for (const part of message.split("\n")) {
          const frame = parseFrame(part);
          if (
            frame?.type === "request" &&
            frame.action === "session.git.commits" &&
            frame.id &&
            frame.payload?.session_id
          ) {
            requests.set(frame.id, frame.payload.session_id);
          }
        }
      }
      server.send(message);
    });
    server.onMessage((message) => {
      if (typeof message !== "string") {
        socket.send(message);
        return;
      }
      for (const part of message.split("\n")) {
        if (!part.trim()) continue;
        const frame = parseFrame(part);
        const sessionId = frame?.id ? requests.get(frame.id) : undefined;
        if (frame?.type === "response" && fixture && sessionId === fixture.sessionId) {
          responses++;
          socket.send(
            JSON.stringify({ ...frame, payload: { commits: fixture.commits, ready: true } }),
          );
        } else {
          socket.send(part);
        }
        if (frame?.id && (frame.type === "response" || frame.type === "error")) {
          requests.delete(frame.id);
        }
      }
    });
  });
  return {
    use: (sessionId, commits) => {
      fixture = { sessionId, commits };
    },
    responseCount: () => responses,
  };
}
