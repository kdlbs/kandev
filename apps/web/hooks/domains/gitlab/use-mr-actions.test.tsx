import type { ReactNode } from "react";
import { act, renderHook } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";
import { ToastProvider } from "@/components/toast-provider";
import { useMRActions } from "./use-mr-actions";

const wrapper = ({ children }: { children: ReactNode }) => (
  <ToastProvider>{children}</ToastProvider>
);

describe("useMRActions", () => {
  it("refreshes after a success unless the run skips it", async () => {
    const onRefresh = vi.fn();
    const { result } = renderHook(() => useMRActions(onRefresh), { wrapper });
    await act(async () => {
      expect(await result.current.run("approve", async () => ({}))).toBe(true);
    });
    expect(onRefresh).toHaveBeenCalledTimes(1);
    await act(async () => {
      expect(await result.current.run("ready", async () => ({}), { skipRefresh: true })).toBe(true);
    });
    expect(onRefresh).toHaveBeenCalledTimes(1);
  });

  it("reports a failure without refreshing and keeps pendingAction shared", async () => {
    const onRefresh = vi.fn();
    const { result } = renderHook(() => useMRActions(onRefresh), { wrapper });
    let release!: () => void;
    let pending!: Promise<boolean>;
    act(() => {
      pending = result.current.run(
        "ready",
        () =>
          new Promise((resolve) => {
            release = () => resolve({});
          }),
        { skipRefresh: true },
      );
    });
    expect(result.current.pendingAction).toBe("ready");
    await act(async () => {
      release();
      await pending;
    });
    expect(result.current.pendingAction).toBeNull();
    await act(async () => {
      expect(
        await result.current.run("ready", async () => Promise.reject(new Error("boom")), {
          skipRefresh: true,
        }),
      ).toBe(false);
    });
    expect(onRefresh).not.toHaveBeenCalled();
  });
});
