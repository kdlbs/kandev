import { describe, expect, it } from "vitest";
import {
  buildExecutorProfileValues,
  parseExecutorProfileSchema,
  serializeExecutorProfileValues,
} from "./executor-profile-schema";

describe("executor profile schema serialization", () => {
  it("submits empty optional values so an edit can clear a prior value", () => {
    const { fields } = parseExecutorProfileSchema({
      type: "object",
      properties: { workers: { type: "integer" } },
    });
    const values = buildExecutorProfileValues(fields, { workers: "4" });

    expect(serializeExecutorProfileValues(fields, { ...values, workers: "" })).toEqual({
      workers: "",
    });
  });
});
