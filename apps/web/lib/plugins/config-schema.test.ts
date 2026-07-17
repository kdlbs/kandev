import { describe, expect, it } from "vitest";
import {
  SECRET_MASK,
  buildInitialValues,
  missingRequiredFields,
  parseConfigSchema,
  serializeConfigValues,
} from "./config-schema";

const githubSchema = {
  type: "object",
  required: ["github_token"],
  properties: {
    github_token: {
      type: "string",
      title: "Personal access token",
      description: "Fine-grained PAT with repo scope.",
      secret: true,
    },
    webhook_key: { type: "string", format: "password" },
    org: { type: "string" },
    max_items: { type: "integer", default: 25 },
    channel: { type: "string", enum: ["dev", "ops"] },
    verbose: { type: "boolean", default: true },
  },
} satisfies Record<string, unknown>;

describe("parseConfigSchema", () => {
  it("maps properties to fields with titles, requiredness, and secrecy", () => {
    const fields = parseConfigSchema(githubSchema);
    const byName = Object.fromEntries(fields.map((f) => [f.name, f]));

    expect(fields).toHaveLength(6);
    expect(byName.github_token).toMatchObject({
      type: "string",
      label: "Personal access token",
      description: "Fine-grained PAT with repo scope.",
      required: true,
      secret: true,
    });
    expect(byName.webhook_key.secret).toBe(true);
    expect(byName.org).toMatchObject({ label: "org", required: false, secret: false });
    expect(byName.max_items).toMatchObject({ type: "integer", defaultValue: 25 });
    expect(byName.channel).toMatchObject({ type: "enum", enumValues: ["dev", "ops"] });
    expect(byName.verbose.type).toBe("boolean");
  });

  it("returns [] for missing or unusable schemas", () => {
    expect(parseConfigSchema(undefined)).toEqual([]);
    expect(parseConfigSchema({})).toEqual([]);
    expect(parseConfigSchema({ properties: "nope" })).toEqual([]);
  });
});

describe("buildInitialValues", () => {
  it("prefers stored config, then defaults, then empties", () => {
    const fields = parseConfigSchema(githubSchema);
    const values = buildInitialValues(fields, { github_token: SECRET_MASK, org: "kdlbs" });

    expect(values.github_token).toBe(SECRET_MASK);
    expect(values.org).toBe("kdlbs");
    expect(values.max_items).toBe("25");
    expect(values.verbose).toBe(true);
    expect(values.channel).toBe("");
  });
});

describe("serializeConfigValues", () => {
  it("parses numerics, keeps booleans, and drops empty strings", () => {
    const fields = parseConfigSchema(githubSchema);
    const config = serializeConfigValues(fields, {
      github_token: SECRET_MASK,
      webhook_key: "",
      org: "kdlbs",
      max_items: "10",
      channel: "",
      verbose: false,
    });

    expect(config).toEqual({
      github_token: SECRET_MASK,
      org: "kdlbs",
      max_items: 10,
      verbose: false,
    });
  });

  it("omits unparseable numeric input", () => {
    const fields = parseConfigSchema(githubSchema);
    const config = serializeConfigValues(fields, { max_items: "abc", verbose: true });
    expect(config).not.toHaveProperty("max_items");
  });
});

describe("missingRequiredFields", () => {
  it("flags blank required fields and accepts the secret mask as set", () => {
    const fields = parseConfigSchema(githubSchema);

    expect(missingRequiredFields(fields, { github_token: "" })).toEqual(["Personal access token"]);
    expect(missingRequiredFields(fields, { github_token: SECRET_MASK })).toEqual([]);
    expect(missingRequiredFields(fields, { github_token: "ghp_x" })).toEqual([]);
  });
});
