import { cleanup, fireEvent, render, screen, within } from "@testing-library/react";
import { TooltipProvider } from "@kandev/ui/tooltip";
import { afterEach, beforeEach, describe, expect, it, vi } from "vitest";

import {
  configOptionToModelOptions,
  ModelConfigSelector,
  type SelectConfigOption,
} from "@/components/model-config-selector";
import type { ModelCatalog, ModelCatalogNode } from "@kandev/types/model-catalog";

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
const DEFAULT_LABEL = "Default";
const FILTER_MODELS = "Filter models...";
const MODEL_ROW_CONTEXT = "model-row-context";
const GLM_GROUP = "GLM-5.2";
const GLM_STANDARD = "glm-standard";
const GLM_TURBO = "glm-turbo";
const TRAILING_CHOICE = "trailing-choice";
const NESTED_ONE = "nested-one";
const NESTED_TWO = "nested-two";
const NESTED_ONE_LABEL = "Nested one";
const NESTED_TWO_LABEL = "Nested two";
const FUSION_COMPOSITION_ROW = "model-catalog-composition-row-fusion";
const PROVIDER_MODEL_ID = "loose-model";
const PROVIDER_MODEL_NAME = "Loose model";

function choice(value: string, label: string, description?: string): ModelCatalogNode {
  return { kind: "choice", value, label, ...(description ? { description } : {}) };
}

function group(id: string, label: string, children: ModelCatalogNode[]): ModelCatalogNode {
  return { kind: "group", id, label, children };
}

/** Leading loose choice, singleton shells, a real group, repeats, one composition. */
function catalogNodes(): ModelCatalogNode[] {
  return [
    choice(PROVIDER_MODEL_ID, PROVIDER_MODEL_NAME, "Listed without a family"),
    group("kimi", "Kimi K2.7", [choice("kimi-k2-7", "Kimi K2.7", "200K context")]),
    group("family-a", "Family A", [
      group("family-a-tier", "Tier 1", [
        choice("family-a-default", DEFAULT_LABEL, "Balanced default"),
      ]),
    ]),
    group(GLM_GROUP, GLM_GROUP, [choice(GLM_STANDARD, "Standard"), choice(GLM_TURBO, "Turbo")]),
    group("dup-a", "Alpha family", [choice("dup-model", "Duplicate", "Alpha copy")]),
    group("dup-b", "Beta family", [choice("dup-model", "Duplicate", "Beta copy")]),
    group("fusion", "Fusion", [choice("fusion-pair", "Fusion pair")]),
  ];
}

function fusionComposition(): NonNullable<ModelCatalog["compositions"]>[number] {
  return {
    id: "fusion",
    group_id: "fusion",
    label: "Fusion",
    dimensions: [
      { id: "lead", label: "Lead", choices: [{ value: "lead-a", label: "Lead A" }] },
      {
        id: "sidekick",
        label: "Sidekick",
        choices: [{ value: "sidekick-a", label: "Sidekick A" }],
      },
    ],
    bindings: [{ values: { lead: "lead-a", sidekick: "sidekick-a" }, model: "fusion-pair" }],
  };
}

function catalogModelOption(currentValue = PROVIDER_MODEL_ID): SelectConfigOption {
  return {
    type: "select",
    id: "model",
    name: "Model",
    currentValue,
    category: "model",
    options: [
      { value: PROVIDER_MODEL_ID, name: PROVIDER_MODEL_NAME },
      { value: "kimi-k2-7", name: "Kimi K2.7" },
      { value: "family-a-default", name: DEFAULT_LABEL },
      { value: GLM_STANDARD, name: "Standard" },
      { value: GLM_TURBO, name: "Turbo" },
      { value: "dup-model", name: "Duplicate" },
      { value: "fusion-pair", name: "Fusion pair" },
    ],
    catalog: { version: 1, nodes: catalogNodes(), compositions: [fusionComposition()] },
  };
}

function renderSelector(overrides: {
  onModelChange?: (model: string) => void;
  currentModel?: string;
}) {
  const modelConfig = catalogModelOption(overrides.currentModel ?? PROVIDER_MODEL_ID);
  return render(
    <TooltipProvider>
      <ModelConfigSelector
        modelOptions={configOptionToModelOptions(modelConfig)}
        currentModel={overrides.currentModel ?? PROVIDER_MODEL_ID}
        configOptions={[modelConfig]}
        onModelChange={overrides.onModelChange ?? (() => {})}
      />
    </TooltipProvider>,
  );
}

function openPicker() {
  fireEvent.click(screen.getByRole("button", { name: MODEL_SETTINGS }));
}

function optionNames() {
  return screen.getAllByRole("option").map((option) => option.getAttribute("aria-label"));
}

describe("ModelConfigSelector singleton projection", () => {
  it("renders one row per ordinary singleton without its redundant heading", () => {
    renderSelector({});
    openPicker();
    expect(screen.queryByRole("group", { name: "Kimi K2.7" })).toBeNull();
    const kimi = screen.getByRole("option", { name: "Kimi K2.7" });
    expect(within(kimi).getByText("200K context")).toBeTruthy();
    // The only removed label duplicated the model label, so no context repeats it.
    expect(within(kimi).queryByTestId(MODEL_ROW_CONTEXT)).toBeNull();
  });

  it("keeps the removed nested path as accessible secondary detail", () => {
    renderSelector({});
    openPicker();
    const fallback = screen.getByRole("option", { name: DEFAULT_LABEL });
    const detail = within(fallback).getByTestId(MODEL_ROW_CONTEXT);
    expect(detail.textContent).toBe("Family A / Tier 1");
    expect(within(fallback).getByText("Balanced default")).toBeTruthy();
    const describedBy = fallback.getAttribute("aria-describedby");
    expect(describedBy).not.toBeNull();
    expect(document.getElementById(describedBy!.split(" ")[0])?.textContent).toBe(
      "Balanced default",
    );
    expect(document.getElementById(describedBy!.split(" ")[1])?.textContent).toBe(
      "Family A / Tier 1",
    );
  });

  it("renders loose choices, projected rows, groups, and compositions in source order", () => {
    renderSelector({});
    openPicker();
    expect(optionNames()).toEqual([
      PROVIDER_MODEL_NAME,
      "Kimi K2.7",
      DEFAULT_LABEL,
      "Standard",
      "Turbo",
      "Duplicate",
      "Duplicate",
      "Configure Fusion",
    ]);
    expect(screen.getByRole("group", { name: GLM_GROUP })).toBeTruthy();
    expect(screen.getByTestId(FUSION_COMPOSITION_ROW)).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Fusion" })).toBeNull();
  });

  it("keeps repeated values as distinct occurrences under their own paths", () => {
    const onModelChange = vi.fn();
    renderSelector({ currentModel: GLM_STANDARD, onModelChange });
    openPicker();
    const duplicates = screen.getAllByRole("option", { name: "Duplicate" });
    expect(duplicates).toHaveLength(2);
    expect(within(duplicates[0]).getByTestId(MODEL_ROW_CONTEXT).textContent).toBe("Alpha family");
    expect(within(duplicates[1]).getByTestId(MODEL_ROW_CONTEXT).textContent).toBe("Beta family");
    expect(within(duplicates[0]).getByText("Alpha copy")).toBeTruthy();
    expect(within(duplicates[1]).getByText("Beta copy")).toBeTruthy();
    expect(
      within(screen.getByTestId("model-config-selected-row")).getByText("Standard"),
    ).toBeTruthy();
    fireEvent.click(duplicates[1]);
    expect(onModelChange).toHaveBeenCalledWith("dup-model");
  });

  it("keeps a single-binding composition as a configuration entry", () => {
    renderSelector({});
    openPicker();
    expect(screen.getByTestId(FUSION_COMPOSITION_ROW)).toBeTruthy();
    expect(screen.queryByRole("option", { name: "Fusion pair" })).toBeNull();
  });

  it("keeps a multi-choice heading when search leaves one visible result", () => {
    renderSelector({});
    openPicker();
    fireEvent.change(screen.getByPlaceholderText(FILTER_MODELS), {
      target: { value: "Turbo" },
    });
    const group = screen.getByRole("group", { name: GLM_GROUP });
    expect(within(group).getAllByRole("option")).toHaveLength(1);
    expect(screen.getAllByRole("option")).toHaveLength(1);
    expect(screen.getByRole("option", { name: "Turbo" })).toBeTruthy();
  });

  it("keeps collapsed rows searchable by their removed path", () => {
    renderSelector({});
    openPicker();
    fireEvent.change(screen.getByPlaceholderText(FILTER_MODELS), {
      target: { value: "Family A" },
    });
    expect(screen.getByRole("option", { name: DEFAULT_LABEL })).toBeTruthy();
  });
});

describe("ModelConfigSelector singleton projection on the phone shell", () => {
  it("uses the same projection in the single mobile sheet", () => {
    responsive.isMobile = true;
    renderSelector({});
    openPicker();
    expect(screen.getByTestId("model-config-sheet-scroll")).toBeTruthy();
    expect(screen.queryByRole("group", { name: "Kimi K2.7" })).toBeNull();
    expect(optionNames()).toEqual([
      PROVIDER_MODEL_NAME,
      "Kimi K2.7",
      DEFAULT_LABEL,
      "Standard",
      "Turbo",
      "Duplicate",
      "Duplicate",
      "Configure Fusion",
    ]);
    const fallback = screen.getByRole("option", { name: DEFAULT_LABEL });
    expect(within(fallback).getByTestId(MODEL_ROW_CONTEXT).textContent).toBe("Family A / Tier 1");
  });
});

describe("ModelConfigSelector flat catalog identity", () => {
  it("keeps the flat provider path free of catalog context", () => {
    const flat: SelectConfigOption = {
      type: "select",
      id: "model",
      name: "Model",
      currentValue: "sonnet",
      category: "model",
      options: [{ value: "sonnet", name: "Sonnet" }],
    };
    render(
      <TooltipProvider>
        <ModelConfigSelector
          modelOptions={configOptionToModelOptions(flat)}
          currentModel="sonnet"
          configOptions={[flat]}
          onModelChange={() => {}}
        />
      </TooltipProvider>,
    );
    openPicker();
    expect(screen.getByRole("group", { name: "Model" })).toBeTruthy();
    expect(screen.queryByTestId(MODEL_ROW_CONTEXT)).toBeNull();
  });
});

const LONG_PATH_ALPHA = "Platform family release channel alpha";
const LONG_ALPHA_MODEL = "long-alpha-model";
const LONG_BETA_MODEL = "long-beta-model";
const ROOT_PAIR = "root-pair";
const NESTED_PAIR = "nested-pair";
const LONG_PATH_BETA = "Platform family release channel beta";

/** Two singleton families whose paths share a long prefix and differ at the end. */
function longPathCatalogOption(): SelectConfigOption {
  return {
    type: "select",
    id: "model",
    name: "Model",
    currentValue: LONG_ALPHA_MODEL,
    category: "model",
    options: [
      { value: LONG_ALPHA_MODEL, name: DEFAULT_LABEL },
      { value: LONG_BETA_MODEL, name: DEFAULT_LABEL },
    ],
    catalog: {
      version: 1,
      nodes: [
        group("long-alpha", LONG_PATH_ALPHA, [
          choice(LONG_ALPHA_MODEL, DEFAULT_LABEL, "Alpha default"),
        ]),
        group("long-beta", LONG_PATH_BETA, [
          choice(LONG_BETA_MODEL, DEFAULT_LABEL, "Beta default"),
        ]),
      ],
      compositions: [],
    },
  };
}

/**
 * Root group id `a/b` and nested group id `b` both produce the path key
 * `a/b`, so the two compositions used to share one command identity.
 */
function slashCompositionCatalogOption(): SelectConfigOption {
  return {
    type: "select",
    id: "model",
    name: "Model",
    currentValue: ROOT_PAIR,
    category: "model",
    options: [
      { value: ROOT_PAIR, name: "Root pairing" },
      { value: NESTED_PAIR, name: "Nested pairing" },
    ],
    catalog: {
      version: 1,
      nodes: [
        group("a/b", "Root Fusion", [choice(ROOT_PAIR, "Root pairing")]),
        group("a", "Family A", [
          group("b", "Nested Fusion", [choice(NESTED_PAIR, "Nested pairing")]),
        ]),
      ],
      compositions: [
        {
          id: "root",
          group_id: "a/b",
          dimensions: [
            { id: "root-choice", label: "Root choice", choices: [{ value: "r1", label: "R1" }] },
          ],
          bindings: [{ values: { "root-choice": "r1" }, model: ROOT_PAIR }],
        },
        {
          id: "nested",
          group_id: "b",
          dimensions: [
            {
              id: "nested-choice",
              label: "Nested choice",
              choices: [{ value: "n1", label: "N1" }],
            },
          ],
          bindings: [{ values: { "nested-choice": "n1" }, model: NESTED_PAIR }],
        },
      ],
    },
  };
}

function renderCatalogOption(modelConfig: SelectConfigOption, currentModel: string) {
  return render(
    <TooltipProvider>
      <ModelConfigSelector
        modelOptions={configOptionToModelOptions(modelConfig)}
        currentModel={currentModel}
        configOptions={[modelConfig]}
        onModelChange={() => {}}
      />
    </TooltipProvider>,
  );
}

describe("ModelConfigSelector composition command identity", () => {
  it("keeps slash-containing nested compositions separately selectable by keyboard", () => {
    renderCatalogOption(slashCompositionCatalogOption(), ROOT_PAIR);
    openPicker();
    const rows = screen.getAllByTestId(/^model-catalog-composition-row-/);
    expect(rows).toHaveLength(2);
    const values = rows.map((row) => row.getAttribute("data-value"));
    expect(new Set(values).size).toBe(2);

    const input = screen.getByPlaceholderText(FILTER_MODELS);
    fireEvent.keyDown(input, { key: "ArrowDown" });
    const selected = rows.filter((row) => row.getAttribute("aria-selected") === "true");
    expect(selected).toHaveLength(1);

    fireEvent.keyDown(input, { key: "Enter" });
    expect(screen.getByTestId("model-composition-dimension-nested-choice")).toBeTruthy();
    expect(screen.queryByTestId("model-composition-dimension-root-choice")).toBeNull();

    fireEvent.click(screen.getByTestId("model-composition-back"));
    fireEvent.click(screen.getByTestId("model-catalog-composition-row-a/b"));
    expect(screen.getByTestId("model-composition-dimension-root-choice")).toBeTruthy();
  });
});

describe("ModelConfigSelector long-path phone context", () => {
  it("wraps both long disambiguating paths instead of ellipsizing them", () => {
    responsive.isMobile = true;
    renderCatalogOption(longPathCatalogOption(), LONG_ALPHA_MODEL);
    openPicker();
    const rows = screen.getAllByRole("option", { name: DEFAULT_LABEL });
    expect(rows).toHaveLength(2);
    const details = rows.map((row) => within(row).getByTestId(MODEL_ROW_CONTEXT));
    expect(details.map((detail) => detail.textContent)).toEqual([LONG_PATH_ALPHA, LONG_PATH_BETA]);
    for (const detail of details) {
      expect(detail.className).toContain("max-md:whitespace-normal");
      expect(detail.className).toContain("max-md:break-words");
      expect(detail.className).toContain("[@media(pointer:coarse)]:whitespace-normal");
    }
    expect(within(rows[0]).getByText("Alpha default")).toBeTruthy();
    expect(within(rows[1]).getByText("Beta default")).toBeTruthy();
  });
});

describe("ModelConfigSelector catalog filtering and headings", () => {
  it("does not match opaque command identities while filtering", () => {
    renderSelector({});
    openPicker();
    fireEvent.change(screen.getByPlaceholderText(FILTER_MODELS), {
      target: { value: "row" },
    });
    expect(screen.queryAllByRole("option")).toHaveLength(0);
    expect(screen.getByText("No models found.")).toBeTruthy();

    fireEvent.change(screen.getByPlaceholderText(FILTER_MODELS), {
      target: { value: "composition" },
    });
    expect(screen.queryAllByRole("option")).toHaveLength(0);
  });

  it("labels top-level loose choices that follow a group", () => {
    const catalog: SelectConfigOption = {
      type: "select",
      id: "model",
      name: "Model",
      currentValue: TRAILING_CHOICE,
      category: "model",
      options: [
        { value: GLM_STANDARD, name: "Standard" },
        { value: GLM_TURBO, name: "Turbo" },
        { value: TRAILING_CHOICE, name: "Trailing choice" },
      ],
      catalog: {
        version: 1,
        nodes: [
          group(GLM_GROUP, GLM_GROUP, [
            choice(GLM_STANDARD, "Standard"),
            choice(GLM_TURBO, "Turbo"),
          ]),
          choice(TRAILING_CHOICE, "Trailing choice"),
        ],
      },
    };
    renderCatalogOption(catalog, TRAILING_CHOICE);
    openPicker();
    const headingGroup = screen.getByRole("group", { name: "Model" });
    expect(within(headingGroup).getByRole("option", { name: "Trailing choice" })).toBeTruthy();
    expect(within(headingGroup).queryByRole("option", { name: "Standard" })).toBeNull();
  });
});

describe("ModelConfigSelector nested group filtering", () => {
  it("keeps a nested-group-only parent visible while searching a nested choice", () => {
    const catalog: SelectConfigOption = {
      type: "select",
      id: "model",
      name: "Model",
      currentValue: NESTED_ONE,
      category: "model",
      options: [
        { value: NESTED_ONE, name: NESTED_ONE_LABEL },
        { value: NESTED_TWO, name: NESTED_TWO_LABEL },
      ],
      catalog: {
        version: 1,
        nodes: [
          group("family", "Family A", [
            group("tier", "Tier 1", [
              choice(NESTED_ONE, NESTED_ONE_LABEL),
              choice(NESTED_TWO, NESTED_TWO_LABEL),
            ]),
          ]),
        ],
      },
    };
    renderCatalogOption(catalog, NESTED_ONE);
    openPicker();
    fireEvent.change(screen.getByPlaceholderText(FILTER_MODELS), {
      target: { value: NESTED_TWO_LABEL },
    });
    expect(screen.getByRole("option", { name: NESTED_TWO_LABEL })).toBeTruthy();
    expect(screen.getByRole("group", { name: "Family A" })).toBeTruthy();
    expect(screen.getByRole("group", { name: "Tier 1" })).toBeTruthy();
  });
});
