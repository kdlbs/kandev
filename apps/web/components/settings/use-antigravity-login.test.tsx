import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, renderHook } from "@testing-library/react";
import { ApiError } from "@/lib/api/client";

const { startAntigravityLogin, getAntigravityLogin, cancelAntigravityLogin } = vi.hoisted(() => ({
  startAntigravityLogin: vi.fn(),
  getAntigravityLogin: vi.fn(),
  cancelAntigravityLogin: vi.fn(),
}));

vi.mock("@/lib/api/domains/antigravity-auth-api", () => ({
  startAntigravityLogin,
  getAntigravityLogin,
  cancelAntigravityLogin,
}));

vi.mock("@/lib/utils/copy-to-clipboard", () => ({ copyToClipboard: vi.fn() }));

import { useAntigravityLogin } from "./use-antigravity-login";

const active = {
  attempt_id: "attempt-1",
  state: "waiting_for_browser" as const,
  expires_at: "2099-01-01T00:00:00Z",
  authorization_url:
    "https://accounts.google.com/o/oauth2/v2/auth?client_id=x&state=y&redirect_uri=http%3A%2F%2F127.0.0.1%3A5555%2Fcb",
};
const succeeded = { ...active, state: "succeeded" as const, authorization_url: undefined };

beforeEach(() => {
  vi.useFakeTimers();
});

afterEach(() => {
  vi.useRealTimers();
  vi.clearAllMocks();
});

describe("useAntigravityLogin", () => {
  it("keeps polling after a transient status failure", async () => {
    getAntigravityLogin
      .mockResolvedValueOnce(active)
      .mockRejectedValueOnce(new Error("temporary"))
      .mockResolvedValueOnce(succeeded);
    const onComplete = vi.fn();
    const { result } = renderHook(() => useAntigravityLogin({ open: true, onComplete }));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    expect(result.current.snapshot?.attempt_id).toBe("attempt-1");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(onComplete).toHaveBeenCalledTimes(1);
  });

  it("clears a missing attempt after a backend restart", async () => {
    getAntigravityLogin
      .mockResolvedValueOnce(active)
      .mockRejectedValueOnce(new ApiError("not found", 404, {}));
    const { result } = renderHook(() => useAntigravityLogin({ open: true }));

    await act(async () => {
      await vi.advanceTimersByTimeAsync(0);
    });
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1000);
    });

    expect(result.current.snapshot).toBeNull();
    expect(result.current.error).toContain("expired");
  });

  it("waits for a pending start before dismissing", async () => {
    let resolveStart: ((value: typeof active) => void) | undefined;
    startAntigravityLogin.mockReturnValueOnce(
      new Promise<typeof active>((resolve) => {
        resolveStart = resolve;
      }),
    );
    cancelAntigravityLogin.mockResolvedValueOnce({ ...active, state: "cancelled" });
    const { result } = renderHook(() => useAntigravityLogin({ open: true }));

    const startPromise = result.current.start();
    const dismissPromise = result.current.dismiss();
    await act(async () => {
      resolveStart?.(active);
      await startPromise;
      await dismissPromise;
    });

    expect(cancelAntigravityLogin).toHaveBeenCalledWith("attempt-1");
  });

  it("discards an old status response that resolves after a new attempt starts", async () => {
    let resolveOld: ((value: typeof active) => void) | undefined;
    const oldResponse = new Promise<typeof active>((resolve) => {
      resolveOld = resolve;
    });
    getAntigravityLogin.mockReturnValueOnce(oldResponse);
    startAntigravityLogin.mockResolvedValueOnce({ ...active, attempt_id: "attempt-2" });
    const { result } = renderHook(() => useAntigravityLogin({ open: true }));

    await act(async () => {
      await Promise.resolve();
    });
    await act(async () => {
      await result.current.start();
    });
    await act(async () => {
      resolveOld?.(active);
      await Promise.resolve();
    });

    expect(result.current.snapshot?.attempt_id).toBe("attempt-2");
  });
});
