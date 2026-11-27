import { describe, expect, it, vi } from "vitest";
import type { StoreApi } from "zustand";
import type { AppState } from "@/lib/state/store";
import { createScopedMetadataRead, readJourneyWorkflowSteps } from "./journey-metadata-resources";

const workflowApiMocks = vi.hoisted(() => ({
  listWorkflowSteps: vi.fn(),
}));

vi.mock("@/lib/api/domains/workflow-api", () => ({
  listWorkflowSteps: workflowApiMocks.listWorkflowSteps,
}));

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

describe("journey workflow step metadata ownership", () => {
  it("shares selected workflow step reads and keeps a second consumer after one releases", async () => {
    workflowApiMocks.listWorkflowSteps.mockReset();
    const pending = deferred<{ steps: never[]; total: number }>();
    workflowApiMocks.listWorkflowSteps.mockReturnValue(pending.promise);
    const { store } = fixture();
    const controller = new AbortController();
    const dialog = readJourneyWorkflowSteps(store, "shared-workflow", {
      signal: controller.signal,
    });
    const picker = readJourneyWorkflowSteps(store, "shared-workflow");
    const cancelled = expect(dialog).rejects.toMatchObject({ name: "AbortError" });

    controller.abort();
    await cancelled;
    pending.resolve({ steps: [], total: 0 });

    expect(await picker).toEqual({ steps: [], total: 0 });
    expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledTimes(1);
  });

  it("queues explicit retry through the scoped shared workflow step owner", async () => {
    workflowApiMocks.listWorkflowSteps.mockReset();
    const first = deferred<{ steps: never[]; total: number }>();
    const retry = deferred<{ steps: never[]; total: number }>();
    workflowApiMocks.listWorkflowSteps
      .mockReturnValueOnce(first.promise)
      .mockReturnValueOnce(retry.promise);
    const { store } = fixture();
    const initial = readJourneyWorkflowSteps(store, "retry-workflow");
    const refreshed = readJourneyWorkflowSteps(store, "retry-workflow", { refresh: true });

    first.resolve({ steps: [], total: 0 });
    expect(await initial).toEqual({ steps: [], total: 0 });
    await Promise.resolve();
    retry.resolve({ steps: [], total: 0 });

    expect(await refreshed).toEqual({ steps: [], total: 0 });
    expect(workflowApiMocks.listWorkflowSteps).toHaveBeenCalledTimes(2);
  });
});
