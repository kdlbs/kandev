import { describe, expect, it } from "vitest";
import { DIMENSION_METAS } from "./filter-dimension-registry";

describe("sidebar filter dimensions", () => {
  it("does not offer the unreachable archived dimension", () => {
    expect(DIMENSION_METAS.map((meta) => meta.dimension)).not.toContain("archived");
  });
});
