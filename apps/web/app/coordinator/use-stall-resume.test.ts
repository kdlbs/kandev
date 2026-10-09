import { act, renderHook } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { WebSocketRequestTimeoutError } from "@/lib/ws/request-error";

const launchMock = vi.hoisted(() => vi.fn());

vi.mock("@/lib/services/session-launch-service", () => ({
  launchSession: (...args: unknown[]) => launchMock(...args),
}));

import {
  pruneStallResumes,
  resetStallResumesForTest,
  resumeStalledTask,
  useStallResume,
} from "./use-stall-resume";

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

beforeEach(() => {
  launchMock.mockReset();
  resetStallResumesForTest();
});

describe("stall resume", () => {
  it("sends a resume request for the session and locks against a second click", async () => {
    const pending = deferred<unknown>();
    launchMock.mockReturnValue(pending.promise);
    const { result } = renderHook(() => useStallResume("t-1"));

    act(() => {
      resumeStalledTask("t-1", "s-1");
      resumeStalledTask("t-1", "s-1");
    });
    expect(launchMock).toHaveBeenCalledTimes(1);
    expect(launchMock.mock.calls[0][0]).toMatchObject({
      task_id: "t-1",
      session_id: "s-1",
      intent: "resume",
    });
    expect(result.current).toEqual({ phase: "sending" });

    await act(async () => pending.resolve({ success: true }));
    expect(result.current).toEqual({ phase: "resuming" });
    act(() => resumeStalledTask("t-1", "s-1"));
    expect(launchMock).toHaveBeenCalledTimes(1);
  });

  it("reports a queued activation", async () => {
    launchMock.mockResolvedValue({ success: true, activation_disposition: "queued" });
    const { result } = renderHook(() => useStallResume("t-1"));
    await act(async () => resumeStalledTask("t-1", "s-1"));
    expect(result.current).toEqual({ phase: "queued" });
  });

  it.each([
    ["success false", { success: false, error: "raw server text" }],
    ["suppressed", { success: true, activation_disposition: "suppressed" }],
  ])("treats %s as a refusal and releases the lock", async (_name, response) => {
    launchMock.mockResolvedValueOnce(response);
    const { result } = renderHook(() => useStallResume("t-1"));
    await act(async () => resumeStalledTask("t-1", "s-1"));
    expect(result.current).toEqual({ phase: "error", unknown: false });
    launchMock.mockResolvedValueOnce({ success: true });
    await act(async () => resumeStalledTask("t-1", "s-1"));
    expect(launchMock).toHaveBeenCalledTimes(2);
    expect(result.current).toEqual({ phase: "resuming" });
  });

  it("marks a timeout as unknown and any other rejection as known", async () => {
    launchMock.mockRejectedValueOnce(new WebSocketRequestTimeoutError("session.launch"));
    const { result } = renderHook(() => useStallResume("t-1"));
    await act(async () => resumeStalledTask("t-1", "s-1"));
    expect(result.current).toEqual({ phase: "error", unknown: true });

    launchMock.mockRejectedValueOnce(new Error("socket closed"));
    await act(async () => resumeStalledTask("t-1", "s-1"));
    expect(result.current).toEqual({ phase: "error", unknown: false });
  });

  it("ignores a late response after the stall left the list", async () => {
    const pending = deferred<unknown>();
    launchMock.mockReturnValue(pending.promise);
    const { result } = renderHook(() => useStallResume("t-1"));
    act(() => resumeStalledTask("t-1", "s-1"));
    act(() => pruneStallResumes(new Set()));
    expect(result.current).toBeUndefined();
    await act(async () => pending.resolve({ success: true }));
    expect(result.current).toBeUndefined();
  });

  it("ignores a superseded response arriving after its successor", async () => {
    const first = deferred<unknown>();
    const second = deferred<unknown>();
    launchMock.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const { result } = renderHook(() => useStallResume("t-1"));

    act(() => resumeStalledTask("t-1", "s-1"));
    act(() => pruneStallResumes(new Set()));
    act(() => resumeStalledTask("t-1", "s-1"));
    expect(launchMock).toHaveBeenCalledTimes(2);

    await act(async () => second.resolve({ success: true, activation_disposition: "queued" }));
    await act(async () => first.reject(new Error("late")));
    expect(result.current).toEqual({ phase: "queued" });
  });

  it("keeps a held entry across a remount and keeps other tasks' entries when pruning", async () => {
    launchMock.mockResolvedValue({ success: true });
    const first = renderHook(() => useStallResume("t-1"));
    await act(async () => resumeStalledTask("t-1", "s-1"));
    first.unmount();
    const second = renderHook(() => useStallResume("t-1"));
    expect(second.result.current).toEqual({ phase: "resuming" });
    act(() => pruneStallResumes(new Set(["t-1"])));
    expect(second.result.current).toEqual({ phase: "resuming" });
  });
});
