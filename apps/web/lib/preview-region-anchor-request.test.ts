import { afterEach, describe, expect, it, vi } from "vitest";
import {
  INSPECTOR_PROTOCOL_VERSION,
  INSPECTOR_SOURCE,
  REGION_ANCHOR_TIMEOUT_MS,
  requestRegionAnchor,
} from "./preview-inspect-bridge";

const region = { x: 10, y: 20, width: 100, height: 40 };
const anchor = {
  anchor_selector: "section#card",
  anchor_offset_x: 10,
  anchor_offset_y: 20,
  anchor_width: 300,
  anchor_height: 200,
};

function message(type: string, payload: Record<string, unknown>) {
  return { source: INSPECTOR_SOURCE, version: INSPECTOR_PROTOCOL_VERSION, type, payload };
}

describe("requestRegionAnchor", () => {
  afterEach(() => vi.useRealTimers());

  function frameWithReply(reply: (requestId: string) => unknown) {
    const contentWindow = {
      postMessage: vi.fn((sent: { payload: { request_id: string } }) => {
        const data = reply(sent.payload.request_id);
        if (data) {
          window.dispatchEvent(
            new MessageEvent("message", {
              source: contentWindow as unknown as Window,
              data,
            }),
          );
        }
      }),
    };
    return { contentWindow } as unknown as HTMLIFrameElement;
  }

  it("resolves the anchor the page reports for a moved region", async () => {
    const iframe = frameWithReply((requestId) =>
      message("region-anchored", { request_id: requestId, anchor }),
    );
    await expect(requestRegionAnchor(iframe, region)).resolves.toEqual(anchor);
    const postMessage = (
      iframe.contentWindow as unknown as { postMessage: ReturnType<typeof vi.fn> }
    ).postMessage;
    expect(postMessage.mock.calls[0][0]).toMatchObject({
      type: "anchor-region",
      payload: { rect: region },
    });
  });

  it("resolves no anchor when the page does not answer in time", async () => {
    vi.useFakeTimers();
    const iframe = frameWithReply(() => null);
    const pending = requestRegionAnchor(iframe, region);
    vi.advanceTimersByTime(REGION_ANCHOR_TIMEOUT_MS);
    await expect(pending).resolves.toBeNull();
  });
});
