import { act, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import {
  fetchShellCommandOutput,
  type ShellCommandOutputSnapshot,
} from "@/lib/api/domains/session-api";
import { useShellCommandOutput } from "./use-shell-command-output";

vi.mock("@/lib/api/domains/session-api", () => ({
  fetchShellCommandOutput: vi.fn(),
}));

const fetchOutputMock = vi.mocked(fetchShellCommandOutput);

function snapshot(
  status: string,
  stdout = "",
  output: ShellCommandOutputSnapshot["output"] = {},
): ShellCommandOutputSnapshot {
  return {
    message_id: "message-1",
    status,
    updated_at: "2026-07-16T12:00:00Z",
    output: { ...output, ...(stdout ? { stdout } : {}) },
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((resolvePromise, rejectPromise) => {
    resolve = resolvePromise;
    reject = rejectPromise;
  });
  return { promise, resolve, reject };
}

async function flushPromises() {
  await act(async () => {
    await Promise.resolve();
    await Promise.resolve();
  });
}

beforeEach(() => {
  vi.useFakeTimers();
  fetchOutputMock.mockReset();
});

afterEach(() => {
  vi.useRealTimers();
});

describe("useShellCommandOutput fetching", () => {
  it("does not fetch while collapsed and fetches one terminal snapshot immediately on open", async () => {
    fetchOutputMock.mockResolvedValue(snapshot("complete", "done", { exit_code: 0 }));
    const { result, rerender } = renderHook(
      ({ isOpen }) =>
        useShellCommandOutput({
          sessionId: "session-1",
          messageId: "message-1",
          isOpen,
          messageStatus: "complete",
        }),
      { initialProps: { isOpen: false } },
    );

    expect(fetchOutputMock).not.toHaveBeenCalled();
    rerender({ isOpen: true });
    expect(result.current.isLoading).toBe(true);
    await flushPromises();

    expect(result.current.snapshot?.output.stdout).toBe("done");
    expect(fetchOutputMock).toHaveBeenCalledTimes(1);
    await act(async () => vi.advanceTimersByTime(10_000));
    expect(fetchOutputMock).toHaveBeenCalledTimes(1);
  });

  it("polls running output after settlement without overlapping requests", async () => {
    const second = deferred<ShellCommandOutputSnapshot>();
    fetchOutputMock
      .mockResolvedValueOnce(snapshot("running", "first"))
      .mockReturnValueOnce(second.promise)
      .mockResolvedValueOnce(snapshot("complete", "unexpected"));
    const { result } = renderHook(() =>
      useShellCommandOutput({
        sessionId: "session-1",
        messageId: "message-1",
        isOpen: true,
        messageStatus: "running",
      }),
    );
    await flushPromises();

    expect(result.current.snapshot?.output.stdout).toBe("first");
    await act(async () => vi.advanceTimersByTime(1_000));
    expect(fetchOutputMock).toHaveBeenCalledTimes(2);
    await act(async () => vi.advanceTimersByTime(10_000));
    expect(fetchOutputMock).toHaveBeenCalledTimes(2);

    second.resolve(snapshot("complete", "finished", { exit_code: 0 }));
    await flushPromises();
    expect(result.current.snapshot?.output.stdout).toBe("finished");
    await act(async () => vi.advanceTimersByTime(10_000));
    expect(fetchOutputMock).toHaveBeenCalledTimes(2);
  });

  it("retains the latest snapshot and caps consecutive failure backoff at five seconds", async () => {
    fetchOutputMock
      .mockResolvedValueOnce(snapshot("running", "retained"))
      .mockRejectedValue(new Error("unavailable"));
    const { result } = renderHook(() =>
      useShellCommandOutput({
        sessionId: "session-1",
        messageId: "message-1",
        isOpen: true,
        messageStatus: "running",
      }),
    );
    await flushPromises();

    const advanceToNextFailure = async (delay: number, expectedCalls: number) => {
      await act(async () => vi.advanceTimersByTime(delay));
      await flushPromises();
      expect(fetchOutputMock).toHaveBeenCalledTimes(expectedCalls);
      expect(result.current.snapshot?.output.stdout).toBe("retained");
      expect(result.current.error?.message).toBe("unavailable");
    };

    await advanceToNextFailure(1_000, 2);
    await advanceToNextFailure(1_000, 3);
    await advanceToNextFailure(2_000, 4);
    await advanceToNextFailure(4_000, 5);
    await advanceToNextFailure(5_000, 6);
    await advanceToNextFailure(5_000, 7);
  });
});

describe("useShellCommandOutput cleanup", () => {
  it("aborts and ignores stale output after collapse", async () => {
    const pending = deferred<ShellCommandOutputSnapshot>();
    let requestSignal: AbortSignal | undefined;
    fetchOutputMock.mockImplementation((_sessionId, _messageId, options) => {
      requestSignal = options?.init?.signal ?? undefined;
      return pending.promise;
    });
    const { result, rerender } = renderHook(
      ({ isOpen }) =>
        useShellCommandOutput({
          sessionId: "session-1",
          messageId: "message-1",
          isOpen,
          messageStatus: "running",
        }),
      { initialProps: { isOpen: true } },
    );

    expect(requestSignal?.aborted).toBe(false);
    rerender({ isOpen: false });
    expect(requestSignal?.aborted).toBe(true);
    pending.resolve(snapshot("running", "stale"));
    await flushPromises();
    expect(result.current.snapshot).toBeNull();
  });

  it("replaces in-flight work with one final fetch when the projected message becomes terminal", async () => {
    const signals: AbortSignal[] = [];
    const finalSnapshot = deferred<ShellCommandOutputSnapshot>();
    fetchOutputMock.mockImplementation((_sessionId, _messageId, options) => {
      const signal = options?.init?.signal;
      if (signal) signals.push(signal);
      if (signals.length === 2) return finalSnapshot.promise;
      return new Promise<ShellCommandOutputSnapshot>(() => undefined);
    });
    const projectedTerminal = renderHook(
      ({ messageStatus }) =>
        useShellCommandOutput({
          sessionId: "session-1",
          messageId: "message-1",
          isOpen: true,
          messageStatus,
        }),
      { initialProps: { messageStatus: "running" } },
    );

    projectedTerminal.rerender({ messageStatus: "complete" });
    expect(signals[0]?.aborted).toBe(true);
    expect(signals).toHaveLength(2);
    expect(signals[1]?.aborted).toBe(false);
    finalSnapshot.resolve(snapshot("complete", "final output", { exit_code: 0 }));
    await flushPromises();
    expect(projectedTerminal.result.current.snapshot?.output.stdout).toBe("final output");
    await act(async () => vi.advanceTimersByTime(10_000));
    expect(fetchOutputMock).toHaveBeenCalledTimes(2);
    projectedTerminal.unmount();
  });
});

describe("useShellCommandOutput terminal lifecycle", () => {
  it("aborts in-flight work on unmount", () => {
    const signals: AbortSignal[] = [];
    fetchOutputMock.mockImplementation((_sessionId, _messageId, options) => {
      const signal = options?.init?.signal;
      if (signal) signals.push(signal);
      return new Promise<ShellCommandOutputSnapshot>(() => undefined);
    });
    const unmounted = renderHook(() =>
      useShellCommandOutput({
        sessionId: "session-1",
        messageId: "message-1",
        isOpen: true,
        messageStatus: "running",
      }),
    );
    expect(signals).toHaveLength(1);
    unmounted.unmount();
    expect(signals[0]?.aborted).toBe(true);
  });

  it.each(["completed", "success", "failed"])(
    "treats the projected %s alias as terminal after the final snapshot",
    async (terminalStatus) => {
      fetchOutputMock
        .mockResolvedValueOnce(snapshot("running", "partial"))
        .mockResolvedValueOnce(snapshot("running", "finalizing"));
      const view = renderHook(
        ({ messageStatus }) =>
          useShellCommandOutput({
            sessionId: "session-1",
            messageId: "message-1",
            isOpen: true,
            messageStatus,
          }),
        { initialProps: { messageStatus: "running" } },
      );
      await flushPromises();

      view.rerender({ messageStatus: terminalStatus });
      await flushPromises();
      expect(fetchOutputMock).toHaveBeenCalledTimes(2);
      await act(async () => vi.advanceTimersByTime(10_000));
      expect(fetchOutputMock).toHaveBeenCalledTimes(2);
    },
  );
});
