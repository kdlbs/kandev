import { describe, expect, it } from "vitest";
import {
  triggerLabel,
  type ModelSelectorOption,
  usableConfigOptions,
} from "@/components/model-config-selector";
import type { ConfigOptionEntry } from "@/lib/state/slices/session-runtime/types";

function compactTriggerLabel(
  modelOptions: ModelSelectorOption[],
  currentModel: string,
  configOptions: ConfigOptionEntry[],
  configBaseline?: Record<string, string>,
): string {
  return triggerLabel(modelOptions, currentModel, configOptions, {
    summary: "changed",
    configBaseline,
  });
}

const providerModelId = "gpt-5.6-sol";
const providerModelName = "GPT-5.6-Sol";
const modelOptions = [{ id: providerModelId, name: providerModelName }];

function sessionConfigOptions(reasoningEffort = "high", fastMode = "off"): ConfigOptionEntry[] {
  return [
    {
      type: "select",
      id: "model",
      name: "Model",
      currentValue: providerModelId,
      category: "model",
      options: [{ value: providerModelId, name: providerModelName }],
    },
    {
      type: "select",
      id: "reasoning_effort",
      name: "Reasoning Effort",
      currentValue: reasoningEffort,
      options: [
        { value: "high", name: "High" },
        { value: "low", name: "Low" },
      ],
    },
    {
      type: "select",
      id: "fast_mode",
      name: "Fast Mode",
      currentValue: fastMode,
      options: [
        { value: "off", name: "Off" },
        { value: "on", name: "On" },
      ],
    },
  ];
}

describe("model selector config options", () => {
  it("keeps model-adjacent config options and excludes mode", () => {
    const options: ConfigOptionEntry[] = [
      {
        type: "select",
        id: "mode",
        name: "Mode",
        currentValue: "agent",
        category: "mode",
        options: [{ value: "agent", name: "Agent" }],
      },
      {
        type: "select",
        id: "model",
        name: "Model",
        currentValue: "gpt-5.5",
        category: "model",
        options: [{ value: "gpt-5.5", name: "GPT-5.5" }],
      },
      {
        type: "select",
        id: "reasoning_effort",
        name: "Reasoning Effort",
        currentValue: "high",
        category: "thought_level",
        options: [{ value: "high", name: "High" }],
      },
    ];

    expect(usableConfigOptions(options).map((option) => option.id)).toEqual([
      "model",
      "reasoning_effort",
    ]);
  });

  it("summarizes model and extra option values in one toolbar label", () => {
    const label = triggerLabel([{ id: "gpt-5.5", name: "GPT-5.5" }], "gpt-5.5", [
      {
        type: "select",
        id: "model",
        name: "Model",
        currentValue: "gpt-5.5",
        category: "model",
        options: [{ value: "gpt-5.5", name: "GPT-5.5" }],
      },
      {
        type: "select",
        id: "reasoning_effort",
        name: "Reasoning Effort",
        currentValue: "medium",
        category: "thought_level",
        options: [{ value: "medium", name: "Medium" }],
      },
    ]);

    expect(label).toBe("GPT-5.5 / Medium");
  });

  it("shows only the model when task configuration matches its baseline", () => {
    const label = compactTriggerLabel(modelOptions, providerModelId, sessionConfigOptions(), {
      model: providerModelId,
      reasoning_effort: "high",
      fast_mode: "off",
    });

    expect(label).toBe(providerModelName);
  });

  it("shows only the model while the task baseline is unavailable", () => {
    const label = compactTriggerLabel(
      modelOptions,
      providerModelId,
      sessionConfigOptions("low", "on"),
    );

    expect(label).toBe(providerModelName);
  });

  it("shows every changed value in ACP option order", () => {
    const label = compactTriggerLabel(
      modelOptions,
      providerModelId,
      sessionConfigOptions("low", "on"),
      {
        model: providerModelId,
        reasoning_effort: "high",
        fast_mode: "off",
      },
    );

    expect(label).toBe(`${providerModelName} / Low / On`);
  });

  it("hides a value after it returns to baseline", () => {
    const label = compactTriggerLabel(modelOptions, providerModelId, sessionConfigOptions("high"), {
      reasoning_effort: "high",
      fast_mode: "off",
    });

    expect(label).toBe(providerModelName);
  });

  it("treats a currently advertised option with no baseline entry as changed", () => {
    const label = compactTriggerLabel(modelOptions, providerModelId, sessionConfigOptions(), {
      reasoning_effort: "high",
      removed_option: "legacy",
    });

    expect(label).toBe(`${providerModelName} / Off`);
  });
});
