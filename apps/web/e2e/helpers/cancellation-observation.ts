import type { Page } from "@playwright/test";

/** Preserve the pending notification until its UI assertions finish. */
export async function holdCancellationSettlement(page: Page) {
  let sessionId: string | null = null;
  const releases = new Set<() => void>();
  await page.routeWebSocket("**/ws", (client) => {
    const server = client.connectToServer();
    const frames: Array<string | Buffer> = [];
    releases.add(() => {
      for (const frame of frames.splice(0)) client.send(frame);
    });
    server.onMessage((message) => {
      const frame = JSON.parse(message.toString()) as {
        action?: string;
        payload?: { session_id?: string; cancellation_pending?: boolean };
      };
      const pendingNotification =
        frame.action === "session.cancellation_changed" &&
        frame.payload?.session_id === sessionId &&
        frame.payload.cancellation_pending === true;
      // Independent event streams can deliver settlement before pending.
      // Freeze at the action boundary so both arrival orders remain observable.
      if (sessionId && !pendingNotification) {
        frames.push(message);
        return;
      }
      client.send(message);
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
