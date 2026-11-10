import { StrictMode } from "react";
import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createStore, useStore } from "zustand";
import { useEditors } from "./use-editors";

const mocks = vi.hoisted(() => ({ listEditors: vi.fn(), getWebSocketClient: vi.fn() }));
const makeStore = (capability?: boolean) =>
  createStore<{
    editors: {
      items: never[];
      loaded: boolean;
      loading: boolean;
      folderOpeningAvailable?: boolean;
    };
    setEditors: (items: never[], available?: boolean) => void;
    setEditorsLoading: (loading: boolean) => void;
  }>((set) => ({
    editors: { items: [], loaded: true, loading: false, folderOpeningAvailable: capability },
    setEditors: (items, available) =>
      set((state) => ({
        editors: { ...state.editors, items, loaded: true, folderOpeningAvailable: available },
      })),
    setEditorsLoading: (loading) => set((state) => ({ editors: { ...state.editors, loading } })),
  }));
let store: ReturnType<typeof makeStore>;
vi.mock("@/lib/api", () => ({ listEditors: mocks.listEditors }));
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: mocks.getWebSocketClient }));
vi.mock("@/hooks/use-ensure-user-settings", () => ({ useEnsureUserSettings: () => {} }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (select: (state: unknown) => unknown) => useStore(store, select),
}));
beforeEach(() => {
  vi.resetAllMocks();
  store = makeStore();
  mocks.getWebSocketClient.mockReturnValue(null);
});

describe("user subscription ownership", () => {
  it("balances subscriptions through Strict Mode effect replay", () => {
    store = makeStore(true);
    let references = 1;
    const client = {
      subscribeUser: vi.fn(() => references++),
      unsubscribeUser: vi.fn(() => references--),
    };
    mocks.getWebSocketClient.mockReturnValue(client);
    const hook = renderHook(() => useEditors(), { wrapper: StrictMode });
    expect(client.subscribeUser).toHaveBeenCalledTimes(2);
    expect(references).toBe(2);
    hook.unmount();
    expect(references).toBe(1);
  });

  it("does not release a client that was absent at mount", () => {
    store = makeStore(true);
    const hook = renderHook(() => useEditors());
    const replacement = { subscribeUser: vi.fn(), unsubscribeUser: vi.fn() };
    mocks.getWebSocketClient.mockReturnValue(replacement);
    hook.unmount();
    expect(replacement.subscribeUser).not.toHaveBeenCalled();
    expect(replacement.unsubscribeUser).not.toHaveBeenCalled();
  });

  it("releases editor consumers after 100 mount and unmount cycles", () => {
    store = makeStore(true);
    let references = 1;
    const client = {
      subscribeUser: vi.fn(() => references++),
      unsubscribeUser: vi.fn(() => references--),
    };
    mocks.getWebSocketClient.mockReturnValue(client);
    for (let index = 0; index < 100; index++) {
      const hook = renderHook(() => [useEditors(), useEditors()]);
      hook.unmount();
    }
    expect(references).toBe(1);
    expect(client.subscribeUser).toHaveBeenCalledTimes(200);
    expect(client.unsubscribeUser).toHaveBeenCalledTimes(200);
    expect(mocks.listEditors).not.toHaveBeenCalled();
  });

  it("keeps one subscription per mounted consumer across rerenders", () => {
    store = makeStore(true);
    const client = { subscribeUser: vi.fn(), unsubscribeUser: vi.fn() };
    mocks.getWebSocketClient.mockReturnValue(client);
    const hook = renderHook(() => useEditors());
    hook.rerender();
    hook.rerender();
    expect(client.subscribeUser).toHaveBeenCalledOnce();
    expect(client.unsubscribeUser).not.toHaveBeenCalled();
    hook.unmount();
    expect(client.unsubscribeUser).toHaveBeenCalledOnce();
  });

  it("releases the acquired client when the global client changes", () => {
    store = makeStore(true);
    const original = { subscribeUser: vi.fn(), unsubscribeUser: vi.fn() };
    const replacement = { subscribeUser: vi.fn(), unsubscribeUser: vi.fn() };
    mocks.getWebSocketClient.mockReturnValue(original);
    const hook = renderHook(() => useEditors());
    mocks.getWebSocketClient.mockReturnValue(replacement);
    hook.unmount();
    expect(original.unsubscribeUser).toHaveBeenCalledOnce();
    expect(replacement.unsubscribeUser).not.toHaveBeenCalled();
  });
});

describe("folder capability hydration", () => {
  it("shares discovery across consumers mounted in the same render", async () => {
    let finish!: (value: { editors: never[]; folder_opening_available: boolean }) => void;
    mocks.listEditors.mockReturnValue(
      new Promise((resolve) => {
        finish = resolve;
      }),
    );
    const { result } = renderHook(() => [useEditors(), useEditors(), useEditors()]);
    const count = mocks.listEditors.mock.calls.length;
    await act(async () => {
      finish({ editors: [], folder_opening_available: true });
    });
    expect(count).toBe(1);
    expect(result.current.every((editor) => editor.folderOpeningAvailable)).toBe(true);
  });
  it("fetches missing capability even when editor items were already loaded", async () => {
    let resolve!: (value: { editors: never[]; folder_opening_available: boolean }) => void;
    mocks.listEditors.mockReturnValue(
      new Promise((done) => {
        resolve = done;
      }),
    );
    const { result } = renderHook(() => useEditors());
    await waitFor(() => expect(mocks.listEditors).toHaveBeenCalledTimes(1));
    expect(result.current.folderOpeningAvailable).toBe(false);
    await act(async () => {
      resolve({ editors: [], folder_opening_available: true });
    });
    expect(result.current.folderOpeningAvailable).toBe(true);
  });
  it.each([true, false])("uses the boot capability %s without another request", (available) => {
    store = makeStore(available);
    const { result } = renderHook(() => useEditors());
    expect(result.current.folderOpeningAvailable).toBe(available);
    expect(mocks.listEditors).not.toHaveBeenCalled();
  });
  it("retries transient discovery once while keeping folder opening disabled", async () => {
    mocks.listEditors.mockRejectedValueOnce(new Error("offline")).mockResolvedValueOnce({
      editors: [],
      folder_opening_available: true,
    });
    const { result } = renderHook(() => useEditors());
    expect(result.current.folderOpeningAvailable).toBe(false);
    await waitFor(() => expect(result.current.folderOpeningAvailable).toBe(true), {
      timeout: 3000,
    });
    expect(mocks.listEditors).toHaveBeenCalledTimes(2);
  });
  it("settles failed discovery as unavailable without repeated fetching", async () => {
    mocks.listEditors.mockRejectedValue(new Error("offline"));
    const { result } = renderHook(() => useEditors());
    await waitFor(() => expect(store.getState().editors.folderOpeningAvailable).toBe(false), {
      timeout: 3000,
    });
    expect(result.current.folderOpeningAvailable).toBe(false);
    expect(mocks.listEditors).toHaveBeenCalledTimes(2);
  });
});
