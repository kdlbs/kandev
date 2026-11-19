import { expect, it } from "vitest";
import { INITIAL_STATE, multiSelectReducer } from "./task-multi-select-state";

it("ranges to the clicked later occurrence without selecting an earlier group's task", () => {
  const anchored = multiSelectReducer(INITIAL_STATE, {
    type: "toggle_select",
    taskId: "y",
    occurrenceKey: "b:y",
  });
  const next = multiSelectReducer(anchored, {
    type: "select_occurrence_range",
    taskId: "r",
    occurrenceKey: "b:r",
    orderedOccurrences: [
      { key: "a:r", taskId: "r" },
      { key: "a:x", taskId: "x" },
      { key: "b:y", taskId: "y" },
      { key: "b:r", taskId: "r" },
      { key: "b:z", taskId: "z" },
    ],
  });
  expect([...next.selectedIds].sort()).toEqual(["r", "y"]);
});

it("retains intervening tasks before deduplicating canonical identities", () => {
  const anchored = multiSelectReducer(INITIAL_STATE, {
    type: "toggle_select",
    taskId: "y",
    occurrenceKey: "b:y",
  });
  const next = multiSelectReducer(anchored, {
    type: "select_occurrence_range",
    taskId: "r",
    occurrenceKey: "b:r",
    orderedOccurrences: [
      { key: "a:r", taskId: "r" },
      { key: "a:x", taskId: "x" },
      { key: "b:y", taskId: "y" },
      { key: "b:w", taskId: "w" },
      { key: "b:r", taskId: "r" },
      { key: "b:z", taskId: "z" },
    ],
  });
  expect([...next.selectedIds].sort()).toEqual(["r", "w", "y"]);
});

it.each(["toggle", "failed subset"])(
  "preserves a surviving task's clicked occurrence after %s",
  (mode) => {
    let state = multiSelectReducer(INITIAL_STATE, {
      type: "toggle_select",
      taskId: "r",
      occurrenceKey: "b:r",
    });
    state = multiSelectReducer(state, { type: "toggle_select", taskId: "y", occurrenceKey: "b:y" });
    state = multiSelectReducer(
      state,
      mode === "toggle"
        ? { type: "toggle_select", taskId: "y", occurrenceKey: "b:y" }
        : { type: "set_selected", ids: new Set(["r"]) },
    );
    const next = multiSelectReducer(state, {
      type: "select_occurrence_range",
      taskId: "z",
      occurrenceKey: "b:z",
      orderedOccurrences: [
        { key: "a:r", taskId: "r" },
        { key: "a:x", taskId: "x" },
        { key: "b:y", taskId: "y" },
        { key: "b:r", taskId: "r" },
        { key: "b:w", taskId: "w" },
        { key: "b:z", taskId: "z" },
      ],
    });
    expect([...next.selectedIds].sort()).toEqual(["r", "w", "z"]);
  },
);
