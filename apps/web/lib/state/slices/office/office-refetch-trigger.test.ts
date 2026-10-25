import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createOfficeSlice } from "./office-slice";
import type { OfficeSlice } from "./types";

function makeStore() {
  return create<OfficeSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createOfficeSlice as any)(...a) })),
  );
}

/**
 * A single WS event routinely fires setOfficeRefetchTrigger more than once in
 * the same synchronous handler (e.g. `task:<id>` immediately followed by
 * `dashboard` for office.task.updated). Both calls must survive together in
 * one trigger object — otherwise React's automatic batching of the resulting
 * store updates only ever lets a listener observe the last write, silently
 * starving any listener keyed on the earlier type.
 */
describe("setOfficeRefetchTrigger — same-tick batching", () => {
  it("carries every type fired synchronously in the same trigger", async () => {
    const store = makeStore();

    store.getState().setOfficeRefetchTrigger("task:t-1");
    store.getState().setOfficeRefetchTrigger("dashboard");

    // The batched flush happens in a microtask.
    await Promise.resolve();
    await Promise.resolve();

    expect(store.getState().office.refetchTrigger?.types).toEqual(["task:t-1", "dashboard"]);
  });

  it("de-dupes a type fired twice in the same tick", async () => {
    const store = makeStore();

    store.getState().setOfficeRefetchTrigger("dashboard");
    store.getState().setOfficeRefetchTrigger("dashboard");

    await Promise.resolve();
    await Promise.resolve();

    expect(store.getState().office.refetchTrigger?.types).toEqual(["dashboard"]);
  });

  it("starts a fresh trigger object for calls in a later tick", async () => {
    const store = makeStore();

    store.getState().setOfficeRefetchTrigger("tasks");
    await Promise.resolve();
    await Promise.resolve();
    const first = store.getState().office.refetchTrigger;

    store.getState().setOfficeRefetchTrigger("dashboard");
    await Promise.resolve();
    await Promise.resolve();
    const second = store.getState().office.refetchTrigger;

    expect(first?.types).toEqual(["tasks"]);
    expect(second?.types).toEqual(["dashboard"]);
    expect(second).not.toBe(first);
  });
});
