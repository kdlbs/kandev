import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { cleanup, renderHook, waitFor } from "@testing-library/react";

const mockRequest = vi.fn();

vi.mock("@/lib/ws/connection", () => ({
  getWebSocketClient: () => ({ request: mockRequest }),
}));

import { useSessionDeleteWarning } from "./use-session-delete-warning";

describe("useSessionDeleteWarning", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("returns null while the dialog is closed", () => {
    const { result } = renderHook(() => useSessionDeleteWarning(false, "session-1"));
    expect(result.current).toBeNull();
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("returns null when no session id is known", () => {
    const { result } = renderHook(() => useSessionDeleteWarning(true, null));
    expect(result.current).toBeNull();
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("reports uncommitted files and unpushed commits from the latest snapshot", async () => {
    mockRequest.mockResolvedValue({
      snapshots: [
        {
          branch: "feature/x",
          ahead: 2,
          files: { "a.ts": {}, "b.ts": {}, "c.ts": {} },
        },
      ],
    });

    const { result } = renderHook(() => useSessionDeleteWarning(true, "session-1"));

    await waitFor(() => {
      expect(result.current).toEqual({ uncommittedFiles: 3, unpushedCommits: 2 });
    });
    expect(mockRequest).toHaveBeenCalledWith(
      "session.git.snapshots",
      expect.objectContaining({ session_id: "session-1" }),
    );
  });

  it("returns zero counts for a clean session level with its remote", async () => {
    mockRequest.mockResolvedValue({
      snapshots: [{ branch: "main", ahead: 0, files: {} }],
    });

    const { result } = renderHook(() => useSessionDeleteWarning(true, "session-clean"));

    await waitFor(() => {
      expect(result.current).toEqual({ uncommittedFiles: 0, unpushedCommits: 0 });
    });
  });

  it("sums counts across a multi-repo session's distinct branches without double-counting history", async () => {
    mockRequest.mockResolvedValue({
      snapshots: [
        // Newest-first history: two rows for "repo-a/main" (only the first/newest
        // should count) interleaved with one row for "repo-b/feature".
        { branch: "repo-a/main", ahead: 1, files: { "x.ts": {} } },
        { branch: "repo-b/feature", ahead: 2, files: { "y.ts": {}, "z.ts": {} } },
        { branch: "repo-a/main", ahead: 5, files: { "stale.ts": {}, "also-stale.ts": {} } },
      ],
    });

    const { result } = renderHook(() => useSessionDeleteWarning(true, "session-multi"));

    await waitFor(() => {
      expect(result.current).toEqual({ uncommittedFiles: 3, unpushedCommits: 3 });
    });
  });

  it("returns null when the fetch fails, rather than throwing", async () => {
    mockRequest.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() => useSessionDeleteWarning(true, "session-error"));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalled();
    });
    expect(result.current).toBeNull();
  });

  it("clears the previous session's result when the dialog closes", async () => {
    mockRequest.mockResolvedValue({
      snapshots: [{ branch: "main", ahead: 1, files: { "a.ts": {} } }],
    });
    const { result, rerender } = renderHook(
      ({ open, sessionId }: { open: boolean; sessionId: string | null }) =>
        useSessionDeleteWarning(open, sessionId),
      { initialProps: { open: true, sessionId: "session-1" } },
    );

    await waitFor(() => {
      expect(result.current).toEqual({ uncommittedFiles: 1, unpushedCommits: 1 });
    });

    rerender({ open: false, sessionId: "session-1" });
    expect(result.current).toBeNull();
  });
});
