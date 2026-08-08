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

  it("returns null/loaded while the dialog is closed", () => {
    const { result } = renderHook(() => useSessionDeleteWarning(false, "session-1"));
    expect(result.current).toEqual({ warning: null, isLoaded: true });
    expect(mockRequest).not.toHaveBeenCalled();
  });

  it("returns null/loaded when no session id is known", () => {
    const { result } = renderHook(() => useSessionDeleteWarning(true, null));
    expect(result.current).toEqual({ warning: null, isLoaded: true });
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
      expect(result.current).toEqual({
        warning: { uncommittedFiles: 3, unpushedCommits: 2 },
        isLoaded: true,
      });
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
      expect(result.current).toEqual({
        warning: { uncommittedFiles: 0, unpushedCommits: 0 },
        isLoaded: true,
      });
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
      expect(result.current).toEqual({
        warning: { uncommittedFiles: 3, unpushedCommits: 3 },
        isLoaded: true,
      });
    });
  });

  it("returns null when the fetch fails, rather than throwing, and still settles to loaded", async () => {
    mockRequest.mockRejectedValue(new Error("boom"));

    const { result } = renderHook(() => useSessionDeleteWarning(true, "session-error"));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalled();
    });
    await waitFor(() => {
      expect(result.current).toEqual({ warning: null, isLoaded: true });
    });
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
      expect(result.current).toEqual({
        warning: { uncommittedFiles: 1, unpushedCommits: 1 },
        isLoaded: true,
      });
    });

    rerender({ open: false, sessionId: "session-1" });
    expect(result.current).toEqual({ warning: null, isLoaded: true });
  });
});

describe("useSessionDeleteWarning isLoaded gating", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("is not loaded while the fetch is still in flight — the confirm control must wait", async () => {
    let resolveRequest!: (value: { snapshots: unknown[] }) => void;
    mockRequest.mockReturnValue(
      new Promise((resolve) => {
        resolveRequest = resolve;
      }),
    );

    const { result } = renderHook(() => useSessionDeleteWarning(true, "session-pending"));

    await waitFor(() => {
      expect(mockRequest).toHaveBeenCalled();
    });
    expect(result.current).toEqual({ warning: null, isLoaded: false });

    resolveRequest({ snapshots: [{ branch: "main", ahead: 0, files: {} }] });

    await waitFor(() => {
      expect(result.current.isLoaded).toBe(true);
    });
  });

  it("resets isLoaded to false when a new session's dialog opens after a prior one settled", async () => {
    mockRequest.mockResolvedValue({
      snapshots: [{ branch: "main", ahead: 0, files: {} }],
    });
    const { result, rerender } = renderHook(
      ({ sessionId }: { sessionId: string }) => useSessionDeleteWarning(true, sessionId),
      { initialProps: { sessionId: "session-a" } },
    );
    await waitFor(() => expect(result.current.isLoaded).toBe(true));

    let resolveNext!: (value: { snapshots: unknown[] }) => void;
    mockRequest.mockReturnValue(
      new Promise((resolve) => {
        resolveNext = resolve;
      }),
    );
    rerender({ sessionId: "session-b" });
    expect(result.current.isLoaded).toBe(false);

    resolveNext({ snapshots: [] });
    await waitFor(() => expect(result.current.isLoaded).toBe(true));
  });
});

describe("useSessionDeleteWarning metadata fallback", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    cleanup();
  });

  it("falls back to metadata file counts when the newest row's files map is empty (round-2 CRITICAL regression)", async () => {
    mockRequest.mockResolvedValue({
      snapshots: [
        // Newest row: a live_monitor tick after the user hand-edited a file
        // outside the agent turn. Its files map is always empty by design
        // (see git_snapshots.go UpsertLatestLiveGitSnapshot), but metadata
        // still carries the real per-category file lists.
        {
          branch: "main",
          ahead: 1,
          files: {},
          metadata: {
            modified: ["a.ts"],
            added: [],
            deleted: [],
            untracked: ["b.txt"],
            renamed: [],
          },
        },
        // Older, now-stale agent_completed row for the same branch. Under
        // the old "agent_completed always first" ordering this would have
        // shadowed the row above and reported 0/0.
        { branch: "main", ahead: 5, files: { "stale.ts": {} } },
      ],
    });

    const { result } = renderHook(() => useSessionDeleteWarning(true, "session-metadata-fallback"));

    await waitFor(() => {
      expect(result.current).toEqual({
        warning: { uncommittedFiles: 2, unpushedCommits: 1 },
        isLoaded: true,
      });
    });
  });

  it("prefers the files map over metadata when both are present on the newest row", async () => {
    mockRequest.mockResolvedValue({
      snapshots: [
        {
          branch: "main",
          ahead: 1,
          files: { "a.ts": {}, "b.ts": {} },
          metadata: {
            modified: ["a.ts", "b.ts", "c.ts"],
            added: [],
            deleted: [],
            untracked: [],
            renamed: [],
          },
        },
      ],
    });

    const { result } = renderHook(() => useSessionDeleteWarning(true, "session-files-preferred"));

    await waitFor(() => {
      expect(result.current).toEqual({
        warning: { uncommittedFiles: 2, unpushedCommits: 1 },
        isLoaded: true,
      });
    });
  });
});
