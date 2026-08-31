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

  it("maps the utility-agent format to a utility agent picker field", () => {
    const fields = parseConfigSchema({
      type: "object",
      required: ["utility_agent"],
      properties: {
        utility_agent: {
          type: "string",
          format: "utility-agent",
          title: "Utility agent",
        },
      },
    });

    expect(fields).toEqual([
      expect.objectContaining({
        name: "utility_agent",
        type: "utility_agent",
        label: "Utility agent",
        required: true,
      }),
    ]);
  });
});

describe("parseConfigSchema formats", () => {
  it("maps the agent-profile format to an agent profile picker field", () => {
    const fields = parseConfigSchema({
      type: "object",
      properties: {
        profile: {
          type: "string",
          format: "agent-profile",
          title: "Coordinator agent",
          description: "Agent profile used for coordinator cycles.",
          default: "profile-gpt4",
        },
      },
    });

    expect(fields).toEqual([
      expect.objectContaining({
        name: "profile",
        type: "agent_profile",
        label: "Coordinator agent",
        description: "Agent profile used for coordinator cycles.",
        required: false,
        defaultValue: "profile-gpt4",
      }),
    ]);
  });

  it("maps the textarea format to a textarea field", () => {
    const fields = parseConfigSchema({
      type: "object",
      properties: {
        base_prompt: {
          type: "string",
          format: "textarea",
          title: "Base prompt",
          description: "Editable coordinator base prompt.",
        },
      },
    });

    expect(fields).toEqual([
      expect.objectContaining({
        name: "base_prompt",
        type: "textarea",
        label: "Base prompt",
        description: "Editable coordinator base prompt.",
      }),
    ]);
  });

  const fixtureCoordinatorPrompt = "You are a coordinator.";
  it("serializes textarea fields as plain strings", () => {
    const fields = parseConfigSchema({
      type: "object",
      properties: {
        base_prompt: {
          type: "string",
          format: "textarea",
        },
      },
    });
    const config = serializeConfigValues(fields, { base_prompt: fixtureCoordinatorPrompt });
    expect(config).toEqual({ base_prompt: fixtureCoordinatorPrompt });
  });

  it("handles textarea with empty value as omitted", () => {
    const fields = parseConfigSchema({
      type: "object",
      properties: {
        base_prompt: { type: "string", format: "textarea" },
      },
    });
    const config = serializeConfigValues(fields, { base_prompt: "" });
    expect(config).not.toHaveProperty("base_prompt");
  });
});

describe("parseConfigSchema bounds metadata", () => {
  it("extracts numeric minimum and maximum bounds", () => {
    const fields = parseConfigSchema({
      type: "object",
      required: ["interval"],
      properties: {
        interval: {
          type: "integer",
          title: "Monitoring interval",
          description: "Minutes between cycles.",
          default: 45,
          minimum: 5,
          maximum: 1440,
        },
      },
    });

    expect(fields).toEqual([
      expect.objectContaining({
        name: "interval",
        type: "integer",
        label: "Monitoring interval",
        description: "Minutes between cycles.",
        defaultValue: 45,
        minimum: 5,
        maximum: 1440,
      }),
    ]);
  });

  it("serializes numeric values respecting bounds metadata", () => {
    const fields = parseConfigSchema({
      type: "object",
      properties: {
        interval: {
          type: "integer",
          minimum: 5,
          maximum: 1440,
        },
        rate: {
          type: "number",
          minimum: 0,
          maximum: 1,
        },
      },
    });

    // The metadata flows through to the field but does not change serialization
    // (bounds are enforced by the HTML input and backend, not by serialization).
    expect(fields.find((f) => f.name === "interval")).toMatchObject({ minimum: 5, maximum: 1440 });
    expect(fields.find((f) => f.name === "rate")).toMatchObject({ minimum: 0, maximum: 1 });
  });

  it("serializes textarea fields as plain strings", () => {
    const fields = parseConfigSchema({
      type: "object",
      properties: {
        base_prompt: {
          type: "string",
          format: "textarea",
        },
      },
    });
    const config = serializeConfigValues(fields, { base_prompt: "You are a coordinator." });
    expect(config).toEqual({ base_prompt: "You are a coordinator." });
  });

  it("handles textarea with empty value as omitted", () => {
    const fields = parseConfigSchema({
      type: "object",
      properties: {
        base_prompt: { type: "string", format: "textarea" },
      },
    });
    const config = serializeConfigValues(fields, { base_prompt: "" });
    expect(config).not.toHaveProperty("base_prompt");
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

  it("rejects (omits) non-integral input for integer fields instead of truncating", () => {
    const fields = parseConfigSchema(githubSchema);
    // parseInt would silently turn "1.5" into 1 — the field must be omitted
    // so the operator is told rather than getting a corrupted value.
    const config = serializeConfigValues(fields, { max_items: "1.5", verbose: true });
    expect(config).not.toHaveProperty("max_items");
    expect(
      serializeConfigValues(fields, { max_items: "Infinity", verbose: true }),
    ).not.toHaveProperty("max_items");
  });

  it("submits numeric enum selections with their original type", () => {
    const fields = parseConfigSchema({
      properties: { level: { type: "integer", enum: [1, 2, 3] } },
    });
    const config = serializeConfigValues(fields, { level: "2" });
    expect(config.level).toBe(2);
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
