import { describe, expect, it } from "vitest";
import { resolveSessionTabTitle } from "./session-tab-title";

const SPARK_MODEL_ID = "gpt-5.3-codex-spark";
const SPARK_MODEL_NAME = "GPT-5.3-Codex-Spark";
const PROFILE_LABEL = "GPT-5.5 (medium)";

const baseArgs = {
  agentLabel: PROFILE_LABEL,
  activeModelId: null,
  currentModelId: null,
  snapshotModel: null,
  modelOptions: [],
  configOptions: [],
};

describe("resolveSessionTabTitle", () => {
  it("prefers the user-supplied custom name over every derived title", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        customName: "reviewer",
        activeModelId: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe("reviewer");
  });

  it("falls back to derived titles when the custom name is empty or absent", () => {
    expect(resolveSessionTabTitle({ ...baseArgs, customName: "" })).toBe(PROFILE_LABEL);
    expect(resolveSessionTabTitle({ ...baseArgs, customName: null })).toBe(PROFILE_LABEL);
  });

  it("uses the agent label over live model state when a profile label is available", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        activeModelId: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe(PROFILE_LABEL);
  });

  it("includes non-model config selections in the title", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        agentLabel: null,
        activeModelId: SPARK_MODEL_ID,
        configOptions: [
          {
            type: "select",
            id: "model",
            name: "Model",
            currentValue: "gpt-5.5",
            options: [
              { value: "gpt-5.5", name: "GPT-5.5" },
              { value: SPARK_MODEL_ID, name: SPARK_MODEL_NAME },
            ],
          },
          {
            type: "select",
            id: "effort",
            name: "Effort",
            currentValue: "high",
            options: [
              { value: "medium", name: "Medium" },
              { value: "high", name: "High" },
            ],
          },
        ],
      }),
    ).toBe(`${SPARK_MODEL_NAME} / High`);
  });

  it("falls back to the agent label when live model state is unavailable", () => {
    expect(resolveSessionTabTitle(baseArgs)).toBe(PROFILE_LABEL);
  });

  it("falls back to currentModelId when active model id is missing", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        agentLabel: null,
        currentModelId: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });

  it("falls back to snapshot model when both agent and live model states are unavailable", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        agentLabel: null,
        snapshotModel: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });

  it("keeps the agent label over the start-time snapshot model", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        snapshotModel: "gpt-5.5",
      }),
    ).toBe(PROFILE_LABEL);
  });
});

describe("resolveSessionTabTitle workflow-step label", () => {
  it("uses the workflow-step label over the agent/model title", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        stepLabel: "Spec",
        rank: 2,
        activeModelId: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe("Spec #2");
  });

  it("lets a user rename win over the workflow-step label", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        customName: "reviewer",
        stepLabel: "Spec",
        rank: 2,
      }),
    ).toBe("reviewer");
  });

  it("disambiguates duplicate step labels by rank", () => {
    expect(resolveSessionTabTitle({ ...baseArgs, stepLabel: "Spec", rank: 1 })).toBe("Spec #1");
    expect(resolveSessionTabTitle({ ...baseArgs, stepLabel: "Spec", rank: 2 })).toBe("Spec #2");
  });

  it("falls back to the agent label plus rank when no step label is present", () => {
    expect(resolveSessionTabTitle({ ...baseArgs, stepLabel: null, rank: 3 })).toBe(
      `${PROFILE_LABEL} #3`,
    );
  });
});
