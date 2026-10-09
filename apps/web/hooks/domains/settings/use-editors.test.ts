import { act, renderHook, waitFor } from "@testing-library/react";
import { beforeEach, describe, expect, it, vi } from "vitest";
import { createStore, useStore } from "zustand";
import { useEditors } from "./use-editors";

const mocks = vi.hoisted(() => ({ listEditors: vi.fn() }));
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
vi.mock("@/lib/ws/connection", () => ({ getWebSocketClient: () => null }));
vi.mock("@/hooks/use-ensure-user-settings", () => ({ useEnsureUserSettings: () => {} }));
vi.mock("@/components/state-provider", () => ({
  useAppStoreApi: () => store,
  useAppStore: (select: (state: unknown) => unknown) => useStore(store, select),
}));
beforeEach(() => {
  vi.resetAllMocks();
  store = makeStore();
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
