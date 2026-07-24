import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
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

  it("selects a model on pointer click, not just keyboard, and closes the popover", () => {
    const onModelChange = vi.fn();

    render(
      <ModelConfigSelector
        modelOptions={[
          { id: "gpt-5.5", name: "GPT-5.5" },
          { id: "gpt-5.6-sol", name: "GPT-5.6 Sol" },
        ]}
        currentModel="gpt-5.5"
        onModelChange={onModelChange}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));

    const otherRow = screen.getByRole("option", { name: /GPT-5\.6 Sol/ });
    // Regression guard: cmdk's onSelect() (fired on click) only synthesizes a
    // click from touch/pointer input in WebKit-based engines when the item
    // looks interactive; a `cursor-default` item silently breaks pointer
    // selection while leaving keyboard (Enter) selection unaffected.
    expect(otherRow.className).toContain("cursor-pointer");
    expect(otherRow.className).not.toContain("cursor-default");

    fireEvent.click(otherRow);

    expect(onModelChange).toHaveBeenCalledWith("gpt-5.6-sol");
    expect(screen.queryByRole("option", { name: /GPT-5\.6 Sol/ })).toBeNull();
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

  it("shows all current option names and values on compact task trigger focus", () => {
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={[{ id: providerModelId, name: "GPT-5.6-Sol" }]}
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
    expect(tooltip.textContent).toContain("Model: GPT-5.6-Sol");
    expect(tooltip.textContent).toContain("Reasoning Effort: Low");
    expect(tooltip.textContent).toContain("Fast Mode: Off");
    expect(tooltip.textContent).not.toContain(optionDescription);
  });

  it("does not add the task details tooltip to shared selectors", () => {
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={[{ id: providerModelId, name: "GPT-5.6-Sol" }]}
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

function renderTwoModelSelector(onModelChange: (id: string) => void) {
  render(
    <ModelConfigSelector
      modelOptions={[
        { id: "gpt-5.5", name: "GPT-5.5" },
        { id: providerModelId, name: "GPT-5.6 Sol" },
      ]}
      currentModel="gpt-5.5"
      onModelChange={onModelChange}
    />,
  );
  fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
  return screen.getByRole("option", { name: /GPT-5\.6 Sol/ });
}

describe("ModelConfigSelector pointer fallback", () => {
  it("selects a model on a touch pointer-up fallback without double-invoking on the synthesized click", () => {
    const onModelChange = vi.fn();
    const otherRow = renderTwoModelSelector(onModelChange);

    // Regression guard: some WebKit/embedded shells fail to synthesize a click
    // from a touch tap even with cursor-pointer set, so ModelRow also selects
    // on a touch/pen pointerup. Some engines still synthesize the click too;
    // the handler must dedupe so a single tap doesn't fire onModelChange twice.
    fireEvent.pointerUp(otherRow, { pointerType: "touch" });
    fireEvent.click(otherRow);

    expect(onModelChange).toHaveBeenCalledTimes(1);
    expect(onModelChange).toHaveBeenCalledWith(providerModelId);
  });

  it("does not trigger selection on a mouse pointer-up (only click)", () => {
    const onModelChange = vi.fn();
    const otherRow = renderTwoModelSelector(onModelChange);

    fireEvent.pointerUp(otherRow, { pointerType: "mouse" });

    expect(onModelChange).not.toHaveBeenCalled();
  });
});
