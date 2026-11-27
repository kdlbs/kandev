import { useState } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import { act, cleanup, fireEvent, render, waitFor } from "@testing-library/react";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useExecutorProfileOptions } from "@/components/task-create-dialog-options";
import { ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import { registerExecutorsHandlers } from "@/lib/ws/handlers/executors";
import { registerExecutorProfileHandlers } from "@/lib/ws/handlers/executor-profiles";
import type { BackendMessage, BackendMessageMap } from "@/lib/types/backend";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { useSettingsData } from "./use-settings-data";

const LIVE_OWNER = "initial-live-owner";
const LIVE_PROFILE = "initial-live-profile";
const SELECTOR = "executor-profile-selector";
const TIME = "2026-10-10T07:00:00Z";

function profile(id = LIVE_PROFILE, executorId = LIVE_OWNER): ExecutorProfile {
  return {
    id,
    executor_id: executorId,
    name: id,
    prepare_script: "",
    cleanup_script: "",
    created_at: TIME,
    updated_at: TIME,
  };
}

function owner(id = LIVE_OWNER): Executor {
  return {
    id,
    name: id,
    type: "local",
    status: "active",
    is_system: false,
    config: { retained: id },
    profiles: [profile(`${id}-profile`, id)],
    created_at: TIME,
    updated_at: TIME,
  };
}

function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function pendingRead() {
  let resolve!: (response: Response) => void;
  const promise = new Promise<Response>((accept) => {
    resolve = accept;
  });
  return { promise, resolve, settled: false };
}

const fixtures: Array<{
  reads: ReturnType<typeof pendingRead>[];
  unexpected: string[];
}> = [];
function transport() {
  const reads: ReturnType<typeof pendingRead>[] = [];
  const unexpected: string[] = [];
  const fetch = vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
    const path = new URL(String(input), window.location.origin).pathname;
    if (path === "/api/v1/executors") {
      expect(init?.cache).toBe("no-store");
      expect(init?.credentials).toBe("include");
      const read = pendingRead();
      reads.push(read);
      return read.promise;
    }
    if (path === "/api/v1/agents") {
      return json({
        agents: [
          {
            id: "initial-agent",
            name: "Mock Agent",
            profiles: [
              {
                id: "initial-agent-profile",
                name: "Default",
                cli_passthrough: false,
              },
            ],
          },
        ],
        total: 1,
      });
    }
    unexpected.push(path);
    throw new Error(`Unexpected initial transport: ${path}`);
  });
  vi.stubGlobal("fetch", fetch);
  const fixture = { reads, unexpected, fetch };
  fixtures.push(fixture);
  return fixture;
}

function mount(
  seed: Executor[] = [],
  enabled = true,
  loaded = false,
  configure?: (store: ReturnType<typeof useAppStoreApi>) => void,
) {
  const capture = {} as { store: ReturnType<typeof useAppStoreApi> };
  let configured = false;
  function Surface({ active }: { active: boolean }) {
    capture.store = useAppStoreApi();
    if (!configured) {
      configure?.(capture.store);
      configured = true;
    }
    useSettingsData(active);
    const items = useAppStore((state) => state.executors.items);
    const [selected, setSelected] = useState("");
    const options = useExecutorProfileOptions(
      items.flatMap((executor) =>
        (executor.profiles ?? []).map((entry) => ({
          ...entry,
          executor_type: entry.executor_type ?? executor.type,
          executor_name: entry.executor_name ?? executor.name,
        })),
      ),
    );
    return (
      <>
        <ExecutorProfileSelector
          options={options}
          value={selected}
          onValueChange={setSelected}
          disabled={false}
          placeholder="Select an executor profile"
          popoverPortal={false}
        />
        <output data-testid="draft-selection">{selected}</output>
        <div data-testid="inventory">
          {options.map((option) => (
            <div
              key={option.value}
              data-testid={`choice-${option.value}`}
              data-disabled={option.disabled}
              data-owner-name={option.executorName}
              data-owner-type={option.executorType}
            >
              {option.renderLabel()}
            </div>
          ))}
        </div>
      </>
    );
  }
  const tree = (active: boolean) => (
    <StateProvider
      initialState={{
        executors: { items: seed },
        settingsData: { agentsLoaded: true, executorsLoaded: loaded },
        availableAgents: { items: [], tools: [], loaded: true, loading: false },
      }}
    >
      <Surface active={active} />
    </StateProvider>
  );
  const view = render(tree(enabled));
  return { ...capture, view, setEnabled: (active: boolean) => view.rerender(tree(active)) };
}
type Mounted = ReturnType<typeof mount>;

function publish<K extends keyof BackendMessageMap>(
  store: Mounted["store"],
  action: K,
  payload: BackendMessageMap[K]["payload"],
) {
  const handlers = {
    ...registerExecutorsHandlers(store),
    ...registerExecutorProfileHandlers(store),
  };
  const handler = handlers[action] as
    | ((message: BackendMessage<K, BackendMessageMap[K]["payload"]>) => void)
    | undefined;
  if (!handler) throw new Error(`Missing registered handler ${action}`);
  act(() => handler({ type: "notification", action, payload }));
}

function createLive(mounted: Mounted, executorId = LIVE_OWNER, profileId = LIVE_PROFILE) {
  publish(mounted.store, "executor.created", owner(executorId));
  publish(mounted.store, "executor.profile.created", profile(profileId, executorId));
}

async function admitted(fixture: ReturnType<typeof transport>) {
  await waitFor(() => expect(fixture.reads).toHaveLength(1));
  return fixture.reads[0];
}

async function settle(read: ReturnType<typeof pendingRead>, items: Executor[] = [], status = 200) {
  read.settled = true;
  await act(async () => {
    read.resolve(
      json(
        status === 200 ? { executors: items, total: items.length } : { error: "Read failed" },
        status,
      ),
    );
    await read.promise;
  });
}

async function select(mounted: Mounted, id: string, label = id) {
  fireEvent.click(mounted.view.getByTestId(SELECTOR));
  const option = await mounted.view.findByRole("option", { name: new RegExp(label) });
  fireEvent.click(option);
  await waitFor(() => expect(mounted.view.getByTestId("draft-selection").textContent).toBe(id));
}

function assertChoice(mounted: Mounted, id = LIVE_PROFILE, executorId = LIVE_OWNER) {
  const choice = mounted.view.getByTestId(`choice-${id}`);
  expect(choice.getAttribute("data-disabled")).toBe("false");
  expect(choice.getAttribute("data-owner-name")).toBe(executorId);
  expect(choice.getAttribute("data-owner-type")).toBe("local");
  expect(mounted.view.getByTestId(SELECTOR).textContent).toContain(id);
  expect(mounted.view.getByTestId(SELECTOR).textContent).not.toContain(
    "Select an executor profile",
  );
}

beforeEach(() => {
  const matchMedia = window.matchMedia.bind(window);
  vi.spyOn(window, "matchMedia").mockImplementation((query) => {
    const result = matchMedia(query);
    if (query === "(pointer: fine)") Object.defineProperty(result, "matches", { value: true });
    return result;
  });
  vi.spyOn(HTMLElement.prototype, "scrollIntoView").mockImplementation(() => {});
});

afterEach(async () => {
  for (const fixture of fixtures) {
    for (const read of fixture.reads) if (!read.settled) await settle(read);
    expect(fixture.unexpected).toEqual([]);
  }
  fixtures.length = 0;
  cleanup();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
});

describe("initial executor publication", () => {
  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.2 AC-EXECUTORS-INITIAL-CATALOGUE-001.3
  it("keeps a live selected profile after a stale successful initial GET", async () => {
    const fixture = transport();
    const mounted = mount();
    const read = await admitted(fixture);
    createLive(mounted);
    await select(mounted, LIVE_PROFILE);
    assertChoice(mounted);
    const current = mounted.store.getState().executors.items;
    expect(read.settled).toBe(false);
    await settle(read, [owner("old-response-owner")]);
    expect.soft(mounted.store.getState().executors.items).toEqual(current);
    expect.soft(mounted.view.queryByTestId(`choice-${LIVE_PROFILE}`)).not.toBeNull();
    expect.soft(mounted.view.getByTestId("draft-selection").textContent).toBe(LIVE_PROFILE);
    expect(mounted.view.getByTestId(SELECTOR).textContent).toContain(LIVE_PROFILE);
  });

  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.1
  it("populates usable choices from an ordinary initial GET", async () => {
    const fixture = transport();
    const mounted = mount();
    const entry = owner();
    await settle(await admitted(fixture), [entry]);
    await waitFor(() => expect(mounted.store.getState().settingsData.executorsLoaded).toBe(true));
    expect(mounted.store.getState().executors.items).toEqual([entry]);
    await select(mounted, `${LIVE_OWNER}-profile`);
    assertChoice(mounted, `${LIVE_OWNER}-profile`);
  });

  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.4
  it("skips GET and preserves usable prepopulated choices", async () => {
    const fixture = transport();
    const entry = owner();
    const mounted = mount([entry]);
    await waitFor(() => expect(mounted.store.getState().settingsData.executorsLoaded).toBe(true));
    expect(fixture.reads).toHaveLength(0);
    expect(mounted.store.getState().executors.items).toEqual([entry]);
    await select(mounted, `${LIVE_OWNER}-profile`);
    assertChoice(mounted, `${LIVE_OWNER}-profile`);
  });

  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.5
  it("preserves live choices after initial GET rejection", async () => {
    const fixture = transport();
    const mounted = mount();
    const read = await admitted(fixture);
    createLive(mounted);
    await select(mounted, LIVE_PROFILE);
    assertChoice(mounted);
    const current = mounted.store.getState().executors.items;
    await settle(read, [], 500);
    expect.soft(mounted.store.getState().executors.items).toEqual(current);
    expect.soft(mounted.view.queryByTestId(`choice-${LIVE_PROFILE}`)).not.toBeNull();
    expect(mounted.view.getByTestId(SELECTOR).textContent).toContain(LIVE_PROFILE);
    expect(mounted.store.getState().settingsData.executorsLoaded).toBe(true);
  });
});

describe("initial executor publication: live transitions", () => {
  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.2 AC-EXECUTORS-INITIAL-CATALOGUE-001.3
  it("preserves current mixed owner/profile values, membership and order", async () => {
    const fixture = transport();
    const mounted = mount();
    const read = await admitted(fixture);
    createLive(mounted);
    createLive(mounted, "second-owner", "second-choice");
    createLive(mounted, "deleted-owner", "deleted-choice");
    const metadata = owner();
    delete metadata.profiles;
    publish(mounted.store, "executor.updated", {
      ...metadata,
      name: "Current owner",
      config: { current: "config" },
    });
    publish(mounted.store, "executor.profile.updated", {
      ...profile(),
      name: "Current profile",
      prepare_script: "current prepare",
      config: { current: "profile" },
    });
    publish(mounted.store, "executor.profile.created", profile("live-sibling"));
    publish(mounted.store, "executor.profile.deleted", { id: "second-choice" });
    publish(mounted.store, "executor.deleted", owner("deleted-owner"));
    // Real immutable publication also permits an intentional current order change.
    act(() =>
      mounted.store
        .getState()
        .setExecutors([...mounted.store.getState().executors.items].reverse()),
    );
    const current = mounted.store.getState().executors.items;
    expect(current.map((entry) => entry.id)).toEqual(["second-owner", LIVE_OWNER]);
    expect(current[1].profiles?.map((entry) => entry.id)).toEqual([LIVE_PROFILE, "live-sibling"]);
    await select(mounted, LIVE_PROFILE, "Current profile");
    await settle(read, [owner(), owner("second-owner"), owner("deleted-owner")]);
    expect(mounted.store.getState().executors.items).toBe(current);
    expect(mounted.view.queryByTestId("choice-second-choice")).toBeNull();
    expect(mounted.view.queryByTestId("choice-deleted-choice")).toBeNull();
    expect(mounted.view.getByTestId(`choice-${LIVE_PROFILE}`).textContent).toContain(
      "Current owner",
    );
    expect(mounted.view.getByTestId(SELECTOR).textContent).toContain("Current profile");
  });

  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.2
  it("remembers live arrival followed by removal back to empty", async () => {
    const fixture = transport();
    const mounted = mount();
    const read = await admitted(fixture);
    createLive(mounted);
    expect(mounted.view.getByTestId(`choice-${LIVE_PROFILE}`)).toBeTruthy();
    publish(mounted.store, "executor.deleted", owner());
    expect(mounted.store.getState().executors.items).toEqual([]);
    await settle(read, [owner()]);
    expect(mounted.store.getState().executors.items).toEqual([]);
    expect(mounted.view.queryByTestId(`choice-${LIVE_PROFILE}`)).toBeNull();
  });
});

describe("initial executor publication: gates and cleanup", () => {
  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.1 AC-EXECUTORS-INITIAL-CATALOGUE-001.5
  it.each([200, 500])("settles an uncontested empty result: %s", async (status) => {
    const fixture = transport();
    const mounted = mount();
    await settle(await admitted(fixture), [], status);
    expect(mounted.store.getState().executors.items).toEqual([]);
    expect(mounted.store.getState().settingsData.executorsLoaded).toBe(true);
    expect(fixture.reads).toHaveLength(1);
  });

  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.4
  it("gates disabled and re-enabled consumers", async () => {
    const fixture = transport();
    const mounted = mount([], false);
    await act(async () => {});
    expect(fixture.reads).toHaveLength(0);
    expect(mounted.store.getState().settingsData.executorsLoaded).toBe(false);
    mounted.setEnabled(true);
    const read = await admitted(fixture);
    await settle(read, [owner()]);
    expect(mounted.store.getState().executors.items).toEqual([owner()]);
    expect(mounted.store.getState().settingsData.executorsLoaded).toBe(true);
    mounted.setEnabled(false);
    mounted.setEnabled(true);
    expect(fixture.reads).toHaveLength(1);
  });

  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.4
  it("skips an already-loaded empty catalogue", async () => {
    const fixture = transport();
    const mounted = mount([], true, true);
    await act(async () => {});
    expect(fixture.reads).toHaveLength(0);
    expect(mounted.store.getState().executors.items).toEqual([]);
  });

  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.6
  it("ignores unrelated settings and empty-array replacement", async () => {
    const fixture = transport();
    const mounted = mount();
    const read = await admitted(fixture);
    act(() => {
      mounted.store.getState().setSettingsData({ agentsLoaded: false });
      mounted.store.getState().setExecutors([]);
    });
    await settle(read, [owner()]);
    expect(mounted.store.getState().executors.items).toEqual([owner()]);
    await select(mounted, `${LIVE_OWNER}-profile`);
    assertChoice(mounted, `${LIVE_OWNER}-profile`);
  });

  // @covers AC-EXECUTORS-INITIAL-CATALOGUE-001.2 AC-EXECUTORS-INITIAL-CATALOGUE-001.5
  it.each([200, 500])(
    "keeps observation through rerender and disposes at settlement: %s",
    async (status) => {
      const fixture = transport();
      let subscriptions = 0;
      const mounted = mount([], true, false, (store) => {
        const subscribe = store.subscribe;
        vi.spyOn(store, "subscribe").mockImplementation((listener) => {
          subscriptions += 1;
          const unsubscribe = subscribe(listener);
          return () => {
            subscriptions -= 1;
            unsubscribe();
          };
        });
      });
      const read = await admitted(fixture);
      createLive(mounted);
      await act(async () => {});
      const pendingSubscriptions = subscriptions;
      mounted.setEnabled(false);
      mounted.view.unmount();
      const afterUnmount = subscriptions;
      expect(afterUnmount).toBeGreaterThan(1);
      expect(afterUnmount).toBeLessThan(pendingSubscriptions);
      publish(mounted.store, "executor.deleted", owner());
      await settle(read, [owner()], status);
      expect(mounted.store.getState().executors.items).toEqual([]);
      expect(mounted.store.getState().settingsData.executorsLoaded).toBe(true);
      expect(subscriptions).toBe(afterUnmount - 1);
    },
  );
});
