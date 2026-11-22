import { act, cleanup, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { createAppStore, type AppState } from "@/lib/state/store";
import type { Executor } from "@/lib/types/http";
import { useExecutorProfileAccess } from "./use-executor-profile-access";

const fixture = vi.hoisted(() => ({ store: null as StoreApi<AppState> | null }));
vi.mock("@/components/state-provider", async () => {
  const { useStore } = await import("zustand");
  return {
    useAppStore: (selector: (state: AppState) => unknown) => useStore(fixture.store!, selector),
  };
});
const alice = {
  id: "alice",
  email: "alice@example.com",
  display_name: "Alice",
  role: "member",
  status: "active",
};

beforeEach(() => {
  fixture.store = createAppStore();
  fixture.store.getState().setAuthState({ mode: "enabled", authenticated: true, user: alice });
  fixture.store.getState().setExecutors([
    {
      id: "parent",
      profiles: [{ id: "private", executor_id: "parent", owner_user_id: "alice" }],
    } as Executor,
  ]);
  fixture.store.getState().setSettingsData({ executorsLoaded: true });
});
afterEach(cleanup);

it("discards retained private editor state across identity changes, including a switch back", () => {
  const { result } = renderHook(() => useExecutorProfileAccess("private"));
  expect(result.current.result?.profile.id).toBe("private");
  act(() =>
    fixture.store!.setState((state) => ({
      executors: { items: [] },
      settingsData: { ...state.settingsData, executorsLoaded: false },
    })),
  );
  expect(result.current.checking).toBe(true);
  act(() =>
    fixture
      .store!.getState()
      .setAuthState({ mode: "enabled", authenticated: true, user: { ...alice, id: "bob" } }),
  );
  expect(result.current.result).toBeNull();
  act(() =>
    fixture.store!.getState().setAuthState({ mode: "enabled", authenticated: true, user: alice }),
  );
  expect(result.current.result).toBeNull();
});
