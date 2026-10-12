import { useState } from "react";
import { useTranslation } from "react-i18next";
import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { expect, vi } from "vitest";
import type { StoreApi } from "zustand";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useExecutorProfileOptions } from "@/components/task-create-dialog-options";
import { ExecutorProfileSelector } from "@/components/task-create-dialog-selectors";
import { usePathname } from "@/lib/routing/client-router";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import type { AppState } from "@/lib/state/store";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import type { ExecutorPayload, ExecutorProfilePayload } from "@/lib/types/backend";
import { WebSocketClient } from "@/lib/ws/client";
import { getWebSocketClient, setWebSocketClient } from "@/lib/ws/connection";
import { registerExecutorsHandlers } from "@/lib/ws/handlers/executors";
import { registerExecutorProfileHandlers } from "@/lib/ws/handlers/executor-profiles";
import { renderSettingsRoute } from "@/src/settings-routes";

export const CREATED = "accepted-owner";
export const SURVIVOR = "surviving-owner";
export const REMOVED = "removed-owner";
export const LIVE = "live-owner";
const TIME = "2026-10-10T00:00:00Z";
const CREATE_ROUTE = "/settings/executor/new";
export const LIST_ROUTE = "/settings/executors";
// i18n-exempt: executor name is persisted test payload data, not product copy.
const SUBMITTED_NAME = "Submitted executor";
const REMOTE_HOST = "tcp://independent.test:2376";

export type Transport = "http" | "ws";
type Options = ReturnType<typeof useExecutorProfileOptions>;
type CreatePayload = { name: string; type: string; status: string; config: Record<string, string> };

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((accept) => {
    resolve = accept;
  });
  return { promise, resolve };
}

export function profile(id: string, executorId: string, name = id): ExecutorProfile {
  return {
    id,
    executor_id: executorId,
    name,
    config: { retained: id },
    prepare_script: `prepare ${id}`,
    cleanup_script: `cleanup ${id}`,
    created_at: TIME,
    updated_at: TIME,
  };
}

export function owner(id: string, profiles: ExecutorProfile[] = []): Executor {
  return {
    id,
    name: id,
    type: "local_pc",
    status: "active",
    is_system: false,
    config: { retained: id },
    profiles,
    created_at: TIME,
    updated_at: TIME,
  };
}

export function ownerPayload(id: string): ExecutorPayload {
  return {
    id,
    name: id,
    type: "local_pc",
    status: "active",
    is_system: false,
    config: { retained: id },
    created_at: TIME,
    updated_at: TIME,
  };
}

export type Fixture = {
  transport: Transport;
  seed: Executor[];
  accepted: Executor;
  response: ReturnType<typeof deferred<Executor>>;
  admission: ReturnType<typeof deferred<void>>;
  submitted?: CreatePayload;
  settled: boolean;
  store: StoreApi<AppState>;
  options: Options;
};
let active: Fixture | undefined;
let previousClient: ReturnType<typeof getWebSocketClient>;
let previousLocation: string;

function admit(payload: CreatePayload) {
  if (!active || active.submitted) throw new Error("Unexpected executor creation admission");
  active.submitted = payload;
  active.admission.resolve();
  return active.response.promise;
}

export function prepare() {
  previousClient = getWebSocketClient();
  previousLocation = window.location.href;
  clearNavigationBlockerForTests();
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: RequestInfo | URL, init?: RequestInit) => {
      expect(new URL(String(input), window.location.href).pathname).toBe("/api/v1/executors");
      expect(init?.method).toBe("POST");
      expect(init?.cache).toBe("no-store");
      expect(init?.headers).toMatchObject({ "Content-Type": "application/json" });
      const accepted = await admit(JSON.parse(String(init?.body)) as CreatePayload);
      return new Response(JSON.stringify(accepted), {
        status: 200,
        headers: { "Content-Type": "application/json" },
      });
    }),
  );
}

function installClient(transport: Transport) {
  if (transport === "http") {
    setWebSocketClient(null);
    return;
  }
  const client = new WebSocketClient("ws://localhost/independent-executor-create");
  vi.spyOn(client, "request").mockImplementation(async <T,>(action: string, payload: unknown) => {
    expect(action).toBe("executor.create");
    return (await admit(payload as CreatePayload)) as T;
  });
  setWebSocketClient(client);
}

export function mount(transport: Transport): Fixture {
  installClient(transport);
  const type = transport === "http" ? "local_docker" : "remote_docker";
  window.history.replaceState({}, "", `${CREATE_ROUTE}?type=${type}`);
  let capturedStore: StoreApi<AppState> | undefined;
  let capturedOptions: Options = [];
  const fixture: Fixture = {
    transport,
    seed: [
      owner(SURVIVOR, [
        profile("survivor-choice", SURVIVOR),
        profile("obsolete-profile", SURVIVOR),
      ]),
      owner(REMOVED, [profile("removed-choice", REMOVED)]),
    ],
    accepted: {
      id: CREATED,
      name: "Accepted normalized executor",
      type,
      status: "active",
      is_system: false,
      config: { docker_host: "unix:///accepted.sock", normalized: "accepted" },
      created_at: TIME,
      updated_at: "2026-10-10T00:01:00Z",
    },
    response: deferred<Executor>(),
    admission: deferred<void>(),
    settled: false,
    get store() {
      if (!capturedStore) throw new Error("Provider store was not mounted");
      return capturedStore;
    },
    get options() {
      return capturedOptions;
    },
  };
  active = fixture;
  function CurrentChoices() {
    const { t } = useTranslation();
    capturedStore = useAppStoreApi();
    const catalogue = useAppStore((state) => state.executors.items);
    const profiles = catalogue.flatMap((executor) =>
      (executor.profiles ?? []).map((entry) => ({
        ...entry,
        executor_type: entry.executor_type ?? executor.type,
        executor_name: entry.executor_name ?? executor.name,
      })),
    );
    capturedOptions = useExecutorProfileOptions(profiles);
    const [value, setValue] = useState("");
    return (
      <ExecutorProfileSelector
        options={capturedOptions}
        value={value}
        onValueChange={setValue}
        disabled={false}
        placeholder={t("task:executorProfile2")}
      />
    );
  }
  function RoutesAndChoices() {
    return (
      <>
        {renderSettingsRoute(usePathname())}
        <CurrentChoices />
      </>
    );
  }
  render(
    <StateProvider initialState={{ executors: { items: fixture.seed } }}>
      <TooltipProvider>
        <RoutesAndChoices />
      </TooltipProvider>
    </StateProvider>,
  );
  return fixture;
}

export async function start(fixture: Fixture) {
  fireEvent.change(screen.getByLabelText(/executor name/i), { target: { value: SUBMITTED_NAME } });
  if (fixture.transport === "ws") {
    fireEvent.change(screen.getByLabelText(/^docker host$/i), { target: { value: REMOTE_HOST } });
    fireEvent.change(screen.getByLabelText(/tls certificate path/i), {
      target: { value: "/independent/certs" },
    });
    fireEvent.change(screen.getByLabelText(/git token.*optional/i), {
      target: { value: "independent-token" },
    });
  }
  fireEvent.click(screen.getByRole("button", { name: /^create executor$/i }));
  await act(async () => fixture.admission.promise);
  expect(fixture.submitted).toEqual({
    name: SUBMITTED_NAME,
    type: fixture.accepted.type,
    status: "active",
    config:
      fixture.transport === "http"
        ? { docker_host: "unix:///var/run/docker.sock" }
        : {
            docker_host: REMOTE_HOST,
            docker_cert_path: "/independent/certs",
            git_token: "independent-token",
          },
  });
  expect(screen.getByRole("button", { name: /^creating/i }).hasAttribute("disabled")).toBe(true);
  expect(window.location.pathname).toBe(CREATE_ROUTE);
}

export function publishOwner(
  fixture: Fixture,
  action: "executor.created" | "executor.updated" | "executor.deleted",
  payload: ExecutorPayload,
) {
  act(() => {
    const handlers = registerExecutorsHandlers(fixture.store);
    const id = `independent-${action}-${payload.id}`;
    if (action === "executor.created")
      handlers[action]!({ id, type: "notification", action, payload });
    else if (action === "executor.updated")
      handlers[action]!({ id, type: "notification", action, payload });
    else handlers[action]!({ id, type: "notification", action, payload });
  });
}

export function publishProfile(
  fixture: Fixture,
  action: "executor.profile.created" | "executor.profile.updated",
  payload: ExecutorProfilePayload,
) {
  act(() => {
    const handlers = registerExecutorProfileHandlers(fixture.store);
    const id = `independent-${action}-${payload.id}`;
    if (action === "executor.profile.created")
      handlers[action]!({ id, type: "notification", action, payload });
    else handlers[action]!({ id, type: "notification", action, payload });
  });
}

export function removeProfile(fixture: Fixture, id: string) {
  act(() =>
    registerExecutorProfileHandlers(fixture.store)["executor.profile.deleted"]!({
      id: `independent-delete-${id}`,
      type: "notification",
      action: "executor.profile.deleted",
      payload: { id },
    }),
  );
}

export async function settle(fixture: Fixture) {
  await act(async () => {
    fixture.settled = true;
    fixture.response.resolve(fixture.accepted);
    await fixture.response.promise;
  });
  expect(window.location.pathname).toBe(LIST_ROUTE);
  expect(screen.getByRole("heading", { name: /^executors$/i })).toBeTruthy();
  expect(screen.queryByRole("button", { name: /^creating/i })).toBeNull();
}

export async function expectInventory(fixture: Fixture, expected: Executor[]) {
  expect.soft(fixture.store.getState().executors.items).toEqual(expected);
  const profiles = expected.flatMap((executor) =>
    (executor.profiles ?? []).map((entry) => ({
      value: entry.id,
      label: entry.name,
      executorType: entry.executor_type ?? executor.type,
      executorName: entry.executor_name ?? executor.name,
      disabled: false,
    })),
  );
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
    .toEqual(profiles);
  const trigger = screen.getByTestId("executor-profile-selector");
  if (trigger.getAttribute("aria-expanded") !== "true") fireEvent.click(trigger);
  const list = await screen.findByRole("listbox");
  const choices = within(list).queryAllByRole("option");
  expect
    .soft(choices.map((choice) => choice.getAttribute("data-value")))
    .toEqual(profiles.map((entry) => entry.value));
  for (const entry of profiles) {
    const choice = choices.find(
      (candidate) => candidate.getAttribute("data-value") === entry.value,
    );
    expect.soft(choice?.textContent ?? "").toContain(entry.label);
    expect.soft(choice?.textContent ?? "").toContain(entry.executorName);
    expect.soft(choice?.getAttribute("aria-disabled")).toBe("false");
  }
}

export async function finish() {
  if (active?.submitted && !active.settled) await settle(active);
  cleanup();
  active = undefined;
  setWebSocketClient(previousClient);
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", previousLocation);
  vi.restoreAllMocks();
  vi.unstubAllGlobals();
}
