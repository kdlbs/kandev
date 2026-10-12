import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useState } from "react";
import { useTranslation } from "react-i18next";
import { expect, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { SettingsSaveProvider } from "@/components/settings/settings-save-provider";
import { LegacyExecutorSettingsRoute } from "@/components/settings/legacy-executor-settings-route";
import { ToastProvider } from "@/components/toast-provider";
import { useExecutorProfileOptions } from "@/components/task-create-dialog-options";
import { ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import { usePathname } from "@/lib/routing/client-router";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import type { AppState } from "@/lib/state/store";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import type { BackendMessageMap } from "@/lib/types/backend";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import { registerExecutorsHandlers } from "@/lib/ws/handlers/executors";
import { registerExecutorProfileHandlers } from "@/lib/ws/handlers/executor-profiles";

export const TARGET = "delete-owner";
export const RETAINED = "retained-owner";
export const REMOVED = "removed-owner";
export const HUB = "/settings/executors";
const TIME = "2026-10-10T00:00:00Z";
export type Transport = "HTTP" | "WS";

export function profile(id: string, owner: string, name = id): ExecutorProfile {
  return {
    id,
    executor_id: owner,
    name,
    prepare_script: "prepare-current",
    cleanup_script: "cleanup-current",
    config: { retained: id },
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
    is_system: false,
    config: { owner: id },
    profiles,
    created_at: TIME,
    updated_at: TIME,
  };
}

function deferred() {
  let resolve!: () => void;
  const promise = new Promise<void>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

export type Fixture = {
  store: StoreApi<AppState>;
  options: ReturnType<typeof useExecutorProfileOptions>;
  seed: Executor[];
  transport: Transport;
  admitted: boolean;
  settled: boolean;
  admission: ReturnType<typeof deferred>;
  response: ReturnType<typeof deferred>;
  view: ReturnType<typeof render>;
};

const fixtures: Fixture[] = [];
let previousClient: ReturnType<typeof getWebSocketClient>;
let previousLocation: string;
export const requests: Array<{ path: string; init?: RequestInit }> = [];

async function admit(transport: Transport) {
  const fixture = fixtures.find((item) => item.transport === transport && !item.admitted);
  if (!fixture) throw new Error("Unexpected owner-delete request");
  fixture.admitted = true;
  fixture.admission.resolve();
  await fixture.response.promise;
}

export function prepare() {
  vi.useFakeTimers();
  previousClient = getWebSocketClient();
  previousLocation = window.location.href;
  requests.length = 0;
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", `/settings/executor/${TARGET}`);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = new URL(String(input), window.location.origin).pathname;
      requests.push({ path, init });
      expect(path).toBe(`/api/v1/executors/${TARGET}`);
      expect(init).toMatchObject({ method: "DELETE", cache: "no-store" });
      expect(init?.headers).toMatchObject({ "Content-Type": "application/json" });
      expect(init?.body).toBeUndefined();
      await admit("HTTP");
      return new Response(null, { status: 204 });
    }),
  );
}

function Scene({ fixture }: { fixture: Fixture }) {
  const { t } = useTranslation();
  fixture.store = useAppStoreApi();
  const catalogue = useAppStore((state) => state.executors.items);
  const pathname = usePathname();
  const [selected, setSelected] = useState("");
  const profiles = catalogue.flatMap((owner) =>
    (owner.profiles ?? []).map((entry) => ({
      ...entry,
      executor_type: entry.executor_type ?? owner.type,
      executor_name: entry.executor_name ?? owner.name,
    })),
  );
  fixture.options = useExecutorProfileOptions(profiles);
  if (pathname !== HUB) return <LegacyExecutorSettingsRoute executorId={TARGET} />;
  return (
    <ExecutorProfileSelector
      options={fixture.options}
      value={selected}
      onValueChange={setSelected}
      disabled={false}
      placeholder={t("task:executorProfile2")}
    />
  );
}

export function mount(transport: Transport = "HTTP", isSystem = false): Fixture {
  if (transport === "WS") {
    const client = new WebSocketClient("ws://localhost/owner-delete-test");
    vi.spyOn(client, "request").mockImplementation(async <T,>(action: string, payload: unknown) => {
      expect(action).toBe("executor.delete");
      expect(payload).toEqual({ id: TARGET });
      await admit("WS");
      return undefined as T;
    });
    setWebSocketClient(client);
  } else {
    setWebSocketClient(null);
  }
  const fixture = {
    transport,
    seed: [
      { ...executor(TARGET, [profile("target-profile", TARGET)]), is_system: isSystem },
      executor(RETAINED, [profile("retained-profile", RETAINED)]),
      executor(REMOVED, [profile("removed-profile", REMOVED)]),
    ],
    admitted: false,
    settled: false,
    admission: deferred(),
    response: deferred(),
  } as Fixture;
  fixtures.push(fixture);
  fixture.view = render(
    <StateProvider initialState={{ executors: { items: fixture.seed } }}>
      <ToastProvider>
        <SettingsSaveProvider>
          <Scene fixture={fixture} />
        </SettingsSaveProvider>
      </ToastProvider>
    </StateProvider>,
  );
  return fixture;
}

export function openConfirmation() {
  const section = screen.getByText("Remove this executor").closest('[data-slot="card"]');
  if (!section) throw new Error("Owner deletion section is missing");
  fireEvent.click(within(section as HTMLElement).getByRole("button", { name: "Delete" }));
  const dialog = screen.getByRole("dialog");
  return {
    dialog,
    input: within(dialog).getByLabelText("Confirm Delete"),
    button: within(dialog).getByRole("button", { name: "Delete" }),
  };
}

export async function start(fixture: Fixture) {
  const { input, button } = openConfirmation();
  fireEvent.change(input, { target: { value: "delete" } });
  expect((button as HTMLButtonElement).disabled).toBe(false);
  fireEvent.click(button);
  await act(async () => fixture.admission.promise);
  expect((screen.getByRole("button", { name: "Deleting..." }) as HTMLButtonElement).disabled).toBe(
    true,
  );
  expect(window.location.pathname).toBe(`/settings/executor/${TARGET}`);
  expect(fixture.settled).toBe(false);
}

export async function accept(fixture: Fixture) {
  await act(async () => {
    fixture.settled = true;
    fixture.response.resolve();
    await fixture.response.promise;
  });
  expect(window.location.pathname).toBe(HUB);
  expect(screen.queryByRole("dialog")).toBeNull();
}

type OwnerEvent = "executor.created" | "executor.updated" | "executor.deleted";
type ProfileEvent =
  | "executor.profile.created"
  | "executor.profile.updated"
  | "executor.profile.deleted";
export function ownerEvent<A extends OwnerEvent>(
  fixture: Fixture,
  action: A,
  payload: BackendMessageMap[A]["payload"],
) {
  act(() => {
    registerExecutorsHandlers(fixture.store)[action]!({
      id: `delete-test-${action}`,
      type: "notification",
      action,
      payload,
    } as BackendMessageMap[A]);
  });
}

export function profileEvent<A extends ProfileEvent>(
  fixture: Fixture,
  action: A,
  payload: BackendMessageMap[A]["payload"],
) {
  act(() => {
    registerExecutorProfileHandlers(fixture.store)[action]!({
      id: `delete-test-${action}`,
      type: "notification",
      action,
      payload,
    } as BackendMessageMap[A]);
  });
}

export async function expectInventory(fixture: Fixture, survivors: Executor[]) {
  const actual = fixture.store.getState().executors.items;
  expect.soft(actual).toEqual(survivors);
  survivors.forEach((owner, index) => expect.soft(actual[index]).toBe(owner));
  fireEvent.click(screen.getByTestId("executor-profile-selector"));
  await act(async () => vi.runOnlyPendingTimersAsync());
  const choices = screen.getAllByRole("option");
  const profiles = survivors.flatMap((owner) => owner.profiles ?? []);
  expect.soft(choices).toHaveLength(profiles.length);
  profiles.forEach((entry) => {
    const choice = choices.find((item) => item.getAttribute("data-value") === entry.id);
    expect.soft(choice).toBeDefined();
    if (choice) {
      expect.soft(choice.textContent).toContain(entry.name);
      const owner = survivors.find((item) => item.id === entry.executor_id)!;
      expect.soft(choice.textContent).toContain(owner.name);
      expect.soft(choice.getAttribute("aria-disabled")).not.toBe("true");
      expect.soft(fixture.options.find((item) => item.value === entry.id)).toMatchObject({
        executorName: owner.name,
        executorType: owner.type,
        disabled: false,
      });
    }
  });
  expect.soft(fixture.options.map((item) => item.value)).toEqual(profiles.map((item) => item.id));
}

export async function teardown() {
  for (const fixture of fixtures) {
    if (fixture.admitted && !fixture.settled) await accept(fixture);
  }
  cleanup();
  fixtures.length = 0;
  setWebSocketClient(previousClient);
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", previousLocation);
  await act(async () => vi.runOnlyPendingTimersAsync());
  vi.useRealTimers();
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
}
