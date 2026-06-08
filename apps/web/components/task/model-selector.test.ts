import { describe, expect, it } from "vitest";
import { triggerLabel, usableConfigOptions } from "@/components/model-config-selector";
import type { ConfigOptionEntry } from "@/lib/state/slices/session-runtime/types";

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
});
