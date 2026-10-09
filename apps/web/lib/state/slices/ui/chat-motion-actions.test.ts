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

describe("chat motion preference actions", () => {
  it("defaults existing installations to enabled", () => {
    expect(makeStore().getState().chatMotion).toEqual({ enabled: true, savedEnabled: true });
  });
  beforeEach(() => window.localStorage.clear());

  it("hydrates the current and saved values from device storage", () => {
    window.localStorage.setItem("kandev.settings.chatAnimations", "false");

    const store = makeStore();

    expect(store.getState().chatMotion).toEqual({ enabled: false, savedEnabled: false });
  });

  it("previews, commits, and restores without persisting a preview", () => {
    const store = makeStore();

    store.getState().previewChatAnimations(false);
    expect(store.getState().chatMotion).toEqual({ enabled: false, savedEnabled: true });
    expect(window.localStorage.getItem(STORAGE_KEYS.CHAT_ANIMATIONS)).toBeNull();

    store.getState().restoreChatAnimations();
    expect(store.getState().chatMotion.enabled).toBe(true);

    store.getState().commitChatAnimations(false);
    expect(store.getState().chatMotion).toEqual({ enabled: false, savedEnabled: false });
    expect(window.localStorage.getItem(STORAGE_KEYS.CHAT_ANIMATIONS)).toBe("false");
  });
});
