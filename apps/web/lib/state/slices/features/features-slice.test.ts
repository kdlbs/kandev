import { describe, expect, it } from "vitest";
import { create } from "zustand";
import { immer } from "zustand/middleware/immer";
import { createAppStore } from "@/lib/state/store";
import { createFeaturesSlice, defaultFeaturesState } from "./features-slice";
import { defaultFeatureFlags } from "./types";
import type { FeatureFlags, FeaturesSlice } from "./types";

function makeStore() {
  return create<FeaturesSlice>()(immer((set) => createFeaturesSlice(set)));
}

function flagsWith(value: boolean): FeatureFlags {
  return Object.fromEntries(
    Object.keys(defaultFeatureFlags).map((name) => [name, value]),
  ) as FeatureFlags;
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
    expect(store.getState().features).toEqual(flagsWith(false));
  });

  it("setFeatures replaces the whole flag map", () => {
    const store = makeStore();
    store.getState().setFeatures(flagsWith(true));
    expect(store.getState().features).toEqual(flagsWith(true));
    store.getState().setFeatures(flagsWith(false));
    expect(store.getState().features).toEqual(flagsWith(false));
  });

  it("composes with hydrated flags in the app store", () => {
    const bootFlags = flagsWith(true);
    const store = createAppStore({ features: bootFlags });
    const unrelatedAuthState = store.getState().auth;
    const unrelatedWorkspaceState = store.getState().workspaces;

    expect(store.getState().features).toEqual(bootFlags);
    store.getState().setFeatures(flagsWith(false));
    expect(store.getState().features).toEqual(flagsWith(false));
    expect(store.getState().auth).toBe(unrelatedAuthState);
    expect(store.getState().workspaces).toBe(unrelatedWorkspaceState);
  });
});
