import { type ReactNode } from "react";
import { act, cleanup, render, renderHook, waitFor } from "@testing-library/react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import type { AppState } from "@/lib/state/store";
import type { AgentRoutePreview, ProviderHealth } from "@/lib/state/slices/office/types";
import { useRoutingPreview } from "./use-routing-preview";
import { useProviderHealth } from "./use-provider-health";

type Response = { agents?: AgentRoutePreview[]; health?: ProviderHealth[] };
const api = vi.hoisted(() => ({
  preview: vi.fn<(workspace: string) => Promise<Response>>(),
  health: vi.fn<(workspace: string) => Promise<Response>>(),
}));
vi.mock("@/lib/api/domains/office-extended-api", () => ({
  getRoutingPreview: api.preview,
  getProviderHealth: api.health,
}));

const readers = [
  {
    name: "routing preview",
    transport: api.preview,
    response: (id: string): Response => ({
      agents: [
        {
          agent_id: id,
          agent_name: id,
          tier_source: "workspace",
          effective_tier: "balanced",
          fallback_chain: [],
          missing: [],
          degraded: false,
        },
      ],
    }),
    useValue: (workspace: string | null) => {
      const value = useRoutingPreview(workspace);
      return { ...value, data: value.agents };
    },
    stored: (state: AppState, workspace: string) =>
      state.office.routing.preview.byWorkspace[workspace],
  },
  {
    name: "provider health",
    transport: api.health,
    response: (id: string): Response => ({
      health: [
        { provider_id: id, scope: "provider", scope_value: "", state: "healthy", backoff_step: 0 },
      ],
    }),
    useValue: (workspace: string | null) => {
      const value = useProviderHealth(workspace);
      return { ...value, data: value.health };
    },
    stored: (state: AppState, workspace: string) =>
      state.office.providerHealth.byWorkspace[workspace],
  },
];
type Reader = (typeof readers)[number];

const pending: Array<{ resolve: (response: Response) => void }> = [];
function deferred() {
  let resolve!: (response: Response) => void;
  let reject!: (error: unknown) => void;
  const promise = new Promise<Response>((yes, no) => {
    resolve = yes;
    reject = no;
  });
  const request = { promise, resolve, reject };
  pending.push(request);
  return request;
}

function data(reader: Reader, id: string) {
  const response = reader.response(id);
  return response.agents ?? response.health;
}

async function succeed(request: ReturnType<typeof deferred>, response: Response) {
  await act(async () => {
    request.resolve(response);
  });
}

async function fail(request: ReturnType<typeof deferred>, error: unknown = new Error("failed")) {
  await act(async () => {
    request.reject(error);
  });
}

function mount(reader: Reader, workspace: string | null = "alpha", strict = false) {
  function Wrapper({ children }: { children: ReactNode }) {
    const content = (
      <StateProvider
        initialState={{ workspaces: { items: [], activeId: workspace, activeIdRevision: 0 } }}
      >
        {children}
      </StateProvider>
    );
    return content;
  }
  return renderHook(
    () => {
      const selected = useAppStore((state) => state.workspaces.activeId);
      return { value: reader.useValue(selected), store: useAppStoreApi() };
    },
    { wrapper: Wrapper, reactStrictMode: strict },
  );
}

beforeEach(() => {
  api.preview.mockReset();
  api.health.mockReset();
  for (const reader of readers) {
    reader.transport.mockImplementation(async (workspace) => reader.response(workspace));
  }
});
afterEach(async () => {
  cleanup();
  await act(async () => {
    for (const request of pending.splice(0)) request.resolve({});
  });
});

type View = ReturnType<typeof mount>;
function select(view: View, workspace: string | null) {
  act(() => view.result.current.store.getState().setActiveWorkspace(workspace));
}
async function settled(view: View) {
  await waitFor(() => expect(view.result.current.value.isLoading).toBe(false));
}

for (const reader of readers) {
  describe(`${reader.name}: workspace reads`, () => {
    // @covers AC-OFFICE-LIVE-UPDATES-002.1, AC-OFFICE-LIVE-UPDATES-002.3
    it("loads beta after alpha completes", async () => {
      reader.transport.mockImplementation(async (workspace) => reader.response(workspace));
      const view = mount(reader);
      await waitFor(() => expect(view.result.current.value.isLoading).toBe(false));
      expect(reader.stored(view.result.current.store.getState(), "alpha")).toEqual(
        reader.response("alpha").agents ?? reader.response("alpha").health,
      );
      act(() => view.result.current.store.getState().setActiveWorkspace("beta"));
      await waitFor(() => expect(reader.transport).toHaveBeenNthCalledWith(2, "beta"));
      await waitFor(() =>
        expect(view.result.current.value.data).toEqual(
          reader.response("beta").agents ?? reader.response("beta").health,
        ),
      );
      expect(reader.stored(view.result.current.store.getState(), "alpha")).toEqual(
        reader.response("alpha").agents ?? reader.response("alpha").health,
      );
    });

    // @covers AC-OFFICE-LIVE-UPDATES-002.1, AC-OFFICE-LIVE-UPDATES-002.6
    it("loads current workspace and permits explicit refresh", async () => {
      const view = mount(reader);
      await settled(view);
      expect(view.result.current.value.data).toEqual(data(reader, "alpha"));
      view.rerender();
      expect(reader.transport).toHaveBeenCalledExactlyOnceWith("alpha");
      reader.transport.mockResolvedValueOnce(reader.response("refreshed"));
      await act(async () => {
        await view.result.current.value.refresh();
      });
      expect(reader.transport).toHaveBeenNthCalledWith(2, "alpha");
      expect(reader.stored(view.result.current.store.getState(), "alpha")).toEqual(
        data(reader, "refreshed"),
      );
    });

    // @covers AC-OFFICE-LIVE-UPDATES-002.2
    it.each([null, ""])("does not read with no workspace (%s)", async (workspace) => {
      const view = mount(reader, workspace);
      await act(async () => {
        await view.result.current.value.refresh();
      });
      expect(reader.transport).not.toHaveBeenCalled();
      expect(view.result.current.value).toMatchObject({ data: [], isLoading: false, error: null });
      select(view, "alpha");
      await settled(view);
      expect(view.result.current.value.data).toEqual(data(reader, "alpha"));
    });

    // @covers AC-OFFICE-LIVE-UPDATES-002.1, AC-OFFICE-LIVE-UPDATES-002.6
    it.each([{}, { agents: [], health: [] }])(
      "treats an empty response as completed (%j)",
      async (response) => {
        reader.transport.mockResolvedValue(response);
        const view = mount(reader);
        await settled(view);
        expect(reader.stored(view.result.current.store.getState(), "alpha")).toEqual([]);
        view.rerender();
        expect(reader.transport).toHaveBeenCalledTimes(1);
      },
    );

    // @covers AC-OFFICE-LIVE-UPDATES-002.1, AC-OFFICE-LIVE-UPDATES-002.3
    it("refreshes a cached workspace on return", async () => {
      const view = mount(reader);
      await settled(view);
      select(view, "beta");
      await settled(view);
      const next = deferred();
      reader.transport.mockReturnValueOnce(next.promise);
      select(view, "alpha");
      expect(view.result.current.value.data).toEqual(data(reader, "alpha"));
      expect(view.result.current.value.isLoading).toBe(true);
      expect(reader.transport).toHaveBeenNthCalledWith(3, "alpha");
      await succeed(next, reader.response("new-alpha"));
      expect(view.result.current.value.data).toEqual(data(reader, "new-alpha"));
    });
  });

  describe(`${reader.name}: departed reads`, () => {
    // @covers AC-OFFICE-LIVE-UPDATES-002.3, AC-OFFICE-LIVE-UPDATES-002.4
    it.each(["success", "failure"])(
      "ignores departed %s while beta is pending",
      async (outcome) => {
        const alpha = deferred();
        const beta = deferred();
        reader.transport.mockReturnValueOnce(alpha.promise).mockReturnValueOnce(beta.promise);
        const view = mount(reader);
        const departedRefresh = view.result.current.value.refresh;
        select(view, "beta");
        await act(async () => {
          await departedRefresh();
        });
        expect(reader.transport).toHaveBeenCalledTimes(2);
        if (outcome === "success") await succeed(alpha, reader.response("obsolete"));
        else await fail(alpha);
        expect(reader.stored(view.result.current.store.getState(), "alpha")).toBeUndefined();
        expect(view.result.current.value).toMatchObject({ data: [], isLoading: true, error: null });
        await succeed(beta, reader.response("beta"));
        expect(view.result.current.value).toMatchObject({
          data: data(reader, "beta"),
          isLoading: false,
          error: null,
        });
      },
    );

    it.each(["success", "failure"])("ignores departed %s after beta completes", async (outcome) => {
      const alpha = deferred();
      reader.transport.mockReturnValueOnce(alpha.promise);
      const view = mount(reader);
      select(view, "beta");
      await settled(view);
      if (outcome === "success") await succeed(alpha, reader.response("obsolete"));
      else await fail(alpha);
      expect(reader.stored(view.result.current.store.getState(), "alpha")).toBeUndefined();
      expect(view.result.current.value).toMatchObject({
        data: data(reader, "beta"),
        isLoading: false,
        error: null,
      });
    });

    // @covers AC-OFFICE-LIVE-UPDATES-002.2, AC-OFFICE-LIVE-UPDATES-002.4
    it.each(["success", "failure"])(
      "invalidates pending %s on empty selection and unmount",
      async (outcome) => {
        const first = deferred();
        reader.transport.mockReturnValueOnce(first.promise);
        const view = mount(reader);
        select(view, null);
        expect(view.result.current.value).toMatchObject({
          data: [],
          isLoading: false,
          error: null,
        });
        if (outcome === "success") await succeed(first, reader.response("obsolete"));
        else await fail(first);
        expect(reader.stored(view.result.current.store.getState(), "alpha")).toBeUndefined();
        expect(view.result.current.value).toMatchObject({
          data: [],
          isLoading: false,
          error: null,
        });
        const second = deferred();
        reader.transport.mockReturnValueOnce(second.promise);
        select(view, "alpha");
        const store = view.result.current.store;
        const snapshot = store.getState();
        view.unmount();
        if (outcome === "success") await succeed(second, reader.response("obsolete"));
        else await fail(second);
        expect(store.getState()).toBe(snapshot);
      },
    );
  });

  describe(`${reader.name}: reentry and recovery`, () => {
    // @covers AC-OFFICE-LIVE-UPDATES-002.4, AC-OFFICE-LIVE-UPDATES-002.5
    it("does not revive an old alpha read or retained callback after returning to alpha", async () => {
      const oldAlpha = deferred();
      const newAlpha = deferred();
      reader.transport.mockReturnValueOnce(oldAlpha.promise);
      const view = mount(reader);
      const oldRefresh = view.result.current.value.refresh;
      select(view, "beta");
      await settled(view);
      reader.transport.mockReturnValueOnce(newAlpha.promise);
      select(view, "alpha");
      act(() => {
        void oldRefresh();
      });
      expect(reader.transport).toHaveBeenCalledTimes(3);
      await succeed(oldAlpha, reader.response("obsolete"));
      expect(view.result.current.value).toMatchObject({ data: [], isLoading: true, error: null });
      expect(reader.stored(view.result.current.store.getState(), "alpha")).toBeUndefined();
      await succeed(newAlpha, reader.response("current"));
      expect(view.result.current.value.data).toEqual(data(reader, "current"));
    });

    // @covers AC-OFFICE-LIVE-UPDATES-002.3, AC-OFFICE-LIVE-UPDATES-002.6
    it("retains current data on failure and recovers manually", async () => {
      const view = mount(reader);
      await settled(view);
      reader.transport.mockRejectedValueOnce(new Error("current failure"));
      await act(async () => {
        await view.result.current.value.refresh();
      });
      expect(view.result.current.value).toMatchObject({
        data: data(reader, "alpha"),
        isLoading: false,
        error: "current failure",
      });
      view.rerender();
      expect(reader.transport).toHaveBeenCalledTimes(2);
      reader.transport.mockResolvedValueOnce(reader.response("recovered"));
      await act(async () => {
        await view.result.current.value.refresh();
      });
      expect(view.result.current.value).toMatchObject({
        data: data(reader, "recovered"),
        isLoading: false,
        error: null,
      });
      select(view, "beta");
      await settled(view);
      expect(view.result.current.value.error).toBeNull();
    });

    it("uses the localized fallback for a current non-Error rejection", async () => {
      reader.transport.mockRejectedValueOnce(null);
      const view = mount(reader);
      await settled(view);
      expect(view.result.current.value.error).toBe(
        reader.name === "routing preview"
          ? "Failed to load routing preview"
          : "Failed to load provider health",
      );
      view.rerender();
      expect(reader.transport).toHaveBeenCalledTimes(1);
    });
  });

  describe(`${reader.name}: refresh races`, () => {
    // @covers AC-OFFICE-LIVE-UPDATES-002.5
    it.each(["success", "failure"])(
      "older refresh %s cannot settle a newer pending read",
      async (outcome) => {
        const view = mount(reader);
        await settled(view);
        const older = deferred();
        const newer = deferred();
        reader.transport.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
        let first!: Promise<void>;
        let second!: Promise<void>;
        act(() => {
          first = view.result.current.value.refresh();
          second = view.result.current.value.refresh();
        });
        if (outcome === "success") await succeed(older, reader.response("obsolete"));
        else await fail(older);
        expect(view.result.current.value).toMatchObject({
          data: data(reader, "alpha"),
          isLoading: true,
          error: null,
        });
        await succeed(newer, reader.response("newest"));
        await act(async () => {
          await Promise.all([first, second]);
        });
        expect(reader.stored(view.result.current.store.getState(), "alpha")).toEqual(
          data(reader, "newest"),
        );
        expect(view.result.current.value.isLoading).toBe(false);
      },
    );

    it.each(["success", "failure"])(
      "newest failure prevents late older refresh %s publication",
      async (outcome) => {
        const view = mount(reader);
        await settled(view);
        const older = deferred();
        const newer = deferred();
        reader.transport.mockReturnValueOnce(older.promise).mockReturnValueOnce(newer.promise);
        act(() => {
          void view.result.current.value.refresh();
          void view.result.current.value.refresh();
        });
        await fail(newer, new Error("newest failure"));
        if (outcome === "success") await succeed(older, reader.response("obsolete"));
        else await fail(older);
        expect(view.result.current.value).toMatchObject({
          data: data(reader, "alpha"),
          isLoading: false,
          error: "newest failure",
        });
        expect(reader.stored(view.result.current.store.getState(), "alpha")).toEqual(
          data(reader, "alpha"),
        );
      },
    );
  });

  describe(`${reader.name}: independent instances`, () => {
    // @covers AC-OFFICE-LIVE-UPDATES-002.7
    it("keeps mounted instances sharing a store independent", async () => {
      const first = deferred();
      const second = deferred();
      reader.transport.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
      const values = new Map<string, ReturnType<Reader["useValue"]>>();
      let store!: ReturnType<typeof useAppStoreApi>;
      function Probe({ id }: { id: string }) {
        const workspace = useAppStore((state) => state.workspaces.activeId);
        values.set(id, reader.useValue(workspace));
        store = useAppStoreApi();
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
      const pair = render(<Pair firstMounted />);
      expect(reader.transport).toHaveBeenCalledTimes(2);
      await succeed(first, reader.response("cached"));
      expect(values.get("first")?.isLoading).toBe(false);
      expect(values.get("second")).toMatchObject({ data: data(reader, "cached"), isLoading: true });
      pair.rerender(<Pair firstMounted={false} />);
      await succeed(second, reader.response("sibling"));
      const expected =
        reader.name === "provider health"
          ? [...data(reader, "sibling")!, ...data(reader, "cached")!]
          : data(reader, "sibling");
      expect(values.get("second")).toMatchObject({
        data: expected,
        isLoading: false,
      });
      expect(reader.stored(store.getState(), "alpha")).toEqual(expected);
    });

    it("unmounting an instance leaves a sibling store's pending read valid", async () => {
      const first = deferred();
      const second = deferred();
      reader.transport.mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
      const left = mount(reader);
      const right = mount(reader);
      const leftStore = left.result.current.store;
      left.unmount();
      await succeed(first, reader.response("obsolete"));
      expect(reader.stored(leftStore.getState(), "alpha")).toBeUndefined();
      expect(right.result.current.value.isLoading).toBe(true);
      await succeed(second, reader.response("right"));
      expect(right.result.current.value.data).toEqual(data(reader, "right"));
    });
  });

  describe(`${reader.name}: StrictMode replay`, () => {
    // @covers AC-OFFICE-LIVE-UPDATES-002.4, AC-OFFICE-LIVE-UPDATES-002.8
    it("StrictMode replay accepts setup after cleanup and ignores the precleanup read", async () => {
      const beforeCleanup = deferred();
      const afterCleanup = deferred();
      reader.transport
        .mockReturnValueOnce(beforeCleanup.promise)
        .mockReturnValueOnce(afterCleanup.promise);
      const view = mount(reader, "alpha", true);
      expect(reader.transport).toHaveBeenCalledTimes(2);
      await succeed(beforeCleanup, reader.response("obsolete"));
      expect(view.result.current.value).toMatchObject({ data: [], isLoading: true, error: null });
      expect(reader.stored(view.result.current.store.getState(), "alpha")).toBeUndefined();
      await succeed(afterCleanup, reader.response("current"));
      expect(view.result.current.value).toMatchObject({
        data: data(reader, "current"),
        isLoading: false,
        error: null,
      });
      view.rerender();
      expect(reader.transport).toHaveBeenCalledTimes(2);
    });
  });
}
