import { describe, expect, it } from "vitest";
import type { ModelCatalog } from "@kandev/types/model-catalog";

import {
  buildCatalogTree,
  composedPairingLabel,
  composeCatalogSelection,
  initialCompositionDraft,
  invalidDraftDimensions,
  isDimensionChoiceAvailable,
  pruneCatalogTree,
  searchCatalogLeaves,
} from "@/lib/model-catalog-selection";
import { normalizeModelCatalog } from "@/lib/model-catalog";

const PLANNER_GROUP = "planner-group";
const SOLO_GROUP = "solo-group";
const PLANNER_DIMENSION = "planner";
const WORKER_SIZE_DIMENSION = "worker-size";
const MODEL_A = "opaque-model-a";
const MODEL_B = "opaque-model-b";
const MODEL_C = "opaque-model-c";
const PLAN_X = "plan-x";
const PLAN_Y = "plan-y";
const SIZE_S = "size-s";
const BALANCED = "Balanced";

function syntheticCatalog(): ModelCatalog {
  return {
    version: 1,
    nodes: [
      {
        kind: "group",
        id: PLANNER_GROUP,
        label: "Planner",
        children: [
          { kind: "choice", value: MODEL_A, label: BALANCED },
          { kind: "choice", value: MODEL_B, label: BALANCED },
        ],
      },
      {
        kind: "group",
        id: SOLO_GROUP,
        label: "Solo",
        children: [{ kind: "choice", value: MODEL_C, label: BALANCED }],
      },
    ],
    compositions: [
      {
        id: "planner",
        group_id: PLANNER_GROUP,
        dimensions: [
          {
            id: PLANNER_DIMENSION,
            label: "Planner",
            choices: [
              { value: PLAN_X, label: "Planner X" },
              { value: PLAN_Y, label: "Planner Y" },
            ],
          },
          {
            id: WORKER_SIZE_DIMENSION,
            label: "Worker size",
            choices: [{ value: SIZE_S, label: "Small" }],
          },
        ],
        bindings: [
          { values: { planner: PLAN_X, [WORKER_SIZE_DIMENSION]: SIZE_S }, model: MODEL_A },
          { values: { planner: PLAN_Y, [WORKER_SIZE_DIMENSION]: SIZE_S }, model: MODEL_B },
        ],
      },
    ],
  };
}

describe("buildCatalogTree", () => {
  it("preserves order and distinguishes repeated labels by path", () => {
    const tree = buildCatalogTree(syntheticCatalog().nodes);
    expect(tree.leaves.map((leaf) => leaf.value)).toEqual([MODEL_A, MODEL_B, MODEL_C]);
    expect(tree.leaves[0].label).toBe(tree.leaves[1].label);
    expect(tree.leaves[0].path.map((segment) => segment.id)).toEqual(["planner-group"]);
    expect(tree.leaves[2].path.map((segment) => segment.id)).toEqual(["solo-group"]);
    expect(new Set(tree.leaves.map((leaf) => leaf.key)).size).toBe(3);
    expect(tree.entries.map((entry) => entry.kind)).toEqual(["group", "group"]);
  });

  it("keeps a singleton group visible with its leaf count", () => {
    const tree = buildCatalogTree(syntheticCatalog().nodes);
    const singleton = tree.entries[1];
    expect(singleton.kind === "group" && singleton.leafCount).toBe(1);
  });

  it("keeps duplicate provider values distinct across groups", () => {
    const catalog: ModelCatalog = {
      version: 1,
      nodes: [
        {
          kind: "group",
          id: "a",
          label: "A",
          children: [{ kind: "choice", value: "same", label: "One" }],
        },
        {
          kind: "group",
          id: "b",
          label: "B",
          children: [{ kind: "choice", value: "same", label: "Two" }],
        },
      ],
    };
    const tree = buildCatalogTree(catalog.nodes);
    expect(tree.leaves).toHaveLength(2);
    expect(tree.leaves[0].key).not.toBe(tree.leaves[1].key);
  });
});

describe("searchCatalogLeaves", () => {
  const catalog = syntheticCatalog();
  const tree = buildCatalogTree(catalog.nodes);

  it("returns matching leaves in stable order with their paths", () => {
    const matches = searchCatalogLeaves(tree, catalog.compositions, "solo");
    expect(matches.map((leaf) => leaf.value)).toEqual([MODEL_C]);
    expect(matches[0].path[0].label).toBe("Solo");
  });

  it("matches a dimension label and dimension value through verified bindings", () => {
    const byDimensionLabel = searchCatalogLeaves(tree, catalog.compositions, "worker size");
    expect(byDimensionLabel.map((leaf) => leaf.value)).toEqual([MODEL_A, MODEL_B]);
    const byDimensionValue = searchCatalogLeaves(tree, catalog.compositions, "planner y");
    expect(byDimensionValue.map((leaf) => leaf.value)).toEqual([MODEL_B]);
  });

  it("returns every leaf for an empty query and none for an unknown one", () => {
    expect(searchCatalogLeaves(tree, catalog.compositions, "  ")).toHaveLength(3);
    expect(searchCatalogLeaves(tree, catalog.compositions, "absent")).toHaveLength(0);
  });

  it("prunes empty groups while keeping the matched structure", () => {
    const allowed = new Set([tree.leaves[2].key]);
    const pruned = pruneCatalogTree(tree, allowed);
    expect(pruned.entries).toHaveLength(1);
    expect(pruned.entries[0].kind === "group" && pruned.entries[0].label).toBe("Solo");
  });
});

describe("composition availability", () => {
  const catalog = syntheticCatalog();
  const composition = catalog.compositions![0];

  it("offers advertised dimension values that participate in a binding", () => {
    expect(isDimensionChoiceAvailable(composition, "planner", PLAN_X)).toBe(true);
    expect(isDimensionChoiceAvailable(composition, "planner", PLAN_Y)).toBe(true);
    expect(isDimensionChoiceAvailable(composition, "planner", "unknown")).toBe(false);
  });

  it("marks a previously valid choice invalid after another dimension changes", () => {
    const draft = { planner: PLAN_X, [WORKER_SIZE_DIMENSION]: SIZE_S };
    expect(composeCatalogSelection(composition, draft, catalog)).toEqual({
      status: "complete",
      model: MODEL_A,
    });

    const changed = { ...draft, [WORKER_SIZE_DIMENSION]: "size-m" };
    expect(invalidDraftDimensions(composition, changed)).toEqual([WORKER_SIZE_DIMENSION]);
    expect(composeCatalogSelection(composition, changed, catalog)).toEqual({ status: "invalid" });
  });

  it("never completes an incomplete or ambiguous draft", () => {
    expect(composeCatalogSelection(composition, { planner: PLAN_X }, catalog)).toEqual({
      status: "incomplete",
    });
    const ambiguous = {
      ...composition,
      bindings: [
        ...composition.bindings,
        { values: { planner: PLAN_X, [WORKER_SIZE_DIMENSION]: SIZE_S }, model: MODEL_B },
      ],
    };
    expect(
      composeCatalogSelection(
        ambiguous,
        { planner: PLAN_X, [WORKER_SIZE_DIMENSION]: SIZE_S },
        catalog,
      ),
    ).toEqual({ status: "ambiguous" });
  });

  it("seeds the draft from the currently selected model", () => {
    expect(initialCompositionDraft(composition, MODEL_B)).toEqual({
      planner: PLAN_Y,
      [WORKER_SIZE_DIMENSION]: SIZE_S,
    });
    expect(initialCompositionDraft(composition, MODEL_C)).toEqual({});
  });

  it("describes the verified pairing behind an advertised model", () => {
    expect(composedPairingLabel(catalog, MODEL_B)).toBe("Planner Y / Small");
    expect(composedPairingLabel(catalog, MODEL_C)).toBeUndefined();
    expect(composedPairingLabel(undefined, MODEL_B)).toBeUndefined();
  });

  it("accepts only a normalized catalog with the same bounds as the backend", () => {
    expect(normalizeModelCatalog(catalog)).toBeDefined();
  });

  it("treats an advertised choice with no binding as unavailable", () => {
    const withUnbound = {
      ...composition,
      dimensions: [
        {
          ...composition.dimensions[0],
          choices: [...composition.dimensions[0].choices, { value: "plan-z", label: "Plan Z" }],
        },
        composition.dimensions[1],
      ],
    };
    expect(isDimensionChoiceAvailable(withUnbound, "planner", "plan-z")).toBe(false);
  });

  it("does not label a model whose value maps to multiple pairings", () => {
    const ambiguous = {
      ...catalog,
      compositions: [
        {
          ...composition,
          bindings: [
            ...composition.bindings,
            { values: { planner: "plan-y", "worker-size": "size-s" }, model: "opaque-model-a" },
          ],
        },
      ],
    };
    expect(composedPairingLabel(ambiguous, "opaque-model-a")).toBeUndefined();
  });
});
