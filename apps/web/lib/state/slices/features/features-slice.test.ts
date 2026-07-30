import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createFeaturesSlice, defaultFeaturesState } from "./features-slice";
import type { FeaturesSlice } from "./types";

function makeStore() {
  return create<FeaturesSlice>()(
    // eslint-disable-next-line @typescript-eslint/no-explicit-any
    immer((...a) => ({ ...(createFeaturesSlice as any)(...a) })),
  );
}

describe("features slice", () => {
  // Production-safety invariant: every flag must be false out of the box.
  // If this test starts failing because a new flag was added defaulting to
  // true, that is the bug — fix the default, not the test.
  it("defaults every flag to false", () => {
    const store = makeStore();
    for (const [name, value] of Object.entries(defaultFeaturesState.features)) {
      expect(value, `default of features.${name}`).toBe(false);
    }
    expect(store.getState().features.office).toBe(false);
    expect(store.getState().features.plugins).toBe(false);
    expect(defaultFeaturesState.features).toHaveProperty("appStatusBar", false);
    expect(defaultFeaturesState.features).toHaveProperty("auth", false);
    expect(defaultFeaturesState.features).toHaveProperty("claudeBackgroundPromptHandoff", false);
  });

  it("setFeatures replaces the whole flag map", () => {
    const store = makeStore();
    store.getState().setFeatures({
      office: true,
      plugins: true,
      appStatusBar: true,
      auth: true,
      claudeBackgroundPromptHandoff: true,
    });
    expect(store.getState().features.office).toBe(true);
    expect(store.getState().features.plugins).toBe(true);
    expect(store.getState().features.appStatusBar).toBe(true);
    expect(store.getState().features.auth).toBe(true);
    expect(store.getState().features.claudeBackgroundPromptHandoff).toBe(true);
    store.getState().setFeatures({
      office: false,
      plugins: false,
      appStatusBar: false,
      auth: false,
      claudeBackgroundPromptHandoff: false,
    });
    expect(store.getState().features.office).toBe(false);
    expect(store.getState().features.plugins).toBe(false);
    expect(store.getState().features.appStatusBar).toBe(false);
    expect(store.getState().features.auth).toBe(false);
    expect(store.getState().features.claudeBackgroundPromptHandoff).toBe(false);
  });
});
