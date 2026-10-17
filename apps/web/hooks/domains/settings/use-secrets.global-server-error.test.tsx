import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import { setOnUnauthorized } from "@/lib/api/client";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { registerSecretsHandlers } from "@/lib/ws/handlers/secrets";
import { useSecrets } from "./use-secrets";

const EMPTY: SecretListItem[] = [];
const timestamp = "2026-10-12T21:00:00Z";
const pending = new Set<() => void>();

function metadata(id: string): SecretListItem {
  return {
    id,
    name: id,
    scope: "global",
    has_value: true,
    created_at: timestamp,
    updated_at: timestamp,
  };
}

function deferredInitialRead() {
  let resolve!: (value: Response) => void;
  let reject!: (error: unknown) => void;
  const read = new Promise<Response>((done, failed) => {
    resolve = done;
    reject = failed;
  });
  const complete = () => resolve(new Response("[]", { status: 200 }));
  pending.add(complete);
  const fetcher = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    expect(new URL(String(input), "http://localhost").pathname).toBe("/api/v1/secrets");
    expect(init?.method ?? "GET").toBe("GET");
    expect(init?.cache).toBe("no-store");
    return read;
  });
  vi.stubGlobal("fetch", fetcher);
  return { resolve, reject, fetcher };
}

function consumers(items: SecretListItem[] = EMPTY, second = false) {
  function wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider initialState={{ secrets: { items, loaded: false, loading: false } }}>
        {children}
      </StateProvider>
    );
  }
  return renderHook(
    ({ first, second }) => ({
      first: useSecrets(first ? "global" : "workspace", undefined, EMPTY),
      second: useSecrets(second ? "global" : "workspace", undefined, EMPTY),
      store: useAppStoreApi(),
    }),
    { wrapper, initialProps: { first: true, second } },
  );
}

function notification<A extends string, T>(action: A, payload: T) {
  return { type: "notification" as const, action, payload, timestamp };
}

function mixedInitialInventory(
  untouched: SecretListItem,
  legacy: SecretListItem,
  workspace: SecretListItem,
) {
  return [
    metadata("callback-edit"),
    metadata("callback-delete"),
    metadata("event-delete"),
    metadata("event-edit"),
    untouched,
    legacy,
    workspace,
  ];
}

async function failRead(
  read: ReturnType<typeof deferredInitialRead>,
  status: number,
  headers?: HeadersInit,
) {
  await act(async () =>
    read.resolve(
      new Response(JSON.stringify({ error: "Synthetic metadata failure" }), { status, headers }),
    ),
  );
}

afterEach(async () => {
  await act(async () => {
    for (const resolve of pending) resolve();
    pending.clear();
  });
  cleanup();
  vi.unstubAllGlobals();
  setOnUnauthorized(null);
});

describe("qualified initial Global server failures", () => {
  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.5
  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.4
  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.4
  it.each([500, 503, 599])(
    "preserves complete mixed inventory for two consumers after initial 5xx (%i)",
    async (status) => {
      const read = deferredInitialRead();
      const edited = {
        ...metadata("callback-edit"),
        name: "Accepted callback rename",
        has_value: false,
        updated_at: "2026-10-12T22:00:00Z",
      };
      const eventEdited = {
        ...metadata("event-edit"),
        name: "Accepted event rename",
        updated_at: "2026-10-12T22:01:00Z",
      };
      const untouched = metadata("untouched");
      const legacy = metadata("legacy");
      delete legacy.scope;
      const workspace = {
        ...metadata("workspace-filtered"),
        scope: "workspace" as const,
        workspace_id: "workspace-a",
      };
      const created = metadata("callback-create");
      const eventCreated = metadata("event-create");
      const view = consumers(mixedInitialInventory(untouched, legacy, workspace), true);
      expect(read.fetcher).toHaveBeenCalledTimes(1);
      const handlers = registerSecretsHandlers(view.result.current.store);
      act(() => {
        view.result.current.first.addSecret(created);
        view.result.current.first.updateSecret(edited);
        view.result.current.first.removeSecret("callback-delete");
        handlers["secrets.created"]!(notification("secrets.created", eventCreated));
        handlers["secrets.updated"]!(notification("secrets.updated", eventEdited));
        handlers["secrets.deleted"]!(notification("secrets.deleted", { id: "event-delete" }));
        handlers["secrets.created"]!(notification("secrets.created", eventCreated));
      });
      const current = view.result.current.store.getState().secrets.items;
      const expected = [edited, eventEdited, untouched, legacy, workspace, created, eventCreated];
      expect(current).toEqual(expected);
      view.rerender({ first: true, second: false });
      view.rerender({ first: true, second: true });
      expect(view.result.current.second).toMatchObject({ loaded: false, loading: true });
      expect(read.fetcher).toHaveBeenCalledTimes(1);
      await failRead(read, status);
      expect(view.result.current.store.getState().secrets).toEqual({
        items: expected,
        loaded: true,
        loading: false,
      });
      expect(view.result.current.store.getState().secrets.items).toBe(current);
      const filtered = [edited, eventEdited, untouched, legacy, created, eventCreated];
      for (const consumer of [view.result.current.first, view.result.current.second]) {
        expect(consumer).toMatchObject({ items: filtered, loaded: true, loading: false });
      }
      view.rerender({ first: false, second: true });
      view.rerender({ first: true, second: true });
      expect(view.result.current.first).toMatchObject({
        items: filtered,
        loaded: true,
        loading: false,
      });
      expect(read.fetcher).toHaveBeenCalledTimes(1);
    },
  );

  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.5
  it("settles an accepted empty inventory after last-row deletion and GET500", async () => {
    const read = deferredInitialRead();
    const view = consumers([metadata("last-row")]);
    act(() => view.result.current.first.removeSecret("last-row"));
    const current = view.result.current.store.getState().secrets.items;
    expect(current).toEqual([]);
    await failRead(read, 500);
    expect(view.result.current.store.getState().secrets.items).toBe(current);
    expect(view.result.current.first).toMatchObject({ items: [], loaded: true, loading: false });
    expect(read.fetcher).toHaveBeenCalledTimes(1);
  });

  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.5
  it("preserves an accepted metadata replacement after GET500", async () => {
    const read = deferredInitialRead();
    const view = consumers();
    const current = [metadata("replacement")];
    act(() => view.result.current.store.getState().setSecrets(current));
    await failRead(read, 500);
    expect(view.result.current.store.getState().secrets.items).toBe(current);
    expect(view.result.current.first).toMatchObject({
      items: current,
      loaded: true,
      loading: false,
    });
    expect(read.fetcher).toHaveBeenCalledTimes(1);
  });
});

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.6
describe("initial failure compatibility", () => {
  it("clears unchanged seeded inventory after uncontested GET500", async () => {
    const read = deferredInitialRead();
    const view = consumers([metadata("unchanged")]);
    await failRead(read, 500);
    expect(view.result.current.first).toMatchObject({ items: [], loaded: true, loading: false });
    expect(view.result.current.store.getState().secrets.items).toEqual([]);
    expect(read.fetcher).toHaveBeenCalledTimes(1);
  });

  it.each([400, 401, 403, 404, 429])(
    "retains 4xx clearing after accepted changes (%i)",
    async (status) => {
      const read = deferredInitialRead();
      const unauthorized = vi.fn();
      setOnUnauthorized(unauthorized);
      const view = consumers();
      act(() => view.result.current.first.addSecret(metadata("accepted")));
      expect(view.result.current.first.items).toEqual([metadata("accepted")]);
      await failRead(read, status);
      expect(view.result.current.store.getState().secrets).toEqual({
        items: [],
        loaded: true,
        loading: false,
      });
      expect(view.result.current.first).toMatchObject({ items: [], loaded: true, loading: false });
      expect(unauthorized).not.toHaveBeenCalled();
      expect(read.fetcher).toHaveBeenCalledTimes(1);
    },
  );

  it("retains challenged 401 notification and clearing", async () => {
    const read = deferredInitialRead();
    const view = consumers();
    const atChallenge: unknown[] = [];
    setOnUnauthorized(() => atChallenge.push(view.result.current.store.getState().secrets.items));
    act(() => view.result.current.first.addSecret(metadata("accepted")));
    await failRead(read, 401, { "WWW-Authenticate": "Bearer realm=kandev" });
    expect(atChallenge).toEqual([[metadata("accepted")]]);
    expect(view.result.current.store.getState().secrets).toEqual({
      items: [],
      loaded: true,
      loading: false,
    });
    expect(read.fetcher).toHaveBeenCalledTimes(1);
  });

  it.each(["network", "abort", "lookalike", "malformed200"] as const)(
    "clears accepted metadata for nonqualified read errors (%s)",
    async (kind) => {
      const read = deferredInitialRead();
      const view = consumers();
      act(() => view.result.current.first.addSecret(metadata("accepted")));
      await act(async () => {
        if (kind === "malformed200") read.resolve(new Response("{invalid-json", { status: 200 }));
        else if (kind === "network") read.reject(new TypeError("Synthetic network failure"));
        else if (kind === "abort") read.reject(new DOMException("Synthetic abort", "AbortError"));
        else read.reject({ name: "ApiError", status: 500 });
      });
      expect(view.result.current.store.getState().secrets).toEqual({
        items: [],
        loaded: true,
        loading: false,
      });
      expect(view.result.current.first).toMatchObject({ items: [], loaded: true, loading: false });
      expect(read.fetcher).toHaveBeenCalledTimes(1);
    },
  );
});
