import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const prepareCanvasExport = vi.fn();
const downloadCanvasExport = vi.fn();
const cancelCanvasExport = vi.fn();
const triggerBlobDownload = vi.fn();

vi.mock("@/lib/api/domains/canvas-distribution-api", () => ({
  prepareCanvasExport: (...args: unknown[]) => prepareCanvasExport(...args),
  downloadCanvasExport: (...args: unknown[]) => downloadCanvasExport(...args),
  cancelCanvasExport: (...args: unknown[]) => cancelCanvasExport(...args),
}));
vi.mock("@/lib/utils/file-download", () => ({
  triggerBlobDownload: (...args: unknown[]) => triggerBlobDownload(...args),
}));

import type { Canvas } from "@/lib/api/domains/canvas-api";
import { useCanvasShare } from "./use-canvas-share";

afterEach(() => cleanup());

const canvas: Canvas = {
  id: "canvas-1",
  plugin_instance_id: "instance-1",
  plugin_id: "canvas-1",
  workspace_id: "workspace-1",
  scope_kind: "workspace",
  title: "Canvas",
  status: "active",
  active_release_id: "release-1",
  active_release_status: "valid",
};

const review = {
  preparation_id: "prep-1",
  canvas_id: "canvas-1",
  workspace_id: "workspace-1",
  release_id: "release-1",
  metadata: { package_id: "canvas-1", version: "1.0.0", display_name: "Canvas" },
  sha256: "digest",
  files: [{ path: "manifest.yaml", bytes: 10 }],
  bundle_bytes: 100,
  source_bytes: 200,
  expires_at: "2026-09-11T12:00:00Z",
  bundle_download: "bundle",
  source_download: "source",
};

beforeEach(() => {
  prepareCanvasExport.mockReset().mockResolvedValue(review);
  downloadCanvasExport.mockReset().mockResolvedValue(new Blob(["data"]));
  cancelCanvasExport.mockReset().mockResolvedValue(undefined);
  triggerBlobDownload.mockReset();
});

describe("useCanvasShare", () => {
  it("prepares from the active release and downloads both archive kinds", async () => {
    const { result } = renderHook(() => useCanvasShare(canvas));

    await act(async () => {
      await result.current.prepare();
    });
    expect(prepareCanvasExport).toHaveBeenCalledWith("canvas-1", {
      workspace_id: "workspace-1",
      expected_release_id: "release-1",
      metadata: undefined,
    });

    await act(async () => {
      await result.current.download("bundle");
      await result.current.download("source");
    });
    expect(downloadCanvasExport).toHaveBeenNthCalledWith(1, "prep-1", "bundle");
    expect(downloadCanvasExport).toHaveBeenNthCalledWith(2, "prep-1", "source");
    expect(triggerBlobDownload.mock.calls[0]?.[1]).toBe("canvas-1-1.0.0.tar.gz");
    expect(triggerBlobDownload.mock.calls[1]?.[1]).toBe("canvas-1-1.0.0.zip");
  });

  it("does not let a stale download update state after the review is reset", async () => {
    let resolveDownload: (value: Blob) => void = () => undefined;
    downloadCanvasExport.mockReturnValueOnce(
      new Promise<Blob>((resolve) => {
        resolveDownload = resolve;
      }),
    );
    const { result } = renderHook(() => useCanvasShare(canvas));
    await act(async () => {
      await result.current.prepare();
    });

    let download: Promise<void> | undefined;
    await act(async () => {
      download = result.current.download("bundle");
    });
    await act(async () => {
      result.current.reset();
      resolveDownload(new Blob(["stale"]));
      await download;
    });

    expect(triggerBlobDownload).not.toHaveBeenCalled();
    expect(result.current.review).toBeNull();
    expect(result.current.loading).toBe(false);
  });
});
