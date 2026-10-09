import type { Page } from "@playwright/test";

/** Preserve the pending notification until its UI assertions finish. */
export async function holdCancellationSettlement(page: Page) {
  let sessionId: string | null = null;
  const releases = new Set<() => void>();
  await page.routeWebSocket("**/ws", (client) => {
    const server = client.connectToServer();
    let held = false;
    const frames: Array<string | Buffer> = [];
    releases.add(() => {
      held = false;
      for (const frame of frames.splice(0)) client.send(frame);
    });
    server.onMessage((message) => {
      if (held) {
        frames.push(message);
        return;
      }
      const frame = JSON.parse(message.toString()) as {
        action?: string;
        payload?: { session_id?: string; cancellation_pending?: boolean };
      };
      client.send(message);
      if (
        frame.action === "session.cancellation_changed" &&
        frame.payload?.session_id === sessionId &&
        frame.payload.cancellation_pending === true
      ) {
        held = true;
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
