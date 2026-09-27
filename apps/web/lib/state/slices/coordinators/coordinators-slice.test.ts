import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createCoordinatorsSlice } from "./coordinators-slice";
import type { CoordinatorsSlice } from "./types";
import type { Coordinator } from "@/lib/api/domains/coordinator-api";

function newStore() {
  // createCoordinatorsSlice types `set` against the full AppState (the
  // zustand slices pattern, so composing it in store.ts needs no cast); this
  // isolated test store only has CoordinatorsSlice, so the two `set` shapes
  // need a cast here, in test-only code the store.ts architecture rule does
  // not cover.
  return create<CoordinatorsSlice>()(
    immer((set) =>
      createCoordinatorsSlice(set as unknown as Parameters<typeof createCoordinatorsSlice>[0]),
    ),
  );
}

function coordinator(overrides: Partial<Coordinator> = {}): Coordinator {
  return {
    id: "c1",
    workspace_id: "w1",
    name: "Coordinator One",
    agent_profile_id: "agent-1",
    executor_profile_id: "executor-1",
    context: "",
    conversation_task_id: null,
    created_at: "2026-09-27T00:00:00Z",
    updated_at: "2026-09-27T00:00:00Z",
    ...overrides,
  };
}

describe("coordinators slice", () => {
  it("starts with no coordinators loaded", () => {
    const store = newStore();
    expect(store.getState().coordinators.items).toEqual([]);
    expect(store.getState().coordinators.loaded).toBe(false);
    expect(store.getState().coordinators.loading).toBe(false);
  });

  it("replaces the item list and marks it loaded on setCoordinators", () => {
    const store = newStore();
    store.getState().setCoordinators([coordinator()]);

    expect(store.getState().coordinators.items).toEqual([coordinator()]);
    expect(store.getState().coordinators.loaded).toBe(true);
  });

  it("tracks the loading flag independently of the item list", () => {
    const store = newStore();
    store.getState().setCoordinatorsLoading(true);
    expect(store.getState().coordinators.loading).toBe(true);
    store.getState().setCoordinatorsLoading(false);
    expect(store.getState().coordinators.loading).toBe(false);
  });

  it("appends a new coordinator on addCoordinator", () => {
    const store = newStore();
    store.getState().setCoordinators([coordinator({ id: "c1" })]);
    store.getState().addCoordinator(coordinator({ id: "c2", name: "Coordinator Two" }));

    expect(store.getState().coordinators.items.map((item) => item.id)).toEqual(["c1", "c2"]);
  });

  it("replaces the matching coordinator in place on updateCoordinator", () => {
    const store = newStore();
    store.getState().setCoordinators([coordinator({ id: "c1" }), coordinator({ id: "c2" })]);
    store.getState().updateCoordinator(coordinator({ id: "c2", name: "Renamed" }));

    const items = store.getState().coordinators.items;
    expect(items.find((item) => item.id === "c2")?.name).toBe("Renamed");
    expect(items).toHaveLength(2);
  });

  it("leaves the list unchanged when updateCoordinator targets an unknown id", () => {
    const store = newStore();
    store.getState().setCoordinators([coordinator({ id: "c1" })]);
    store.getState().updateCoordinator(coordinator({ id: "unknown" }));

    expect(store.getState().coordinators.items.map((item) => item.id)).toEqual(["c1"]);
  });

  it("removes the matching coordinator on removeCoordinator", () => {
    const store = newStore();
    store.getState().setCoordinators([coordinator({ id: "c1" }), coordinator({ id: "c2" })]);
    store.getState().removeCoordinator("c1");

    expect(store.getState().coordinators.items.map((item) => item.id)).toEqual(["c2"]);
  });
});
