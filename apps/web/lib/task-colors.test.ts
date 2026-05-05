import { beforeEach, describe, expect, it, vi } from "vitest";
import {
  getTaskColor,
  setTaskColor,
  TASK_COLORS_CHANGED_EVENT,
  TASK_COLORS_STORAGE_KEY,
} from "./task-colors";

describe("task colors storage", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("returns null when no color is stored", () => {
    expect(getTaskColor("task-1")).toBeNull();
  });

  it("stores and reads a color", () => {
    setTaskColor("task-1", "blue");
    expect(getTaskColor("task-1")).toBe("blue");
  });

  it("removes a color when set to null", () => {
    setTaskColor("task-1", "blue");
    setTaskColor("task-1", null);
    expect(getTaskColor("task-1")).toBeNull();
  });

  it("ignores invalid colors loaded from storage", () => {
    window.localStorage.setItem(
      TASK_COLORS_STORAGE_KEY,
      JSON.stringify({ "task-1": "fuchsia", "task-2": "red" }),
    );
    expect(getTaskColor("task-1")).toBeNull();
    expect(getTaskColor("task-2")).toBe("red");
  });

  it("returns null on malformed storage", () => {
    window.localStorage.setItem(TASK_COLORS_STORAGE_KEY, "{not json");
    expect(getTaskColor("task-1")).toBeNull();
  });

  it("dispatches a change event when a color is set", () => {
    const listener = vi.fn();
    window.addEventListener(TASK_COLORS_CHANGED_EVENT, listener);
    setTaskColor("task-1", "green");
    expect(listener).toHaveBeenCalledTimes(1);
    window.removeEventListener(TASK_COLORS_CHANGED_EVENT, listener);
  });

  it("does not dispatch when setting the same color twice", () => {
    setTaskColor("task-1", "green");
    const listener = vi.fn();
    window.addEventListener(TASK_COLORS_CHANGED_EVENT, listener);
    setTaskColor("task-1", "green");
    expect(listener).not.toHaveBeenCalled();
    window.removeEventListener(TASK_COLORS_CHANGED_EVENT, listener);
  });

  it("does not dispatch when clearing a non-existent color", () => {
    const listener = vi.fn();
    window.addEventListener(TASK_COLORS_CHANGED_EVENT, listener);
    setTaskColor("task-1", null);
    expect(listener).not.toHaveBeenCalled();
    window.removeEventListener(TASK_COLORS_CHANGED_EVENT, listener);
  });
});
