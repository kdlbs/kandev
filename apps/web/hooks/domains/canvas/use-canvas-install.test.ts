import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const prepareCanvasInstall = vi.fn();
const confirmCanvasInstall = vi.fn();
const cancelCanvasInstall = vi.fn();
const uploadCanvasInstall = vi.fn();
const WORKSPACE_ID = "workspace-1";
const FIRST_URL = "https://example.test/one.tar.gz";

vi.mock("@/lib/api/domains/canvas-distribution-api", () => ({
  prepareCanvasInstall: (...args: unknown[]) => prepareCanvasInstall(...args),
  confirmCanvasInstall: (...args: unknown[]) => confirmCanvasInstall(...args),
  cancelCanvasInstall: (...args: unknown[]) => cancelCanvasInstall(...args),
  uploadCanvasInstall: (...args: unknown[]) => uploadCanvasInstall(...args),
}));

import { useCanvasInstall } from "./use-canvas-install";

afterEach(() => cleanup());

function review(id: string) {
  return {
    preparation_id: id,
    workspace_id: WORKSPACE_ID,
    metadata: {
      package_id: "canvas-one",
      version: "1.0.0",
      display_name: "Canvas One",
      description: "",
      author: "author",
      license: "MIT",
      source_mode: "static" as const,
      min_kandev_version: "0.1.0",
    },
    sha256: `package-${id}`,
    archive_sha256: `archive-${id}`,
    permissions: {},
    origin_kind: "url" as const,
    expires_at: "2026-09-11T12:00:00Z",
  };
}

beforeEach(() => {
  prepareCanvasInstall.mockReset();
  confirmCanvasInstall.mockReset();
  cancelCanvasInstall.mockReset().mockResolvedValue(undefined);
  uploadCanvasInstall.mockReset();
});

describe("useCanvasInstall", () => {
  it("does not let an older inspection overwrite a newer one", async () => {
    let resolveFirst: (value: ReturnType<typeof review>) => void = () => undefined;
    const first = new Promise<ReturnType<typeof review>>((resolve) => {
      resolveFirst = resolve;
    });
    prepareCanvasInstall.mockReturnValueOnce(first).mockResolvedValueOnce(review("second"));
    const { result } = renderHook(() => useCanvasInstall(WORKSPACE_ID));

    let firstRequest: Promise<unknown> | undefined;
    await act(async () => {
      firstRequest = result.current.prepareUrl(FIRST_URL);
    });
    await act(async () => {
      await result.current.prepareUrl("https://example.test/two.tar.gz");
    });
    expect(result.current.review?.preparation_id).toBe("second");

    resolveFirst(review("first"));
    await act(async () => {
      await firstRequest;
    });
    expect(result.current.review?.preparation_id).toBe("second");
  });

  it("confirms the archive digest shown by the review", async () => {
    prepareCanvasInstall.mockResolvedValueOnce(review("one"));
    confirmCanvasInstall.mockResolvedValueOnce({ canvas: { id: "canvas-1" }, receipt: {} });
    const { result } = renderHook(() => useCanvasInstall(WORKSPACE_ID));

    await act(async () => {
      await result.current.prepareUrl(FIRST_URL);
    });
    await act(async () => {
      await result.current.confirm();
    });

    expect(confirmCanvasInstall).toHaveBeenCalledWith("one", "archive-one");
    await waitFor(() => expect(result.current.result?.canvas.id).toBe("canvas-1"));
  });

  it("invalidates a staged review and cancels its server preparation", async () => {
    prepareCanvasInstall.mockResolvedValueOnce(review("one"));
    const { result } = renderHook(() => useCanvasInstall(WORKSPACE_ID));

    await act(async () => {
      await result.current.prepareUrl(FIRST_URL);
    });
    act(() => result.current.invalidate());

    expect(result.current.review).toBeNull();
    expect(result.current.loading).toBe(false);
    expect(cancelCanvasInstall).toHaveBeenCalledWith("one");
  });

  it("does not restore an inspection that finishes after source editing", async () => {
    let resolveInspection: (value: ReturnType<typeof review>) => void = () => undefined;
    const inspection = new Promise<ReturnType<typeof review>>((resolve) => {
      resolveInspection = resolve;
    });
    prepareCanvasInstall.mockReturnValueOnce(inspection);
    const { result } = renderHook(() => useCanvasInstall("workspace-1"));

    let request: Promise<unknown> | undefined;
    await act(async () => {
      request = result.current.prepareUrl("https://example.test/one.tar.gz");
    });
    act(() => result.current.invalidate());
    resolveInspection(review("one"));
    await act(async () => {
      await request;
    });

    expect(result.current.review).toBeNull();
    expect(result.current.loading).toBe(false);
  });
});
