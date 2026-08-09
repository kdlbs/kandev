import { describe, expect, it } from "vitest";
import { resolveParkedTriple } from "./parked-projection";

describe("resolveParkedTriple (D1, AC-39, AC-49)", () => {
  it("accepts the incoming triple when there is no existing revision to compare against", () => {
    const resolved = resolveParkedTriple(
      { parked: undefined, epoch: undefined, revision: undefined },
      { parked: true, epoch: 100, revision: 1 },
    );

    expect(resolved).toEqual({ parked: true, epoch: 100, revision: 1 });
  });

  it("discards a strictly lower revision within the same epoch", () => {
    const resolved = resolveParkedTriple(
      { parked: true, epoch: 100, revision: 7 },
      { parked: false, epoch: 100, revision: 6 },
    );

    expect(resolved).toEqual({ parked: true, epoch: 100, revision: 7 });
  });

  it("accepts a strictly higher epoch even carrying a lower revision (AC-77 restart reset)", () => {
    const resolved = resolveParkedTriple(
      { parked: true, epoch: 100, revision: 7 },
      { parked: false, epoch: 200, revision: 0 },
    );

    expect(resolved).toEqual({ parked: false, epoch: 200, revision: 0 });
  });

  it("discards a strictly lower epoch even carrying a higher revision", () => {
    const resolved = resolveParkedTriple(
      { parked: true, epoch: 200, revision: 1 },
      { parked: false, epoch: 100, revision: 999 },
    );

    expect(resolved).toEqual({ parked: true, epoch: 200, revision: 1 });
  });

  it("accepts a higher revision within the same epoch", () => {
    const resolved = resolveParkedTriple(
      { parked: false, epoch: 100, revision: 1 },
      { parked: true, epoch: 100, revision: 2 },
    );

    expect(resolved).toEqual({ parked: true, epoch: 100, revision: 2 });
  });

  it("accepts an equal (epoch, revision) pair — ties go to the incoming value", () => {
    const resolved = resolveParkedTriple(
      { parked: false, epoch: 100, revision: 5 },
      { parked: true, epoch: 100, revision: 5 },
    );

    expect(resolved).toEqual({ parked: true, epoch: 100, revision: 5 });
  });

  it("treats a missing incoming epoch/revision as 0 for the comparison", () => {
    const resolved = resolveParkedTriple(
      { parked: true, epoch: 1, revision: 1 },
      { parked: false, epoch: undefined, revision: undefined },
    );

    expect(resolved).toEqual({ parked: true, epoch: 1, revision: 1 });
  });
});
