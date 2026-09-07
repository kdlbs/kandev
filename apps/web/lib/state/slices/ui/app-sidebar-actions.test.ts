import { beforeEach, describe, expect, it, vi } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createUISlice } from "./ui-slice";
import type { UISlice } from "./types";

vi.mock("@/lib/api/domains/settings-api", () => ({
  updateUserSettings: vi.fn(() => Promise.resolve({ settings: {} })),
}));

function makeStore() {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createUISlice as any)(...a) })),
  );
}

const COLLAPSED_KEY = "kandev.appSidebar.collapsed";
const PICKER_KEY = "kandev.appSidebar.workspacePickerOpen";
const SECTION_KEY = "kandev.appSidebar.sectionExpanded";

describe("appSidebar workspace picker flag", () => {
  beforeEach(() => {
    window.localStorage.clear();
  });

  it("defaults closed and is never read from storage", () => {
    window.localStorage.setItem(PICKER_KEY, JSON.stringify(true));
    const store = makeStore();
    expect(store.getState().appSidebar.workspacePickerOpen).toBe(false);
  });

  it("setWorkspacePickerOpen opens and closes the picker without persisting it", () => {
    const store = makeStore();

    store.getState().setWorkspacePickerOpen(true);
    expect(store.getState().appSidebar.workspacePickerOpen).toBe(true);
    expect(window.localStorage.getItem(PICKER_KEY)).toBeNull();

    store.getState().setWorkspacePickerOpen(false);
    expect(store.getState().appSidebar.workspacePickerOpen).toBe(false);
  });

  it("opening the picker while collapsed force-expands the rail", () => {
    window.localStorage.setItem(COLLAPSED_KEY, JSON.stringify(true));
    const store = makeStore();
    expect(store.getState().appSidebar.collapsed).toBe(true);

    store.getState().setWorkspacePickerOpen(true);

    expect(store.getState().appSidebar.workspacePickerOpen).toBe(true);
    expect(store.getState().appSidebar.collapsed).toBe(false);
    expect(JSON.parse(window.localStorage.getItem(COLLAPSED_KEY) ?? "null")).toBe(false);
  });

  it("closing the picker leaves the collapsed flag untouched", () => {
    window.localStorage.setItem(COLLAPSED_KEY, JSON.stringify(true));
    const store = makeStore();

    store.getState().setWorkspacePickerOpen(false);

    expect(store.getState().appSidebar.collapsed).toBe(true);
  });

  it("keeps Canvases folded by default and persists an explicit expansion", () => {
    const store = makeStore();

    expect(store.getState().appSidebar.sectionExpanded.canvases).toBe(false);

    store.getState().toggleAppSidebarSection("canvases", false);

    expect(store.getState().appSidebar.sectionExpanded.canvases).toBe(true);
    expect(JSON.parse(window.localStorage.getItem(SECTION_KEY) ?? "{}").canvases).toBe(true);
  });
});
