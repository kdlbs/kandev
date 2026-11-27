import { cleanup, fireEvent, render, screen } from "@testing-library/react";
import { afterEach, describe, expect, it, vi } from "vitest";

import { ModelConfigSelector } from "@/components/model-config-selector";

afterEach(() => {
  cleanup();
});

const modelSettingsButtonName = "Model settings";
const filterPlaceholder = "Filter models...";
const sonnetModelId = "sonnet";
const customRowTestId = "model-config-custom-row";
const makeModelOptions = (count: number) =>
  Array.from({ length: count }, (_, index) => ({
    id: `model-${index + 1}`,
    name: `Model ${index + 1}`,
  }));

function typeFilter(value: string) {
  fireEvent.change(screen.getByPlaceholderText(filterPlaceholder), { target: { value } });
}

describe("ModelConfigSelector custom model entry", () => {
  it("shows the filter input and no custom row for five or fewer models when allowed", () => {
    render(
      <ModelConfigSelector
        modelOptions={[{ id: sonnetModelId, name: "Sonnet" }]}
        currentModel={sonnetModelId}
        onModelChange={() => {}}
        allowCustomModel
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
    expect(screen.getByPlaceholderText(filterPlaceholder)).not.toBeNull();
    expect(screen.queryByTestId(customRowTestId)).toBeNull();
  });

  it("offers a custom row selecting the typed text verbatim while no option matches", () => {
    const onModelChange = vi.fn();
    render(
      <ModelConfigSelector
        modelOptions={[{ id: sonnetModelId, name: "Sonnet" }]}
        currentModel={sonnetModelId}
        onModelChange={onModelChange}
        allowCustomModel
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
    typeFilter("gpt-6-nova");

    const customRow = screen.getByTestId(customRowTestId);
    fireEvent.click(customRow);
    expect(onModelChange).toHaveBeenCalledWith("gpt-6-nova");
  });

  it("hides the custom row once the typed text exactly matches a listed model", () => {
    render(
      <ModelConfigSelector
        modelOptions={[
          { id: sonnetModelId, name: "Sonnet" },
          { id: "opus", name: "Opus" },
        ]}
        currentModel={sonnetModelId}
        onModelChange={() => {}}
        allowCustomModel
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
    typeFilter("opus");
    expect(screen.queryByTestId(customRowTestId)).toBeNull();
  });

  it("never shows a custom row when the caller does not opt in", () => {
    render(
      <ModelConfigSelector
        modelOptions={makeModelOptions(6)}
        currentModel="model-1"
        onModelChange={() => {}}
      />,
    );

    fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
    typeFilter("nonexistent-model");
    expect(screen.queryByTestId(customRowTestId)).toBeNull();
  });
});
