import { cleanup, fireEvent, render, screen, waitFor, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  configOptionToModelOptions,
  ModelConfigSelector,
  type SelectConfigOption,
} from "@/components/model-config-selector";

const responsive = vi.hoisted(() => ({ isMobile: false }));
vi.mock("@/hooks/use-responsive-breakpoint", () => ({
  useResponsiveBreakpoint: () => ({ isMobile: responsive.isMobile }),
}));

beforeEach(() => {
  responsive.isMobile = false;
});

afterEach(() => {
  cleanup();
  responsive.isMobile = false;
});

const MODEL_SETTINGS = "Model settings";
const COMPOSITION_ROW = "model-catalog-composition-row-fusion";
const DIMENSION_LEAD = "model-composition-dimension-lead";
const DIMENSION_SIDEKICK = "model-composition-dimension-sidekick";
const CHOICE_LEAD_B = "model-composition-choice-lead-lead-b";
const CHOICE_SIDEKICK_B = "model-composition-choice-sidekick-sidekick-b";
const LEAD_B_LABEL = "Lead: Lead B";
const ARIA_TRUE = "true";
const ARIA_LABEL = "aria-label";
const COMPOSITION_APPLY = "model-composition-apply";
const BACK_TO_LIST = "model-composition-back";

const modelSettingsButtonName = MODEL_SETTINGS;

function catalogModelOption(currentValue = "pair-a"): SelectConfigOption {
  return {
    type: "select",
    id: "model",
    name: "Model",
    currentValue,
    category: "model",
    options: [
      { value: "pair-a", name: "Balanced" },
      { value: "pair-b", name: "Balanced" },
      { value: "pair-c", name: "Balanced" },
      { value: "solo", name: "Solo" },
    ],
    catalog: {
      version: 1,
      nodes: [
        {
          kind: "group",
          id: "fusion",
          label: "Fusion",
          children: [
            { kind: "choice", value: "pair-a", label: "Balanced" },
            { kind: "choice", value: "pair-b", label: "Balanced" },
          ],
        },
        {
          kind: "group",
          id: "standard",
          label: "Standard",
          children: [{ kind: "choice", value: "solo", label: "Solo" }],
        },
      ],
      compositions: [
        {
          id: "fusion",
          group_id: "fusion",
          label: "Fusion",
          dimensions: [
            {
              id: "lead",
              label: "Lead",
              choices: [
                { value: "lead-a", label: "Lead A" },
                { value: "lead-b", label: "Lead B" },
              ],
            },
            {
              id: "sidekick",
              label: "Sidekick",
              choices: [
                { value: "sidekick-a", label: "Sidekick A" },
                { value: "sidekick-b", label: "Sidekick B" },
              ],
            },
          ],
          bindings: [
            { values: { lead: "lead-a", sidekick: "sidekick-a" }, model: "pair-a" },
            { values: { lead: "lead-b", sidekick: "sidekick-b" }, model: "pair-b" },
          ],
        },
      ],
    },
  };
}

function renderSelector(overrides: {
  onConfigurationApply?: (selection: {
    model: string;
    configOptions: Record<string, string>;
  }) => void;
  onModelChange?: (model: string) => void;
  modelConfig?: SelectConfigOption;
  currentModel?: string;
  resolveConfigurationOptions?: (model: string) => Promise<
    | {
        type: string;
        id: string;
        name: string;
        currentValue: string;
        options: { value: string; name: string }[];
      }[]
    | undefined
  >;
}) {
  const modelConfig =
    overrides.modelConfig ?? catalogModelOption(overrides.currentModel ?? "pair-a");
  return render(
    <TooltipProvider>
      <ModelConfigSelector
        modelOptions={configOptionToModelOptions(modelConfig)}
        currentModel={overrides.currentModel ?? "pair-a"}
        configOptions={[modelConfig]}
        onModelChange={overrides.onModelChange ?? (() => {})}
        onConfigurationApply={overrides.onConfigurationApply}
        resolveConfigurationOptions={overrides.resolveConfigurationOptions}
      />
    </TooltipProvider>,
  );
}

function openPicker() {
  fireEvent.click(screen.getByRole("button", { name: modelSettingsButtonName }));
}

function openFusionComposition() {
  openPicker();
  fireEvent.click(screen.getByTestId(COMPOSITION_ROW));
}

describe("ModelConfigSelector catalog navigation", () => {
  it("collapses a singleton group into a row with path context", () => {
    renderSelector({});
    openPicker();
    expect(screen.getByTestId(COMPOSITION_ROW)).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Standard" })).toBeNull();
    const solo = screen.getByRole("option", { name: "Solo" });
    expect(within(solo).getByTestId("model-row-context").textContent).toBe("Standard");
  });

  it("opens a composition view with one row per catalog dimension", () => {
    renderSelector({});
    openFusionComposition();
    expect(screen.getByTestId("model-composition-view")).toBeTruthy();
    expect(screen.getByTestId(DIMENSION_LEAD)).toBeTruthy();
    expect(screen.getByTestId(DIMENSION_SIDEKICK)).toBeTruthy();
    expect((screen.getByTestId(COMPOSITION_APPLY) as HTMLButtonElement).disabled).toBe(false);
  });

  it("requires every dimension before enabling the configuration", () => {
    renderSelector({ currentModel: "solo" });
    openFusionComposition();
    expect((screen.getByTestId(COMPOSITION_APPLY) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId(DIMENSION_LEAD));
    fireEvent.click(screen.getByTestId(CHOICE_LEAD_B));
    expect((screen.getByTestId(COMPOSITION_APPLY) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId(DIMENSION_SIDEKICK));
    fireEvent.click(screen.getByTestId(CHOICE_SIDEKICK_B));
    expect((screen.getByTestId(COMPOSITION_APPLY) as HTMLButtonElement).disabled).toBe(false);
  });

  it("applies the exact advertised model and dimension values", () => {
    const onConfigurationApply = vi.fn();
    renderSelector({ currentModel: "solo", onConfigurationApply });
    openFusionComposition();
    fireEvent.click(screen.getByTestId(DIMENSION_LEAD));
    fireEvent.click(screen.getByTestId(CHOICE_LEAD_B));
    fireEvent.click(screen.getByTestId(DIMENSION_SIDEKICK));
    fireEvent.click(screen.getByTestId(CHOICE_SIDEKICK_B));
    fireEvent.click(screen.getByTestId(COMPOSITION_APPLY));
    expect(onConfigurationApply).toHaveBeenCalledWith({
      model: "pair-b",
      configOptions: {},
    });
  });

  it("marks a choice invalid after another dimension changes and blocks apply", () => {
    renderSelector({});
    openFusionComposition();
    expect(screen.getByTestId(DIMENSION_LEAD).getAttribute(ARIA_LABEL)).toBe("Lead: Lead A");
    fireEvent.click(screen.getByTestId(DIMENSION_LEAD));
    fireEvent.click(screen.getByTestId(CHOICE_LEAD_B));
    expect(screen.getByTestId("model-composition-error").textContent).toContain("available value");
    expect((screen.getByTestId(COMPOSITION_APPLY) as HTMLButtonElement).disabled).toBe(true);
    expect(screen.getByTestId(DIMENSION_SIDEKICK).getAttribute("aria-invalid")).toBe(ARIA_TRUE);
  });
});

describe("ModelConfigSelector catalog search and draft continuity", () => {
  it("keeps an unfinished draft across Back navigation and dismissal", () => {
    renderSelector({ currentModel: "solo" });
    openFusionComposition();
    fireEvent.click(screen.getByTestId(DIMENSION_LEAD));
    fireEvent.click(screen.getByTestId(CHOICE_LEAD_B));
    expect(screen.getByTestId(DIMENSION_LEAD).getAttribute(ARIA_LABEL)).toBe(LEAD_B_LABEL);

    fireEvent.click(screen.getByTestId(DIMENSION_LEAD));
    fireEvent.click(screen.getByTestId("model-composition-choices-back"));
    expect(screen.getByTestId(DIMENSION_LEAD).getAttribute(ARIA_LABEL)).toBe(LEAD_B_LABEL);

    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    openPicker();
    expect(screen.getByTestId(DIMENSION_LEAD).getAttribute(ARIA_LABEL)).toBe(LEAD_B_LABEL);
  });

  it("does not mutate the owner when the composition is dismissed", () => {
    const onModelChange = vi.fn();
    const onConfigurationApply = vi.fn();
    renderSelector({ currentModel: "solo", onModelChange, onConfigurationApply });
    openFusionComposition();
    fireEvent.click(screen.getByTestId(DIMENSION_LEAD));
    fireEvent.click(screen.getByTestId(CHOICE_LEAD_B));
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    expect(onModelChange).not.toHaveBeenCalled();
    expect(onConfigurationApply).not.toHaveBeenCalled();
  });

  it("keeps the flat provider path unchanged", () => {
    const flat: SelectConfigOption = {
      type: "select",
      id: "model",
      name: "Model",
      currentValue: "flat-a",
      category: "model",
      options: [
        { value: "flat-a", name: "Flat A" },
        { value: "flat-b", name: "Flat B" },
      ],
    };
    renderSelector({ modelConfig: flat, currentModel: "flat-a" });
    openPicker();
    expect(screen.getByRole("option", { name: "Flat A" })).toBeTruthy();
    expect(screen.queryByTestId("model-catalog-composition-row-fusion")).toBeNull();
  });

  it("keeps a described leaf findable by its group path", () => {
    const withDescription = catalogModelOption();
    withDescription.catalog = {
      ...withDescription.catalog!,
      nodes: [
        {
          kind: "group",
          id: "family-a",
          label: "Family A",
          children: [
            {
              kind: "choice",
              value: "fast-model",
              label: "Fast",
              description: "low latency",
            },
          ],
        },
      ],
      compositions: [],
    };
    renderSelector({ modelConfig: withDescription, currentModel: "fast-model" });
    openPicker();
    fireEvent.change(screen.getByPlaceholderText("Filter models..."), {
      target: { value: "Family A" },
    });
    expect(screen.getByRole("option", { name: "Fast" })).toBeTruthy();
  });

  it("keeps an unfinished composition when navigating Back to the model list", () => {
    const onModelChange = vi.fn();
    renderSelector({ currentModel: "solo", onModelChange });
    openFusionComposition();
    fireEvent.click(screen.getByTestId(DIMENSION_LEAD));
    fireEvent.click(screen.getByTestId(CHOICE_LEAD_B));
    fireEvent.click(screen.getByTestId(BACK_TO_LIST));
    expect(screen.getByTestId(COMPOSITION_ROW)).toBeTruthy();
    fireEvent.click(screen.getByTestId(COMPOSITION_ROW));
    expect(screen.getByTestId(DIMENSION_LEAD).getAttribute(ARIA_LABEL)).toBe(LEAD_B_LABEL);
    expect(onModelChange).not.toHaveBeenCalled();
  });
});

describe("ModelConfigSelector candidate options and sizing", () => {
  it("exposes the candidate model's real provider options and applies them", async () => {
    const onConfigurationApply = vi.fn();
    const resolveConfigurationOptions = vi.fn().mockResolvedValue([
      {
        type: "select",
        id: "thinking",
        name: "Thinking",
        currentValue: "low",
        options: [
          { value: "low", name: "Low" },
          { value: "high", name: "High" },
        ],
      },
      {
        type: "select",
        id: "model",
        name: "Model",
        currentValue: "pair-a",
        options: [{ value: "pair-a", name: "Balanced" }],
      },
    ]);
    renderSelector({ onConfigurationApply, resolveConfigurationOptions });
    openFusionComposition();
    await waitFor(() => expect(screen.getByTestId("model-provider-option-thinking")).toBeTruthy());
    expect(screen.queryByTestId("model-provider-option-model")).toBeNull();
    fireEvent.click(screen.getByTestId("model-provider-option-select-thinking"));
    fireEvent.click(await screen.findByRole("option", { name: "High" }));
    fireEvent.click(screen.getByTestId(COMPOSITION_APPLY));
    expect(onConfigurationApply).toHaveBeenCalledWith({
      model: "pair-a",
      configOptions: { thinking: "high" },
    });
  });

  it("blocks apply and offers retry when candidate options fail", async () => {
    const resolveConfigurationOptions = vi.fn().mockResolvedValue(undefined);
    renderSelector({ currentModel: "solo", resolveConfigurationOptions });
    openFusionComposition();
    fireEvent.click(screen.getByTestId(DIMENSION_LEAD));
    fireEvent.click(screen.getByTestId(CHOICE_LEAD_B));
    fireEvent.click(screen.getByTestId(DIMENSION_SIDEKICK));
    fireEvent.click(screen.getByTestId(CHOICE_SIDEKICK_B));
    await waitFor(() => expect(screen.getByTestId("model-provider-options-failed")).toBeTruthy());
    expect((screen.getByTestId(COMPOSITION_APPLY) as HTMLButtonElement).disabled).toBe(true);
    fireEvent.click(screen.getByTestId("model-provider-options-retry"));
    await waitFor(() => expect(resolveConfigurationOptions).toHaveBeenCalledTimes(2));
  });

  it("keeps phone and coarse-pointer composition controls at 44px with desktop density", () => {
    renderSelector({});
    openFusionComposition();
    for (const testId of [
      "model-composition-back",
      "model-composition-dimension-lead",
      "model-composition-apply",
    ]) {
      const control = screen.getByTestId(testId);
      expect(control.className).toContain("min-h-7");
      expect(control.className).toContain("max-md:min-h-11");
      expect(control.className).toContain("[@media(pointer:coarse)]:min-h-11");
    }
  });
});

describe("ModelConfigSelector catalog viewport and availability", () => {
  it("keeps the unfinished draft across a viewport change", () => {
    responsive.isMobile = false;
    const modelConfig = catalogModelOption("solo");
    const view = render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={configOptionToModelOptions(modelConfig)}
          currentModel="solo"
          configOptions={[modelConfig]}
          onModelChange={() => {}}
        />
      </TooltipProvider>,
    );
    openFusionComposition();
    fireEvent.click(screen.getByTestId(DIMENSION_LEAD));
    fireEvent.click(screen.getByTestId(CHOICE_LEAD_B));
    responsive.isMobile = true;
    view.rerender(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={configOptionToModelOptions(modelConfig)}
          currentModel="solo"
          configOptions={[modelConfig]}
          onModelChange={() => {}}
        />
      </TooltipProvider>,
    );
    expect(screen.getByTestId(DIMENSION_LEAD).getAttribute(ARIA_LABEL)).toBe(LEAD_B_LABEL);
    responsive.isMobile = false;
  });

  it("keeps the custom-model row for a catalog-backed selector", () => {
    const modelConfig = catalogModelOption();
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={configOptionToModelOptions(modelConfig)}
          currentModel="pair-a"
          configOptions={[modelConfig]}
          onModelChange={() => {}}
          allowCustomModel
        />
      </TooltipProvider>,
    );
    const trigger = screen.getByRole("button", { name: modelSettingsButtonName });
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
    fireEvent.change(screen.getByPlaceholderText("Filter models..."), {
      target: { value: "unlisted-custom-model" },
    });
    expect(screen.getByTestId("model-config-custom-row")).toBeTruthy();
  });

  it("keeps a withdrawn current model visible and disabled in the catalog", () => {
    const modelConfig = catalogModelOption("removed-model");
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={[
            ...configOptionToModelOptions(modelConfig),
            {
              id: "removed-model",
              name: "Removed model",
              disabled: true,
              disabledReason: "No longer advertised",
            },
          ]}
          currentModel="removed-model"
          configOptions={[modelConfig]}
          onModelChange={() => {}}
        />
      </TooltipProvider>,
    );
    const trigger = screen.getByRole("button", { name: modelSettingsButtonName });
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
    const row = screen.getByRole("option", { name: "Removed model" });
    expect(row.getAttribute("aria-disabled")).toBe(ARIA_TRUE);
  });
});

describe("ModelConfigSelector focus return", () => {
  it("returns focus to the trigger after the picker closes", async () => {
    const modelConfig = catalogModelOption();
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={configOptionToModelOptions(modelConfig)}
          currentModel="pair-a"
          configOptions={[modelConfig]}
          onModelChange={() => {}}
        />
      </TooltipProvider>,
    );
    const trigger = screen.getByRole("button", { name: modelSettingsButtonName });
    fireEvent.pointerDown(trigger);
    fireEvent.click(trigger);
    fireEvent.keyDown(document.activeElement ?? document.body, { key: "Escape" });
    await waitFor(() => expect(document.activeElement).toBe(trigger));
  });
});
