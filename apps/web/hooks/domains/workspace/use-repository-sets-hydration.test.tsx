import {
  act,
  cleanup,
  fireEvent,
  render,
  renderHook,
  screen,
  waitFor,
} from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { DropdownMenu, DropdownMenuContent, DropdownMenuTrigger } from "@kandev/ui/dropdown-menu";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { RepositorySetMenuItems } from "@/components/task-create-dialog-repository-sets-control";
import { registerRepositorySetsHandlers } from "@/lib/ws/handlers/repository-sets";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { RepositorySet } from "@/lib/types/http";
import { workspaceId } from "@/lib/types/ids";
import { useRepositorySets } from "./use-repository-sets";

const WS = "ws-hydration";
const DELETED = "set-deleted";
const SURVIVOR = "set-survivor";

function repositorySet(id: string, ws = WS): RepositorySet {
  return {
    id,
    workspace_id: workspaceId(ws),
    name: id,
    description: "",
    repositories: [],
    created_at: "",
    updated_at: "",
  };
}

type HeldRead = { url: string; resolve: (response: Response) => void };
let reads: HeldRead[];
let pending: Set<HeldRead>;

beforeEach(() => {
  reads = [];
  pending = new Set();
  vi.stubGlobal(
    "fetch",
    vi.fn(
      (url: string, options?: RequestInit) =>
        new Promise<Response>((resolve, reject) => {
          const read = {
            url,
            resolve: (value: Response) => {
              options?.signal?.removeEventListener("abort", abort);
              resolve(value);
            },
          };
          const abort = () => {
            pending.delete(read);
            reject(options?.signal?.reason);
          };
          reads.push(read);
          pending.add(read);
          if (options?.signal?.aborted) abort();
          else options?.signal?.addEventListener("abort", abort, { once: true });
        }),
    ),
  );
});

function response(sets: RepositorySet[], status = 200) {
  return new Response(JSON.stringify({ repository_sets: sets, total: sets.length }), { status });
}

async function release(index: number, sets: RepositorySet[], status = 200) {
  await act(async () => {
    const read = reads[index];
    pending.delete(read);
    read.resolve(response(sets, status));
  });
}

afterEach(async () => {
  cleanup();
  // Settle all held transports even after an assertion fails during RED.
  vi.stubGlobal(
    "fetch",
    vi.fn(async () => response([])),
  );
  await act(async () => {
    for (const read of pending) read.resolve(response([]));
    pending.clear();
  });
  vi.useRealTimers();
  vi.unstubAllGlobals();
});

function Wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

async function mount(ws = WS) {
  const count = reads.length;
  const view = renderHook(
    ({ id, enabled }) => ({
      ...useRepositorySets(id, enabled),
      store: useAppStoreApi(),
    }),
    { initialProps: { id: ws, enabled: true }, wrapper: Wrapper },
  );
  await waitFor(() => expect(reads).toHaveLength(count + 1));
  return view;
}

function deleted(store: StoreApi<AppState>, ws = WS, id = DELETED) {
  registerRepositorySetsHandlers(store)["repository_set.deleted"]!({
    id: "event-delete",
    type: "notification",
    action: "repository_set.deleted",
    timestamp: "2026-10-21T00:00:00Z",
    payload: { id, workspace_id: ws },
  });
}

// @covers AC-WORKSPACES-REPOSITORY-SETS-001.6
// @covers AC-WORKSPACES-REPOSITORY-SETS-001.7
describe("initial repository-set hydration and live deletion", () => {
  it.each(["event", "action"] as const)(
    "fences missing-cache deletion via %s and recovers every survivor",
    async (entry) => {
      const view = await mount();
      await waitFor(() => expect(reads).toHaveLength(1));
      expect(
        view.result.current.store.getState().repositorySets.itemsByWorkspaceId[WS],
      ).toBeUndefined();
      act(() => {
        if (entry === "event") deleted(view.result.current.store);
        else view.result.current.store.getState().removeRepositorySet(WS, DELETED);
      });
      expect(
        view.result.current.store.getState().repositorySets.loadedByWorkspaceId[WS],
      ).toBeFalsy();

      await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
      expect(view.result.current.sets.map((set) => set.id)).not.toContain(DELETED);
      await waitFor(() => expect(reads).toHaveLength(2));
      expect(view.result.current.isLoading).toBe(true);
      await release(1, [repositorySet(SURVIVOR), repositorySet("set-another-survivor")]);
      expect(view.result.current.sets.map((set) => set.id)).toEqual([
        "set-another-survivor",
        SURVIVOR,
      ]);
      expect(view.result.current.isLoading).toBe(false);
      expect(view.result.current.store.getState().repositorySets.loadedByWorkspaceId[WS]).toBe(
        true,
      );
    },
  );

  it("recovers an existing empty but unloaded bucket", async () => {
    const view = await mount();
    await waitFor(() => expect(reads).toHaveLength(1));
    act(() =>
      view.result.current.store.setState((state) => ({
        repositorySets: {
          ...state.repositorySets,
          itemsByWorkspaceId: { [WS]: [] },
        },
      })),
    );
    expect(view.result.current.store.getState().repositorySets.itemsByWorkspaceId[WS]).toEqual([]);
    act(() => deleted(view.result.current.store));
    await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
    expect(view.result.current.sets).toEqual([]);
    await waitFor(() => expect(reads).toHaveLength(2));
    await release(1, [repositorySet(SURVIVOR)]);
    expect(view.result.current.sets.map((set) => set.id)).toEqual([SURVIVOR]);
  });

  it("preserves the complete loaded event-updated cache without recovery", async () => {
    const view = await mount();
    await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
    let refresh!: Promise<void>;
    act(() => {
      refresh = view.result.current.refresh();
    });
    await waitFor(() => expect(reads).toHaveLength(2));
    act(() => deleted(view.result.current.store));
    await release(1, [repositorySet(DELETED)]);
    await refresh;
    expect(view.result.current.sets.map((set) => set.id)).toEqual([SURVIVOR]);
    expect(view.result.current.isLoading).toBe(false);
    expect(reads).toHaveLength(2);
  });

  it("isolates another workspace's deletion", async () => {
    const view = await mount();
    act(() => deleted(view.result.current.store, "ws-other"));
    await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
    expect(view.result.current.sets.map((set) => set.id)).toEqual([DELETED, SURVIVOR]);
    expect(reads).toHaveLength(1);
  });
});

describe("repository-set recovery failures", () => {
  it("settles a failed recovery unloaded and allows a manual retry", async () => {
    const view = await mount();
    act(() => deleted(view.result.current.store));
    await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
    await waitFor(() => expect(reads).toHaveLength(2));
    await release(1, [], 503);
    expect(view.result.current.sets).toEqual([]);
    expect(view.result.current.isLoading).toBe(false);
    expect(view.result.current.store.getState().repositorySets.loadedByWorkspaceId[WS]).toBeFalsy();
    expect(reads).toHaveLength(2);
    let refresh!: Promise<void>;
    act(() => {
      refresh = view.result.current.refresh();
    });
    await waitFor(() => expect(reads).toHaveLength(3));
    await release(2, [repositorySet(SURVIVOR)]);
    await refresh;
    expect(view.result.current.sets.map((set) => set.id)).toEqual([SURVIVOR]);
  });

  it("settles an initial failure and retries when re-enabled", async () => {
    const view = await mount();
    await release(0, [], 503);
    expect(view.result.current.isLoading).toBe(false);
    const store = view.result.current.store;
    view.rerender({ id: WS, enabled: false });
    view.rerender({ id: WS, enabled: true });
    await waitFor(() => expect(reads).toHaveLength(2));
    await release(1, [repositorySet(SURVIVOR)]);
    expect(store.getState().repositorySets.loadedByWorkspaceId[WS]).toBe(true);
  });

  it("fences another deletion during recovery and keeps one read in flight", async () => {
    const view = await mount();
    act(() => {
      deleted(view.result.current.store);
      deleted(view.result.current.store, WS, "set-second-deleted");
    });
    expect(reads).toHaveLength(1);
    await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
    await waitFor(() => expect(reads).toHaveLength(2));
    act(() => deleted(view.result.current.store, WS, "set-third-deleted"));
    await release(1, [repositorySet("set-third-deleted"), repositorySet(SURVIVOR)]);
    expect(view.result.current.sets).toEqual([]);
    await waitFor(() => expect(reads).toHaveLength(3));
    await release(2, [repositorySet(SURVIVOR)]);
    expect(view.result.current.sets.map((set) => set.id)).toEqual([SURVIVOR]);
    expect(pending.size).toBe(0);
  });
});

describe("repository-set consumer lifetime", () => {
  it("completes store-owned recovery after the initiating consumer unmounts", async () => {
    const view = await mount();
    const store = view.result.current.store;
    act(() => deleted(store));
    view.unmount();
    await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
    await waitFor(() => expect(reads).toHaveLength(2));
    await release(1, [repositorySet(SURVIVOR)]);
    expect(store.getState().repositorySets.itemsByWorkspaceId[WS].map((set) => set.id)).toEqual([
      SURVIVOR,
    ]);
    expect(store.getState().repositorySets.loadingByWorkspaceId[WS]).toBe(false);
  });

  it("keeps a surviving consumer current when the initiator unmounts", async () => {
    let current!: ReturnType<typeof useRepositorySets>;
    let store!: StoreApi<AppState>;
    function Initiator() {
      useRepositorySets(WS);
      return null;
    }
    function Observer() {
      current = useRepositorySets(WS);
      store = useAppStoreApi();
      return null;
    }
    function Consumers({ initiating }: { initiating: boolean }) {
      return (
        <StateProvider>
          {initiating && <Initiator />}
          <Observer />
        </StateProvider>
      );
    }
    const view = render(<Consumers initiating />);
    await waitFor(() => expect(reads).toHaveLength(1));
    act(() => deleted(store));
    view.rerender(<Consumers initiating={false} />);
    await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
    await waitFor(() => expect(reads).toHaveLength(2));
    expect(current.isLoading).toBe(true);
    await release(1, [repositorySet(SURVIVOR)]);
    expect(current.sets.map((set) => set.id)).toEqual([SURVIVOR]);
    expect(current.isLoading).toBe(false);
    expect(reads).toHaveLength(2);
  });

  it("preserves valid cached data when a second consumer becomes enabled", async () => {
    const view = renderHook(
      ({ second }) => ({
        first: useRepositorySets(WS),
        second: useRepositorySets(WS, second),
      }),
      { initialProps: { second: false }, wrapper: Wrapper },
    );
    await waitFor(() => expect(reads).toHaveLength(1));
    await release(0, [repositorySet(SURVIVOR)]);
    view.rerender({ second: true });
    expect(view.result.current.second.sets.map((set) => set.id)).toEqual([SURVIVOR]);
    expect(view.result.current.second.isLoading).toBe(false);
    expect(reads).toHaveLength(1);
  });
});

describe("repository-set workspace and provider isolation", () => {
  it("keeps old workspace completion separate after a key switch and disable", async () => {
    const view = await mount();
    const store = view.result.current.store;
    act(() => deleted(store));
    view.rerender({ id: "ws-other", enabled: true });
    await waitFor(() => expect(reads).toHaveLength(2));
    expect(reads[1].url).toContain("ws-other");
    view.rerender({ id: "ws-other", enabled: false });
    await release(0, [repositorySet(DELETED)]);
    await waitFor(() => expect(reads).toHaveLength(3));
    expect(reads[2].url).toContain(WS);
    await release(1, [repositorySet("set-other", "ws-other")]);
    await release(2, [repositorySet(SURVIVOR)]);
    expect(view.result.current.sets.map((set) => set.id)).toEqual(["set-other"]);
    expect(store.getState().repositorySets.itemsByWorkspaceId[WS].map((set) => set.id)).toEqual([
      SURVIVOR,
    ]);
    expect(store.getState().repositorySets.loadingByWorkspaceId).toMatchObject({
      [WS]: false,
      "ws-other": false,
    });
  });

  it("isolates independent providers for the same workspace", async () => {
    const first = await mount();
    const second = await mount();
    expect(reads).toHaveLength(2);
    act(() => deleted(first.result.current.store));
    await release(1, [repositorySet(DELETED)]);
    await release(0, [repositorySet(DELETED)]);
    await waitFor(() => expect(reads).toHaveLength(3));
    await release(2, [repositorySet(SURVIVOR)]);
    expect(first.result.current.sets.map((set) => set.id)).toEqual([SURVIVOR]);
    expect(second.result.current.sets.map((set) => set.id)).toEqual([DELETED]);
  });
});

describe("repository-set menu consumer", () => {
  it("shares recovery with another consumer and updates actual set menu options", async () => {
    const onApply = vi.fn();
    function Menu() {
      const { sets } = useRepositorySets(WS);
      return (
        <DropdownMenu defaultOpen>
          <DropdownMenuTrigger>Sets</DropdownMenuTrigger>
          <DropdownMenuContent>
            <RepositorySetMenuItems sets={sets} repositories={[]} rows={[]} onApply={onApply} />
          </DropdownMenuContent>
        </DropdownMenu>
      );
    }
    function Shared({ children }: { children: ReactNode }) {
      return (
        <StateProvider>
          {children}
          <Menu />
        </StateProvider>
      );
    }
    const view = renderHook(() => ({ ...useRepositorySets(WS), store: useAppStoreApi() }), {
      wrapper: Shared,
    });
    await waitFor(() => expect(reads).toHaveLength(1));
    let refresh!: Promise<void>;
    act(() => {
      refresh = view.result.current.refresh();
      deleted(view.result.current.store);
    });
    expect(reads).toHaveLength(1);
    await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
    await waitFor(() => expect(reads).toHaveLength(2));
    expect(screen.queryByText(DELETED)).toBeNull();
    await release(1, [repositorySet(SURVIVOR)]);
    await refresh;
    expect(screen.getByText(SURVIVOR)).toBeDefined();
    expect(screen.queryByText(DELETED)).toBeNull();
    fireEvent.click(screen.getByText(SURVIVOR));
    expect(onApply).toHaveBeenCalledWith(repositorySet(SURVIVOR));
    expect(view.result.current.isLoading).toBe(false);
    expect(reads).toHaveLength(2);
  });
});

describe("repository-set shared read deadlines", () => {
  it("settles a hung refresh, preserves accepted data, and permits a fresh refresh", async () => {
    const view = await mount();
    await release(0, [repositorySet(SURVIVOR)]);
    vi.useFakeTimers();
    let refresh!: Promise<void>;
    act(() => {
      refresh = view.result.current.refresh();
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(reads).toHaveLength(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(29_999);
    });
    expect(view.result.current.isLoading).toBe(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(view.result.current.isLoading).toBe(false);
    await refresh;
    expect(view.result.current.sets.map((set) => set.id)).toEqual([SURVIVOR]);
    act(() => {
      refresh = view.result.current.refresh();
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(reads).toHaveLength(3);
    await release(2, [repositorySet(SURVIVOR), repositorySet("set-new")]);
    await refresh;
    expect(view.result.current.sets.map((set) => set.id)).toEqual(["set-new", SURVIVOR]);
    expect(view.result.current.isLoading).toBe(false);
    expect(vi.getTimerCount()).toBe(0);
  });

  it("bounds the whole unloaded recovery operation and can retry without resurrection", async () => {
    vi.useFakeTimers();
    const view = renderHook(() => ({ ...useRepositorySets(WS), store: useAppStoreApi() }), {
      wrapper: Wrapper,
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(reads).toHaveLength(1);
    act(() => deleted(view.result.current.store));
    await act(async () => {
      await vi.advanceTimersByTimeAsync(15_000);
    });
    await release(0, [repositorySet(DELETED), repositorySet(SURVIVOR)]);
    expect(reads).toHaveLength(2);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(14_999);
    });
    expect(view.result.current.isLoading).toBe(true);
    await act(async () => {
      await vi.advanceTimersByTimeAsync(1);
    });
    expect(view.result.current.isLoading).toBe(false);
    expect(view.result.current.sets).toEqual([]);
    expect(view.result.current.store.getState().repositorySets.loadedByWorkspaceId[WS]).toBeFalsy();
    let refresh!: Promise<void>;
    act(() => {
      refresh = view.result.current.refresh();
    });
    await act(async () => {
      await Promise.resolve();
    });
    expect(reads).toHaveLength(3);
    await release(2, [repositorySet(SURVIVOR)]);
    await refresh;
    expect(view.result.current.sets.map((set) => set.id)).toEqual([SURVIVOR]);
    expect(view.result.current.isLoading).toBe(false);
    expect(vi.getTimerCount()).toBe(0);
  });
});
