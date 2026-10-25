import { act, cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { expect, vi } from "vitest";
import type { StoreApi } from "zustand";
import ProfileEditPage from "@/app/settings/executors/[profileId]/page";
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

export const OWNER = "catalogue-owner";
export const TARGET = "edited-profile";
export const PROFILE_ROUTE = `/settings/executors/${TARGET}`;
const ADDED_EXECUTOR = "added-executor";
const OTHER_PROFILE = "other-profile";
const OTHER = "other-executor";
const STAMP = "2026-10-09T00:00:00Z";
const PATH = `/api/v1/executors/${OWNER}/profiles/${TARGET}`;

export function profile(id: string, executorId = OWNER, name = id): ExecutorProfile {
  return {
    id,
    executor_id: executorId,
    name,
    prepare_script: "",
    cleanup_script: "",
    env_vars: [],
    config: {},
    created_at: STAMP,
    updated_at: STAMP,
  };
}

export function executor(id: string, profiles: ExecutorProfile[]): Executor {
  return {
    id,
    name: id,
    type: "local",
    status: "active",
    is_system: false,
    config: {},
    profiles,
    created_at: STAMP,
    updated_at: STAMP,
  };
}

export function initialCatalogue(initialName: string): Executor[] {
  return [
    executor(OWNER, [profile(TARGET, OWNER, initialName), profile("sibling")]),
    executor(OTHER, [profile(OTHER_PROFILE, OTHER)]),
    executor("removed-executor", [profile("removed-profile", "removed-executor")]),
  ];
}

export function addedCatalogue(initialName: string): Executor[] {
  const [owner, ...others] = initialCatalogue(initialName);
  return [
    { ...owner, profiles: [...owner.profiles!, profile("added-sibling")] },
    ...others,
    executor(ADDED_EXECUTOR, [profile("added-profile", ADDED_EXECUTOR)]),
  ];
}

export function updatedCatalogue(initialName: string): Executor[] {
  const [owner, other, removed] = initialCatalogue(initialName);
  return [
    {
      ...owner,
      name: "Current owner",
      status: "connected",
      config: { current: "owner-config" },
      profiles: [
        owner.profiles![0],
        { ...profile("sibling"), name: "Current sibling", config: { current: "sibling-config" } },
      ],
    },
    {
      ...other,
      name: "Current other",
      profiles: [profile(OTHER_PROFILE, other.id, "Current other profile")],
    },
    removed,
  ];
}

export function removedCatalogue(initialName: string): Executor[] {
  const [owner, other] = initialCatalogue(initialName);
  return [{ ...owner, profiles: [owner.profiles![0]] }, other];
}

export function mixedCatalogue(initialName: string): Executor[] {
  const [owner, other] = updatedCatalogue(initialName);
  return [
    { ...owner, profiles: [owner.profiles![0], profile("added-sibling")] },
    other,
    executor(ADDED_EXECUTOR, [profile("added-profile", ADDED_EXECUTOR)]),
  ];
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (reason: Error) => void;
  const promise = new Promise<T>((done, fail) => {
    resolve = done;
    reject = fail;
  });
  return { promise, resolve, reject };
}

type MutationResponse = ExecutorProfile | { success: boolean };
type SaveResult = Awaited<ReturnType<SettingsSaveCoordinator["saveAll"]>>;
type ProfileNames = { initialName: string; savedName: string };
export type Fixture = {
  names: ProfileNames;
  store: StoreApi<AppState>;
  coordinator: SettingsSaveCoordinator;
  options: ReturnType<typeof useExecutorProfileOptions>;
  response: ReturnType<typeof deferred<MutationResponse>>;
  admitted: ReturnType<typeof deferred<void>>;
  completion?: Promise<SaveResult>;
};
const activeFixtures: Fixture[] = [];

export async function mountProfile(
  method: "PATCH" | "DELETE",
  names: ProfileNames,
): Promise<Fixture> {
  const response = deferred<MutationResponse>();
  const admitted = deferred<void>();
  const fixture = { response, admitted, names } as Fixture;
  activeFixtures.push(fixture);
  vi.mocked(fetchJson).mockImplementation(
    async <T,>(url: string, options?: Parameters<typeof fetchJson>[1]) => {
      if (url === PATH && options?.init?.method === method) {
        admitted.resolve();
        return response.promise as Promise<T>;
      }
      if (url === "/api/v1/script-placeholders") return { placeholders: [] } as T;
      if (url === "/api/v1/system/logs/frontend-errors") return undefined as T;
      throw new Error(`Unexpected transport: ${options?.init?.method ?? "GET"} ${url}`);
    },
  );
  window.history.replaceState({}, "", PROFILE_ROUTE);
  function Consumer() {
    fixture.store = useAppStoreApi();
    fixture.coordinator = useSettingsSaveCoordinator();
    const catalogue = useAppStore((state) => state.executors.items);
    // Task-create and subtask consumers use this same owner metadata fallback.
    const profiles = catalogue.flatMap((owner) =>
      (owner.profiles ?? []).map((entry) => ({
        ...entry,
        executor_type: entry.executor_type ?? owner.type,
        executor_name: entry.executor_name ?? owner.name,
      })),
    );
    fixture.options = useExecutorProfileOptions(profiles);
    return <ProfileEditPage profileId={TARGET} />;
  }
  render(
    <StateProvider
      initialState={{
        executors: { items: initialCatalogue(names.initialName) },
        secrets: { items: [], loaded: true, loading: false },
      }}
    >
      <TooltipProvider>
        <ToastProvider>
          <SettingsSaveProvider>
            <Consumer />
          </SettingsSaveProvider>
        </ToastProvider>
      </TooltipProvider>
    </StateProvider>,
  );
  await act(async () => {
    await vi.advanceTimersByTimeAsync(0);
  });
  expect(fixture.coordinator.hasDirty).toBe(false);
  return fixture;
}

export async function startSave(fixture: Fixture) {
  fireEvent.change(screen.getByLabelText("Name"), { target: { value: fixture.names.savedName } });
  expect(fixture.coordinator.hasDirty).toBe(true);
  await act(async () => {
    fixture.completion = fixture.coordinator.saveAll();
    await fixture.admitted.promise;
  });
}

export async function startDelete(fixture: Fixture) {
  fireEvent.click(screen.getByRole("button", { name: "Delete Profile" }));
  fireEvent.click(within(screen.getByRole("dialog")).getByRole("button", { name: /^Delete$/ }));
  await act(async () => {
    await fixture.admitted.promise;
  });
}

export function publish(fixture: Fixture, catalogue: Executor[]) {
  act(() => fixture.store.getState().setExecutors(catalogue));
  expect(fixture.store.getState().executors.items).toEqual(catalogue);
  expect(fixture.options.map((item) => item.value)).toEqual(
    catalogue.flatMap((item) => item.profiles?.map((entry) => entry.id) ?? []),
  );
}

export async function accept(fixture: Fixture, response: MutationResponse) {
  await act(async () => {
    fixture.response.resolve(response);
    await fixture.completion;
    await vi.advanceTimersByTimeAsync(0);
  });
}

export async function reject(fixture: Fixture) {
  await act(async () => {
    fixture.response.reject(new Error("Profile request rejected"));
    await fixture.completion;
    await vi.advanceTimersByTimeAsync(0);
  });
}

export function acceptedProfile(savedName: string): ExecutorProfile {
  return { ...profile(TARGET, OWNER, savedName), updated_at: "2026-10-09T00:01:00Z" };
}

export function expectPreserved(fixture: Fixture, current: Executor[], deleted = false) {
  const expected = current.map((item) =>
    item.id === OWNER
      ? {
          ...item,
          profiles: deleted
            ? item.profiles?.filter((entry) => entry.id !== TARGET)
            : item.profiles?.map((entry) =>
                entry.id === TARGET ? acceptedProfile(fixture.names.savedName) : entry,
              ),
        }
      : item,
  );
  expect.soft(fixture.store.getState().executors.items).toEqual(expected);
  const options = expected.flatMap((item) =>
    (item.profiles ?? []).map((entry) => ({
      value: entry.id,
      label: entry.name,
      executorName: entry.executor_name ?? item.name,
      executorType: entry.executor_type ?? item.type,
      disabled: false,
    })),
  );
  expect
    .soft(
      fixture.options.map(({ value, label, executorName, executorType, disabled }) => ({
        value,
        label,
        executorName,
        executorType,
        disabled,
      })),
    )
    .toEqual(options);
}

export async function cleanFixtures() {
  await act(async () => {
    for (const fixture of activeFixtures.splice(0)) {
      fixture.response.resolve(acceptedProfile(fixture.names.savedName));
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
