import { act, cleanup, renderHook } from "@testing-library/react";
import { type PropsWithChildren } from "react";
import { vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useExecutorProfileOptions } from "@/components/task-create-dialog-options";
import type { AppState } from "@/lib/state/store";
import type { StoreApi } from "zustand";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import type { WsHandlers } from "@/lib/ws/handlers/types";
import { registerExecutorsHandlers } from "@/lib/ws/handlers/executors";
import { registerExecutorProfileHandlers } from "@/lib/ws/handlers/executor-profiles";
import { buildSSHExecutorConfig } from "@/app/settings/executors/new/[type]/ssh-config";
import { useSaveExecutorConnection } from "./use-save-executor-connection";

export const OWNER = "connection-owner";
export const OTHER = "connection-other";
const TIME = "2026-10-10T01:00:00Z";
export const form = {
  name: "Submitted host",
  host: "box.lan",
  identity_source: "agent" as const,
  host_fingerprint: "SHA256:submitted",
};
export const submittedConfig = () => buildSSHExecutorConfig(form);

export function profile(id: string, executorId = OWNER): ExecutorProfile {
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
export function executor(id = OWNER, type: Executor["type"] = "ssh"): Executor {
  return {
    id,
    name: id,
    type,
    status: "active",
    is_system: false,
    config: {
      ssh_host: "old.lan",
      ssh_identity_source: "agent",
      ssh_host_fingerprint: "SHA256:old",
      clear: "old",
    },
    profiles: [profile(`${id}-profile`, id)],
    created_at: TIME,
    updated_at: TIME,
  };
}
export function deferred<T>() {
  let resolve!: (value: T) => void;
  let reject!: (error: Error) => void;
  const promise = new Promise<T>((accept, fail) => {
    resolve = accept;
    reject = fail;
  });
  return { promise, resolve, reject };
}
export function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

export type ConnectionFixture = {
  seed: Executor[];
  fresh: Executor[];
  patch: ReturnType<typeof deferred<Response>>;
  list: ReturnType<typeof deferred<Response>>;
  patchAdmitted: ReturnType<typeof deferred<void>>;
  listAdmitted: ReturnType<typeof deferred<void>>;
  payload: unknown;
  calls: string[];
  reads: number;
  completions: Promise<unknown>[];
};

export function transport(seed = [executor(), executor(OTHER)]): ConnectionFixture {
  const patch = deferred<Response>();
  const list = deferred<Response>();
  const patchAdmitted = deferred<void>();
  const listAdmitted = deferred<void>();
  const fresh = seed.map((item) =>
    item.id === OWNER
      ? {
          ...item,
          name: "Normalized host",
          config: { ...submittedConfig(), ssh_host_fingerprint: "SHA256:normalized" },
        }
      : item,
  );
  const fixture = {
    seed,
    fresh,
    patch,
    list,
    patchAdmitted,
    listAdmitted,
    payload: undefined as unknown,
    calls: [] as string[],
    reads: 0,
    completions: [] as Promise<unknown>[],
  };
  transports.push(fixture);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = new URL(String(input), "http://localhost").pathname;
      fixture.calls.push(`${init?.method ?? "GET"} ${path}`);
      if (path === `/api/v1/executors/${OWNER}` && init?.method === "PATCH") {
        fixture.payload = JSON.parse(String(init.body));
        patchAdmitted.resolve();
        return patch.promise;
      }
      if (path === "/api/v1/executors") {
        listAdmitted.resolve();
        return list.promise;
      }
      if (path === `/api/v1/executors/${OWNER}`) {
        const items = fixture.reads++ === 0 ? fixture.seed : fixture.fresh;
        return json(items.find((item) => item.id === OWNER) ?? executor());
      }
      const response = auxiliaryResponse(path);
      if (response) return response;
      throw new Error(`Unexpected connection transport ${init?.method} ${path}`);
    }),
  );
  return fixture;
}
const transports: ConnectionFixture[] = [];

function auxiliaryResponse(path: string) {
  if (path === "/api/v1/ssh/test" || path === "/api/v1/remote-docker/test")
    return json({
      success: true,
      fingerprint: "SHA256:submitted",
      steps: [],
      total_duration_ms: 1,
    });
  if (path.startsWith("/api/v1/ssh/executors/") && path.endsWith("/sessions")) return json([]);
  if (path.startsWith("/api/v1/ssh/executors/") && path.endsWith("/reachability"))
    return json({
      executor_id: OWNER,
      state: "unknown",
      probing_enabled: false,
      probe_interval_seconds: 0,
      checked_at: null,
      last_success_at: null,
      consecutive_failures: 0,
    });
  if (path === "/api/v1/ssh/identities") return json({ identities: [] });
  return null;
}

export function useDestinations() {
  const items = useAppStore((state) => state.executors.items);
  return useExecutorProfileOptions(
    items.flatMap((owner) =>
      (owner.profiles ?? []).map((entry) => ({
        ...entry,
        executor_type: entry.executor_type ?? owner.type,
        executor_name: entry.executor_name ?? owner.name,
      })),
    ),
  );
}
export function Destinations() {
  const options = useDestinations();
  return (
    <div>
      {options.map((option) => (
        <div
          key={option.value}
          data-testid={`destination-${option.value}`}
          data-disabled={option.disabled}
        >
          {option.renderLabel()}
        </div>
      ))}
    </div>
  );
}
export function mountHook(
  fixture: ReturnType<typeof transport>,
  onSaved?: () => void | Promise<void>,
) {
  function Wrapper({ children }: PropsWithChildren) {
    return (
      <StateProvider initialState={{ executors: { items: fixture.seed } }}>
        {children}
      </StateProvider>
    );
  }
  return renderHook(
    () => ({
      store: useAppStoreApi(),
      save: useSaveExecutorConnection(OWNER, submittedConfig, onSaved),
      options: useDestinations(),
    }),
    { wrapper: Wrapper },
  );
}
export function publish(store: StoreApi<AppState>, type: keyof WsHandlers, payload: unknown) {
  const handlers = {
    ...registerExecutorsHandlers(store),
    ...registerExecutorProfileHandlers(store),
  };
  const handler = handlers[type] as ((message: unknown) => void) | undefined;
  if (!handler) throw new Error(`Missing registered handler ${type}`);
  handler({ type, payload });
}
export function createDestination(store: StoreApi<AppState>, id = "new-owner") {
  publish(store, "executor.created", executor(id));
  publish(store, "executor.profile.created", profile(`${id}-profile`, id));
}
export async function beginSave(fixture: ReturnType<typeof transport>, save: () => Promise<void>) {
  let completion!: Promise<void>;
  await act(async () => {
    completion = save();
    fixture.completions.push(completion);
    void completion.catch(() => undefined);
    await fixture.patchAdmitted.promise;
    fixture.patch.resolve(json(executor()));
    await fixture.listAdmitted.promise;
  });
  return { completion };
}
export async function settle(
  fixture: ReturnType<typeof transport>,
  completion: Promise<void>,
  fail = false,
) {
  await act(async () => {
    if (fail) fixture.list.reject(new Error("refresh offline"));
    else fixture.list.resolve(json({ executors: fixture.fresh }));
    await completion;
  });
}
export async function disposeFixtures() {
  await act(async () => {
    for (const fixture of transports) {
      fixture.patch.resolve(json(executor()));
      fixture.list.resolve(json({ executors: fixture.fresh }));
      await Promise.allSettled(fixture.completions);
    }
  });
  cleanup();
  transports.length = 0;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
}
