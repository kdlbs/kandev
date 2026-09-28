import { afterEach, describe, expect, it, vi } from "vitest";
import type { DesktopV1Adapter } from "./adapter";
import { listenForDesktopDownloadReady } from "./download-ready";

afterEach(() => {
  document.body.replaceChildren();
  vi.restoreAllMocks();
});

describe("listenForDesktopDownloadReady", () => {
  it("retries the selected URL using its native suggested name", async () => {
    let onReady: ((payload: unknown) => void) | undefined;
    const listen = vi.fn(async (_event: string, callback: (payload: unknown) => void) => {
      onReady = callback;
      return () => undefined;
    });
    const adapter = {
      isAvailable: () => true,
      listen,
    } as unknown as DesktopV1Adapter;
    const clickedAnchors: HTMLAnchorElement[] = [];
    vi.spyOn(HTMLAnchorElement.prototype, "click").mockImplementation(function (
      this: HTMLAnchorElement,
    ) {
      clickedAnchors.push(this);
    });

    await listenForDesktopDownloadReady(adapter);
    onReady?.({ url: "/download/bundle.zip", fileName: "diagnostic.zip" });

    expect(listen).toHaveBeenCalledWith("download-ready", expect.any(Function));
    expect(clickedAnchors).toHaveLength(1);
    expect(clickedAnchors[0].href).toBe(new URL("/download/bundle.zip", window.location.href).href);
    expect(clickedAnchors[0].download).toBe("diagnostic.zip");
  });
});
