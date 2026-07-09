import { describe, expect, it } from "vitest";
import { WALKTHROUGH_STEP_BODY_CLASS } from "./walkthrough-step-card";

describe("walkthrough step card", () => {
  it("keeps walkthrough description text left-aligned", () => {
    expect(WALKTHROUGH_STEP_BODY_CLASS).toContain("text-left");
    expect(WALKTHROUGH_STEP_BODY_CLASS).toContain("[text-align:left]");
    expect(WALKTHROUGH_STEP_BODY_CLASS).toContain("[&_p]:text-left");
    expect(WALKTHROUGH_STEP_BODY_CLASS).toContain("[&_li]:text-left");
  });
});
