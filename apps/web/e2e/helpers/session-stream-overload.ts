import { expect, type Page } from "@playwright/test";
import type { ApiClient } from "./api-client";
import type { GatewayTrafficFrame } from "./ws-traffic";

export const REASONING_BURST_COUNT = 2_000;

export function reasoningBurstPrompt(count = REASONING_BURST_COUNT): string {
  return `e2e:reasoning_burst(${count})`;
}

export function expectedReasoningContent(count = REASONING_BURST_COUNT): string {
  let content = "";
  for (let index = 1; index <= count; index += 1) {
    content += `reasoning-burst-${String(index).padStart(6, "0")}|`;
  }
  return content;
}

export async function waitForExactReasoningBurst(
  apiClient: ApiClient,
  sessionId: string,
  count = REASONING_BURST_COUNT,
): Promise<{ sourceChunks: number; reasoningBytes: number }> {
  const expected = expectedReasoningContent(count);
  const firstChunk = `reasoning-burst-${String(1).padStart(6, "0")}|`;
  const findBurst = (messages: Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"]) =>
    messages.find((message) => String(message.metadata?.thinking ?? "").startsWith(firstChunk));
  let latestMessages: Awaited<ReturnType<ApiClient["listSessionMessages"]>>["messages"] = [];
  await expect
    .poll(
      async () => {
        latestMessages = (await apiClient.listSessionMessages(sessionId)).messages;
        const marker = latestMessages.some(
          (message) => message.content === `reasoning-burst-produced:${count}`,
        );
        const reasoning = findBurst(latestMessages);
        return marker && reasoning?.metadata?.thinking === expected;
      },
      {
        timeout: 120_000,
        message: `reasoning burst did not persist exact ${count}-chunk content`,
      },
    )
    .toBe(true);

  const reasoning = findBurst(latestMessages);
  return {
    sourceChunks: count,
    reasoningBytes: Buffer.byteLength(String(reasoning?.metadata?.thinking ?? ""), "utf8"),
  };
}

export function noisyReceivedFrames(
  frames: readonly GatewayTrafficFrame[],
  sessionId: string,
): GatewayTrafficFrame[] {
  return frames.filter(
    (frame) =>
      frame.direction === "received" &&
      frame.sessionId === sessionId &&
      frame.action === "session.message.updated",
  );
}

export async function assertNoHorizontalOverflow(page: Page, label: string): Promise<void> {
  const dimensions = await page.evaluate(() => ({
    scrollWidth: document.documentElement.scrollWidth,
    clientWidth: document.documentElement.clientWidth,
  }));
  expect(dimensions.scrollWidth, `${label} scroll width`).toBeLessThanOrEqual(
    dimensions.clientWidth,
  );
}
