import { describe, expect, it } from "vitest";
import { validCounts, type ActionCounts } from "./use-action-summary";

const zero = {
  proposed: 0,
  approved: 0,
  approved_with_edits: 0,
  rejected: 0,
  failed: 0,
  refused: 0,
  undone: 0,
};

function sixActions(): ActionCounts {
  return {
    create_task: { ...zero, approved: 3, rejected: 1 },
    start_agent: { ...zero },
    message: { ...zero },
    move: { ...zero },
    resume: { ...zero },
    stop: { ...zero },
  };
}

describe("validCounts", () => {
  it("accepts a fresh coordinator's summary, which has no unknown class", () => {
    const classes = sixActions();
    expect(validCounts(classes)).toBe(classes);
  });

  it("accepts a summary that also carries a valid unknown class", () => {
    const classes = { ...sixActions(), unknown: { ...zero, approved: 0, rejected: 2 } };
    expect(validCounts(classes)).toBe(classes);
  });

  it("rejects a summary missing a policy action", () => {
    const classes = sixActions();
    delete classes.stop;
    expect(validCounts(classes)).toBeNull();
  });

  it("rejects non-integer or negative counts, including on unknown", () => {
    expect(validCounts({ ...sixActions(), move: { ...zero, approved: 1.5 } })).toBeNull();
    expect(validCounts({ ...sixActions(), unknown: { ...zero, rejected: -1 } })).toBeNull();
  });

  it("rejects a missing summary", () => {
    expect(validCounts(undefined)).toBeNull();
  });
});
