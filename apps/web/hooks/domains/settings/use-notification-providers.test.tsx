import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { useNotificationProviders } from "./use-notification-providers";

const mocks = vi.hoisted(() => ({
  listNotificationProviders: vi.fn(),
  setAppriseAvailable: vi.fn(),
  setNotificationProviders: vi.fn(),
  setNotificationProvidersLoading: vi.fn(),
}));

vi.mock("@/lib/api", () => ({
  listNotificationProviders: mocks.listNotificationProviders,
}));

const storeState = {
  notificationProviders: {
    items: [],
    events: [],
    appriseAvailable: false,
    loaded: true,
    loading: false,
  },
};

vi.mock("@/components/state-provider", () => ({
  useAppStore: (selector: (state: Record<string, unknown>) => unknown) =>
    selector({
      ...storeState,
      setAppriseAvailable: mocks.setAppriseAvailable,
      setNotificationProviders: mocks.setNotificationProviders,
      setNotificationProvidersLoading: mocks.setNotificationProvidersLoading,
    }),
}));

function providerResponse(appriseAvailable: boolean) {
  return {
    providers: [],
    events: [],
    apprise_available: appriseAvailable,
  };
}

beforeEach(() => {
  vi.clearAllMocks();
  storeState.notificationProviders.appriseAvailable = false;
  mocks.listNotificationProviders.mockReset();
});

describe("useNotificationProviders Apprise rescan", () => {
  it.each([
    [false, true],
    [true, false],
  ])("updates availability from %s to %s", async (initial, next) => {
    storeState.notificationProviders.appriseAvailable = initial;
    mocks.listNotificationProviders.mockResolvedValueOnce(providerResponse(next));

    const { result } = renderHook(() => useNotificationProviders());

    await act(async () => {
      await result.current.rescanApprise();
    });

    expect(mocks.listNotificationProviders).toHaveBeenCalledWith({ cache: "no-store" });
    expect(mocks.setAppriseAvailable).toHaveBeenCalledWith(next);
    expect(result.current.appriseRescanResult).toBe(next);
    expect(result.current.appriseRescanError).toBe(false);
  });

  it("blocks duplicate requests while the first request is pending", async () => {
    let resolveRequest: (value: ReturnType<typeof providerResponse>) => void = () => undefined;
    const request = new Promise<ReturnType<typeof providerResponse>>((resolve) => {
      resolveRequest = resolve;
    });
    mocks.listNotificationProviders.mockReturnValueOnce(request);

    const { result } = renderHook(() => useNotificationProviders());

    act(() => {
      void result.current.rescanApprise();
      void result.current.rescanApprise();
    });

    await waitFor(() => expect(result.current.appriseRescanPending).toBe(true));
    expect(mocks.listNotificationProviders).toHaveBeenCalledOnce();

    await act(async () => {
      resolveRequest(providerResponse(true));
      await request;
    });
    await waitFor(() => expect(result.current.appriseRescanPending).toBe(false));
  });

  it("retains the last availability after failure and retries successfully", async () => {
    storeState.notificationProviders.appriseAvailable = true;
    mocks.listNotificationProviders
      .mockRejectedValueOnce(new Error("backend unavailable"))
      .mockResolvedValueOnce(providerResponse(false));

    const { result } = renderHook(() => useNotificationProviders());

    await act(async () => {
      await result.current.rescanApprise();
    });

    expect(result.current.appriseRescanError).toBe(true);
    expect(result.current.appriseRescanResult).toBeNull();
    expect(mocks.setAppriseAvailable).not.toHaveBeenCalled();

    await act(async () => {
      await result.current.rescanApprise();
    });

    expect(mocks.setAppriseAvailable).toHaveBeenCalledWith(false);
    expect(result.current.appriseRescanError).toBe(false);
    expect(result.current.appriseRescanResult).toBe(false);
  });

  it("updates only availability when a delayed response resolves", async () => {
    let resolveRequest: (value: ReturnType<typeof providerResponse>) => void = () => undefined;
    const request = new Promise<ReturnType<typeof providerResponse>>((resolve) => {
      resolveRequest = resolve;
    });
    mocks.listNotificationProviders.mockReturnValueOnce(request);

    const { result } = renderHook(() => useNotificationProviders());

    act(() => {
      void result.current.rescanApprise();
    });
    await waitFor(() => expect(result.current.appriseRescanPending).toBe(true));

    await act(async () => {
      resolveRequest(providerResponse(true));
      await request;
    });

    expect(mocks.setAppriseAvailable).toHaveBeenCalledWith(true);
    expect(mocks.setNotificationProviders).not.toHaveBeenCalled();
  });
});
