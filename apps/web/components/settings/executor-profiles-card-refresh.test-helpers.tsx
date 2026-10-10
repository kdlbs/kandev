import { act, cleanup, fireEvent, render, waitFor, within } from "@testing-library/react";
import { vi } from "vitest";
import { StateProvider, useAppStore, useAppStoreApi } from "@/components/state-provider";
import { useExecutorProfileOptions } from "@/components/task-create-dialog-options";
import { registerExecutorsHandlers } from "@/lib/ws/handlers/executors";
import { registerExecutorProfileHandlers } from "@/lib/ws/handlers/executor-profiles";
import type { BackendMessage, BackendMessageMap } from "@/lib/types/backend";
import type { Executor, ExecutorProfile } from "@/lib/types/http";
import { ExecutorProfilesCard } from "./executor-profiles-card";

export const TARGET = "refresh-target";
export const SIBLING = "refresh-sibling";
const TIME = "2026-10-10T02:00:00Z";

export function profile(id: string, executorId = TARGET): ExecutorProfile {
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
export function executor(id = TARGET): Executor {
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
export function json(value: unknown, status = 200) {
  return new Response(JSON.stringify(value), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}
function deferred() {
  let resolve!: (response: Response) => void;
  const promise = new Promise<Response>((accept) => {
    resolve = accept;
  });
  return { promise, resolve, settled: false };
}

type Read = ReturnType<typeof deferred> & { executorId: string };
export type Transport = {
  reads: Read[];
  deletes: string[];
  posts: unknown[];
  deleteFailure: boolean;
  holdDelete: ReturnType<typeof deferred> | null;
};
const fixtures: Transport[] = [];
const initialLocation = window.location.href;

export function transport(): Transport {
  const fixture: Transport = {
    reads: [],
    deletes: [],
    posts: [],
    deleteFailure: false,
    holdDelete: null,
  };
  fixtures.push(fixture);
  vi.stubGlobal(
    "fetch",
    vi.fn(async (input: string | URL | Request, init?: RequestInit) => {
      const path = new URL(String(input), "http://localhost").pathname;
      const match = /^\/api\/v1\/executors\/([^/]+)\/profiles(?:\/([^/]+))?$/.exec(path);
      if (!match) throw new Error(`Unexpected refresh transport: ${init?.method ?? "GET"} ${path}`);
      if (init?.method === "DELETE") {
        fixture.deletes.push(match[2]);
        if (fixture.holdDelete) return fixture.holdDelete.promise;
        return json(
          fixture.deleteFailure ? { error: "Delete rejected" } : { success: true },
          fixture.deleteFailure ? 500 : 200,
        );
      }
      if (init?.method === "POST") {
        fixture.posts.push(JSON.parse(String(init.body)));
        return json(profile("created-via-dialog", match[1]));
      }
      if (init?.cache !== "no-store") throw new Error("Expected uncached profile read");
      const read = { ...deferred(), executorId: match[1] };
      fixture.reads.push(read);
      return read.promise;
    }),
  );
  return fixture;
}

function Inventory() {
  const items = useAppStore((state) => state.executors.items);
  const options = useExecutorProfileOptions(
    items.flatMap((owner) =>
      (owner.profiles ?? []).map((entry) => ({
        ...entry,
        executor_type: entry.executor_type ?? owner.type,
        executor_name: entry.executor_name ?? owner.name,
      })),
    ),
  );
  return (
    <div data-testid="refresh-inventory">
      {options.map((option) => (
        <div
          key={option.value}
          data-testid={`choice-${option.value}`}
          data-disabled={option.disabled}
          data-executor-type={option.executorType}
        >
          {option.renderLabel()}
        </div>
      ))}
    </div>
  );
}

export function mountCard(seed = [executor(), executor(SIBLING)], targetId = TARGET) {
  const capture = {} as { store: ReturnType<typeof useAppStoreApi> };
  function Surface() {
    capture.store = useAppStoreApi();
    const owner = useAppStore((state) =>
      state.executors.items.find((item) => item.id === targetId),
    );
    return (
      <>
        <ExecutorProfilesCard executorId={targetId} profiles={owner?.profiles ?? []} />
        <Inventory />
      </>
    );
  }
  const view = render(
    <StateProvider initialState={{ executors: { items: seed } }}>
      <Surface />
    </StateProvider>,
  );
  return { ...capture, view };
}
export type Mounted = ReturnType<typeof mountCard>;

export function publish<K extends keyof BackendMessageMap>(
  store: Mounted["store"],
  type: K,
  payload: BackendMessageMap[K]["payload"],
) {
  const handlers = {
    ...registerExecutorsHandlers(store),
    ...registerExecutorProfileHandlers(store),
  };
  const handler = handlers[type] as
    | ((message: BackendMessage<K, BackendMessageMap[K]["payload"]>) => void)
    | undefined;
  if (!handler) throw new Error(`Missing registered handler ${type}`);
  act(() => handler({ type: "notification", action: type, payload }));
}
export function createOwner(store: Mounted["store"], id: string) {
  publish(store, "executor.created", executor(id));
  publish(store, "executor.profile.created", profile(`${id}-profile`, id));
}
export async function startDelete(
  mount: Mounted,
  fixture: Transport,
  profileId = `${TARGET}-profile`,
  count = fixture.reads.length + 1,
) {
  fireEvent.click(
    within(mount.view.getByTestId(`executor-profile-card-${profileId}`)).getByTestId(
      "executor-profile-delete-button",
    ),
  );
  await waitFor(() => {
    if (fixture.reads.length !== count) throw new Error("Profile GET not admitted yet");
  });
  return fixture.reads[count - 1];
}
export async function releaseRead(read: Read, profiles: ExecutorProfile[], fail = false) {
  await act(async () => {
    read.settled = true;
    read.resolve(fail ? json({ error: "Read rejected" }, 500) : json({ profiles }));
    await read.promise;
  });
}
export function holdDeletion(fixture: Transport) {
  fixture.holdDelete = deferred();
  return fixture.holdDelete;
}
export async function disposeFixtures() {
  await act(async () => {
    for (const fixture of fixtures) {
      fixture.holdDelete?.resolve(json({ error: "Cleanup deletion" }, 500));
      for (const read of fixture.reads) {
        if (!read.settled) read.resolve(json({ error: "Cleanup read" }, 500));
        await read.promise;
      }
    }
  });
  cleanup();
  window.history.replaceState({}, "", initialLocation);
  fixtures.length = 0;
  vi.unstubAllGlobals();
  vi.restoreAllMocks();
}
