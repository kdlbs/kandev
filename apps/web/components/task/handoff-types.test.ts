import { describe, expect, it } from "vitest";
import { buildHandoffInitialState, summarizeContextValue } from "./handoff-types";

describe("handoff-types", () => {
  it("buildHandoffInitialState selects target profile and summarize context", () => {
    const result = buildHandoffInitialState({
      sourceSessionId: "session-a",
      targetProfileId: "profile-b",
    });
    expect(result).toEqual({
      selectedProfileId: "profile-b",
      contextValue: "summarize:session-a",
    });
  });

  it("summarizeContextValue prefixes session id", () => {
    expect(summarizeContextValue("session-123")).toBe("summarize:session-123");
  });
});
