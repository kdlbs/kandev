import type { Locator } from "@playwright/test";

export type TranscriptScrollState = {
  scrollTop: number;
  scrollHeight: number;
  clientHeight: number;
  scrollOwnerCount: number;
};

/** Reads the native transcript metrics and counts scrollable ancestors. */
export async function readTranscriptScrollState(
  transcript: Locator,
): Promise<TranscriptScrollState> {
  return transcript.evaluate((node) => {
    const element = node as HTMLElement;
    let current: HTMLElement | null = element;
    let scrollOwnerCount = 0;
    while (current) {
      const overflowY = getComputedStyle(current).overflowY;
      if (overflowY === "auto" || overflowY === "scroll") scrollOwnerCount += 1;
      if (current.dataset.testid === "session-chat") break;
      current = current.parentElement;
    }
    return {
      scrollTop: element.scrollTop,
      scrollHeight: element.scrollHeight,
      clientHeight: element.clientHeight,
      scrollOwnerCount,
    };
  });
}

/** Applies a user-like transcript scroll and returns the resulting offset. */
export async function setTranscriptScrollTop(
  transcript: Locator,
  scrollTop: number,
): Promise<number> {
  return transcript.evaluate((node, nextScrollTop) => {
    const element = node as HTMLElement;
    element.scrollTop = nextScrollTop;
    element.dispatchEvent(new Event("scroll", { bubbles: true }));
    return element.scrollTop;
  }, scrollTop);
}
