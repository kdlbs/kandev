import { describe, expect, it, vi } from "vitest";
import { createAppStore } from "@/lib/state/store";
import type { Executor } from "@/lib/types/http";
import { registerExecutorsHandlers } from "./executors";
import { listExecutors } from "@/lib/api/domains/settings-api";

vi.mock("@/lib/api/domains/settings-api", () => ({ listExecutors: vi.fn() }));

function pending<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((done) => {
    resolve = done;
  });
  return { promise, resolve };
}

describe("executor ownership catalogue", () => {
  it("clears configuration when the authenticated user leaves", () => {
    const store = createAppStore();
    store.getState().setExecutors([{ id: "private" } as Executor]);
    store.getState().clearAuthenticated();
    expect(store.getState().executors.items).toEqual([]);
  });
  it("evicts revoked choices immediately and rejects an older refetch", async () => {
    const store = createAppStore();
    store.getState().setExecutors([{ id: "revoked" } as Executor]);
    const first = pending<{ executors: Executor[]; total: number }>();
    const second = pending<{ executors: Executor[]; total: number }>();
    vi.mocked(listExecutors).mockReturnValueOnce(first.promise).mockReturnValueOnce(second.promise);
    const handlers = registerExecutorsHandlers(store) as Record<string, (message: unknown) => void>;
    const invalidate = handlers["executor.catalogue.invalidated"];
    expect(invalidate).toBeTypeOf("function");
    invalidate({ payload: {} });
    expect(store.getState().executors.items).toEqual([]);
    invalidate({ payload: {} });
    second.resolve({ executors: [{ id: "allowed" } as Executor], total: 1 });
    await vi.waitFor(() =>
      expect(store.getState().executors.items.map((item) => item.id)).toEqual(["allowed"]),
    );
    first.resolve({ executors: [{ id: "revoked" } as Executor], total: 1 });
    await first.promise;
    await Promise.resolve();
    expect(store.getState().executors.items.map((item) => item.id)).toEqual(["allowed"]);
  });
});

it("evicts administrator configuration when the same user's role is revoked", () => {
  const store = createAppStore();
  const user = {
    id: "alice",
    email: "alice@example.com",
    display_name: "Alice",
    role: "admin",
    status: "active",
  };
  store.getState().setAuthState({ mode: "enabled", authenticated: true, user });
  store.getState().setExecutors([{ id: "other-private" } as Executor]);
  store
    .getState()
    .setAuthState({ mode: "enabled", authenticated: true, user: { ...user, role: "member" } });
  expect(store.getState().executors.items).toEqual([]);
});
