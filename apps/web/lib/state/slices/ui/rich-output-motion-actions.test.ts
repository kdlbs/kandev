import { beforeEach, describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { STORAGE_KEYS } from "@/lib/settings/constants";
import { createUISlice } from "./ui-slice";
import type { UISlice } from "./types";

function makeStore() {
  return create<UISlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...args) => ({ ...(createUISlice as any)(...args) })),
  );
}

describe("rich-output motion preference actions", () => {
  beforeEach(() => window.localStorage.clear());

  it("hydrates the current and saved values from device storage", () => {
    window.localStorage.setItem(STORAGE_KEYS.RICH_OUTPUT_ANIMATIONS, "false");

    const store = makeStore();

    expect(store.getState().richOutputMotion).toEqual({ enabled: false, savedEnabled: false });
  });

  it("previews, commits, and restores without persisting a preview", () => {
    const store = makeStore();

    store.getState().previewRichOutputAnimations(false);
    expect(store.getState().richOutputMotion).toEqual({ enabled: false, savedEnabled: true });
    expect(window.localStorage.getItem(STORAGE_KEYS.RICH_OUTPUT_ANIMATIONS)).toBeNull();

    store.getState().restoreRichOutputAnimations();
    expect(store.getState().richOutputMotion.enabled).toBe(true);

    store.getState().commitRichOutputAnimations(false);
    expect(store.getState().richOutputMotion).toEqual({ enabled: false, savedEnabled: false });
    expect(window.localStorage.getItem(STORAGE_KEYS.RICH_OUTPUT_ANIMATIONS)).toBe("false");
  });
});
