import { describe, expect, it } from "vitest";
import { DIMENSION_METAS } from "./filter-dimension-registry";

describe("sidebar filter dimensions", () => {
  it("offers the archived boolean dimension", () => {
    const archived = DIMENSION_METAS.find((meta) => meta.dimension === "archived");
    expect(archived).toMatchObject({
      dimension: "archived",
      valueKind: "boolean",
      ops: ["is", "is_not"],
      defaultOp: "is",
      defaultValue: true,
    });
  });
});
