import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { createScopedMetadataRead } from "./journey-metadata-resources";

function fixture() {
  const state = { workspaceContextGeneration: 0 } as AppState;
  const store = { getState: () => state } as StoreApi<AppState>;
  return { state, store };
}

function deferred<T>() {
  let resolve!: (value: T) => void;
  const promise = new Promise<T>((next) => {
    resolve = next;
  });
  return { promise, resolve };
}

describe("journey metadata ownership", () => {
  it("shares a projection and cancels only the departing consumer", async () => {
    const read = createScopedMetadataRead<string>();
    const { store } = fixture();
    const pending = deferred<string>();
    let sharedSignal!: AbortSignal;
    const load = vi.fn((signal: AbortSignal) => {
      sharedSignal = signal;
      return pending.promise;
    });
    const controller = new AbortController();
    const first = read(store, "profile", load, { signal: controller.signal });
    const second = read(store, "profile", load);
    const cancelled = expect(first).rejects.toMatchObject({ name: "AbortError" });
    controller.abort();
    await cancelled;
    expect(sharedSignal.aborted).toBe(false);
    expect(load).toHaveBeenCalledTimes(1);
    pending.resolve("config");
    expect(await second).toBe("config");
  });

  it("releases a cancelled final consumer before the next synchronous acquisition", async () => {
    const read = createScopedMetadataRead<string>();
    const { store } = fixture();
    const pending = deferred<string>();
    let sharedSignal!: AbortSignal;
    const load = vi.fn((signal: AbortSignal) => {
      sharedSignal = signal;
      return pending.promise;
    });
    const controller = new AbortController();
    const first = read(store, "settings", load, { signal: controller.signal });
    const cancelled = expect(first).rejects.toMatchObject({ name: "AbortError" });
    controller.abort();
    const replacement = vi.fn().mockResolvedValue("current");
    const second = read(store, "settings", replacement);
    expect(sharedSignal.aborted).toBe(true);
    expect(replacement).toHaveBeenCalledOnce();
    await cancelled;
    expect(await second).toBe("current");
    pending.resolve("old");
  });

  it("keeps response options and stores separate", async () => {
    const read = createScopedMetadataRead<string>();
    const { store } = fixture();
    const load = vi.fn().mockResolvedValue("projection");
    await Promise.all([
      read(store, "repo:scripts", load),
      read(store, "repo:compact", load),
      read(fixture().store, "repo:scripts", load),
    ]);
    expect(load).toHaveBeenCalledTimes(3);
  });

  it("rejects retired generations even if a transport ignores cancellation", async () => {
    const read = createScopedMetadataRead<string>();
    const { store, state } = fixture();
    const pending = deferred<string>();
    let signal!: AbortSignal;
    const first = read(store, "settings", (next) => {
      signal = next;
      return pending.promise;
    });
    const stale = expect(first).rejects.toMatchObject({ name: "AbortError" });
    state.workspaceContextGeneration++;
    expect(await read(store, "settings", async () => "new")).toBe("new");
    expect(signal.aborted).toBe(true);
    pending.resolve("old");
    await stale;
  });

  it("bounds invalidations to one trailing read and permits retry after failure", async () => {
    const read = createScopedMetadataRead<string>();
    const { store } = fixture();
    const pending = deferred<string>();
    const load = vi.fn().mockReturnValueOnce(pending.promise).mockResolvedValue("new");
    const first = read(store, "ci", load);
    const forced = read(store, "ci", load, { refresh: true });
    const joined = read(store, "ci", load, { refresh: true });
    pending.resolve("old");
    expect(await first).toBe("old");
    expect(await forced).toBe("new");
    expect(await joined).toBe("new");
    expect(load).toHaveBeenCalledTimes(2);
    await expect(
      read(store, "ci", async () => {
        throw new Error("unavailable");
      }),
    ).rejects.toThrow("unavailable");
    expect(await read(store, "ci", async () => "recovered")).toBe("recovered");
  });
});
