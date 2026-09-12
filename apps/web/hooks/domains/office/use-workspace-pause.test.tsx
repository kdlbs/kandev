import { describe, expect, it, vi, beforeEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { ApiError } from "@/lib/api/client";
import { useWorkspacePause } from "./use-workspace-pause";

const mocks = vi.hoisted(() => ({
  getWorkspacePause: vi.fn(),
  postWorkspacePause: vi.fn(),
  postWorkspaceResume: vi.fn(),
}));

vi.mock("@/lib/api/domains/office-pause-api", () => ({
  getWorkspacePause: mocks.getWorkspacePause,
  postWorkspacePause: mocks.postWorkspacePause,
  postWorkspaceResume: mocks.postWorkspaceResume,
}));

const beginPauseRequest = vi.fn();
const resetPauseState = vi.fn();
const applyPauseResponse = vi.fn();

// A mutable snapshot read via `storeApi.getState()` inside the hook's async
// callbacks, so a test can change `activeId` between issuing a request and
// resolving it to simulate the operator switching workspaces mid-flight.
let snapshot: {
  workspaces: { activeId: string | null };
  office: { pause: { record: unknown; status: "unknown" | "known" } };
  connection: { status: string };
} = {
  workspaces: { activeId: "ws-1" },
  office: { pause: { record: null, status: "unknown" } },
  connection: { status: "connected" },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (sel: (state: unknown) => unknown) =>
    sel({ ...snapshot, beginPauseRequest, resetPauseState, applyPauseResponse }),
  useAppStoreApi: () => ({ getState: () => snapshot }),
}));

beforeEach(() => {
  beginPauseRequest.mockReset();
  resetPauseState.mockReset();
  applyPauseResponse.mockReset();
  applyPauseResponse.mockReturnValue(true);
  let nextTag = 1;
  beginPauseRequest.mockImplementation(() => nextTag++);
  snapshot = {
    workspaces: { activeId: "ws-1" },
    office: { pause: { record: null, status: "unknown" } },
    connection: { status: "connected" },
  };
  mocks.getWorkspacePause.mockReset();
  mocks.postWorkspacePause.mockReset();
  mocks.postWorkspaceResume.mockReset();
  mocks.getWorkspacePause.mockResolvedValue({ workspaceId: "ws-1", paused: false, record: null });
});

describe("useWorkspacePause: mount, workspace change, reconnect", () => {
  it("resets and reads on mount", async () => {
    renderHook(() => useWorkspacePause("ws-1"));
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledWith("ws-1"));
    expect(resetPauseState).toHaveBeenCalledTimes(1);
    expect(applyPauseResponse).toHaveBeenCalledWith(
      expect.any(Number),
      "ws-1",
      "ws-1",
      expect.objectContaining({ kind: "read-success", paused: false }),
    );
  });

  it("resets and re-reads when the workspace changes", async () => {
    const { rerender } = renderHook(({ id }: { id: string | null }) => useWorkspacePause(id), {
      initialProps: { id: "ws-1" },
    });
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledTimes(1));
    snapshot = { ...snapshot, workspaces: { activeId: "ws-2" } };
    rerender({ id: "ws-2" });
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledWith("ws-2"));
    expect(resetPauseState).toHaveBeenCalledTimes(2);
  });

  it("issues a read when the WS connection transitions to connected", async () => {
    snapshot = { ...snapshot, connection: { status: "reconnecting" } };
    const { rerender } = renderHook(() => useWorkspacePause("ws-1"));
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledTimes(1));
    snapshot = { ...snapshot, connection: { status: "connected" } };
    rerender();
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledTimes(2));
    // Reconnect does not reset status/record first (design: "status
    // unchanged, the last known state still being the best available").
    expect(resetPauseState).toHaveBeenCalledTimes(1);
  });
});

describe("useWorkspacePause: reads", () => {
  it("applies read-failure on a rejected GET", async () => {
    mocks.getWorkspacePause.mockRejectedValueOnce(new Error("boom"));
    renderHook(() => useWorkspacePause("ws-1"));
    await waitFor(() =>
      expect(applyPauseResponse).toHaveBeenCalledWith(expect.any(Number), "ws-1", "ws-1", {
        kind: "read-failure",
      }),
    );
  });

  it("refresh() issues another read on demand", async () => {
    const { result } = renderHook(() => useWorkspacePause("ws-1"));
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledTimes(1));
    await act(async () => {
      await result.current.refresh();
    });
    expect(mocks.getWorkspacePause).toHaveBeenCalledTimes(2);
  });
});

describe("useWorkspacePause: mutations", () => {
  it("pause() posts the reason and applies a mutate-success outcome", async () => {
    mocks.postWorkspacePause.mockResolvedValue({
      workspaceId: "ws-1",
      paused: true,
      record: { id: "p1", reason: "r", createdBy: "u", createdByKind: "user", createdAt: "t" },
    });
    const { result } = renderHook(() => useWorkspacePause("ws-1"));
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledTimes(1));

    let outcome: Awaited<ReturnType<typeof result.current.pause>> | undefined;
    await act(async () => {
      outcome = await result.current.pause("incident");
    });

    expect(mocks.postWorkspacePause).toHaveBeenCalledWith("ws-1", "incident");
    expect(outcome).toEqual({ ok: true });
    expect(applyPauseResponse).toHaveBeenCalledWith(
      expect.any(Number),
      "ws-1",
      "ws-1",
      expect.objectContaining({ kind: "mutate-success", paused: true }),
    );
  });

  it("resume() defaults to an empty reason", async () => {
    mocks.postWorkspaceResume.mockResolvedValue({
      workspaceId: "ws-1",
      paused: false,
      record: null,
    });
    const { result } = renderHook(() => useWorkspacePause("ws-1"));
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledTimes(1));
    await act(async () => {
      await result.current.resume();
    });
    expect(mocks.postWorkspaceResume).toHaveBeenCalledWith("ws-1", "");
  });

  it("a superseded mutation failure (guard rejects) still surfaces an error to its own caller", async () => {
    // The guard result only decides whether this outcome updates the shared
    // store (AC-006.6's F51 supersession guard); it must not swallow the
    // failure from the caller who is still awaiting this specific promise.
    applyPauseResponse.mockReturnValueOnce(true); // the read on mount
    applyPauseResponse.mockReturnValueOnce(false); // the superseded mutate-failure
    mocks.postWorkspacePause.mockRejectedValueOnce(new ApiError("stale request", 409, null));
    const { result } = renderHook(() => useWorkspacePause("ws-1"));
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledTimes(1));

    let outcome: Awaited<ReturnType<typeof result.current.pause>> | undefined;
    await act(async () => {
      outcome = await result.current.pause("incident");
    });

    expect(outcome?.ok).toBe(false);
    expect((outcome as { ok: false; error?: string }).error).toBe("stale request");
  });

  it("a non-superseded mutation failure surfaces an error message", async () => {
    // The hook only surfaces ApiError messages (server-controlled, user-facing
    // text); an arbitrary JS Error falls back to a generic i18n message.
    mocks.postWorkspacePause.mockRejectedValueOnce(new ApiError("server exploded", 500, null));
    const { result } = renderHook(() => useWorkspacePause("ws-1"));
    await waitFor(() => expect(mocks.getWorkspacePause).toHaveBeenCalledTimes(1));

    let outcome: Awaited<ReturnType<typeof result.current.pause>> | undefined;
    await act(async () => {
      outcome = await result.current.pause("incident");
    });

    expect(outcome?.ok).toBe(false);
    expect((outcome as { ok: false; error?: string }).error).toBe("server exploded");
  });
});

describe("useWorkspacePause: no workspace selected", () => {
  it("does nothing when no workspace is selected", () => {
    renderHook(() => useWorkspacePause(null));
    expect(mocks.getWorkspacePause).not.toHaveBeenCalled();
    expect(resetPauseState).not.toHaveBeenCalled();
  });
});
