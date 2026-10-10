import type { ReactNode } from "react";
import { act, cleanup, render, renderHook } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { ProviderHealth } from "@/lib/state/slices/office/types";
import { registerOfficeHandlers } from "@/lib/ws/handlers/office";
import { useProviderHealth } from "./use-provider-health";

function health(overrides: Partial<ProviderHealth> = {}): ProviderHealth {
  return {
    workspace_id: "alpha",
    provider_id: "claude-acp",
    scope: "provider",
    scope_value: "",
    state: "healthy",
    backoff_step: 0,
    ...overrides,
  };
}

function deferred() {
  let resolve!: (response: Response) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<Response>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}

const requests: ReturnType<typeof deferred>[] = [];
const refreshes: Promise<void>[] = [];
const fetchSpy = vi.fn();

beforeEach(() => {
  fetchSpy.mockReset().mockImplementation(() => {
    const request = deferred();
    requests.push(request);
    return request.promise;
  });
  vi.stubGlobal("fetch", fetchSpy);
});

afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const request of requests.splice(0)) request.resolve(response([]));
    await Promise.all(refreshes.splice(0));
  });
  vi.unstubAllGlobals();
});

function response(rows?: ProviderHealth[]) {
  return new Response(JSON.stringify(rows ? { health: rows } : {}), {
    status: 200,
    headers: { "Content-Type": "application/json" },
  });
}

function mount(workspace: string | null = "alpha", strict = false) {
  function Wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider
        initialState={{ workspaces: { items: [], activeId: workspace, activeIdRevision: 0 } }}
      >
        {children}
      </StateProvider>
    );
  }
  return renderHook(
    () => {
      const selected = useAppStore((state) => state.workspaces.activeId);
      return { value: useProviderHealth(selected), store: useAppStoreApi() };
    },
    { wrapper: Wrapper, reactStrictMode: strict },
  );
}

type View = ReturnType<typeof mount>;

function emit(view: View, row: ProviderHealth) {
  const handler = registerOfficeHandlers(view.result.current.store)[
    "office.provider.health_changed"
  ]!;
  act(() => {
    handler({ type: "notification", action: "office.provider.health_changed", payload: row });
  });
}

function refresh(view: View) {
  let promise!: Promise<void>;
  act(() => {
    promise = view.result.current.value.refresh();
    refreshes.push(promise);
  });
  return promise;
}

async function succeed(index: number, rows?: ProviderHealth[]) {
  expect(requests[index]).toBeDefined();
  await act(async () => requests[index].resolve(response(rows)));
}

async function fail(index: number) {
  await act(async () => requests[index].reject(new Error("offline")));
}

async function startManual(rows: ProviderHealth[] = [health()]) {
  const view = mount();
  await succeed(0, rows);
  refresh(view);
  return view;
}

function select(view: View, workspace: string | null) {
  act(() => view.result.current.store.getState().setActiveWorkspace(workspace));
}

function expectRows(view: View, rows: ProviderHealth[], workspace = "alpha") {
  expect(view.result.current.store.getState().office.providerHealth.byWorkspace[workspace]).toEqual(
    rows,
  );
  expect(view.result.current.value.health).toEqual(rows);
}

describe("Office provider health snapshot publication", () => {
  // @covers AC-OFFICE-LIVE-UPDATES-002.9, AC-OFFICE-LIVE-UPDATES-002.10
  it.each(["initial", "manual"])("retains live health across delayed %s snapshot", async (mode) => {
    const view = mount();
    const healthy = health();
    const other = health({ provider_id: "opencode" });
    if (mode === "manual") {
      await succeed(0, [healthy, other]);
      refresh(view);
    }
    const live = health({
      state: "degraded",
      error_code: "quota_limited",
      retry_at: "2026-10-06T05:00:00Z",
      last_failure: "2026-10-06T04:00:00Z",
      last_success: "2026-10-06T03:00:00Z",
      raw_excerpt: "quota event",
      backoff_step: 2,
    });
    emit(view, live);
    expect(view.result.current.value.health).toContainEqual(live);
    expect(view.result.current.value.isLoading).toBe(true);
    await succeed(mode === "manual" ? 1 : 0, [healthy, other]);
    expectRows(view, [live, other]);
    expect(view.result.current.value).toMatchObject({ isLoading: false, error: null });
    expect(fetchSpy.mock.calls[0][0]).toContain("/api/v1/office/workspaces/alpha/routing/health");
  });

  it("hydrates ordinary snapshots and applies later events", async () => {
    const view = mount();
    await succeed(0, [health()]);
    expectRows(view, [health()]);
    emit(view, health({ state: "degraded" }));
    expectRows(view, [health({ state: "degraded" })]);
    refresh(view);
    await succeed(1, [health({ state: "short_retry", backoff_step: 1 })]);
    expectRows(view, [health({ state: "short_retry", backoff_step: 1 })]);
    view.rerender();
    expect(fetchSpy).toHaveBeenCalledTimes(2);
  });
});

describe("Office health full-key publication", () => {
  // @covers AC-OFFICE-LIVE-UPDATES-002.9, AC-OFFICE-LIVE-UPDATES-002.10
  it("reconciles provider, scope and scope value independently without key collisions", async () => {
    const rows = [
      health(),
      health({ scope: "model" }),
      health({ scope: "tier", scope_value: "balanced" }),
      health({ scope: "model", scope_value: "model:" }),
      health({ provider_id: "claude-acp:model", scope: "model" }),
    ];
    const view = await startManual(rows);
    const live = { ...rows[3], state: "degraded" as const, backoff_step: 2 };
    emit(view, live);
    const fresh = rows.map((row) => ({ ...row, raw_excerpt: "HTTP update" }));
    await succeed(1, fresh);
    expectRows(view, [fresh[0], fresh[1], fresh[2], live, fresh[4]]);
  });

  it("keeps the latest repeated event and permits healthy recovery during a read", async () => {
    const view = await startManual();
    const live = health({ state: "degraded", raw_excerpt: "same state, new observation" });
    emit(view, live);
    emit(view, { ...live });
    await succeed(1, [health()]);
    expectRows(view, [live]);
    refresh(view);
    const recovered = health({ raw_excerpt: "recovered", last_success: "2026-10-06T04:00:00Z" });
    emit(view, health({ state: "short_retry", backoff_step: 1 }));
    emit(view, recovered);
    await succeed(2, [live]);
    expectRows(view, [recovered]);
  });

  it("retains updated and inserted keys omitted by HTTP while removing untouched keys", async () => {
    const other = health({ provider_id: "opencode" });
    const untouched = health({ provider_id: "codex-acp" });
    const view = await startManual([health(), other, untouched]);
    const live = health({ state: "degraded" });
    const inserted = health({ provider_id: "new-provider", state: "short_retry" });
    emit(view, live);
    emit(view, inserted);
    const freshOther = { ...other, raw_excerpt: "HTTP other" };
    const newHttp = health({ provider_id: "http-only" });
    await succeed(1, [freshOther, newHttp]);
    expectRows(view, [freshOther, newHttp, live, inserted]);
  });

  it("preserves a live inserted key also present in HTTP", async () => {
    const view = mount();
    const inserted = health({ provider_id: "new-provider", state: "degraded" });
    emit(view, inserted);
    await succeed(0, [health({ provider_id: "new-provider" }), health()]);
    expectRows(view, [inserted, health()]);
  });

  it("retains a same-value live observation over a different HTTP result", async () => {
    const live = health({ state: "degraded", raw_excerpt: "unchanged event" });
    const view = await startManual([live]);
    emit(view, { ...live });
    await succeed(1, [health()]);
    expectRows(view, [live]);
  });

  it.each(["empty array", "missing health"])(
    "accepts %s with only intervening rows",
    async (kind) => {
      const view = await startManual([health(), health({ provider_id: "opencode" })]);
      const live = health({ state: "degraded" });
      emit(view, live);
      await succeed(1, kind === "empty array" ? [] : undefined);
      expectRows(view, [live]);
      expect(view.result.current.value.isLoading).toBe(false);
      refresh(view);
      await succeed(2, []);
      expectRows(view, []);
    },
  );
});

describe("Office health admission and failure", () => {
  // @covers AC-OFFICE-LIVE-UPDATES-002.5, AC-OFFICE-LIVE-UPDATES-002.6, AC-OFFICE-LIVE-UPDATES-002.9
  it("captures actual admission state before React rerenders", async () => {
    const view = mount();
    await succeed(0, [health()]);
    const handler = registerOfficeHandlers(view.result.current.store)[
      "office.provider.health_changed"
    ]!;
    act(() => {
      handler({
        type: "notification",
        action: "office.provider.health_changed",
        payload: health({ state: "degraded" }),
      });
      refreshes.push(view.result.current.value.refresh());
    });
    await succeed(1, [health({ raw_excerpt: "fresh after event" })]);
    expectRows(view, [health({ raw_excerpt: "fresh after event" })]);
  });

  it("keeps live rows on current failure and permits explicit recovery without retry loops", async () => {
    const view = await startManual();
    const live = health({ state: "degraded" });
    emit(view, live);
    await fail(1);
    expectRows(view, [live]);
    expect(view.result.current.value).toMatchObject({ isLoading: false, error: "offline" });
    view.rerender();
    expect(fetchSpy).toHaveBeenCalledTimes(2);
    refresh(view);
    await succeed(2, [health()]);
    expectRows(view, [health()]);
    expect(view.result.current.value).toMatchObject({ isLoading: false, error: null });
  });

  it.each(["success", "failure"])(
    "ignores older %s while newest success is pending",
    async (old) => {
      const view = await startManual();
      refresh(view);
      const live = health({ state: "degraded" });
      emit(view, live);
      if (old === "success") await succeed(1, [health()]);
      else await fail(1);
      expectRows(view, [live]);
      expect(view.result.current.value).toMatchObject({ isLoading: true, error: null });
      const other = health({ provider_id: "opencode" });
      await succeed(2, [health(), other]);
      expectRows(view, [live, other]);
      expect(view.result.current.value.isLoading).toBe(false);
    },
  );

  it.each(["success", "failure"])("ignores late older %s after newest failure", async (old) => {
    const view = await startManual();
    refresh(view);
    const live = health({ state: "degraded" });
    emit(view, live);
    await fail(2);
    if (old === "success") await succeed(1, [health()]);
    else await fail(1);
    expectRows(view, [live]);
    expect(view.result.current.value).toMatchObject({ isLoading: false, error: "offline" });
  });
});

describe("Office health real-store publication", () => {
  // @covers AC-OFFICE-LIVE-UPDATES-002.7, AC-OFFICE-LIVE-UPDATES-002.9, AC-OFFICE-LIVE-UPDATES-002.10
  it("isolates equal keys in separate application stores", async () => {
    const first = mount();
    const second = mount();
    const live = health({ state: "degraded" });
    emit(first, live);
    await succeed(0, [health()]);
    await succeed(1, [health({ raw_excerpt: "second store HTTP" })]);
    expectRows(first, [live]);
    expectRows(second, [health({ raw_excerpt: "second store HTTP" })]);
    expect(first.result.current.store).not.toBe(second.result.current.store);
  });

  it("does not protect untouched alpha rows when beta in the same store changes", async () => {
    const view = await startManual();
    const beta = health({ workspace_id: "beta", state: "degraded" });
    act(() => view.result.current.store.getState().setProviderHealth("beta", [beta]));
    const fresh = health({ raw_excerpt: "fresh alpha" });
    await succeed(1, [fresh]);
    expectRows(view, [fresh]);
    expect(view.result.current.store.getState().office.providerHealth.byWorkspace.beta).toEqual([
      beta,
    ]);
  });

  it("isolates workspaces and rejects an event from another workspace", async () => {
    const first = mount();
    const second = mount("beta");
    const live = health({ state: "degraded" });
    emit(first, live);
    emit(second, live);
    await succeed(0, [health()]);
    const beta = health({ workspace_id: "beta" });
    await succeed(1, [beta]);
    expectRows(first, [live]);
    expectRows(second, [beta], "beta");
    expect(
      second.result.current.store.getState().office.providerHealth.byWorkspace.alpha,
    ).toBeUndefined();
  });

  it("publishes one complete reconciled entry and keeps direct setters immediate", async () => {
    const view = mount();
    const live = health({ state: "degraded" });
    emit(view, live);
    const observed: ProviderHealth[][] = [];
    const unsubscribe = view.result.current.store.subscribe((state, previous) => {
      const rows = state.office.providerHealth.byWorkspace.alpha;
      if (rows !== previous.office.providerHealth.byWorkspace.alpha) observed.push(rows);
    });
    try {
      const other = health({ provider_id: "opencode" });
      await succeed(0, [health(), other]);
      expect(observed).toEqual([[live, other]]);
      act(() => view.result.current.store.getState().setProviderHealth("alpha", []));
      expectRows(view, []);
      expect(observed).toEqual([[live, other], []]);
    } finally {
      unsubscribe();
    }
  });
});

describe("Office health event lifecycle fences", () => {
  // @covers AC-OFFICE-LIVE-UPDATES-002.2, AC-OFFICE-LIVE-UPDATES-002.4, AC-OFFICE-LIVE-UPDATES-002.9
  it.each(["success", "failure"])("fences departed %s across A-to-B-to-A", async (outcome) => {
    const view = mount();
    emit(view, health({ state: "degraded" }));
    const departedRefresh = view.result.current.value.refresh;
    select(view, "beta");
    const beta = health({ workspace_id: "beta", state: "short_retry" });
    emit(view, beta);
    await succeed(1, [health({ workspace_id: "beta" })]);
    select(view, "alpha");
    const live = health({ state: "user_action_required", error_code: "quota_limited" });
    emit(view, live);
    await act(async () => departedRefresh());
    expect(requests).toHaveLength(3);
    if (outcome === "success") await succeed(0, [health()]);
    else await fail(0);
    expectRows(view, [live]);
    expect(view.result.current.value).toMatchObject({ isLoading: true, error: null });
    await succeed(2, [health()]);
    expectRows(view, [live]);
    expect(view.result.current.store.getState().office.providerHealth.byWorkspace.beta).toEqual([
      beta,
    ]);
  });

  it.each(["success", "failure"])(
    "invalidates pending %s on empty selection and unmount",
    async (outcome) => {
      const view = mount();
      const live = health({ state: "degraded" });
      emit(view, live);
      select(view, null);
      expect(view.result.current.value).toMatchObject({
        health: [],
        isLoading: false,
        error: null,
      });
      view.unmount();
      const store = view.result.current.store;
      const before = store.getState();
      if (outcome === "success") await succeed(0, [health()]);
      else await fail(0);
      expect(store.getState()).toBe(before);
      expect(store.getState().office.providerHealth.byWorkspace.alpha).toEqual([live]);
    },
  );

  it("retains setup ownership through real StrictMode replay", async () => {
    const view = mount("alpha", true);
    expect(requests).toHaveLength(2);
    const live = health({ state: "degraded" });
    emit(view, live);
    await succeed(0, [health()]);
    expectRows(view, [live]);
    expect(view.result.current.value.isLoading).toBe(true);
    await succeed(1, [health()]);
    expectRows(view, [live]);
    expect(view.result.current.value.isLoading).toBe(false);
  });
});

describe("Office health sibling consumers", () => {
  // @covers AC-OFFICE-LIVE-UPDATES-002.7, AC-OFFICE-LIVE-UPDATES-002.9
  it.each([false, true])(
    "keeps sibling requests independent with initiator unmount=%s",
    async (unmount) => {
      const values = new Map<string, ReturnType<typeof useProviderHealth>>();
      let store!: ReturnType<typeof useAppStoreApi>;
      function Probe({ id }: { id: string }) {
        store = useAppStoreApi();
        values.set(id, useProviderHealth(useAppStore((state) => state.workspaces.activeId)));
        return null;
      }
      function Pair({ firstMounted }: { firstMounted: boolean }) {
        return (
          <StateProvider
            initialState={{ workspaces: { items: [], activeId: "alpha", activeIdRevision: 0 } }}
          >
            {firstMounted && <Probe id="first" />}
            <Probe id="second" />
          </StateProvider>
        );
      }
      const view = render(<Pair firstMounted />);
      expect(requests).toHaveLength(2);
      const live = health({ state: "degraded" });
      const handler = registerOfficeHandlers(store)["office.provider.health_changed"]!;
      act(() =>
        handler({ type: "notification", action: "office.provider.health_changed", payload: live }),
      );
      const liveReference = store.getState().office.providerHealth.byWorkspace.alpha[0];
      if (unmount) view.rerender(<Pair firstMounted={false} />);
      const other = health({ provider_id: "opencode" });
      await succeed(0, [health(), other]);
      expect(values.get("second")?.isLoading).toBe(true);
      expect(store.getState().office.providerHealth.byWorkspace.alpha[0]).toBe(liveReference);
      await succeed(1, [health(), other]);
      expect(store.getState().office.providerHealth.byWorkspace.alpha).toEqual([live, other]);
      expect(store.getState().office.providerHealth.byWorkspace.alpha[0]).toBe(liveReference);
      expect(values.get("second")).toMatchObject({
        health: [live, other],
        isLoading: false,
        error: null,
      });
    },
  );
});
