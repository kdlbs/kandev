import { act, cleanup, render, renderHook } from "@testing-library/react";
import { startTransition, Suspense, useLayoutEffect, useState, type ReactNode } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { SettingsSaveCancelledError } from "@/components/settings/settings-save-provider";
import type { ApiRequestOptions } from "@/lib/api/client";
import type { AppState } from "@/lib/state/store";
import type { SleepInhibitionResponse } from "@/lib/types/system";
import { useSleepInhibitionSettings } from "./use-sleep-inhibition-settings";

const jsonTransport = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: jsonTransport,
  fetchJsonWithRetry: jsonTransport,
}));

type Remote = ReturnType<typeof useSleepInhibitionSettings>;
type Flight = {
  method: string;
  options: ApiRequestOptions | undefined;
  resolve: (value: SleepInhibitionResponse) => void;
  reject: (error: Error) => void;
  settled: boolean;
};
const flights: Flight[] = [];
let store: StoreApi<AppState>;

function snapshot(enabled = false, active = false): SleepInhibitionResponse {
  return { settings: { enabled }, status: { platform: "linux", supported: true, active } };
}
function CaptureStore({ capture }: { capture?: (value: StoreApi<AppState>) => void }) {
  const current = useAppStoreApi();
  useLayoutEffect(() => {
    store = current;
    capture?.(current);
  }, [capture, current]);
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
function LoadedProvider({ children }: { children: ReactNode }) {
  return (
    <StateProvider
      initialState={{
        sleepInhibition: { response: snapshot(), loaded: true, loading: false, error: false },
      }}
    >
      <CaptureStore />
      {children}
    </StateProvider>
  );
}
async function settle(index: number, outcome: SleepInhibitionResponse | Error) {
  const flight = flights[index];
  if (!flight) throw new Error(`No admitted request at ${index}`);
  await act(async () => {
    flight.settled = true;
    if (outcome instanceof Error) flight.reject(outcome);
    else flight.resolve(outcome);
  });
}
function readState(owner = store) {
  return owner.getState().sleepInhibition;
}
function Probe({ capture }: { capture: (remote: Remote) => void }) {
  const remote = useSleepInhibitionSettings();
  useLayoutEffect(() => capture(remote));
  return null;
}

beforeEach(() => {
  flights.length = 0;
  jsonTransport.mockReset();
  jsonTransport.mockImplementation((url: string, options?: ApiRequestOptions) => {
    if (url !== "/api/v1/system/sleep-inhibition") throw new Error(`Unexpected transport: ${url}`);
    return new Promise<SleepInhibitionResponse>((resolve, reject) => {
      flights.push({
        method: options?.init?.method ?? "GET",
        options,
        resolve,
        reject,
        settled: false,
      });
    });
  });
});
afterEach(async () => {
  cleanup();
  for (let index = 0; index < flights.length; index++) {
    if (!flights[index].settled) await settle(index, snapshot());
  }
  vi.useRealTimers();
});

// @covers AC-PLATFORM-TASK-SLEEP-INHIBITION-001.9
// @covers AC-PLATFORM-TASK-SLEEP-INHIBITION-001.10
describe("sleep settings acknowledgement and real transport", () => {
  it.each([
    ["before save", "success"],
    ["before save", "failure"],
    ["during save", "success"],
    ["during save", "failure"],
  ])("fences GET %s after ACK followed by old %s", async (order, outcome) => {
    const hook = renderHook(useSleepInhibitionSettings, { wrapper: Providers });
    await settle(0, snapshot());
    let saved!: ReturnType<Remote["save"]>;
    act(() => {
      if (order === "before save") void hook.result.current.refresh();
      saved = hook.result.current.save({ enabled: true });
      if (order === "during save") void hook.result.current.refresh();
    });
    const patch = flights.findIndex((flight) => flight.method === "PATCH");
    const read = flights.findIndex((flight, index) => index > 0 && flight.method === "GET");
    expect(flights[patch].options?.init?.body).toBe('{"enabled":true}');
    expect(flights[read].options?.cache).toBe("no-store");
    const acknowledged = snapshot(true, true);
    await settle(patch, acknowledged);
    expect(await saved).toEqual(acknowledged);
    await settle(read, outcome === "success" ? snapshot(false) : new Error("status unavailable"));
    expect(readState()).toMatchObject({
      response: acknowledged,
      loaded: true,
      loading: false,
      error: false,
    });
  });

  it.each(["success", "failure"])(
    "old GET %s/finally cannot clear a post-ACK pending read",
    async (outcome) => {
      const hook = renderHook(useSleepInhibitionSettings, { wrapper: LoadedProvider });
      act(() => void hook.result.current.refresh());
      let saved!: ReturnType<Remote["save"]>;
      act(() => {
        saved = hook.result.current.save({ enabled: true });
      });
      await settle(1, snapshot(true));
      await saved;
      act(() => void hook.result.current.refresh());
      expect(flights.map((flight) => flight.method)).toEqual(["GET", "PATCH", "GET"]);
      expect(readState().loading).toBe(true);
      await settle(0, outcome === "success" ? snapshot(false) : new Error("retired GET"));
      expect(readState()).toMatchObject({ response: snapshot(true), loading: true, error: false });
      await settle(2, snapshot(true, true));
      expect(readState()).toMatchObject({
        response: snapshot(true, true),
        loading: false,
        error: false,
      });
    },
  );
});

describe("current sleep settings read and save controls", () => {
  it("accepts GET before ACK, then accepts a new GET after ACK", async () => {
    const hook = renderHook(useSleepInhibitionSettings, { wrapper: LoadedProvider });
    let saved!: ReturnType<Remote["save"]>;
    act(() => {
      saved = hook.result.current.save({ enabled: true });
    });
    act(() => void hook.result.current.refresh());
    await settle(1, snapshot(false, true));
    expect(readState().response).toEqual(snapshot(false, true));
    await settle(0, snapshot(true));
    expect(await saved).toEqual(snapshot(true));
    act(() => void hook.result.current.refresh());
    await settle(2, snapshot(true, true));
    expect(readState().response).toEqual(snapshot(true, true));
  });

  it.each(["read first", "failure first"])(
    "failed PATCH preserves a useful current GET: %s",
    async (order) => {
      const hook = renderHook(useSleepInhibitionSettings, { wrapper: LoadedProvider });
      const failure = new Error("PATCH denied");
      let result!: Promise<SleepInhibitionResponse | Error>;
      act(() => {
        result = hook.result.current.save({ enabled: true }).catch((error: Error) => error);
      });
      act(() => void hook.result.current.refresh());
      if (order === "read first") await settle(1, snapshot(false, true));
      await settle(0, failure);
      expect(await result).toBe(failure);
      if (order === "failure first") {
        expect(readState().loading).toBe(true);
        await settle(1, snapshot(false, true));
      }
      expect(readState()).toMatchObject({
        response: snapshot(false, true),
        loading: false,
        error: false,
      });
    },
  );

  it("keeps current load failure/retry and cached-response refresh behavior", async () => {
    const hook = renderHook(useSleepInhibitionSettings, { wrapper: Providers });
    await settle(0, new Error("initial load"));
    expect(readState()).toMatchObject({
      response: null,
      loaded: true,
      loading: false,
      error: true,
    });
    act(() => void hook.result.current.refresh());
    expect(readState()).toMatchObject({ loading: true, error: false });
    await settle(1, snapshot());
    act(() => void hook.result.current.refresh());
    await settle(2, new Error("current refresh"));
    expect(readState()).toMatchObject({ response: snapshot(), loading: false, error: true });
    act(() => void hook.result.current.refresh());
    await settle(3, snapshot(false, true));
    expect(readState()).toMatchObject({
      response: snapshot(false, true),
      loading: false,
      error: false,
    });
  });
});

// @covers AC-PLATFORM-TASK-SLEEP-INHIBITION-001.11
describe("sleep settings store sharing and committed lifetimes", () => {
  it("shares the ACK fence with a sibling while independent stores stay isolated", async () => {
    const first = renderHook(useSleepInhibitionSettings, { wrapper: LoadedProvider });
    const sharedStore = store;
    let sibling!: Remote;
    let unrelated!: Remote;
    let unrelatedStore!: StoreApi<AppState>;
    // Separate roots are independent; nested providers in this second view reuse its root.
    render(
      <LoadedProvider>
        <StateProvider>
          <Probe
            capture={(remote) => {
              sibling = remote;
            }}
          />
        </StateProvider>
        <Probe
          capture={(remote) => {
            unrelated = remote;
            unrelatedStore = store;
          }}
        />
      </LoadedProvider>,
    );
    const otherStore = store;
    expect(otherStore).not.toBe(sharedStore);
    act(() => void sibling.refresh());
    let saved!: ReturnType<Remote["save"]>;
    act(() => {
      saved = unrelated.save({ enabled: true });
    });
    await settle(1, snapshot(true));
    await saved;
    await settle(0, snapshot(false));
    expect(readState(otherStore)).toMatchObject({ response: snapshot(true), error: false });
    expect(unrelatedStore).toBe(otherStore);
    expect(readState(sharedStore).response).toEqual(snapshot());
    act(() => void first.result.current.refresh());
    await settle(2, snapshot(false, true));
    expect(readState(sharedStore).response).toEqual(snapshot(false, true));
    expect(readState(otherStore).response).toEqual(snapshot(true));
  });

  it.each(["success", "failure"])(
    "latest sibling owns loading and error through old %s",
    async (outcome) => {
      let first!: Remote;
      let second!: Remote;
      render(
        <LoadedProvider>
          <Probe
            capture={(remote) => {
              first = remote;
            }}
          />
          <Probe
            capture={(remote) => {
              second = remote;
            }}
          />
        </LoadedProvider>,
      );
      act(() => void first.refresh());
      act(() => void second.refresh());
      expect(flights).toHaveLength(2);
      await settle(0, outcome === "success" ? snapshot(true) : new Error("older sibling"));
      expect(readState()).toMatchObject({ response: snapshot(), loading: true, error: false });
      await settle(1, new Error("current sibling"));
      expect(readState()).toMatchObject({ response: snapshot(), loading: false, error: true });
      act(() => void second.refresh());
      await settle(2, snapshot(false, true));
      expect(readState()).toMatchObject({
        response: snapshot(false, true),
        loading: false,
        error: false,
      });
    },
  );
});

describe("sleep settings committed callback retirement", () => {
  it("retires A across A-B-A without reviving its retained callbacks", async () => {
    let current!: Remote;
    const capture = (remote: Remote) => {
      current = remote;
    };
    const content = (key: string) => (
      <LoadedProvider>
        <Probe key={key} capture={capture} />
      </LoadedProvider>
    );
    const view = render(content("A"));
    const firstA = { refresh: current.refresh, save: current.save };
    act(() => void firstA.refresh());
    view.rerender(content("B"));
    act(() => void current.refresh());
    view.rerender(content("A"));
    act(() => void current.refresh());
    expect(flights).toHaveLength(3);
    await settle(2, snapshot(true, true));
    await settle(0, snapshot(false));
    await settle(1, new Error("B retired"));
    act(() => void firstA.refresh());
    const cancelled = firstA.save({ enabled: false }).catch((error: unknown) => error);
    expect(flights).toHaveLength(3);
    expect(await cancelled).toBeInstanceOf(SettingsSaveCancelledError);
    expect(readState()).toMatchObject({
      response: snapshot(true, true),
      loading: false,
      error: false,
    });
  });

  it("refuses retained actions in replacement layout before passive cleanup", async () => {
    let retained!: Pick<Remote, "refresh" | "save">;
    let cancelled!: Promise<unknown>;
    function Committed({ name }: { name: string }) {
      const remote = useSleepInhibitionSettings();
      useLayoutEffect(() => {
        if (name === "A") retained = { refresh: remote.refresh, save: remote.save };
        else {
          void retained.refresh();
          cancelled = retained.save({ enabled: true }).catch((error: unknown) => error);
        }
      }, [name, remote.refresh, remote.save]);
      return null;
    }
    const view = render(
      <LoadedProvider>
        <Committed key="A" name="A" />
      </LoadedProvider>,
    );
    view.rerender(
      <LoadedProvider>
        <Committed key="B" name="B" />
      </LoadedProvider>,
    );
    expect(flights).toHaveLength(0);
    expect(await cancelled).toBeInstanceOf(SettingsSaveCancelledError);
  });
});

describe("sleep settings admitted saves and StrictMode replay", () => {
  it("keeps an admitted PATCH result after unmount while retiring the pending GET", async () => {
    const hook = renderHook(useSleepInhibitionSettings, { wrapper: LoadedProvider });
    const owner = store;
    act(() => void hook.result.current.refresh());
    let saved!: ReturnType<Remote["save"]>;
    act(() => {
      saved = hook.result.current.save({ enabled: true });
    });
    hook.unmount();
    const acknowledged = snapshot(true, true);
    await settle(1, acknowledged);
    expect(await saved).toEqual(acknowledged);
    await settle(0, new Error("closed view GET"));
    expect(readState(owner)).toMatchObject({
      response: acknowledged,
      loading: false,
      error: false,
    });
  });

  it("does not readmit the first StrictMode producing-activation callback after replay", async () => {
    let first!: Pick<Remote, "refresh" | "save">;
    let live!: Pick<Remote, "refresh" | "save">;
    function ActivationCapture() {
      const remote = useSleepInhibitionSettings();
      useLayoutEffect(() => {
        // Capture committed setup callbacks, never a discarded render's output.
        const produced = { refresh: remote.refresh, save: remote.save };
        if (!first) {
          first = produced;
          void first.refresh();
        }
        live = produced;
      });
      return null;
    }
    render(
      <LoadedProvider>
        <ActivationCapture />
      </LoadedProvider>,
      { reactStrictMode: true },
    );
    expect(flights).toHaveLength(1);
    await settle(0, snapshot());
    act(() => void first.refresh());
    const rejected = first.save({ enabled: true }).catch((error: unknown) => error);
    expect(flights).toHaveLength(1);
    expect(await rejected).toBeInstanceOf(SettingsSaveCancelledError);
    act(() => void live.refresh());
    expect(flights).toHaveLength(2);
    await settle(1, snapshot(false, true));
    expect(readState().response).toEqual(snapshot(false, true));
  });
});

describe("sleep settings speculative render control", () => {
  it("keeps visible committed callbacks valid during a speculative suspended render", async () => {
    let select!: (view: string) => void;
    let committed!: Remote;
    let release!: () => void;
    const suspension = new Promise<void>((resolve) => {
      release = resolve;
    });
    let shouldSuspend = true;
    function View({ name }: { name: string }) {
      const remote = useSleepInhibitionSettings();
      useLayoutEffect(() => {
        committed = remote;
      });
      if (name === "B" && shouldSuspend) throw suspension;
      return <span>{name}</span>;
    }
    function Screen() {
      const [name, setName] = useState("A");
      useLayoutEffect(() => {
        select = setName;
      }, []);
      return (
        <Suspense fallback={<span>pending</span>}>
          <View key={name} name={name} />
        </Suspense>
      );
    }
    const view = render(
      <LoadedProvider>
        <Screen />
      </LoadedProvider>,
    );
    act(() => void committed.refresh());
    const visibleRefresh = committed.refresh;
    act(() => startTransition(() => select("B")));
    expect(view.container.textContent).toBe("A");
    await settle(0, snapshot(true));
    expect(readState()).toMatchObject({ response: snapshot(true), loading: false, error: false });
    act(() => void visibleRefresh());
    expect(flights).toHaveLength(2);
    await settle(1, snapshot(true, true));
    expect(readState().response).toEqual(snapshot(true, true));
    shouldSuspend = false;
    await act(async () => release());
  });
});
