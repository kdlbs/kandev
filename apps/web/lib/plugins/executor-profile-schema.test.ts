import { describe, expect, it } from "vitest";
import {
  buildExecutorProfileValues,
  parseExecutorProfileSchema,
  serializeExecutorProfileValues,
  validateExecutorProfileValues,
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

describe("executor profile schema defaults and length keywords", () => {
  const schema = {
    type: "object",
    required: ["region", "name"],
    properties: {
      region: { type: "string", default: "eu-west", pattern: "[a-z]+-[a-z]+" },
      name: { type: "string", minLength: 2, maxLength: 4 },
      cpus: { type: "integer", default: 2 },
      spot: { type: "boolean", default: false },
      token: { type: "string", secret: true },
    },
  };

  it("reads the default as display text and the length bounds", () => {
    const { fields, supported } = parseExecutorProfileSchema(schema);
    expect(supported).toBe(true);
    const byName = Object.fromEntries(fields.map((field) => [field.name, field]));
    expect(byName.region.defaultValue).toBe("eu-west");
    expect(byName.cpus.defaultValue).toBe("2");
    expect(byName.spot.defaultValue).toBe("false");
    expect(byName.name.minLength).toBe(2);
    expect(byName.name.maxLength).toBe(4);
    expect(byName.token.defaultValue).toBeUndefined();
  });

  it("does not report required for an empty value that takes its default", () => {
    const { fields } = parseExecutorProfileSchema(schema);
    const errors = validateExecutorProfileValues(fields, { region: "", name: "ok" });
    expect(errors.region).toBeUndefined();
  });

  it("still reports required without a default", () => {
    const { fields } = parseExecutorProfileSchema(schema);
    expect(validateExecutorProfileValues(fields, { region: "", name: "" }).name).toEqual({
      code: "required",
    });
  });

  it("checks code point length bounds", () => {
    const { fields } = parseExecutorProfileSchema(schema);
    expect(validateExecutorProfileValues(fields, { region: "", name: "a" }).name).toEqual({
      code: "minLength",
      value: 2,
    });
    expect(validateExecutorProfileValues(fields, { region: "", name: "abcde" }).name).toEqual({
      code: "maxLength",
      value: 4,
    });
    expect(
      validateExecutorProfileValues(fields, { region: "", name: "日本語字" }).name,
    ).toBeUndefined();
  });

  it("leaves the pattern to the host", () => {
    const { fields } = parseExecutorProfileSchema(schema);
    expect(
      validateExecutorProfileValues(fields, { region: "EU", name: "ok" }).region,
    ).toBeUndefined();
  });
});
