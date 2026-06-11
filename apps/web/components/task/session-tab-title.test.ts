import { describe, expect, it } from "vitest";
import { resolveSessionTabTitle } from "./session-tab-title";

const SPARK_MODEL_ID = "gpt-5.3-codex-spark";
const SPARK_MODEL_NAME = "GPT-5.3-Codex-Spark";

const baseArgs = {
  agentLabel: "GPT-5.5 (medium)",
  activeModelId: null,
  currentModelId: null,
  snapshotModel: null,
  modelOptions: [],
  configOptions: [],
};

describe("resolveSessionTabTitle", () => {
  it("uses the active model label over the original agent label", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
        activeModelId: SPARK_MODEL_ID,
        modelOptions: [{ id: SPARK_MODEL_ID, name: SPARK_MODEL_NAME }],
      }),
    ).toBe(SPARK_MODEL_NAME);
  });

  it("includes non-model config selections in the title", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
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
    expect(resolveSessionTabTitle(baseArgs)).toBe("GPT-5.5 (medium)");
  });

  it("falls back to currentModelId when active model id is missing", () => {
    expect(
      resolveSessionTabTitle({
        ...baseArgs,
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
    ).toBe("GPT-5.5 (medium)");
  });
});
