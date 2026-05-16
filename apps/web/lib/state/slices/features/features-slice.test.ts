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
  });

  it("setFeatures replaces the whole flag map", () => {
    const store = makeStore();
    store.getState().setFeatures({ office: true });
    expect(store.getState().features.office).toBe(true);
    store.getState().setFeatures({ office: false });
    expect(store.getState().features.office).toBe(false);
  });
});
