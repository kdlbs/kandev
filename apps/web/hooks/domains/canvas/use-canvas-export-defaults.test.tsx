import { act, cleanup, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

const getDefaults = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/domains/canvas-distribution-api", () => ({
  getCanvasExportDefaults: getDefaults,
}));

import { useCanvasExportDefaults } from "./use-canvas-export-defaults";

afterEach(() => cleanup());
beforeEach(() => getDefaults.mockReset());

describe("useCanvasExportDefaults", () => {
  it("rejects a response for another release", async () => {
    getDefaults.mockResolvedValue({
      expected_release_id: "old-release",
      metadata: {},
      missing_required: [],
    });
    const { result } = renderHook(() => useCanvasExportDefaults("canvas-1", "new-release", true));
    await waitFor(() => expect(result.current.error).toBe(true));
    expect(result.current.defaults).toBeNull();
  });

  it("retries after an error and ignores the previous release after a change", async () => {
    getDefaults.mockRejectedValueOnce(new Error("offline"));
    getDefaults.mockResolvedValueOnce({
      expected_release_id: "release-1",
      metadata: { source_mode: "static" },
      missing_required: [],
    });
    getDefaults.mockResolvedValueOnce({
      expected_release_id: "release-2",
      metadata: { source_mode: "project" },
      missing_required: [],
    });
    const { result, rerender } = renderHook(
      ({ releaseId }) => useCanvasExportDefaults("canvas-1", releaseId, true),
      {
        initialProps: { releaseId: "release-1" },
      },
    );
    await waitFor(() => expect(result.current.error).toBe(true));
    act(() => result.current.retry());
    await waitFor(() => expect(result.current.defaults?.expected_release_id).toBe("release-1"));
    rerender({ releaseId: "release-2" });
    expect(result.current.defaults).toBeNull();
    await waitFor(() => expect(result.current.defaults?.expected_release_id).toBe("release-2"));
  });
});
