import { act, cleanup, fireEvent, render, screen } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { expect, vi } from "vitest";
import type { StoreApi } from "zustand";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { ToastProvider } from "@/components/toast-provider";
import { useExecutorProfileOptions } from "@/components/task-create-dialog-options";
import {
  SettingsSaveProvider,
  useSettingsSaveCoordinator,
  type SettingsSaveCoordinator,
} from "@/components/settings/settings-save-provider";
import { fetchJson } from "@/lib/api/client";
import { clearNavigationBlockerForTests } from "@/lib/routing/navigation-guard";
import type { AppState } from "@/lib/state/store";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { registerExecutorProfileHandlers } from "@/lib/ws/handlers/executor-profiles";
import CreateProfilePage from "./page";

export const OWNER = "exec-worktree";
export const OTHER = "exec-local";
export const TARGET = "new-worktree-profile";
export const ROUTE = "/settings/executors/new/worktree";
export const CONTRIBUTOR = `executor-profile:new:${OWNER}`;
// i18n-exempt: Synthetic profile draft used only by component tests, never application copy.
export const DRAFT = " New worktree profile ";
const STAMP = "2026-10-09T03:00:00Z";
const PREPARE = "echo prepare-worktree";
const CREATE_PATH = `/api/v1/executors/${OWNER}/profiles`;

export function profile(id: string, owner = OWNER, name = id): ExecutorProfile {
  return {
    id,
    executor_id: owner,
    name,
    config: {},
    env_vars: [],
    prepare_script: "",
    cleanup_script: "",
    created_at: STAMP,
    updated_at: STAMP,
  };
}

export function executor(id: string, profiles: ExecutorProfile[]): Executor {
  return {
    id,
    name: id,
    type: id === OWNER ? "worktree" : "local",
    is_system: true,
    status: "active",
    config: {},
    profiles,
    created_at: STAMP,
    updated_at: STAMP,
  };
}

export function initialCatalogue(): Executor[] {
  return [
    executor(OWNER, [profile("worktree-keep"), profile("worktree-remove")]),
    executor(OTHER, [profile("local-keep", OTHER), profile("local-remove", OTHER)]),
    executor("removed-owner", [profile("removed-owner-profile", "removed-owner")]),
  ];
}

export const ACCEPTED: ExecutorProfile = {
  ...profile(TARGET, OWNER, DRAFT.trim()),
  prepare_script: PREPARE,
  updated_at: "2026-10-09T03:01:00Z",
};

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

type SaveResult = Awaited<ReturnType<SettingsSaveCoordinator["saveAll"]>>;
export type Fixture = {
  store: StoreApi<AppState>;
  coordinator: SettingsSaveCoordinator;
  options: ReturnType<typeof useExecutorProfileOptions>;
  response: ReturnType<typeof deferred<ExecutorProfile>>;
  admitted: ReturnType<typeof deferred<void>>;
  payload?: unknown;
  completion?: Promise<SaveResult>;
  result?: SaveResult;
};
const fixtures: Fixture[] = [];

function Consumer({ fixture }: { fixture: Fixture }) {
  fixture.store = useAppStoreApi();
  fixture.coordinator = useSettingsSaveCoordinator();
  const catalogue = useAppStore((state) => state.executors.items);
  // Task-create and subtask consumers supply the same owning metadata fallback.
  const profiles = catalogue.flatMap((owner) =>
    (owner.profiles ?? []).map((entry) => ({
      ...entry,
      executor_type: entry.executor_type ?? owner.type,
      executor_name: entry.executor_name ?? owner.name,
    })),
  );
  fixture.options = useExecutorProfileOptions(profiles);
  return <CreateProfilePage executorType="worktree" />;
}

export async function mountCreation(): Promise<Fixture> {
  const fixture = {
    response: deferred<ExecutorProfile>(),
    admitted: deferred<void>(),
  } as Fixture;
  fixtures.push(fixture);
  vi.mocked(fetchJson).mockImplementation(
    async <T,>(url: string, options?: Parameters<typeof fetchJson>[1]) => {
      if (url === CREATE_PATH && options?.init?.method === "POST") {
        if (typeof options.init.body !== "string") throw new Error("Expected JSON POST body");
        fixture.payload = JSON.parse(options.init.body);
        fixture.admitted.resolve();
        return fixture.response.promise as Promise<T>;
      }
      if (url === "/api/v1/script-placeholders") return { placeholders: [] } as T;
      if (url === "/api/v1/executor-profiles/default-script?type=worktree") {
        return { prepare_script: PREPARE, cleanup_script: "" } as T;
      }
      if (url === "/api/v1/system/logs/frontend-errors") return undefined as T;
      throw new Error(`Unexpected external request: ${url}`);
    },
  );
  window.history.replaceState({}, "", ROUTE);
  render(
    <StateProvider
      initialState={{
        executors: { items: initialCatalogue() },
        secrets: { items: [], loaded: true, loading: false },
      }}
    >
      <TooltipProvider>
        <ToastProvider>
          <SettingsSaveProvider>
            <Consumer fixture={fixture} />
          </SettingsSaveProvider>
        </ToastProvider>
      </TooltipProvider>
    </StateProvider>,
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(fixture.coordinator.invalidReason).toBe("Enter a profile name.");
  return fixture;
}

export async function startCreation(fixture: Fixture) {
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: DRAFT } });
  expect(fixture.coordinator.hasDirty).toBe(true);
  expect(fixture.coordinator.contributorStates).toEqual([
    { id: CONTRIBUTOR, isDirty: true, invalid: false, saveFailed: false },
  ]);
  await act(async () => {
    fixture.completion = fixture.coordinator.saveAll();
    await fixture.admitted.promise;
  });
  expect(screen.getByLabelText("Name").closest("fieldset")?.hasAttribute("disabled")).toBe(true);
  expect(fixture.payload).toEqual({
    name: DRAFT.trim(),
    prepare_script: PREPARE,
    cleanup_script: "",
    env_vars: [],
  });
}

export function profileEvent(
  fixture: Fixture,
  action: "executor.profile.created" | "executor.profile.updated",
  entry: ExecutorProfile,
) {
  act(() => {
    const handlers = registerExecutorProfileHandlers(fixture.store);
    const envelope = {
      id: `creation-${entry.id}`,
      type: "notification" as const,
      payload: entry,
      timestamp: STAMP,
    };
    if (action === "executor.profile.created") {
      handlers["executor.profile.created"]!({ ...envelope, action });
    } else {
      handlers["executor.profile.updated"]!({ ...envelope, action });
    }
  });
}

export function deleted(fixture: Fixture, id: string) {
  act(() => {
    registerExecutorProfileHandlers(fixture.store)["executor.profile.deleted"]!({
      id: `deletion-${id}`,
      type: "notification",
      action: "executor.profile.deleted",
      payload: { id },
      timestamp: STAMP,
    });
  });
}

export function publish(fixture: Fixture, catalogue: Executor[]) {
  act(() => fixture.store.getState().setExecutors(catalogue));
}

export async function accept(fixture: Fixture) {
  await act(async () => {
    fixture.response.resolve(ACCEPTED);
    fixture.result = await fixture.completion;
    await vi.advanceTimersByTimeAsync(0);
  });
}

export async function reject(fixture: Fixture) {
  await act(async () => {
    fixture.response.reject(new Error("Creation was rejected"));
    fixture.result = await fixture.completion;
    await vi.advanceTimersByTimeAsync(0);
  });
}

export function expectCatalogue(fixture: Fixture, current: Executor[], accepted = true) {
  const expected = current.map((owner) =>
    accepted && owner.id === OWNER
      ? { ...owner, profiles: [...(owner.profiles ?? []).filter((p) => p.id !== TARGET), ACCEPTED] }
      : owner,
  );
  expect.soft(fixture.store.getState().executors.items).toEqual(expected);
  const options = expected.flatMap((owner) =>
    (owner.profiles ?? []).map((entry) => ({
      value: entry.id,
      label: entry.name,
      executorType: entry.executor_type ?? owner.type,
      executorName: entry.executor_name ?? owner.name,
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
    .toEqual(options);
}

export function expectSuccess(fixture: Fixture) {
  expect(fixture.result).toEqual({ canLeave: true, failedIds: new Set() });
  expect(fixture.coordinator.status).toBe("saved");
  expect(window.location.pathname).toBe(`/settings/executors/${TARGET}`);
  expect(screen.getByLabelText("Name").closest("fieldset")?.hasAttribute("disabled")).toBe(false);
}

export function expectRejection(fixture: Fixture) {
  expect(fixture.result).toEqual({ canLeave: false, failedIds: new Set([CONTRIBUTOR]) });
  expect(fixture.coordinator.status).toBe("error");
  expect(fixture.coordinator.errorKind).toBe("save");
  expect(fixture.coordinator.hasDirty).toBe(true);
  expect(fixture.coordinator.contributorStates).toEqual([
    { id: CONTRIBUTOR, isDirty: true, invalid: false, saveFailed: true },
  ]);
  expect((screen.getByLabelText("Name") as HTMLInputElement).value).toBe(DRAFT);
  expect(screen.getByLabelText("Name").closest("fieldset")?.hasAttribute("disabled")).toBe(false);
  expect(screen.getByText("Creation was rejected")).toBeTruthy();
  expect(window.location.pathname).toBe(ROUTE);
}

export async function cleanFixtures() {
  await act(async () => {
    for (const fixture of fixtures.splice(0)) {
      fixture.response.resolve(ACCEPTED);
      await fixture.completion;
    }
  });
  cleanup();
  await act(async () => {
    await vi.runOnlyPendingTimersAsync();
  });
  vi.clearAllTimers();
  vi.useRealTimers();
  clearNavigationBlockerForTests();
  window.history.replaceState({}, "", "/");
  vi.mocked(fetchJson).mockReset();
}
