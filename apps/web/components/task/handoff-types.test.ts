import { describe, expect, it } from "vitest";
import { buildHandoffInitialState } from "./handoff-types";

describe("handoff-types", () => {
  it("buildHandoffInitialState selects target profile and blank context", () => {
    const result = buildHandoffInitialState({
      sourceSessionId: "session-a",
      targetProfileId: "profile-b",
    });
    expect(result).toEqual({
      selectedProfileId: "profile-b",
      contextValue: "blank",
    });
  });
});
