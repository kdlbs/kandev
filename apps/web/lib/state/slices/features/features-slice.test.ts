import { describe, expect, it } from "vitest";
import { defaultFeaturesState } from "./features-slice";

describe("features slice", () => {
  // Production-safety invariant: every flag must be false out of the box.
  // If this test starts failing because a new flag was added defaulting to
  // true, that is the bug — fix the default, not the test.
  it("defaults every flag to false", () => {
    for (const [name, value] of Object.entries(defaultFeaturesState.features)) {
      expect(value, `default of features.${name}`).toBe(false);
    }
  });
});
