import type { Page } from "@playwright/test";

export type MarkReadResponseHold = {
  waitUntilHeld: () => Promise<void>;
  releaseHeldResponse: () => Promise<void>;
};

/**
 * Lets one mark-read request reach the backend, then holds its successful
 * response before the browser receives it. Other sessions remain live so a
 * task switch can dispatch through the same mounted read-tracking hook.
 */
export async function routeMarkReadResponseHold(
  page: Page,
  sessionId: string,
): Promise<MarkReadResponseHold> {
  let consumed = false;
  let resolveHeld: (() => void) | undefined;
  let resolveRelease: (() => void) | undefined;
  let resolveDelivered: (() => void) | undefined;
  const held = new Promise<void>((resolve) => {
    resolveHeld = resolve;
  });
  const release = new Promise<void>((resolve) => {
    resolveRelease = resolve;
  });
  const delivered = new Promise<void>((resolve) => {
    resolveDelivered = resolve;
  });

  await page.route(`**/api/v1/task-sessions/${sessionId}/mark-read`, async (route) => {
    if (consumed) {
      await route.continue();
      return;
    }
    consumed = true;
    const response = await route.fetch();
    resolveHeld?.();
    await release;
    await route.fulfill({ response });
    resolveDelivered?.();
  });

  return {
    waitUntilHeld: () => held,
    releaseHeldResponse: async () => {
      resolveRelease?.();
      await delivered;
    },
  };
}
