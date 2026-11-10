import { beforeEach, describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createUISlice } from "./ui-slice";
import type { UISlice } from "./types";

const storageKey = "kandev.settings.imagePreviewsExpandedByDefault";
function makeStore() {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...args) => ({ ...(createUISlice as any)(...args) })),
  );
}

describe("initial image preview disclosure preference", () => {
  beforeEach(() => window.localStorage.clear());

  it.each([null, "{invalid", '"false"', "42"])(
    "defaults on for absent or invalid storage: %s",
    (value) => {
      if (value !== null) window.localStorage.setItem(storageKey, value);
      expect(makeStore().getState().imagePreviewsExpandedByDefault).toBe(true);
    },
  );

  it("does not reinterpret the removed enlargement preference", () => {
    window.localStorage.setItem("kandev.settings.imagePreviewExpansion", "false");
    expect(makeStore().getState().imagePreviewsExpandedByDefault).toBe(true);
  });

  it("persists both choices and hydrates them on the next store", () => {
    const store = makeStore();
    store.getState().commitImagePreviewsExpandedByDefault(false);
    expect(store.getState().imagePreviewsExpandedByDefault).toBe(false);
    expect(window.localStorage.getItem(storageKey)).toBe("false");
    expect(makeStore().getState().imagePreviewsExpandedByDefault).toBe(false);
    store.getState().commitImagePreviewsExpandedByDefault(true);
    expect(makeStore().getState().imagePreviewsExpandedByDefault).toBe(true);
  });
});
