import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ModelConfigSelector } from "@/components/model-config-selector";

afterEach(() => {
  cleanup();
});

const effortSectionTestId = "config-option-section-effort";
const effortTriggerTestId = "config-option-trigger-effort";
const modelSettingsButtonName = "Model settings";
const providerModelId = "gpt-5.6-sol";
const optionDescription = "Controls how much reasoning the model performs.";
const effortOptionName = "Reasoning Effort";
const makeModelOptions = (count: number) =>
  Array.from({ length: count }, (_, index) => ({
    id: `model-${index + 1}`,
    name: `Model ${index + 1}`,
  }));

describe("ModelConfigSelector", () => {
  it("passes custom trigger classes to the button", () => {
    render(
      <ModelConfigSelector
        modelOptions={[{ id: "gpt-5.5", name: "GPT-5.5" }]}
        currentModel="gpt-5.5"
        onModelChange={() => {}}
        triggerClassName="max-w-[56vw]"
      />,
    );

    expect(screen.getByRole("button", { name: modelSettingsButtonName }).className).toContain(
      "max-w-[56vw]",
    );
  });

  it("opens extra config options from compact sub-selector rows", () => {
    const onConfigChange = vi.fn();

    render(
      <ModelConfigSelector
        modelOptions={[{ id: "sonnet", name: "Sonnet" }]}
        currentModel="sonnet"
        onModelChange={() => {}}
        onConfigChange={onConfigChange}
        configOptions={[
          {
            type: "select",
            id: "model",
            name: "Model",
            currentValue: "sonnet",
            category: "model",
            options: [{ value: "sonnet", name: "Sonnet" }],
          },
          {
            type: "select",
            id: "effort",
            name: "Effort",
            currentValue: "medium",
            options: [
              { value: "low", name: "Low" },
              { value: "medium", name: "Medium" },
              { value: "high", name: "High" },
            ],
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));

    const effortTrigger = screen.getByTestId(effortTriggerTestId);
    expect(effortTrigger.textContent).toContain("Effort");
    expect(effortTrigger.textContent).toContain("Medium");
    expect(screen.queryByTestId(effortSectionTestId)).toBeNull();

    fireEvent.click(effortTrigger);

    const effortSection = screen.getByTestId(effortSectionTestId);
    expect(effortSection).not.toBeNull();
    const backButton = screen.getByRole("button", { name: /back to model settings from effort/i });
    expect(document.activeElement).toBe(backButton);

    fireEvent.click(backButton);

    expect(screen.queryByTestId(effortSectionTestId)).toBeNull();
    expect(document.activeElement).toBe(screen.getByTestId(effortTriggerTestId));

    fireEvent.click(screen.getByTestId(effortTriggerTestId));
    const reopenedEffortSection = screen.getByTestId(effortSectionTestId);
    fireEvent.click(within(reopenedEffortSection).getByRole("button", { name: /^High$/ }));

    expect(onConfigChange).toHaveBeenCalledWith("effort", "high");
    expect(screen.queryByTestId(effortSectionTestId)).toBeNull();
  });
});

describe("ModelConfigSelector filtering", () => {
  it("hides the model filter when there are five or fewer models", () => {
    render(
      <ModelConfigSelector
        modelOptions={makeModelOptions(5)}
        currentModel="model-1"
        onModelChange={() => {}}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));

    expect(screen.queryByPlaceholderText("Filter models...")).toBeNull();
  });

  it("shows the model filter when there are more than five models", () => {
    render(
      <ModelConfigSelector
        modelOptions={makeModelOptions(6)}
        currentModel="model-1"
        onModelChange={() => {}}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));

    expect(screen.getByPlaceholderText("Filter models...")).not.toBeNull();
  });
});

describe("ModelConfigSelector provider descriptions", () => {
  it("shows provider descriptions only inside the selected option submenu", () => {
    render(
      <ModelConfigSelector
        modelOptions={[{ id: providerModelId, name: "GPT-5.6-Sol" }]}
        currentModel={providerModelId}
        onModelChange={() => {}}
        onConfigChange={() => {}}
        configOptions={[
          {
            type: "select",
            id: "effort",
            name: effortOptionName,
            description: optionDescription,
            currentValue: "high",
            options: [
              {
                value: "high",
                name: "High",
                description: "More thorough reasoning for complex tasks.",
              },
            ],
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
    expect(screen.queryByText(optionDescription)).toBeNull();

    fireEvent.click(screen.getByTestId(effortTriggerTestId));
    expect(screen.getByText(optionDescription)).not.toBeNull();
    expect(screen.getByText("More thorough reasoning for complex tasks.")).not.toBeNull();
  });

  it("does not show configuration details when the compact task trigger receives focus", () => {
    render(
      <ModelConfigSelector
        modelOptions={[{ id: providerModelId, name: "GPT-5.6-Sol" }]}
        currentModel={providerModelId}
        onModelChange={() => {}}
        triggerSummary="changed"
        configBaseline={{ effort: "high" }}
        configOptions={[
          {
            type: "select",
            id: "effort",
            name: effortOptionName,
            description: optionDescription,
            currentValue: "low",
            options: [
              {
                value: "low",
                name: "Low",
                description: "Faster responses with less reasoning.",
              },
            ],
          },
        ]}
      />,
    );

    fireEvent.focus(screen.getByRole("button", { name: modelSettingsButtonName }));
    expect(screen.queryByRole("tooltip")).toBeNull();
    expect(screen.queryByText(optionDescription)).toBeNull();
  });
});
