import { StrictMode, type ReactNode } from "react";
import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { useSettingsBreadcrumbs } from "@/components/settings/use-settings-breadcrumbs";
import type { Automation, CreateAutomationRequest } from "@/lib/types/automation";
import { useAutomations } from "./use-automations";

const transport = vi.hoisted(() => ({
  list: vi.fn(),
  create: vi.fn(),
  update: vi.fn(),
  remove: vi.fn(),
  enable: vi.fn(),
  disable: vi.fn(),
  trigger: vi.fn(),
}));
vi.mock("@/lib/api/domains/automation-api", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/domains/automation-api")>()),
  listAutomations: transport.list,
  createAutomation: transport.create,
  updateAutomation: transport.update,
  deleteAutomation: transport.remove,
  enableAutomation: transport.enable,
  disableAutomation: transport.disable,
  triggerAutomation: transport.trigger,
}));

function wrapper({ children }: { children: ReactNode }) {
  return <StateProvider>{children}</StateProvider>;
}

function record(workspace: string, id: string): Automation {
  return {
    id,
    workspace_id: workspace,
    name: id,
    description: "",
    workflow_id: "workflow",
    workflow_step_id: "step",
    agent_profile_id: "agent",
    executor_profile_id: "executor",
    repository_ids: [],
    prompt: "prompt",
    task_title_template: "title",
    enabled: true,
    max_concurrent_runs: 1,
    last_triggered_at: null,
    created_at: "2026-10-05T00:00:00Z",
    updated_at: "2026-10-05T00:00:00Z",
    triggers: [],
  };
}

function createDeferred() {
  let resolve!: (rows: Automation[]) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<Automation[]>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  return { promise, resolve, reject };
}
const pending: ReturnType<typeof createDeferred>[] = [];
const OLDEST_FIRST = "oldest-first";
const SETTLEMENT_ORDERS = ["newest-first", OLDEST_FIRST] as const;
const OBSOLETE_A = "obsolete-a";
function deferred() {
  const read = createDeferred();
  pending.push(read);
  return read;
}
async function settle(read: ReturnType<typeof deferred>, rows: Automation[]) {
  await act(async () => {
    read.resolve(rows);
    await read.promise;
  });
}
async function fail(read: ReturnType<typeof deferred>) {
  await act(async () => {
    read.reject(new Error("list unavailable"));
  });
}
function useSnapshot(workspace: string | null) {
  return { view: useAutomations(workspace), store: useAppStoreApi() };
}
async function loaded() {
  const initial = record("a", "initial-a");
  transport.list.mockResolvedValueOnce([initial]);
  const rendered = renderHook(() => useSnapshot("a"), { wrapper });
  await waitFor(() => expect(rendered.result.current.view.items).toEqual([initial]));
  return { ...rendered, initial };
}
beforeEach(() => Object.values(transport).forEach((mock) => mock.mockReset()));
afterEach(async () => {
  await act(async () => {
    pending.splice(0).forEach((read) => read.resolve([]));
  });
  cleanup();
});

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.12
describe("workspace-scoped settings lists", () => {
  it("publishes a current initial load and settles its flags", async () => {
    const { result, initial } = await loaded();
    expect(result.current.view).toMatchObject({ items: [initial], loaded: true, loading: false });
    expect(transport.list).toHaveBeenCalledExactlyOnceWith("a");
  });

  it("restores cached A immediately and keeps late B out of A", async () => {
    const a = record("a", "cached-a");
    const b = deferred();
    transport.list.mockResolvedValueOnce([a]).mockReturnValueOnce(b.promise);
    const { result, rerender } = renderHook(({ workspace }) => useSnapshot(workspace), {
      initialProps: { workspace: "a" },
      wrapper,
    });
    await waitFor(() => expect(result.current.view.items).toEqual([a]));
    rerender({ workspace: "b" });
    expect(result.current.view).toMatchObject({ items: [], loaded: false, loading: true });
    rerender({ workspace: "a" });
    expect(result.current.view).toMatchObject({ items: [a], loaded: true, loading: false });
    await settle(b, [record("b", "late-b")]);
    expect(result.current.view.items).toEqual([a]);
    expect(transport.list).toHaveBeenCalledTimes(2);
  });

  it("keeps pending workspaces isolated and makes null idle", async () => {
    const a = deferred();
    const b = deferred();
    transport.list.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
    const { result, rerender } = renderHook(({ workspace }) => useSnapshot(workspace), {
      initialProps: { workspace: "a" as string | null },
      wrapper,
    });
    rerender({ workspace: "b" });
    await settle(a, [record("a", "old-a")]);
    expect(result.current.view).toMatchObject({ items: [], loaded: false, loading: true });
    rerender({ workspace: null });
    await settle(b, [record("b", "old-b")]);
    act(() => result.current.view.refresh());
    expect(result.current.view).toMatchObject({ items: [], loaded: false, loading: false });
    expect(transport.list).toHaveBeenCalledTimes(2);
  });

  it("excludes foreign rows from a mixed current response", async () => {
    const eligible = record("a", "eligible");
    transport.list.mockResolvedValue([record("b", "foreign"), eligible]);
    const { result } = renderHook(() => useSnapshot("a"), { wrapper });
    await waitFor(() => expect(result.current.view.loaded).toBe(true));
    expect(result.current.view.items).toEqual([eligible]);
  });

  it("does not fetch for an initially null workspace", () => {
    const { result } = renderHook(() => useSnapshot(null), { wrapper });
    expect(result.current.view).toMatchObject({ items: [], loaded: false, loading: false });
    expect(transport.list).not.toHaveBeenCalled();
  });
});

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.13
// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.15
describe("newest list request publication", () => {
  it.each(SETTLEMENT_ORDERS)("keeps newer refresh authority: %s", async (order) => {
    const { result, initial } = await loaded();
    const old = deferred();
    const current = deferred();
    const latest = record("a", "latest-a");
    transport.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    act(() => {
      result.current.view.refresh();
      result.current.view.refresh();
    });
    if (order === OLDEST_FIRST) {
      await settle(old, [record("a", OBSOLETE_A)]);
      expect(result.current.view).toMatchObject({ items: [initial], loading: true });
    }
    await settle(current, [latest]);
    await settle(old, [record("a", OBSOLETE_A)]);
    expect(result.current.view).toMatchObject({ items: [latest], loaded: true, loading: false });
  });

  it("accepts the newest empty response rather than resurrecting old rows", async () => {
    const { result } = await loaded();
    const old = deferred();
    const current = deferred();
    transport.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    act(() => {
      result.current.view.refresh();
      result.current.view.refresh();
    });
    await settle(current, []);
    await settle(old, [record("a", OBSOLETE_A)]);
    expect(result.current.view).toMatchObject({ items: [], loaded: true, loading: false });
  });

  it.each(SETTLEMENT_ORDERS)("retains baseline on current failure: %s", async (order) => {
    const { result, initial } = await loaded();
    const old = deferred();
    const current = deferred();
    transport.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    act(() => {
      result.current.view.refresh();
      result.current.view.refresh();
    });
    if (order === OLDEST_FIRST) {
      await settle(old, [record("a", OBSOLETE_A)]);
      expect(result.current.view).toMatchObject({ items: [initial], loading: true });
    }
    await fail(current);
    await settle(old, [record("a", OBSOLETE_A)]);
    expect(result.current.view).toMatchObject({ items: [initial], loading: false });
  });

  it.each(SETTLEMENT_ORDERS)("ignores obsolete failure: %s", async (order) => {
    const { result, initial } = await loaded();
    const old = deferred();
    const current = deferred();
    const latest = record("a", "latest-a");
    transport.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(current.promise);
    act(() => {
      result.current.view.refresh();
      result.current.view.refresh();
    });
    if (order === OLDEST_FIRST) {
      await fail(old);
      expect(result.current.view).toMatchObject({ items: [initial], loading: true });
    }
    await settle(current, [latest]);
    await fail(old);
    expect(result.current.view).toMatchObject({ items: [latest], loading: false });
  });

  it("settles initial failure and recovers from initial and refresh failures", async () => {
    const first = deferred();
    transport.list.mockReturnValue(first.promise);
    const { result } = renderHook(() => useSnapshot("a"), { wrapper });
    await fail(first);
    expect(result.current.view).toMatchObject({ items: [], loaded: true, loading: false });
    const recovered = record("a", "recovered-a");
    transport.list.mockResolvedValueOnce([recovered]);
    act(() => result.current.view.refresh());
    await waitFor(() => expect(result.current.view.items).toEqual([recovered]));
    const failedRefresh = deferred();
    transport.list.mockReturnValueOnce(failedRefresh.promise);
    act(() => result.current.view.refresh());
    await fail(failedRefresh);
    expect(result.current.view).toMatchObject({ items: [recovered], loaded: true, loading: false });
    transport.list.mockResolvedValueOnce(null);
    act(() => result.current.view.refresh());
    await waitFor(() => expect(result.current.view.items).toEqual([]));
  });
});

function usePair() {
  return { first: useAutomations("a"), second: useAutomations("a"), store: useAppStoreApi() };
}

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.14
describe("shared store request lifecycle", () => {
  it("shares initial transport and cache, and orders refresh across siblings", async () => {
    const first = deferred();
    transport.list.mockReturnValue(first.promise);
    const { result } = renderHook(usePair, { wrapper });
    expect(transport.list).toHaveBeenCalledTimes(1);
    const initial = record("a", "shared-a");
    await settle(first, [initial]);
    expect(result.current.first.items).toEqual([initial]);
    expect(result.current.second.items).toEqual([initial]);
    const old = deferred();
    const latest = deferred();
    transport.list.mockReturnValueOnce(old.promise).mockReturnValueOnce(latest.promise);
    act(() => {
      result.current.first.refresh();
      result.current.second.refresh();
    });
    expect(result.current.first.loading).toBe(true);
    expect(result.current.second.loading).toBe(true);
    const current = record("a", "current-shared");
    await settle(latest, [current]);
    await settle(old, [record("a", "obsolete-shared")]);
    expect(result.current.first).toMatchObject({ items: [current], loading: false });
    expect(result.current.second).toMatchObject({ items: [current], loading: false });
  });

  it.each(["a-first", "b-first"])(
    "keeps simultaneous different workspaces independent: %s",
    async (order) => {
      const a = deferred();
      const b = deferred();
      transport.list.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
      const { result } = renderHook(() => ({ a: useAutomations("a"), b: useAutomations("b") }), {
        wrapper,
      });
      const aRow = record("a", "current-a");
      const bRow = record("b", "current-b");
      if (order === "b-first") {
        await settle(b, [bRow]);
        expect(result.current.a).toMatchObject({ items: [], loading: true });
        expect(result.current.b).toMatchObject({ items: [bRow], loading: false });
      } else {
        await settle(a, [aRow]);
        expect(result.current.a).toMatchObject({ items: [aRow], loading: false });
        expect(result.current.b).toMatchObject({ items: [], loading: true });
      }
      await settle(a, [aRow]);
      await settle(b, [bRow]);
      expect(result.current.a.items).toEqual([aRow]);
      expect(result.current.b.items).toEqual([bRow]);
    },
  );
});

// @covers AC-OFFICE-AUTOMATIONS-SETTINGS-001.14
describe("store isolation and consumer lifetime", () => {
  it("keeps independent stores separate even with the same workspace", async () => {
    const a = deferred();
    const b = deferred();
    transport.list.mockReturnValueOnce(a.promise).mockReturnValueOnce(b.promise);
    const first = renderHook(() => useSnapshot("a"), { wrapper });
    const second = renderHook(() => useSnapshot("a"), { wrapper });
    await settle(b, [record("a", "second-store")]);
    expect(first.result.current.view.loading).toBe(true);
    await settle(a, [record("a", "first-store")]);
    expect(first.result.current.view.items[0].id).toBe("first-store");
    expect(second.result.current.view.items[0].id).toBe("second-store");
  });

  it.each([true, false])(
    "settles after initiating unmount with sibling=%s and reuses on remount",
    async (sibling) => {
      const request = deferred();
      transport.list.mockReturnValue(request.promise);
      const snapshots: Record<string, ReturnType<typeof useSnapshot>> = {};
      function Consumer({ id }: { id: string }) {
        snapshots[id] = useSnapshot("a");
        return null;
      }
      function tree(show: boolean) {
        return (
          <StateProvider>
            {show && <Consumer id="initiator" />}
            {sibling && <Consumer id="sibling" />}
          </StateProvider>
        );
      }
      const consumers = render(tree(true));
      expect(transport.list).toHaveBeenCalledTimes(1);
      consumers.rerender(tree(false));
      const current = record("a", "surviving-a");
      await settle(request, [current]);
      if (sibling)
        expect(snapshots.sibling.view).toMatchObject({ items: [current], loading: false });
      consumers.rerender(tree(true));
      expect(snapshots.initiator.view).toMatchObject({
        items: [current],
        loaded: true,
        loading: false,
      });
      expect(transport.list).toHaveBeenCalledTimes(1);
    },
  );

  it("deduplicates a pending StrictMode effect replay", async () => {
    const request = deferred();
    transport.list.mockReturnValue(request.promise);
    function strictWrapper({ children }: { children: ReactNode }) {
      return (
        <StrictMode>
          <StateProvider>{children}</StateProvider>
        </StrictMode>
      );
    }
    const { result } = renderHook(() => useSnapshot("a"), { wrapper: strictWrapper });
    expect(transport.list).toHaveBeenCalledTimes(1);
    const current = record("a", "strict-a");
    await settle(request, [current]);
    expect(result.current.view).toMatchObject({ items: [current], loaded: true, loading: false });
  });
});

describe("scoped breadcrumb and mutation compatibility", () => {
  it("retains a cached breadcrumb after another workspace publishes", async () => {
    const a = record("a", "a-row");
    const b = deferred();
    transport.list.mockResolvedValueOnce([a]).mockReturnValueOnce(b.promise);
    const { result, rerender } = renderHook(
      ({ workspace }) => ({
        ...useSnapshot(workspace),
        crumb: useSettingsBreadcrumbs("/settings/workspaces/a/automations/a-row"),
      }),
      { initialProps: { workspace: "a" }, wrapper },
    );
    await waitFor(() => expect(result.current.crumb.title).toBe("a-row"));
    rerender({ workspace: "b" });
    rerender({ workspace: "a" });
    await settle(b, [record("b", "b-row")]);
    expect(result.current.crumb.title).toBe("a-row");
    transport.list.mockResolvedValueOnce([]);
    act(() => result.current.view.refresh());
    await waitFor(() => expect(result.current.view.items).toEqual([]));
    act(() => result.current.store.getState().setAutomations([a]));
    expect(result.current.crumb.title).toBe("Automation");
  });

  it("preserves the scoped sequential mutation results and one-time secret", async () => {
    const { result, initial } = await loaded();
    const created = { ...record("a", "created-a"), webhook_secret: "one-time-secret" };
    const request: CreateAutomationRequest = {
      workspace_id: "a",
      name: created.name,
      workflow_id: "workflow",
      workflow_step_id: "step",
      agent_profile_id: "agent",
      executor_profile_id: "executor",
    };
    transport.create.mockResolvedValue(created);
    await act(async () => expect(await result.current.view.create(request)).toEqual(created));
    const stored = { ...created };
    Reflect.deleteProperty(stored, "webhook_secret");
    expect(result.current.view.items).toEqual([stored, initial]);
    expect(result.current.store.getState().automations.items[0]).toEqual(stored);
    const updated = { ...stored, name: "Updated" };
    transport.update.mockResolvedValue(updated);
    await act(async () =>
      expect(await result.current.view.update(created.id, { name: "Updated" })).toEqual(updated),
    );
    expect(result.current.view.items[0].name).toBe("Updated");
    const disabled = { ...updated, enabled: false };
    transport.disable.mockResolvedValue(disabled);
    await act(async () => expect(await result.current.view.disable(created.id)).toEqual(disabled));
    expect(result.current.view.items[0].enabled).toBe(false);
    transport.enable.mockResolvedValue(updated);
    await act(async () => expect(await result.current.view.enable(created.id)).toEqual(updated));
    expect(result.current.view.items[0].enabled).toBe(true);
    const trigger = { triggered: false, skipped: true, reason: "at capacity" };
    transport.trigger.mockResolvedValue(trigger);
    await act(async () => expect(await result.current.view.trigger(created.id)).toEqual(trigger));
    transport.remove.mockResolvedValue(undefined);
    await act(async () => {
      await result.current.view.remove(created.id);
    });
    expect(result.current.view.items.some((item) => item.id === created.id)).toBe(false);
  });

  it("supports a workspace-checked legacy breadcrumb when no scoped entry exists", () => {
    function legacyWrapper({ children }: { children: ReactNode }) {
      return (
        <StateProvider
          initialState={{
            automations: {
              items: [record("a", "legacy-row")],
              loaded: true,
              loading: false,
              triggerTypes: {},
            },
          }}
        >
          {children}
        </StateProvider>
      );
    }
    const { result, rerender } = renderHook(
      ({ workspace }) =>
        useSettingsBreadcrumbs(`/settings/workspaces/${workspace}/automations/legacy-row`),
      { initialProps: { workspace: "a" }, wrapper: legacyWrapper },
    );
    expect(result.current.title).toBe("legacy-row");
    rerender({ workspace: "b" });
    expect(result.current.title).toBe("Automation");
    expect(transport.list).not.toHaveBeenCalled();
  });
});
