import { afterEach, describe, expect, it, vi } from "vitest";
import {
  getEffectiveTaskListingView,
  getStoredTaskListingView,
  parseTaskListingView,
  resolveHomeTaskListingRedirect,
  resolveTaskListingView,
  setStoredTaskListingView,
  TASK_LISTING_VIEW_CHANGE_EVENT,
  TASK_LISTING_VIEW_STORAGE_KEY,
} from "./view-preference";

let originalLocalStorage: Storage | null = null;

afterEach(() => {
  if (originalLocalStorage) {
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      value: originalLocalStorage,
    });
    originalLocalStorage = null;
  }
  window.localStorage.clear();
  vi.restoreAllMocks();
});

describe("task listing view preference", () => {
  it("accepts the versioned List preference", () => {
    expect(parseTaskListingView('"list"')).toBe("list");
  });

  it("ignores malformed values", () => {
    expect(parseTaskListingView("pipeline")).toBeNull();
  });

  it("uses the legacy Pipeline mode only when no device preference exists", () => {
    expect(resolveTaskListingView(null, "graph2")).toBe("pipeline");
  });

  it("defaults malformed device preferences to Kanban instead of using the legacy mode", () => {
    window.localStorage.setItem(TASK_LISTING_VIEW_STORAGE_KEY, '"unsupported"');

    expect(resolveTaskListingView(getStoredTaskListingView(), "graph2")).toBe("kanban");
  });

  it("defaults to Kanban when neither a device nor legacy preference exists", () => {
    expect(resolveTaskListingView(null, null)).toBe("kanban");
  });

  it("prefers a device-local preference over the legacy mode", () => {
    expect(resolveTaskListingView("kanban", "graph2")).toBe("kanban");
  });

  it("renders Pipeline as Kanban on phone without changing the preference", () => {
    expect(getEffectiveTaskListingView("pipeline", true)).toBe("kanban");
  });

  it("writes the versioned local preference and notifies this document", () => {
    const listener = vi.fn();
    window.addEventListener(TASK_LISTING_VIEW_CHANGE_EVENT, listener);

    setStoredTaskListingView("pipeline");

    expect(window.localStorage.getItem(TASK_LISTING_VIEW_STORAGE_KEY)).toBe('"pipeline"');
    expect(listener).toHaveBeenCalledOnce();
    window.removeEventListener(TASK_LISTING_VIEW_CHANGE_EVENT, listener);
  });

  it("keeps the selected view for this interaction when storage rejects the write", () => {
    originalLocalStorage = window.localStorage;
    Object.defineProperty(window, "localStorage", {
      configurable: true,
      value: {
        getItem: () => null,
        setItem: () => {
          throw new Error("storage blocked");
        },
        clear: () => {},
      },
    });

    setStoredTaskListingView("pipeline");

    expect(getStoredTaskListingView()).toBe("pipeline");
  });

  it("accepts the versioned Threads preference", () => {
    expect(parseTaskListingView('"threads"')).toBe("threads");
  });

  it("keeps Threads on phone because its columns page one at a time", () => {
    expect(getEffectiveTaskListingView("threads", true)).toBe("threads");
  });

  it("routes Home to the view that owns its own page", () => {
    expect(resolveHomeTaskListingRedirect("list", undefined, undefined)).toBe("list");
    expect(resolveHomeTaskListingRedirect("threads", undefined, undefined)).toBe("threads");
    expect(resolveHomeTaskListingRedirect("kanban", undefined, undefined)).toBeNull();
    expect(resolveHomeTaskListingRedirect("pipeline", undefined, undefined)).toBeNull();
  });

  it("never redirects Home away from an explicitly opened task or session", () => {
    expect(resolveHomeTaskListingRedirect("threads", "task-1", undefined)).toBeNull();
    expect(resolveHomeTaskListingRedirect("threads", undefined, "session-1")).toBeNull();
  });
});
