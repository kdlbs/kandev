import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ModelConfigSelector } from "@/components/model-config-selector";

afterEach(() => {
  cleanup();
});

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

    expect(screen.getByRole("button", { name: "Model settings" }).className).toContain(
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

    fireEvent.click(screen.getByRole("button", { name: "Model settings" }));

    const effortTrigger = screen.getByTestId("config-option-trigger-effort");
    expect(effortTrigger.textContent).toContain("Effort");
    expect(effortTrigger.textContent).toContain("Medium");
    expect(screen.queryByTestId("config-option-section-effort")).toBeNull();

    fireEvent.click(effortTrigger);

    const effortSection = screen.getByTestId("config-option-section-effort");
    fireEvent.click(within(effortSection).getByRole("button", { name: "High" }));

    expect(onConfigChange).toHaveBeenCalledWith("effort", "high");
  });
});
