import { act, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const raster = vi.hoisted(() => ({ rasterizePreviewRegion: vi.fn() }));
vi.mock("@/lib/preview-screenshot", () => raster);
vi.mock("html2canvas-pro", () => ({ default: vi.fn() }));
const anchorRequest = vi.hoisted(() => ({ requestRegionAnchor: vi.fn() }));
vi.mock("@/lib/preview-inspect-bridge", async () => ({
  ...(await vi.importActual<object>("@/lib/preview-inspect-bridge")),
  ...anchorRequest,
}));
vi.mock("@/lib/api/domains/attachment-api", () => ({
  deleteAttachment: vi.fn(),
  uploadAttachment: vi.fn(),
}));

import type { PreviewScreenshotRegion } from "@/lib/preview-inspect-bridge";
import { usePreviewDrafts } from "./use-preview-drafts";
import { usePreviewScreenshots } from "./use-preview-screenshots";

const region: PreviewScreenshotRegion = {
  page_route: "/",
  page_title: "Home",
  capture_rect: { x: 10, y: 20, width: 100, height: 50, document_x: 10, document_y: 20 },
};

function renderScreenshots() {
  const iframe = document.createElement("iframe");
  document.body.appendChild(iframe);
  const iframeRef = { current: iframe };
  return renderHook(
    () => {
      const drafts = usePreviewDrafts();
      const screenshots = usePreviewScreenshots({
        iframeRef,
        drafts,
        workspaceId: "ws",
        items: [],
        replaceSaved: vi.fn(),
      });
      return { drafts, screenshots };
    },
    { reactStrictMode: true },
  );
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

const image = { blob: new Blob(["png"]), width: 100, height: 50, uniformOverCanvas: false };

beforeEach(() => {
  vi.clearAllMocks();
  URL.createObjectURL = vi.fn(() => "blob:capture");
  URL.revokeObjectURL = vi.fn();
});

afterEach(() => {
  document.body.innerHTML = "";
});

describe("usePreviewScreenshots region capture", () => {
  it("adds a capturing draft at once and fills its image under StrictMode", async () => {
    const pending = deferred<typeof image>();
    raster.rasterizePreviewRegion.mockReturnValue(pending.promise);
    const { result } = renderScreenshots();

    act(() => result.current.screenshots.captureRegion(region));

    expect(result.current.drafts.drafts).toHaveLength(1);
    expect(result.current.drafts.drafts[0]).toMatchObject({
      open: true,
      payload: { kind: "screenshot", screenshot: null },
    });
    expect(result.current.screenshots.isRasterizing).toBe(true);

    await act(async () => pending.resolve(image));

    await waitFor(() => expect(result.current.screenshots.isRasterizing).toBe(false));
    expect(result.current.drafts.drafts[0].payload).toMatchObject({
      screenshot: { width: 100, height: 50, previewUrl: "blob:capture" },
    });
  });

  it("removes the capturing draft and reports the error when capture fails", async () => {
    raster.rasterizePreviewRegion.mockRejectedValue(new Error("timed out"));
    const { result } = renderScreenshots();

    act(() => result.current.screenshots.captureRegion(region));

    await waitFor(() => expect(result.current.screenshots.captureError).toBe("raster"));
    expect(result.current.drafts.drafts).toHaveLength(0);
    expect(result.current.screenshots.isRasterizing).toBe(false);
  });

  it("drops the image of a draft discarded while it was capturing", async () => {
    const pending = deferred<typeof image>();
    raster.rasterizePreviewRegion.mockReturnValue(pending.promise);
    const { result } = renderScreenshots();

    act(() => result.current.screenshots.captureRegion(region));
    act(() => result.current.drafts.discard(result.current.drafts.drafts[0].id));
    await act(async () => pending.resolve(image));

    await waitFor(() => expect(result.current.screenshots.isRasterizing).toBe(false));
    expect(result.current.drafts.drafts).toHaveLength(0);
    expect(URL.revokeObjectURL).toHaveBeenCalledWith("blob:capture");
  });
});

describe("usePreviewScreenshots recapture", () => {
  it("stores the anchor the page reports for a moved region", async () => {
    raster.rasterizePreviewRegion.mockResolvedValue(image);
    const anchor = {
      anchor_selector: "section#card",
      anchor_offset_x: 5,
      anchor_offset_y: 6,
      anchor_width: 300,
      anchor_height: 200,
    };
    anchorRequest.requestRegionAnchor.mockResolvedValue(anchor);
    const { result } = renderScreenshots();
    act(() => result.current.screenshots.captureRegion(region));
    await waitFor(() => expect(result.current.screenshots.isRasterizing).toBe(false));
    const id = result.current.drafts.drafts[0].id;

    let ok = false;
    await act(async () => {
      ok = await result.current.screenshots.recaptureRegion(id, {
        x: 40,
        y: 50,
        width: 80,
        height: 30,
      });
    });

    expect(ok).toBe(true);
    expect(anchorRequest.requestRegionAnchor).toHaveBeenCalledWith(expect.anything(), {
      x: 40,
      y: 50,
      width: 80,
      height: 30,
    });
    const payload = result.current.drafts.drafts[0].payload;
    expect(payload.kind === "screenshot" && payload.capture_rect).toMatchObject({
      x: 40,
      ...anchor,
    });
  });
});
