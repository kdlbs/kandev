import { beforeEach, describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createUISlice } from "./ui-slice";
import type { UISlice } from "./types";

function makeStore() {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createUISlice as any)(...a) })),
  );
}

const KEY = "kandev.sidebar.collapsedSubtasks";
const TASK_A = "task-a";
const TASK_B = "task-b";

describe("toggleSubtaskCollapsed", () => {
  beforeEach(() => {
    window.sessionStorage.clear();
  });

  it("hydrates initial state from sessionStorage", () => {
    window.sessionStorage.setItem(KEY, JSON.stringify(["task-hydrated"]));
    const store = makeStore();
    expect(store.getState().collapsedSubtaskParents).toEqual(["task-hydrated"]);
  });

  it("adds a parent id on first toggle and persists it", () => {
    const store = makeStore();
    store.getState().toggleSubtaskCollapsed(TASK_A);

    expect(store.getState().collapsedSubtaskParents).toEqual([TASK_A]);
    expect(JSON.parse(window.sessionStorage.getItem(KEY) ?? "null")).toEqual([TASK_A]);
  });

  it("removes a parent id on second toggle", () => {
    const store = makeStore();
    store.getState().toggleSubtaskCollapsed(TASK_A);
    store.getState().toggleSubtaskCollapsed(TASK_A);

    expect(store.getState().collapsedSubtaskParents).toEqual([]);
    expect(JSON.parse(window.sessionStorage.getItem(KEY) ?? "null")).toEqual([]);
  });

  it("tracks multiple parents independently", () => {
    const store = makeStore();
    store.getState().toggleSubtaskCollapsed(TASK_A);
    store.getState().toggleSubtaskCollapsed(TASK_B);

    expect(store.getState().collapsedSubtaskParents).toEqual([TASK_A, TASK_B]);

    store.getState().toggleSubtaskCollapsed(TASK_A);
    expect(store.getState().collapsedSubtaskParents).toEqual([TASK_B]);
  });
});
