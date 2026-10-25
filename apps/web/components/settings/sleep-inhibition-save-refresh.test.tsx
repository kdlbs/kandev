import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { useLayoutEffect } from "react";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { ApiRequestOptions } from "@/lib/api/client";
import type { AppState } from "@/lib/state/store";
import type {
  MessageQueueSettingsResponse,
  SessionCapacitySettingsResponse,
  SleepInhibitionResponse,
} from "@/lib/types/system";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import { SleepInhibitionSettings } from "./sleep-inhibition-settings";
import { TaskBehaviorSettings } from "./task-behavior-settings";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "./settings-save-provider";

const transport = vi.hoisted(() => vi.fn());
vi.mock("@/lib/api/client", async (importOriginal) => ({
  ...(await importOriginal<typeof import("@/lib/api/client")>()),
  fetchJson: transport,
  fetchJsonWithRetry: transport,
}));

type Flight = {
  method: string;
  options?: ApiRequestOptions;
  resolve: (value: SleepInhibitionResponse) => void;
  reject: (error: Error) => void;
  settled: boolean;
};
const flights: Flight[] = [];
let store: StoreApi<AppState>;
let coordinator: SettingsSaveCoordinator;
let priorLocation: string;
const sleepId = "general-task-sleep-inhibition";
const queue: MessageQueueSettingsResponse = {
  settings: { max_per_session: 0, merge_enabled: true, auto_merge_enabled: true },
  effective: {
    max_per_session: 0,
    merge_enabled: true,
    auto_merge_enabled: true,
    source: "default",
    locked: false,
  },
};
const capacity: SessionCapacitySettingsResponse = {
  settings: { enabled: false, max_sessions: 2 },
  effective: { enabled: false, max_sessions: 2, source: "default", locked: false },
};
function snapshot(enabled = false, active = false): SleepInhibitionResponse {
  return { settings: { enabled }, status: { platform: "linux", supported: true, active } };
}
function StoreCapture() {
  const current = useAppStoreApi();
  useLayoutEffect(() => {
    store = current;
  }, [current]);
  return null;
}
function CoordinatorCapture() {
  const current = useSettingsSaveCoordinator();
  useLayoutEffect(() => {
    coordinator = current;
  });
  return null;
}
function Frame({
  open = true,
  runtime = false,
  member = false,
}: {
  open?: boolean;
  runtime?: boolean;
  member?: boolean;
}) {
  return (
    <StateProvider
      initialState={{
        auth: {
          mode: "enabled",
          authenticated: true,
          user: {
            id: "sleep-admin",
            email: "sleep@example.test",
            display_name: "Sleep settings",
            role: member ? "member" : "admin",
            status: "active",
          },
          ssoProviders: [],
        },
      }}
    >
      <StoreCapture />
      {open && (
        <SettingsSaveProvider>
          <CoordinatorCapture />
          {runtime ? <TaskBehaviorSettings /> : <SleepInhibitionSettings />}
        </SettingsSaveProvider>
      )}
    </StateProvider>
  );
}
async function settle(index: number, outcome: SleepInhibitionResponse | Error) {
  const flight = flights[index];
  if (!flight) throw new Error(`No admitted sleep transport ${index}`);
  await act(async () => {
    flight.settled = true;
    if (outcome instanceof Error) flight.reject(outcome);
    else flight.resolve(outcome);
  });
}
function beginSave() {
  let result!: ReturnType<SettingsSaveCoordinator["saveAll"]>;
  act(() => {
    result = coordinator.saveAll();
  });
  return result;
}
function visibilityRefresh() {
  act(() => document.dispatchEvent(new Event("visibilitychange")));
}
function toggle() {
  return screen.getByTestId("sleep-inhibition-switch");
}
function expectDraft(checked: boolean, dirty: boolean) {
  expect(toggle().getAttribute("data-state")).toBe(checked ? "checked" : "unchecked");
  expect(toggle().getAttribute("data-settings-dirty")).toBe(String(dirty));
  expect(coordinator.contributorStates.find((entry) => entry.id === sleepId)?.isDirty).toBe(dirty);
}
function runtimeAttention() {
  return within(screen.getByRole("tab", { name: "Runtime" })).queryByText("Needs attention", {
    selector: '[role="status"]',
  });
}

beforeEach(() => {
  flights.length = 0;
  transport.mockReset();
  clearNavigationBlockerForTests();
  priorLocation = window.location.href;
  window.history.replaceState({}, "", "/settings/preferences/task-behavior?tab=runtime");
  vi.spyOn(document, "visibilityState", "get").mockReturnValue("visible");
  transport.mockImplementation((url: string, options?: ApiRequestOptions) => {
    if (url === "/api/v1/system/message-queue/settings") return Promise.resolve(queue);
    if (url === "/api/v1/system/session-capacity/settings") return Promise.resolve(capacity);
    if (url !== "/api/v1/system/sleep-inhibition")
      throw new Error(`Unexpected actual consumer transport: ${url}`);
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
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", priorLocation);
  vi.restoreAllMocks();
  vi.useRealTimers();
});

// @covers AC-PLATFORM-TASK-SLEEP-INHIBITION-001.9
// @covers AC-PLATFORM-TASK-SLEEP-INHIBITION-001.10
describe("real sleep card, saveAll and Runtime refresh ordering", () => {
  it.each(["success", "failure"])(
    "acknowledged save survives old GET %s in actual Runtime consumer",
    async (outcome) => {
      render(<Frame runtime />);
      await settle(0, snapshot());
      expect(runtimeAttention()).toBeNull();
      fireEvent.click(toggle());
      expectDraft(true, true);
      const saved = beginSave();
      expect(flights[1]).toMatchObject({ method: "PATCH" });
      expect(flights[1].options?.init?.body).toBe('{"enabled":true}');
      visibilityRefresh();
      expect(flights.map((flight) => flight.method)).toEqual(["GET", "PATCH", "GET"]);
      const acknowledged = snapshot(true, true);
      await settle(1, acknowledged);
      expect(await saved).toMatchObject({ canLeave: true, failedIds: new Set() });
      await settle(2, outcome === "success" ? snapshot(false) : new Error("late status failure"));
      expect(store.getState().sleepInhibition).toMatchObject({
        response: acknowledged,
        loading: false,
        error: false,
      });
      expectDraft(true, false);
      expect(coordinator.status).toBe("saved");
      expect(runtimeAttention()).toBeNull();
    },
  );

  it("accepts the opposite completion order without replacing the dirty draft", async () => {
    render(<Frame />);
    await settle(0, snapshot());
    fireEvent.click(toggle());
    const saved = beginSave();
    visibilityRefresh();
    await settle(2, snapshot(false, true));
    expectDraft(true, true);
    expect(store.getState().sleepInhibition.response).toEqual(snapshot(false, true));
    await settle(1, snapshot(true));
    expect((await saved).canLeave).toBe(true);
    expectDraft(true, false);
  });

  it("keeps save without competing GET and newer post-ACK refresh controls working", async () => {
    render(<Frame />);
    await settle(0, snapshot());
    fireEvent.click(toggle());
    const saved = beginSave();
    await settle(1, snapshot(true));
    expect((await saved).canLeave).toBe(true);
    expectDraft(true, false);
    visibilityRefresh();
    await settle(2, snapshot(true, true));
    expect(store.getState().sleepInhibition.response).toEqual(snapshot(true, true));
    expectDraft(true, false);
  });
});

describe("real card draft and failed-save controls", () => {
  it("preserves an edit made while an admitted save is pending", async () => {
    render(<Frame />);
    await settle(0, snapshot());
    fireEvent.click(toggle());
    const saved = beginSave();
    // Saving itself does not disable the card; a separate loading GET would.
    expect(toggle()).toHaveProperty("disabled", false);
    fireEvent.click(toggle());
    await settle(1, snapshot(true));
    expectDraft(false, true);
    expect((await saved).canLeave).toBe(false);
    expect(coordinator.status).toBe("dirty");
    expect(store.getState().sleepInhibition.response).toEqual(snapshot(true));
  });

  it("retains the existing boolean revision when an in-save edit returns to submitted value", async () => {
    render(<Frame />);
    await settle(0, snapshot());
    fireEvent.click(toggle());
    const saved = beginSave();
    fireEvent.click(toggle());
    fireEvent.click(toggle());
    await settle(1, snapshot(true));
    expect((await saved).canLeave).toBe(true);
    expectDraft(true, false);
  });

  it("keeps a failed save dirty while its useful status GET completes", async () => {
    render(<Frame runtime />);
    await settle(0, snapshot());
    fireEvent.click(toggle());
    const saved = beginSave();
    visibilityRefresh();
    await settle(1, new Error("save denied"));
    expect(await saved).toMatchObject({ canLeave: false, failedIds: new Set([sleepId]) });
    expect(coordinator.status).toBe("error");
    expectDraft(true, true);
    expect(runtimeAttention()).not.toBeNull();
    await settle(2, snapshot(false, true));
    expect(store.getState().sleepInhibition).toMatchObject({
      response: snapshot(false, true),
      loading: false,
      error: false,
    });
    expectDraft(true, true);
    expect(runtimeAttention()).not.toBeNull();
    const retry = beginSave();
    await settle(3, snapshot(true));
    expect((await retry).canLeave).toBe(true);
    expectDraft(true, false);
    expect(runtimeAttention()).toBeNull();
  });
});

describe("real Runtime current failure and polling controls", () => {
  it("reports actual initial failure and retry, then current cached status failure", async () => {
    render(<Frame runtime />);
    await settle(0, new Error("initial load failed"));
    expect(runtimeAttention()).not.toBeNull();
    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    await settle(1, snapshot());
    expect(runtimeAttention()).toBeNull();
    visibilityRefresh();
    await settle(2, new Error("current status failed"));
    expect(store.getState().sleepInhibition.error).toBe(true);
    expect(runtimeAttention()).not.toBeNull();
    visibilityRefresh();
    await settle(3, snapshot(false, true));
    expect(runtimeAttention()).toBeNull();
    expectDraft(false, false);
  });

  it("retries a cached load error immediately after the card reopens", async () => {
    const view = render(<Frame />);
    await settle(0, new Error("initial status failure"));
    view.rerender(<Frame open={false} />);
    view.rerender(<Frame />);
    fireEvent.click(screen.getByRole("button", { name: /retry/i }));
    expect(flights).toHaveLength(2);
    await settle(1, snapshot());
    expectDraft(false, false);
    expect(store.getState().sleepInhibition.error).toBe(false);
  });

  it("preserves an unsaved draft through visible polling and member read-only admission", async () => {
    vi.useFakeTimers();
    const view = render(<Frame />);
    await settle(0, snapshot());
    fireEvent.click(toggle());
    act(() => vi.advanceTimersByTime(15_000));
    expect(flights).toHaveLength(2);
    await settle(1, snapshot(false, true));
    expectDraft(true, true);
    view.unmount();
    render(<Frame member />);
    await settle(2, snapshot(true));
    expect(toggle()).toHaveProperty("disabled", true);
    fireEvent.click(toggle());
    expect((await beginSave()).canLeave).toBe(true);
    expect(flights.map((flight) => flight.method)).toEqual(["GET", "GET", "GET"]);
  });
});

// @covers AC-PLATFORM-TASK-SLEEP-INHIBITION-001.11
describe("real card close and remount on its surviving app store", () => {
  it.each(["success", "failure"])(
    "retired GET %s/finally cannot clear reopened pending status",
    async (outcome) => {
      const view = render(<Frame />);
      await settle(0, snapshot());
      visibilityRefresh();
      expect(flights).toHaveLength(2);
      view.rerender(<Frame open={false} />);
      view.rerender(<Frame />);
      visibilityRefresh();
      expect(flights).toHaveLength(3);
      await settle(1, outcome === "success" ? snapshot(true) : new Error("closed card"));
      expect(store.getState().sleepInhibition).toMatchObject({
        response: snapshot(),
        loading: true,
        error: false,
      });
      await settle(2, snapshot(false, true));
      expectDraft(false, false);
      expect(store.getState().sleepInhibition.loading).toBe(false);
    },
  );

  it("keeps an admitted save real after close and blocks the reopened pre-ACK read", async () => {
    const view = render(<Frame />);
    await settle(0, snapshot());
    fireEvent.click(toggle());
    const saved = beginSave();
    view.rerender(<Frame open={false} />);
    view.rerender(<Frame />);
    visibilityRefresh();
    expect(flights.map((flight) => flight.method)).toEqual(["GET", "PATCH", "GET"]);
    await settle(1, snapshot(true));
    expect((await saved).canLeave).toBe(true);
    await settle(2, snapshot(false));
    expect(store.getState().sleepInhibition.response).toEqual(snapshot(true));
    expectDraft(true, false);
  });
});
