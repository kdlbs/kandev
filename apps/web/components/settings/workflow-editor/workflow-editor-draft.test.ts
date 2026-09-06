import { describe, expect, it } from "vitest";
import { acknowledgePersistedDeletedStepIds } from "./workflow-editor-draft";

describe("acknowledgePersistedDeletedStepIds", () => {
  it("keeps a deletion recorded while the save was in flight", () => {
    expect(
      acknowledgePersistedDeletedStepIds(["old-step", "concurrent-step"], ["old-step"]),
    ).toEqual(["concurrent-step"]);
  });

  it("removes every deletion included in the submitted snapshot", () => {
    expect(acknowledgePersistedDeletedStepIds(["step-a", "step-b"], ["step-a", "step-b"])).toEqual(
      [],
    );
  });
});
