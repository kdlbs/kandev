import { act, cleanup, renderHook } from "@testing-library/react";
import type { ReactNode } from "react";
import { afterEach, describe, expect, it, vi } from "vitest";
import { StateProvider, useAppStoreApi } from "@/components/state-provider";
import type { SecretListItem } from "@/lib/types/http-secrets";
import { registerSecretsHandlers } from "@/lib/ws/handlers/secrets";
import { useSecrets } from "./use-secrets";

const EMPTY: SecretListItem[] = [];
const TIMESTAMP = "2026-10-10T00:00:00Z";
const pending: Array<() => void> = [];

function metadata(id: string): SecretListItem {
  return {
    id,
    name: id,
    scope: "global",
    has_value: true,
    created_at: TIMESTAMP,
    updated_at: TIMESTAMP,
  };
}

function response(items: SecretListItem[], status = 200) {
  return new Response(JSON.stringify(items), {
    status,
    headers: { "Content-Type": "application/json" },
  });
}

function heldRead() {
  let resolve!: (value: Response) => void;
  const promise = new Promise<Response>((done) => {
    resolve = done;
  });
  pending.push(() => resolve(response([])));
  const fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    expect(new URL(String(input), "http://localhost").pathname).toBe("/api/v1/secrets");
    expect(init?.method ?? "GET").toBe("GET");
    expect(init?.cache).toBe("no-store");
    return promise;
  });
  vi.stubGlobal("fetch", fetch);
  return { fetch, resolve };
}

function renderSecrets(items = EMPTY, second = false) {
  function wrapper({ children }: { children: ReactNode }) {
    return (
      <StateProvider initialState={{ secrets: { items, loaded: false, loading: false } }}>
        {children}
      </StateProvider>
    );
  }
  return renderHook(
    ({ second, first }) => ({
      first: useSecrets(first ? "global" : "workspace", undefined, EMPTY),
      second: useSecrets(second ? "global" : "workspace", undefined, EMPTY),
      store: useAppStoreApi(),
    }),
    { wrapper, initialProps: { second, first: true } },
  );
}

function event<A extends string, T>(action: A, payload: T) {
  return {
    id: "metadata-event",
    type: "notification" as const,
    action,
    payload,
    timestamp: TIMESTAMP,
  };
}

async function settle(read: ReturnType<typeof heldRead>, items: SecretListItem[], status = 200) {
  await act(async () => {
    read.resolve(response(items, status));
  });
}

afterEach(async () => {
  await act(async () => {
    pending.splice(0).forEach((resolve) => resolve());
  });
  cleanup();
  vi.unstubAllGlobals();
});

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.2
describe("accepted global metadata publication", () => {
  it("preserves the whole current list after mixed accepted metadata changes", async () => {
    const read = heldRead();
    const original = metadata("original");
    const removed = metadata("removed");
    const untouched = metadata("untouched");
    const created = metadata("created");
    const updated = {
      ...original,
      name: "Accepted rename",
      has_value: false,
      updated_at: "2026-10-10T01:00:00Z",
    };
    const view = renderSecrets([original, removed, untouched]);
    const handlers = registerSecretsHandlers(view.result.current.store);
    act(() => {
      handlers["secrets.created"]!(event("secrets.created", created));
      handlers["secrets.updated"]!(event("secrets.updated", updated));
      handlers["secrets.deleted"]!(event("secrets.deleted", { id: removed.id }));
      handlers["secrets.created"]!(event("secrets.created", created));
    });
    const accepted = view.result.current.store.getState().secrets.items;
    expect(accepted).toEqual([updated, untouched, created]);
    await settle(read, [original, removed, untouched, metadata("snapshot-only")]);
    expect(view.result.current.store.getState().secrets.items).toEqual([
      updated,
      untouched,
      created,
    ]);
    expect(view.result.current.store.getState().secrets.items).toBe(accepted);
    expect(view.result.current.first).toMatchObject({
      items: accepted,
      loaded: true,
      loading: false,
    });
    expect(read.fetch).toHaveBeenCalledTimes(1);
  });

  it("keeps an accepted empty list after removal during initial GET", async () => {
    const read = heldRead();
    const item = metadata("removed");
    const view = renderSecrets([item]);
    act(() => {
      registerSecretsHandlers(view.result.current.store)["secrets.deleted"]!(
        event("secrets.deleted", { id: item.id }),
      );
    });
    expect(view.result.current.first.items).toEqual([]);
    await settle(read, [item]);
    expect(view.result.current.first).toMatchObject({ items: [], loaded: true, loading: false });
  });

  it("preserves a newer authoritative metadata replacement", async () => {
    const read = heldRead();
    const view = renderSecrets();
    const accepted = [metadata("replacement")];
    act(() => {
      view.result.current.store.getState().setSecrets(accepted);
    });
    await settle(read, [metadata("obsolete")]);
    expect(view.result.current.store.getState().secrets.items).toBe(accepted);
    expect(view.result.current.first).toMatchObject({
      items: accepted,
      loaded: true,
      loading: false,
    });
  });

  // @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.4
  it("settles superseded success without losing current metadata or refetching", async () => {
    const read = heldRead();
    const view = renderSecrets();
    const accepted = metadata("accepted");
    act(() => {
      registerSecretsHandlers(view.result.current.store)["secrets.created"]!(
        event("secrets.created", accepted),
      );
    });
    view.rerender({ first: false, second: true });
    expect(view.result.current.second).toMatchObject({
      items: [accepted],
      loaded: false,
      loading: true,
    });
    await settle(read, []);
    expect(view.result.current.second).toMatchObject({
      items: [accepted],
      loaded: true,
      loading: false,
    });
    expect(read.fetch).toHaveBeenCalledTimes(1);
  });
});

// @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.3
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-002.4
// @covers AC-WORKSPACES-REPOSITORY-SECRETS-001.4
describe("initial global read compatibility", () => {
  it.each(["populated", "empty"] as const)(
    "settles uncontested %s reads with shared gates and filtering",
    async (kind) => {
      const read = heldRead();
      const view = renderSecrets(EMPTY, true);
      expect(view.result.current.first).toMatchObject({ loaded: false, loading: true });
      expect(view.result.current.second).toMatchObject({ loaded: false, loading: true });
      expect(read.fetch).toHaveBeenCalledTimes(1);
      const global = metadata("global");
      const legacy = metadata("legacy");
      delete legacy.scope;
      const workspace = {
        ...metadata("workspace"),
        scope: "workspace" as const,
        workspace_id: "workspace-a",
      };
      const items = kind === "populated" ? [workspace, global, legacy] : [];
      await settle(read, items);
      const expected = {
        items: kind === "populated" ? [global, legacy] : [],
        loaded: true,
        loading: false,
      };
      expect(view.result.current.first).toMatchObject(expected);
      expect(view.result.current.second).toMatchObject(expected);
      expect(view.result.current.store.getState().secrets.items).toEqual(items);
      view.rerender({ first: false, second: true });
      view.rerender({ first: true, second: true });
      expect(view.result.current.first).toMatchObject(expected);
      expect(read.fetch).toHaveBeenCalledTimes(1);
    },
  );

  it("retains ordinary initial-read failure settlement", async () => {
    const read = heldRead();
    const view = renderSecrets();
    await settle(read, [], 500);
    expect(view.result.current.first).toMatchObject({ items: [], loaded: true, loading: false });
    expect(read.fetch).toHaveBeenCalledTimes(1);
  });
});
