import { beforeEach, describe, expect, it } from "vitest";
import { STORAGE_KEYS } from "./constants";
import {
  DEFAULT_RICH_OUTPUT_ANIMATIONS_ENABLED,
  readRichOutputAnimationsEnabled,
  writeRichOutputAnimationsEnabled,
} from "./rich-output-motion";

describe("rich-output motion preference", () => {
  beforeEach(() => window.localStorage.clear());

  it("defaults to enabled when no preference is stored", () => {
    expect(DEFAULT_RICH_OUTPUT_ANIMATIONS_ENABLED).toBe(true);
    expect(readRichOutputAnimationsEnabled()).toBe(true);
  });

  it("falls back to enabled for malformed or non-boolean storage", () => {
    window.localStorage.setItem(STORAGE_KEYS.RICH_OUTPUT_ANIMATIONS, "{not json");
    expect(readRichOutputAnimationsEnabled()).toBe(true);

    window.localStorage.setItem(STORAGE_KEYS.RICH_OUTPUT_ANIMATIONS, JSON.stringify("disabled"));
    expect(readRichOutputAnimationsEnabled()).toBe(true);
  });

  it("persists an explicit device preference", () => {
    writeRichOutputAnimationsEnabled(false);

    expect(readRichOutputAnimationsEnabled()).toBe(false);
    expect(window.localStorage.getItem(STORAGE_KEYS.RICH_OUTPUT_ANIMATIONS)).toBe("false");
  });
});
