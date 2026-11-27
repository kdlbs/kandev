import { act, renderHook, waitFor } from "@testing-library/react";
import { createElement, type ReactNode } from "react";
import { StateProvider } from "@/components/state-provider";
import { afterEach, describe, expect, it, vi } from "vitest";
import {
  invalidateIntegrationAvailability,
  useIntegrationAuthed,
  type IntegrationConfigStatus,
} from "./use-integration-availability";

afterEach(() => {
  vi.clearAllMocks();
});

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((r) => {
    resolve = r;
  });
  return { promise, resolve };
}

function wrapper({ children }: { children: ReactNode }) {
  return createElement(StateProvider, null, children);
}

describe("useIntegrationAuthed", () => {
  it("lets concurrent null results settle once across consumers", async () => {
    const fetchConfig = vi.fn(async (): Promise<IntegrationConfigStatus | null> => null);
    const { result } = renderHook(
      () =>
        [
          useIntegrationAuthed(fetchConfig, { provider: "jira", workspaceId: "ws-1" }),
          useIntegrationAuthed(fetchConfig, { provider: "jira", workspaceId: "ws-1" }),
        ] as const,
      { wrapper },
    );

    await waitFor(() => expect(fetchConfig).toHaveBeenCalled());
    expect(fetchConfig).toHaveBeenCalledTimes(1);
    expect(result.current).toEqual([false, false]);
  });

  it("reports authed once a healthy config resolves", async () => {
    const fetchConfig = vi.fn(
      async (): Promise<IntegrationConfigStatus | null> => ({
        hasSecret: true,
        lastOk: true,
      }),
    );

    const { result } = renderHook(
      () => useIntegrationAuthed(fetchConfig, { provider: "jira", workspaceId: "ws-1" }),
      { wrapper },
    );

    await waitFor(() => expect(result.current).toBe(true));
  });

  it("clears stale authed state while a new fetchConfig probe is in flight", async () => {
    // First workspace: configured and healthy.
    const first = vi.fn(
      async (): Promise<IntegrationConfigStatus | null> => ({
        hasSecret: true,
        lastOk: true,
      }),
    );

    const { result, rerender } = renderHook(
      ({ fetchConfig, workspaceId }) =>
        useIntegrationAuthed(fetchConfig, { provider: "jira", workspaceId }),
      { initialProps: { fetchConfig: first, workspaceId: "ws-1" }, wrapper },
    );

    await waitFor(() => expect(result.current).toBe(true));

    // Switch to a second workspace whose probe has not resolved yet. The hook
    // must drop the previous "authed" result immediately rather than keep
    // showing the prior workspace's state during the in-flight recheck.
    const pending = deferred<IntegrationConfigStatus | null>();
    const second = vi.fn(() => pending.promise);

    rerender({ fetchConfig: second, workspaceId: "ws-2" });

    expect(result.current).toBe(false);

    // The unconfigured workspace resolves to no config; stays not-authed.
    await act(async () => {
      pending.resolve(null);
      await pending.promise;
    });

    await waitFor(() => expect(result.current).toBe(false));
  });

  it("does not probe for a disabled consumer", async () => {
    const fetchConfig = vi.fn(
      async (): Promise<IntegrationConfigStatus | null> => ({
        hasSecret: true,
        lastOk: true,
      }),
    );

    const { result, rerender } = renderHook(
      ({ active }) =>
        useIntegrationAuthed(fetchConfig, { provider: "jira", workspaceId: "ws-1", active }),
      { initialProps: { active: true }, wrapper },
    );

    await waitFor(() => expect(result.current).toBe(true));

    rerender({ active: false });

    expect(result.current).toBe(false);
    expect(fetchConfig).toHaveBeenCalledTimes(1);
  });

  it("refreshes after credential availability is invalidated", async () => {
    const fetchConfig = vi
      .fn<() => Promise<IntegrationConfigStatus | null>>()
      .mockResolvedValueOnce(null)
      .mockResolvedValueOnce({ hasSecret: true, lastOk: true });

    const { result } = renderHook(
      () => useIntegrationAuthed(fetchConfig, { provider: "jira", workspaceId: "ws-1" }),
      { wrapper },
    );

    await waitFor(() => expect(fetchConfig).toHaveBeenCalledTimes(1));
    expect(result.current).toBe(false);

    act(() => invalidateIntegrationAvailability());

    await waitFor(() => expect(result.current).toBe(true));
    expect(fetchConfig).toHaveBeenCalledTimes(2);
  });
});
