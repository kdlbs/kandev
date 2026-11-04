import { describe, expect, it } from "vitest";

import {
  catalogComposition,
  catalogGroup,
  catalogHasChoice,
  leafNodes,
  normalizeModelCatalog,
  resolveModelCatalogDraft,
} from "@/lib/model-catalog";

const FUSION_GROUP = "fusion";
const PAIR_A = "pair-a";
const LEAD_A = "lead-a";
const SIDEKICK_A = "sidekick-a";

function catalogValue(): unknown {
  return {
    version: 1,
    nodes: [
      {
        kind: "group",
        id: FUSION_GROUP,
        label: "Fusion",
        children: [
          {
            kind: "choice",
            value: PAIR_A,
            label: "Balanced",
            description: "Lead A with Sidekick A",
          },
          { kind: "choice", value: "pair-b", label: "Balanced" },
        ],
      },
      {
        kind: "group",
        id: "standard",
        label: "Standard",
        children: [{ kind: "choice", value: "solo", label: "Balanced" }],
      },
    ],
    compositions: [
      {
        id: FUSION_GROUP,
        group_id: FUSION_GROUP,
        label: "Fusion",
        dimensions: [
          { id: "lead", label: "Lead", choices: [{ value: LEAD_A, label: "Lead A" }] },
          {
            id: "sidekick",
            label: "Sidekick",
            choices: [{ value: SIDEKICK_A, label: "Sidekick A" }],
          },
        ],
        bindings: [{ values: { lead: LEAD_A, sidekick: SIDEKICK_A }, model: PAIR_A }],
      },
    ],
  };
}

describe("normalizeModelCatalog", () => {
  it("accepts a bounded provider-neutral catalog", () => {
    const catalog = normalizeModelCatalog(catalogValue());
    expect(catalog?.version).toBe(1);
    expect(catalog?.nodes).toHaveLength(2);
    expect(catalog?.compositions?.[0].bindings[0].model).toBe(PAIR_A);
  });

  it("discards malformed and out-of-bounds data as a whole", () => {
    const wrongVersion = { ...(catalogValue() as Record<string, unknown>), version: 2 };
    expect(normalizeModelCatalog(wrongVersion)).toBeUndefined();
    expect(normalizeModelCatalog(null)).toBeUndefined();

    const duplicateGroups = catalogValue() as { nodes: { id: string }[] };
    duplicateGroups.nodes[1].id = "fusion";
    expect(normalizeModelCatalog(duplicateGroups)).toBeUndefined();

    const unknownModel = catalogValue() as {
      compositions: { bindings: { model: string }[] }[];
    };
    unknownModel.compositions[0].bindings[0].model = "synthetic";
    expect(normalizeModelCatalog(unknownModel)).toBeUndefined();

    const missingDimension = catalogValue() as {
      compositions: { bindings: { values: Record<string, string> }[] }[];
    };
    missingDimension.compositions[0].bindings[0].values = { lead: LEAD_A };
    expect(normalizeModelCatalog(missingDimension)).toBeUndefined();
  });

  it("rejects catalogs deeper than the shared bound", () => {
    const deep = {
      version: 1,
      nodes: [
        {
          kind: "group",
          id: "g",
          label: "G",
          children: [{ kind: "choice", value: "a", label: "A" }],
        },
      ],
    };
    let node: Record<string, unknown> = deep.nodes[0];
    for (let depth = 0; depth < 6; depth += 1) {
      node.children = [{ kind: "group", id: `g${depth}`, label: "G", children: node.children }];
      node = (node.children as Record<string, unknown>[])[0];
    }
    expect(normalizeModelCatalog(deep)).toBeUndefined();
  });
});

describe("catalog lookups", () => {
  it("finds groups, leaves, and compositions", () => {
    const catalog = normalizeModelCatalog(catalogValue());
    expect(catalogGroup(catalog, "standard")?.label).toBe("Standard");
    expect(catalogGroup(catalog, "missing")).toBeUndefined();
    expect(catalogComposition(catalog, "fusion")?.id).toBe("fusion");
    expect(leafNodes(catalog?.nodes ?? []).map((node) => node.value)).toEqual([
      PAIR_A,
      "pair-b",
      "solo",
    ]);
    expect(catalogHasChoice(catalog, "solo")).toBe(true);
    expect(catalogHasChoice(catalog, "unknown")).toBe(false);
  });
});

describe("resolveModelCatalogDraft", () => {
  const catalog = normalizeModelCatalog(catalogValue());
  const composition = catalog?.compositions?.[0];

  it("resolves only a complete exact pairing to an advertised model", () => {
    if (!composition) throw new Error("fixture composition missing");
    expect(
      resolveModelCatalogDraft(composition, { lead: LEAD_A, sidekick: SIDEKICK_A }, catalog),
    ).toEqual({
      status: "complete",
      model: PAIR_A,
    });
    expect(resolveModelCatalogDraft(composition, { lead: LEAD_A }, catalog)).toEqual({
      status: "incomplete",
      missing: ["sidekick"],
      invalid: [],
    });
    expect(
      resolveModelCatalogDraft(composition, { lead: "lead-z", sidekick: SIDEKICK_A }, catalog),
    ).toEqual({ status: "invalid", missing: [], invalid: ["lead"] });
  });

  it("rejects draft keys that are not composition dimensions", () => {
    if (!composition) throw new Error("fixture composition missing");
    expect(
      resolveModelCatalogDraft(
        composition,
        { lead: LEAD_A, sidekick: SIDEKICK_A, retired: "value" },
        catalog,
      ),
    ).toEqual({ status: "invalid", missing: [], invalid: ["retired"] });
  });

  it("never resolves an ambiguous or unadvertised model", () => {
    const ambiguous = {
      id: "ambiguous",
      group_id: FUSION_GROUP,
      dimensions: [{ id: "lead", label: "Lead", choices: [{ value: "a", label: "A" }] }],
      bindings: [
        { values: { lead: "a" }, model: PAIR_A },
        { values: { lead: "a" }, model: "pair-b" },
      ],
    };
    expect(resolveModelCatalogDraft(ambiguous, { lead: "a" }, catalog)).toEqual({
      status: "ambiguous",
    });
    expect(resolveModelCatalogDraft(ambiguous, { lead: "a" })).toEqual({ status: "ambiguous" });

    const unadvertised = {
      id: "unadvertised",
      group_id: FUSION_GROUP,
      dimensions: [{ id: "lead", label: "Lead", choices: [{ value: "a", label: "A" }] }],
      bindings: [{ values: { lead: "a" }, model: "removed-model" }],
    };
    expect(resolveModelCatalogDraft(unadvertised, { lead: "a" }, catalog)?.status).not.toBe(
      "complete",
    );
  });

  it("rejects orphaned compositions and duplicate bindings", () => {
    const orphan = catalogValue() as { compositions: { group_id: string }[] };
    orphan.compositions[0].group_id = "missing-group";
    expect(normalizeModelCatalog(orphan)).toBeUndefined();

    const duplicate = catalogValue() as { compositions: { bindings: unknown[] }[] };
    duplicate.compositions[0].bindings = [
      ...duplicate.compositions[0].bindings,
      ...duplicate.compositions[0].bindings,
    ];
    expect(normalizeModelCatalog(duplicate)).toBeUndefined();
  });

  it("keeps prototype-like dimension ids as ordinary data", () => {
    const catalog = catalogValue() as {
      nodes: { children?: unknown[] }[];
      compositions: {
        dimensions: { id: string }[];
        bindings: { values: Record<string, string> }[];
      }[];
    };
    catalog.compositions[0].dimensions[0].id = "__proto__";
    catalog.compositions[0].bindings[0].values = JSON.parse(
      '{"__proto__":"lead-a","sidekick":"sidekick-a"}',
    ) as Record<string, string>;
    expect(normalizeModelCatalog(catalog)).toBeDefined();
  });
});
