import { describe, expect, it } from "vitest";

import { formatMessageSessionConfig } from "./message-session-config";

const LEGACY_MODEL = "gpt-legacy";

function turnMetadata(overrides: Record<string, unknown> = {}) {
  return {
    runtime_config_snapshot: {
      model: "gpt-5.6-sol",
      mode: "agent",
      config_options: [
        {
          id: "collaboration_mode",
          name: "Collaboration mode",
          value: "default",
          value_name: "Default",
        },
        {
          id: "reasoning_effort",
          name: "Reasoning effort",
          value: "high",
          value_name: "High",
        },
      ],
      config_baseline: {
        collaboration_mode: "default",
        reasoning_effort: "medium",
      },
      ...overrides,
    },
  };
}

describe("formatMessageSessionConfig", () => {
  it("keeps changed options in captured provider order", () => {
    const metadata = turnMetadata({
      config_options: [
        { id: "fast_mode", name: "Fast mode", value: "on", value_name: "On" },
        {
          id: "reasoning_effort",
          name: "Reasoning effort",
          value: "high",
          value_name: "High",
        },
      ],
      config_baseline: { fast_mode: "off", reasoning_effort: "medium" },
    });

    expect(formatMessageSessionConfig(undefined, metadata)).toBe(
      "gpt-5.6-sol · Fast mode: On · Reasoning effort: High",
    );
  });

  it("shows only the model when all options match the baseline", () => {
    const metadata = turnMetadata({
      config_options: [
        {
          id: "reasoning_effort",
          name: "Reasoning effort",
          value: "medium",
          value_name: "Medium",
        },
      ],
    });

    expect(formatMessageSessionConfig(undefined, metadata)).toBe("gpt-5.6-sol");
  });

  it("shows captured options when the turn has no baseline", () => {
    const metadata = turnMetadata({ config_baseline: undefined });

    expect(formatMessageSessionConfig(undefined, metadata)).toBe(
      "gpt-5.6-sol · Collaboration mode: Default · Reasoning effort: High",
    );
  });

  it("prefers provider-refined turn model over cached message and captured models", () => {
    expect(
      formatMessageSessionConfig(
        { model: "message-model" },
        { ...turnMetadata(), model: "turn-model" },
      ),
    ).toContain("turn-model ·");
  });

  it("prefers captured turn model over legacy message metadata", () => {
    expect(formatMessageSessionConfig({ model: "message-model" }, turnMetadata())).toContain(
      "gpt-5.6-sol ·",
    );
  });

  it("uses raw identifiers and values when captured display names are absent", () => {
    const metadata = turnMetadata({
      config_options: [{ id: "reasoning_effort", value: "high" }],
    });

    expect(formatMessageSessionConfig(undefined, metadata)).toBe(
      "gpt-5.6-sol · Reasoning effort: high",
    );
  });

  it("does not infer session options for legacy or malformed metadata", () => {
    expect(formatMessageSessionConfig({ model: LEGACY_MODEL }, {})).toBe(LEGACY_MODEL);
    expect(
      formatMessageSessionConfig(
        { model: LEGACY_MODEL },
        { runtime_config_snapshot: { config_options: "invalid" } },
      ),
    ).toBe(LEGACY_MODEL);
  });
});
