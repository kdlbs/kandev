import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, describe, expect, it, vi } from "vitest";

import {
  configOptionToModelOptions,
  ModelConfigSelector,
  type SelectConfigOption,
} from "@/components/model-config-selector";

afterEach(() => {
  cleanup();
});

const effortSectionTestId = "config-option-section-effort";
const effortTriggerTestId = "config-option-trigger-effort";
const modelSettingsButtonName = "Model settings";
const providerModelId = "gpt-5.6-sol";
const providerModelName = "GPT-5.6-Sol";
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

describe("ModelConfigSelector selected options", () => {
  it("puts the current model and nested config value first", () => {
    render(
      <ModelConfigSelector
        modelOptions={[
          { id: "model-1", name: "Model 1" },
          { id: "model-2", name: "Model 2" },
          { id: "model-3", name: "Model 3" },
        ]}
        currentModel="model-2"
        onModelChange={() => {}}
        onConfigChange={() => {}}
        configOptions={[
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
    expect(screen.getAllByRole("option").map((option) => option.textContent?.trim())).toEqual([
      "Model 2",
      "Model 1",
      "Model 3",
    ]);
    expect(screen.getByTestId("model-config-selected-row").className).toContain("bg-card");
    expect(screen.getByTestId("model-config-selected-row").className).toContain(
      "border-primary/50",
    );

    fireEvent.click(screen.getByTestId(effortTriggerTestId));
    const effortSection = screen.getByTestId(effortSectionTestId);
    expect(
      within(effortSection)
        .getAllByRole("button")
        .map((button) => button.textContent?.trim()),
    ).toEqual(["Medium", "Low", "High"]);
    expect(within(effortSection).getByRole("button", { name: "Medium" }).className).toContain(
      "bg-card",
    );
  });
});

describe("ModelConfigSelector loading behavior", () => {
  it("keeps the picker open after selecting a model without existing dependent options", () => {
    const onModelChange = vi.fn();

    render(
      <ModelConfigSelector
        modelOptions={[
          { id: "model-a", name: "Model A" },
          { id: "model-b", name: "Model B" },
        ]}
        currentModel="model-a"
        onModelChange={onModelChange}
        keepOpenOnModelChange
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
    fireEvent.click(screen.getByRole("option", { name: /Model B/ }));

    expect(onModelChange).toHaveBeenCalledWith("model-b");
    expect(screen.getByRole("option", { name: /Model B/ })).toBeTruthy();
  });

  it("replaces dependent options with a loading row until the snapshot resolves", () => {
    const { rerender } = render(
      <ModelConfigSelector
        modelOptions={[{ id: "model-a", name: "Model A" }]}
        currentModel="model-a"
        onModelChange={() => {}}
        configOptions={[
          {
            type: "select",
            id: "effort",
            name: "Effort",
            currentValue: "medium",
            options: [{ value: "medium", name: "Medium" }],
          },
        ]}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
    expect(screen.getByTestId(effortTriggerTestId)).toBeTruthy();

    rerender(
      <ModelConfigSelector
        modelOptions={[{ id: "model-b", name: "Model B" }]}
        currentModel="model-b"
        onModelChange={() => {}}
        configOptions={[
          {
            type: "select",
            id: "effort",
            name: "Effort",
            currentValue: "medium",
            options: [{ value: "medium", name: "Medium" }],
          },
        ]}
        configOptionsLoading
        keepOpenOnModelChange
      />,
    );

    const loadingRow = screen.getByTestId("model-config-options-loading");
    expect(loadingRow.getAttribute("role")).toBe("status");
    expect(screen.getByRole("status", { name: "Loading model options…" })).toBe(loadingRow);
    expect(screen.queryByTestId(effortTriggerTestId)).toBeNull();
    const selectedModelRow = screen.getByTestId("model-config-selected-row");
    expect(selectedModelRow.querySelector("svg.tabler-icon-loader")).toBeTruthy();
    expect(selectedModelRow.querySelector("svg.tabler-icon-check.absolute")).toBeNull();

    rerender(
      <ModelConfigSelector
        modelOptions={[{ id: "model-b", name: "Model B" }]}
        currentModel="model-b"
        onModelChange={() => {}}
        configOptions={[
          {
            type: "select",
            id: "effort",
            name: "Effort",
            currentValue: "max",
            options: [{ value: "max", name: "Max" }],
          },
        ]}
        keepOpenOnModelChange
      />,
    );

    expect(screen.queryByTestId("model-config-options-loading")).toBeNull();
    expect(selectedModelRow.querySelector("svg.tabler-icon-loader")).toBeNull();
    expect(selectedModelRow.querySelector("svg.tabler-icon-check.absolute")).toBeTruthy();
    expect(screen.getByTestId(effortTriggerTestId).textContent).toContain("Max");
  });
});

describe("ModelConfigSelector loading guard", () => {
  it("keeps the picker open when options are loading without the explicit open flag", () => {
    const onModelChange = vi.fn();

    render(
      <ModelConfigSelector
        modelOptions={[
          { id: "model-a", name: "Model A" },
          { id: "model-b", name: "Model B" },
        ]}
        currentModel="model-a"
        onModelChange={onModelChange}
        configOptionsLoading
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
    fireEvent.click(screen.getByRole("option", { name: /Model B/ }));

    expect(onModelChange).toHaveBeenCalledWith("model-b");
    expect(screen.getByTestId("model-config-options-loading")).toBeTruthy();
  });
});

describe("ModelConfigSelector filtering", () => {
  it("deduplicates repeated model ids from provider config", () => {
    const modelConfig: SelectConfigOption = {
      type: "select",
      id: "model",
      name: "Model",
      currentValue: providerModelId,
      category: "model",
      options: [
        {
          value: providerModelId,
          name: providerModelName,
          description: "Latest frontier agentic coding model.",
        },
        {
          value: "gpt-5.6-terra",
          name: "GPT-5.6-Terra",
          description: "Balanced agentic coding model for everyday work.",
        },
        {
          value: "gpt-5.6-terra",
          name: "GPT-5.6-Terra",
          description: "Balanced agentic coding model for everyday work.",
        },
      ],
    };

    expect(configOptionToModelOptions(modelConfig).map((model) => model.id)).toEqual([
      providerModelId,
      "gpt-5.6-terra",
    ]);
  });

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
        modelOptions={[{ id: providerModelId, name: providerModelName }]}
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

  it("shows all current option names and values on compact task trigger focus", () => {
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={[{ id: providerModelId, name: providerModelName }]}
          currentModel={providerModelId}
          onModelChange={() => {}}
          triggerSummary="changed"
          configBaseline={{ effort: "high", fast_mode: "off" }}
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
            {
              type: "select",
              id: "fast_mode",
              name: "Fast Mode",
              currentValue: "off",
              options: [{ value: "off", name: "Off" }],
            },
          ]}
        />
      </TooltipProvider>,
    );

    fireEvent.focus(screen.getByRole("button", { name: modelSettingsButtonName }));

    const tooltip = screen.getByRole("tooltip");
    expect(tooltip.textContent).toContain(`Model: ${providerModelName}`);
    expect(tooltip.textContent).toContain("Reasoning Effort: Low");
    expect(tooltip.textContent).toContain("Fast Mode: Off");
    expect(tooltip.textContent).not.toContain(optionDescription);
  });

  it("does not add the task details tooltip to shared selectors", () => {
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={[{ id: providerModelId, name: providerModelName }]}
          currentModel={providerModelId}
          onModelChange={() => {}}
          configOptions={[
            {
              type: "select",
              id: "effort",
              name: effortOptionName,
              currentValue: "low",
              options: [{ value: "low", name: "Low" }],
            },
          ]}
        />
      </TooltipProvider>,
    );

    fireEvent.focus(screen.getByRole("button", { name: modelSettingsButtonName }));
    expect(screen.queryByRole("tooltip")).toBeNull();
  });
});

describe("ModelConfigSelector fallback suffix", () => {
  it("appends the suffix to the trigger label when provided", () => {
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={[{ id: "gpt-5", name: "GPT-5" }]}
          currentModel="gpt-5"
          onModelChange={() => {}}
          currentModelSuffix=" (fallback)"
          triggerSummary="changed"
        />
      </TooltipProvider>,
    );

    expect(screen.getByRole("button", { name: modelSettingsButtonName }).textContent).toContain(
      "GPT-5 (fallback)",
    );
  });

  it("renders a config-option model with the suffix on the trigger", () => {
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={[{ id: "gpt-5", name: "GPT-5" }]}
          currentModel="gpt-5"
          onModelChange={() => {}}
          currentModelSuffix=" (fallback)"
          triggerSummary="changed"
          configOptions={[
            {
              type: "select",
              id: "model",
              name: "Model",
              currentValue: "gpt-5",
              category: "model",
              options: [{ value: "gpt-5", name: "GPT-5" }],
            },
          ]}
        />
      </TooltipProvider>,
    );

    expect(screen.getByRole("button", { name: modelSettingsButtonName }).textContent).toContain(
      "GPT-5 (fallback)",
    );
  });
});

describe("ModelConfigSelector disabled (gone) models", () => {
  it("renders a disabled model greyed out with its reason and blocks selection", () => {
    const onModelChange = vi.fn();

    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={[
            { id: "claude-sonnet", name: "Claude Sonnet" },
            {
              id: "claude-gone",
              name: "Claude Gone",
              disabled: true,
              disabledReason: "no longer available",
            },
          ]}
          currentModel="claude-gone"
          onModelChange={onModelChange}
        />
      </TooltipProvider>,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));

    const disabledItem = screen.getByRole("option", { name: /claude gone/i });
    expect(disabledItem.className).toContain("opacity-40");
    expect(disabledItem.getAttribute("aria-disabled")).toBe("true");

    fireEvent.click(disabledItem);
    expect(onModelChange).not.toHaveBeenCalled();
  });

  it("keeps the disabled (gone) model as the current value display", () => {
    const onModelChange = vi.fn();
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={[
            {
              id: "claude-gone",
              name: "Claude Gone",
              disabled: true,
              disabledReason: "no longer available",
            },
            { id: "gpt-5", name: "GPT-5" },
          ]}
          currentModel="claude-gone"
          onModelChange={onModelChange}
        />
      </TooltipProvider>,
    );

    // The trigger still shows the configured (gone) model name so the user
    // sees what was configured.
    expect(screen.getByRole("button", { name: modelSettingsButtonName }).textContent).toContain(
      "Claude Gone",
    );
  });
});
