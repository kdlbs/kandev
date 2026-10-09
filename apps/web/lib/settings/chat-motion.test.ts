import { beforeEach, describe, expect, it } from "vitest";
import { STORAGE_KEYS } from "./constants";
import {
  DEFAULT_CHAT_ANIMATIONS_ENABLED,
  readChatAnimationsEnabled,
  writeChatAnimationsEnabled,
} from "./chat-motion";

describe("chat motion preference", () => {
  beforeEach(() => window.localStorage.clear());

  it("defaults to enabled when no preference is stored", () => {
    expect(DEFAULT_CHAT_ANIMATIONS_ENABLED).toBe(true);
    expect(readChatAnimationsEnabled()).toBe(true);
  });

  it("falls back to enabled for malformed or non-boolean storage", () => {
    window.localStorage.setItem(STORAGE_KEYS.CHAT_ANIMATIONS, "{not json");
    expect(readChatAnimationsEnabled()).toBe(true);

    window.localStorage.setItem(STORAGE_KEYS.CHAT_ANIMATIONS, JSON.stringify("disabled"));
    expect(readChatAnimationsEnabled()).toBe(true);
  });

  it("persists an explicit device preference", () => {
    writeChatAnimationsEnabled(false);

    expect(readChatAnimationsEnabled()).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEYS.CHAT_ANIMATIONS)).toBe("false");
  });
});
