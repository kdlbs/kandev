import { act, cleanup, render, renderHook } from "@testing-library/react";
import { useLayoutEffect, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useCoordinators } from "./use-coordinators";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { Coordinator, CoordinatorListResponse } from "@/lib/api/domains/coordinator-api";

const transport = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/client", async (original) => ({
  ...(await original<typeof import("@/lib/api/client")>()),
  fetchJson: transport,
}));

type Read = {
  workspace: string;
  resolve: (result: CoordinatorListResponse) => void;
  reject: (error: Error) => void;
  settled: boolean;
};
const reads: Read[] = [];
let store: StoreApi<AppState>;
function CaptureStore() {
  store = useAppStoreApi();
  return null;
}
function Providers({ children }: { children: ReactNode }) {
  return (
    <StateProvider>
      <CaptureStore />
      {children}
    </StateProvider>
  );
}
function coordinator(workspace: string, id: string): Coordinator {
  return {
    id,
    workspace_id: workspace,
    name: id,
    agent_profile_id: "agent",
    executor_profile_id: "executor",
    context: "",
    conversation_task_id: null,
    created_at: "2026-10-06T00:00:00Z",
    updated_at: "2026-10-06T00:00:00Z",
  };
}
async function settle(index: number, outcome: string | Error | null) {
  const read = reads[index];
  await act(async () => {
    read.settled = true;
    if (outcome instanceof Error) read.reject(outcome);
    else read.resolve({ coordinators: outcome ? [coordinator(read.workspace, outcome)] : [] });
  });
}
function ids() {
  return store.getState().coordinators.items.map((item) => item.id);
}

beforeEach(() => {
  reads.length = 0;
  transport.mockReset();
  transport.mockImplementation((url: string) => {
    const match = url.match(/^\/api\/v1\/workspaces\/([^/]+)\/coordinators$/);
    if (!match) throw new Error(`Unexpected list transport: ${url}`);
    return new Promise<CoordinatorListResponse>((resolve, reject) => {
      reads.push({ workspace: match[1], resolve, reject, settled: false });
    });
  });
});
afterEach(async () => {
  cleanup();
  for (let index = 0; index < reads.length; index++) {
    if (!reads[index].settled) await settle(index, null);
  }
});

// @covers AC-COORDINATOR-COORDINATORS-004.8
describe("Coordinator list publication hook", () => {
  it.each(["success", "failure"])(
    "retires B on a cached return to A before B %s",
    async (outcome) => {
      const view = renderHook(({ workspace }) => useCoordinators(workspace), {
        wrapper: Providers,
        initialProps: { workspace: "A" },
      });
      await settle(0, "A-cached");
      view.rerender({ workspace: "B" });
      view.rerender({ workspace: "A" });
      expect(reads.map((read) => read.workspace)).toEqual(["A", "B"]);
      expect(view.result.current.loading).toBe(false);
      await settle(1, outcome === "success" ? "B-obsolete" : new Error("B failed"));
      expect(ids()).toEqual(["A-cached"]);
      expect(view.result.current).toMatchObject({ loaded: true, loading: false, loadError: false });
    },
  );

  it.each(["success", "failure"])(
    "keeps the newer refresh pending after older %s/finally",
    async (outcome) => {
      const view = renderHook(() => useCoordinators("A"), { wrapper: Providers });
      await settle(0, "A-cached");
      act(() => view.result.current.refresh());
      act(() => view.result.current.refresh());
      await settle(1, outcome === "success" ? "A-old" : new Error("old failure"));
      expect(ids()).toEqual(["A-cached"]);
      expect(view.result.current).toMatchObject({ loaded: true, loading: true, loadError: false });
      await settle(2, "A-new");
      expect(ids()).toEqual(["A-new"]);
      expect(view.result.current.loading).toBe(false);
    },
  );

  it.each(["success", "failure"])(
    "keeps a newer settled refresh after older %s",
    async (outcome) => {
      const view = renderHook(() => useCoordinators("A"), { wrapper: Providers });
      await settle(0, "A-cached");
      act(() => view.result.current.refresh());
      act(() => view.result.current.refresh());
      await settle(2, "A-new");
      await settle(1, outcome === "success" ? "A-old" : new Error("old failure"));
      expect(ids()).toEqual(["A-new"]);
      expect(view.result.current).toMatchObject({ loaded: true, loading: false, loadError: false });
    },
  );
});

describe("Coordinator list publication hook refresh", () => {
  it("does not erase a newer refresh error with an older success", async () => {
    const view = renderHook(() => useCoordinators("A"), { wrapper: Providers });
    await settle(0, "A-cached");
    act(() => view.result.current.refresh());
    act(() => view.result.current.refresh());
    await settle(2, new Error("current failure"));
    await settle(1, "A-old");
    expect(ids()).toEqual(["A-cached"]);
    expect(view.result.current).toMatchObject({ loaded: true, loading: false, loadError: true });
    act(() => view.result.current.refresh());
    await settle(3, "A-recovered");
    expect(ids()).toEqual(["A-recovered"]);
    expect(view.result.current).toMatchObject({ loaded: true, loading: false, loadError: false });
  });

  it("supersedes a pending initial read with a refresh", async () => {
    const view = renderHook(() => useCoordinators("A"), { wrapper: Providers });
    act(() => view.result.current.refresh());
    await settle(0, "A-old");
    expect(ids()).toEqual([]);
    expect(view.result.current).toMatchObject({ loaded: false, loading: true });
    await settle(1, "A-current");
    expect(ids()).toEqual(["A-current"]);
  });

  it("accepts B-current and ignores the distinct A-pending read (positive control)", async () => {
    const view = renderHook(({ workspace }) => useCoordinators(workspace), {
      wrapper: Providers,
      initialProps: { workspace: "A" },
    });
    view.rerender({ workspace: "B" });
    await settle(1, "B-current");
    await settle(0, "A-old");
    expect(ids()).toEqual(["B-current"]);
    expect(view.result.current).toMatchObject({ loaded: true, loading: false, loadError: false });
  });

  it.each(["B", null])("retires pending A across committed A -> %s -> A", async (middle) => {
    const view = renderHook(({ workspace }) => useCoordinators(workspace), {
      wrapper: Providers,
      initialProps: { workspace: "A" as string | null },
    });
    view.rerender({ workspace: middle });
    view.rerender({ workspace: "A" });
    const current = reads.length - 1;
    await settle(current, "A-current");
    await settle(0, "A-old");
    if (middle) await settle(1, new Error("B obsolete"));
    expect(ids()).toEqual(["A-current"]);
    expect(view.result.current).toMatchObject({ loaded: true, loading: false, loadError: false });
  });
});

describe("Coordinator list publication hook lifetime", () => {
  it("does not fetch for null or refetch an accepted same-workspace rerender", async () => {
    const view = renderHook(({ workspace }) => useCoordinators(workspace), {
      wrapper: Providers,
      initialProps: { workspace: null as string | null },
    });
    expect(reads).toHaveLength(0);
    view.rerender({ workspace: "A" });
    await settle(0, null);
    view.rerender({ workspace: "A" });
    expect(reads).toHaveLength(1);
    expect(view.result.current).toMatchObject({
      items: [],
      loaded: true,
      loading: false,
      loadError: false,
    });
  });

  it("leaves both real store owners unchanged when a retired read finishes", async () => {
    const first = renderHook(() => useCoordinators("A"), { wrapper: Providers });
    const oldStore = store;
    const oldRefresh = first.result.current.refresh;
    first.unmount();
    const oldSnapshot = oldStore.getState().coordinators;
    const second = renderHook(() => useCoordinators("A"), { wrapper: Providers });
    expect(store).not.toBe(oldStore);
    act(oldRefresh);
    expect(reads).toHaveLength(2);
    await settle(1, "new-owner");
    await settle(0, "retired-owner");
    expect(oldStore.getState().coordinators).toEqual(oldSnapshot);
    expect(ids()).toEqual(["new-owner"]);
    expect(second.result.current.loading).toBe(false);
  });

  it("admits the current StrictMode replacement after retiring the first read", async () => {
    const view = renderHook(() => useCoordinators("A"), {
      wrapper: Providers,
      reactStrictMode: true,
    });
    expect(reads).toHaveLength(2);
    await settle(0, "retired-strict-read");
    expect(ids()).toEqual([]);
    expect(view.result.current.loading).toBe(true);
    await settle(1, "current-strict-read");
    expect(ids()).toEqual(["current-strict-read"]);
    expect(view.result.current).toMatchObject({ loaded: true, loading: false, loadError: false });
  });
});

type List = ReturnType<typeof useCoordinators>;
function CommitProbe({
  workspace,
  commit,
  retire,
}: {
  workspace: string | null;
  commit: (list: List) => void;
  retire?: () => void;
}) {
  const list = useCoordinators(workspace);
  useLayoutEffect(() => {
    commit(list);
    return retire;
  }, [workspace, commit, retire]); // Intentional commit-boundary observation, not passive timing.
  return null;
}

describe("Coordinator list publication committed callback", () => {
  it.each(["B", null, "return-A"])(
    "refuses the retained A callback at the %s commit",
    async (destination) => {
      let refresh: (() => void) | undefined;
      const capture = (list: List) => {
        refresh = list.refresh;
      };
      const content = (workspace: string | null, commit: (list: List) => void) => (
        <Providers>
          <CommitProbe workspace={workspace} commit={commit} />
        </Providers>
      );
      const view = render(content("A", capture));
      await settle(0, "A-cached");
      const retiredRefresh = refresh!;
      if (destination === "return-A") view.rerender(content("B", () => {}));
      const workspace = destination === "return-A" ? "A" : destination;
      view.rerender(content(workspace, () => retiredRefresh()));
      expect(reads.filter((read) => read.workspace === "A")).toHaveLength(1);
    },
  );

  it("refuses retained refresh during layout unmount cleanup and after unmount", () => {
    let refresh: (() => void) | undefined;
    const view = render(
      <Providers>
        <CommitProbe
          workspace="A"
          commit={(list) => {
            refresh = list.refresh;
          }}
          retire={() => refresh!()}
        />
      </Providers>,
    );
    view.unmount();
    act(() => refresh!());
    expect(reads).toHaveLength(1);
    expect(store.getState().coordinators.loading).toBe(false);
  });
});
