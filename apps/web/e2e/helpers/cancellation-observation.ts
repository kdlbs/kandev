import type { Page } from "@playwright/test";

/** Isolate the real pending notification until its UI assertions finish. */
export async function holdCancellationSettlement(page: Page) {
  let sessionId: string | null = null;
  const releases = new Set<() => void>();
  await page.routeWebSocket(/\/ws(?:\?.*)?$/, (client) => {
    const server = client.connectToServer();
    const frames: Array<string | Buffer> = [];
    releases.add(() => {
      for (const frame of frames.splice(0)) client.send(frame);
    });
    server.onMessage((message) => {
      if (sessionId === null) {
        client.send(message);
        return;
      }
      // Settlement can overtake pending delivery. Buffer from arm, including
      // other replies, so a newer revision cannot erase the observed state.
      if (typeof message !== "string") {
        frames.push(message);
        return;
      }
      for (const part of message.split("\n")) {
        let frame: {
          action?: string;
          payload?: { session_id?: string; cancellation_pending?: boolean };
        };
        try {
          frame = JSON.parse(part);
        } catch {
          frames.push(part);
          continue;
        }
        if (
          frame?.action === "session.cancellation_changed" &&
          frame.payload?.session_id === sessionId &&
          frame.payload.cancellation_pending === true
        ) {
          client.send(part);
        } else {
          frames.push(part);
        }
      }
    });
  });
  return {
    arm: (targetSessionId: string) => {
      sessionId = targetSessionId;
    },
    release: () => {
      sessionId = null;
      for (const release of releases) release();
    },
  };
}
