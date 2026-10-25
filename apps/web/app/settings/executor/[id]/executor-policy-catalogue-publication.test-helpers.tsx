import { act, cleanup, fireEvent, render } from "@testing-library/react";
import { expect, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { useExecutorProfileOptions } from "@/components/task-create-dialog-options";
import type { AppState } from "@/lib/state/store";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import type { BackendMessageMap } from "@/lib/types/backend";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import { registerExecutorProfileHandlers } from "@/lib/ws/handlers/executor-profiles";
import ExecutorEditPage from "./page";

export const OWNER = "policy-owner";
export const OTHER = "other-owner";
export const POLICY = '{"allow_http":true}';
const TIME = "2026-10-09T00:00:00Z";

export function profile(id: string, executorId = OTHER, name = id): ExecutorProfile {
  return {
    id,
    executor_id: executorId,
    name,
    prepare_script: "",
    cleanup_script: "",
    created_at: TIME,
    updated_at: TIME,
  };
}

export function executor(id: string, profiles: ExecutorProfile[] = []): Executor {
  return {
    id,
    name: id,
    type: "local_pc",
    status: "active",
    is_system: true,
    config: { mcp_policy: "", retained: id },
    profiles,
    created_at: TIME,
    updated_at: TIME,
  };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  return { promise, resolve, reject };
}

type SaveResult = Awaited<ReturnType<SettingsSaveCoordinator["saveAll"]>>;
export type Transport = "rest" | "ws";
export type Fixture = {
  transport: Transport;
  seed: Executor[];
  store: StoreApi<AppState>;
  coordinator: SettingsSaveCoordinator;
  options: ReturnType<typeof useExecutorProfileOptions>;
  response: ReturnType<typeof deferred<Executor>>;
  admission: ReturnType<typeof deferred<void>>;
  payload?: { id?: string; name?: string; config: Record<string, string> };
  submitted?: string;
  completion?: Promise<SaveResult>;
  result?: SaveResult;
  settled: boolean;
  view: ReturnType<typeof render>;
};
const fixtures: Fixture[] = [];
let previousClient: ReturnType<typeof getWebSocketClient>;

function admit(transport: Transport, payload: Fixture["payload"]) {
  const fixture = fixtures.find(
    (candidate) =>
      candidate.transport === transport &&
      !candidate.payload &&
      candidate.submitted === payload?.config.mcp_policy,
  );
  if (!fixture || !payload) throw new Error("Unexpected policy transport admission");
  fixture.payload = payload;
  fixture.admission.resolve();
  return fixture.response.promise;
}

export function prepareTransport() {
  vi.useFakeTimers();
  previousClient = getWebSocketClient();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const url = new URL(String(input));
      if (url.pathname !== `/api/v1/executors/${OWNER}` || init?.method !== "PATCH") {
        throw new Error(`Unexpected HTTP transport: ${init?.method} ${url.pathname}`);
      }
      expect(init.headers).toMatchObject({ "Content-Type": "application/json" });
      expect(init.cache).toBe("no-store");
      const payload = JSON.parse(String(init.body)) as Fixture["payload"];
      const accepted = await admit("rest", payload);
      return new Response(JSON.stringify(accepted), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

export function mountPolicy(transport: Transport = "rest", isSystem = true): Fixture {
  if (transport === "ws") {
    const client = new WebSocketClient("ws://localhost/policy-test");
    vi.spyOn(client, "request").mockImplementation(async <T,>(action: string, payload: unknown) => {
      expect(action).toBe("executor.update");
      expect(payload).toMatchObject({ id: OWNER });
      return (await admit("ws", payload as Fixture["payload"])) as T;
    });
    setWebSocketClient(client);
  } else {
    setWebSocketClient(null);
  }
  const owner = { ...executor(OWNER, [profile("own-profile", OWNER)]), is_system: isSystem };
  const fixture = {
    transport,
    seed: [owner, executor(OTHER, [profile("keep"), profile("remove")])],
    response: deferred<Executor>(),
    admission: deferred<void>(),
    settled: false,
  } as Fixture;
  fixtures.push(fixture);
  function Observe() {
    fixture.store = useAppStoreApi();
    fixture.coordinator = useSettingsSaveCoordinator();
    const catalogue = useAppStore((state) => state.executors.items);
    // Task creation and subtasks share this owner metadata fallback.
    const profiles = catalogue.flatMap((item) =>
      (item.profiles ?? []).map((entry) => ({
        ...entry,
        executor_type: entry.executor_type ?? item.type,
        executor_name: entry.executor_name ?? item.name,
      })),
    );
    fixture.options = useExecutorProfileOptions(profiles);
    return null;
  }
  fixture.view = render(
    <StateProvider initialState={{ executors: { items: fixture.seed } }}>
      <SettingsSaveProvider>
        <ExecutorEditPage executorId={OWNER} />
        <Observe />
      </SettingsSaveProvider>
    </StateProvider>,
  );
  return fixture;
}

export function policyInput(fixture: Fixture) {
  const input = fixture.view.container.querySelector<HTMLTextAreaElement>("#mcp-policy");
  if (!input) throw new Error("Policy field is unavailable");
  return input;
}

export function edit(fixture: Fixture, value: string) {
  fireEvent.change(policyInput(fixture), { target: { value } });
}

export async function startSave(fixture: Fixture, policy = POLICY) {
  edit(fixture, policy);
  fixture.submitted = policy;
  act(() => {
    fixture.completion = fixture.coordinator.saveAll();
  });
  await act(async () => fixture.admission.promise);
  expect(fixture.coordinator.status).toBe("saving");
  expect(fixture.coordinator.hasDirty).toBe(true);
  const { is_system: isSystem, name, config } = fixture.seed[0];
  expect(fixture.payload).toEqual({
    ...(fixture.transport === "ws" ? { id: OWNER } : {}),
    ...(!isSystem ? { name } : {}),
    config: { ...config, mcp_policy: policy },
  });
}

export function accepted(fixture: Fixture, policy = fixture.submitted ?? POLICY): Executor {
  const { profiles: _profiles, ...owner } = fixture.seed[0];
  return { ...owner, config: { ...owner.config, mcp_policy: policy }, updated_at: TIME };
}

export async function settle(fixture: Fixture, response: Executor | Error = accepted(fixture)) {
  await act(async () => {
    fixture.settled = true;
    if (response instanceof Error) fixture.response.reject(response);
    else fixture.response.resolve(response);
    fixture.result = await fixture.completion;
  });
}

export function publish(fixture: Fixture, current: Executor[]) {
  act(() => fixture.store.getState().setExecutors(current));
}

type ProfileEvent = "executor.profile.created" | "executor.profile.updated";
export function event(fixture: Fixture, action: ProfileEvent, entry: ExecutorProfile) {
  const message: BackendMessageMap[ProfileEvent] = {
    id: `test-${action}`,
    type: "notification",
    action,
    payload: entry,
  };
  act(() => {
    const handlers = registerExecutorProfileHandlers(fixture.store);
    if (message.action === "executor.profile.created") {
      handlers["executor.profile.created"]!(message);
    } else {
      handlers["executor.profile.updated"]!(message);
    }
  });
}

export function deleted(fixture: Fixture, id: string) {
  act(() =>
    registerExecutorProfileHandlers(fixture.store)["executor.profile.deleted"]!({
      id: "test-delete",
      type: "notification",
      action: "executor.profile.deleted",
      payload: { id },
    }),
  );
}

export function expectCatalogue(fixture: Fixture, expected: Executor[]) {
  expect.soft(fixture.store.getState().executors.items).toEqual(expected);
  expect
    .soft(
      fixture.options.map(({ value, label, executorType, executorName, disabled }) => ({
        value,
        label,
        executorType,
        executorName,
        disabled,
      })),
    )
    .toEqual(
      expected.flatMap((owner) =>
        (owner.profiles ?? []).map((entry) => ({
          value: entry.id,
          label: entry.name,
          executorType: entry.executor_type ?? owner.type,
          executorName: entry.executor_name ?? owner.name,
          disabled: false,
        })),
      ),
    );
}

export function withAccepted(fixture: Fixture, current: Executor[], policy?: string) {
  const response = accepted(fixture, policy);
  return current.map((item) => (item.id === OWNER ? { ...item, ...response } : item));
}

export function expectClean(fixture: Fixture) {
  expect(fixture.result).toMatchObject({ canLeave: true, failedIds: new Set() });
  expect(fixture.coordinator.hasDirty).toBe(false);
  expect(policyInput(fixture).dataset.settingsDirty).toBe("false");
}

export async function cleanFixtures() {
  for (const fixture of fixtures) {
    if (fixture.completion && !fixture.settled) await settle(fixture);
  }
  cleanup();
  fixtures.length = 0;
  setWebSocketClient(previousClient);
  await act(async () => vi.runOnlyPendingTimersAsync());
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
}
