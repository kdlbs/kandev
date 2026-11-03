import type {
  ModelCatalog,
  ModelCatalogComposition,
  ModelCatalogDimension,
  ModelCatalogDraft,
  ModelCatalogNode,
} from "@kandev/types/model-catalog";

import { leafNodes, resolveModelCatalogDraft } from "@/lib/model-catalog";

export type CatalogPathSegment = { id: string; label: string };

export type CatalogLeafEntry = {
  kind: "choice";
  /** Unique scoped identity: repeated labels and values stay distinguishable. */
  key: string;
  value: string;
  label: string;
  description?: string;
  path: CatalogPathSegment[];
};

export type CatalogGroupEntry = {
  kind: "group";
  key: string;
  id: string;
  label: string;
  description?: string;
  path: CatalogPathSegment[];
  children: CatalogEntry[];
  leafCount: number;
};

export type CatalogEntry = CatalogGroupEntry | CatalogLeafEntry;

export type CatalogTree = {
  entries: CatalogEntry[];
  /** Every leaf in stable document order. */
  leaves: CatalogLeafEntry[];
};

/**
 * Flattens a catalog into one ordered tree. Repeated labels keep distinct
 * paths, and repeated provider values keep distinct keys, so the renderer never
 * deduplicates by display text.
 */
export function buildCatalogTree(nodes: ModelCatalogNode[]): CatalogTree {
  const leaves: CatalogLeafEntry[] = [];
  const occurrences = new Map<string, number>();
  const build = (list: ModelCatalogNode[], path: CatalogPathSegment[]): CatalogEntry[] =>
    list.map((node) => {
      if (node.kind === "group") {
        const groupPath = [...path, { id: node.id ?? node.label, label: node.label }];
        const children = build(node.children ?? [], groupPath);
        return {
          kind: "group",
          key: groupPath.map((segment) => segment.id).join("/"),
          id: node.id ?? node.label,
          label: node.label,
          ...(node.description ? { description: node.description } : {}),
          path: groupPath,
          children,
          leafCount: children.reduce(
            (total, child) => total + (child.kind === "group" ? child.leafCount : 1),
            0,
          ),
        } satisfies CatalogGroupEntry;
      }
      const pathKey = path.map((segment) => segment.id).join("/");
      const occurrenceKey = `${pathKey}|${node.value ?? ""}`;
      const occurrence = occurrences.get(occurrenceKey) ?? 0;
      occurrences.set(occurrenceKey, occurrence + 1);
      const leaf: CatalogLeafEntry = {
        kind: "choice",
        key: `${occurrenceKey}|${occurrence}`,
        value: node.value ?? "",
        label: node.label,
        ...(node.description ? { description: node.description } : {}),
        path,
      };
      leaves.push(leaf);
      return leaf;
    });
  return { entries: build(nodes, []), leaves };
}

function searchableText(
  leaf: CatalogLeafEntry,
  compositions: ModelCatalogComposition[] | undefined,
): string {
  const parts = [
    leaf.label,
    leaf.value,
    leaf.description ?? "",
    ...leaf.path.map((segment) => segment.label),
  ];
  for (const composition of compositions ?? []) {
    for (const binding of composition.bindings) {
      if (binding.model !== leaf.value) continue;
      for (const dimension of composition.dimensions) {
        parts.push(dimension.label, binding.values[dimension.id] ?? "");
        const choice = dimension.choices.find(
          (entry) => entry.value === binding.values[dimension.id],
        );
        if (choice) parts.push(choice.label);
      }
    }
  }
  return parts.join("\n").toLowerCase();
}

function matchesQuery(
  leaf: CatalogLeafEntry,
  compositions: ModelCatalogComposition[] | undefined,
  query: string,
) {
  const trimmed = query.trim().toLowerCase();
  if (!trimmed) return true;
  return searchableText(leaf, compositions).includes(trimmed);
}

/**
 * Returns the matching leaves in stable catalog order. A match on a dimension
 * label or dimension value resolves through the verified bindings, so a
 * composed family is discoverable by either half of its pairing.
 */
export function searchCatalogLeaves(
  tree: CatalogTree,
  compositions: ModelCatalogComposition[] | undefined,
  query: string,
): CatalogLeafEntry[] {
  return tree.leaves.filter((leaf) => matchesQuery(leaf, compositions, query));
}

/** Restricts a tree to the supplied leaf keys while keeping the original order. */
export function pruneCatalogTree(tree: CatalogTree, allowed: Set<string>): CatalogTreeNodeResult {
  const prune = (entries: CatalogEntry[]): CatalogEntry[] => {
    const out: CatalogEntry[] = [];
    for (const entry of entries) {
      if (entry.kind === "choice") {
        if (allowed.has(entry.key)) out.push(entry);
        continue;
      }
      const children = prune(entry.children);
      if (children.length === 0) continue;
      out.push({
        ...entry,
        children,
        leafCount: children.reduce(
          (total, child) => total + (child.kind === "group" ? child.leafCount : 1),
          0,
        ),
      });
    }
    return out;
  };
  return { entries: prune(tree.entries) };
}

export type CatalogTreeNodeResult = { entries: CatalogEntry[] };

/**
 * Whether a dimension value is advertised and participates in at least one
 * verified binding. Selecting it never traps the user: switching a pairing is
 * allowed, and the dimensions it invalidates stay visible and must be replaced.
 */
export function isDimensionChoiceAvailable(
  composition: ModelCatalogComposition,
  dimensionId: string,
  value: string,
): boolean {
  const dimension = composition.dimensions.find((entry) => entry.id === dimensionId);
  if (!dimension?.choices.some((choice) => choice.value === value)) return false;
  return composition.bindings.some((binding) => binding.values[dimensionId] === value);
}

/** Dimensions whose current value can no longer complete a verified pairing. */
export function invalidDraftDimensions(
  composition: ModelCatalogComposition,
  draft: ModelCatalogDraft,
): string[] {
  const invalid: string[] = [];
  for (const dimension of composition.dimensions) {
    const current = draft[dimension.id];
    if (!current || !hasChoice(dimension, current)) {
      if (current) invalid.push(dimension.id);
      continue;
    }
    const complete = composition.bindings.some((binding) =>
      composition.dimensions.every((other) => {
        const value = draft[other.id];
        if (!value || !hasChoice(other, value)) return true;
        return binding.values[other.id] === value;
      }),
    );
    if (!complete) invalid.push(dimension.id);
  }
  return invalid;
}

/**
 * Full completion check for one composition draft. Only `complete` exposes a
 * model; incomplete, invalid, and ambiguous drafts never apply.
 */
export function composeCatalogSelection(
  composition: ModelCatalogComposition,
  draft: ModelCatalogDraft,
  catalog?: ModelCatalog,
): { status: "complete"; model: string } | { status: "incomplete" | "invalid" | "ambiguous" } {
  const resolution = resolveModelCatalogDraft(composition, draft, catalog);
  return resolution.status === "complete"
    ? { status: "complete", model: resolution.model }
    : { status: resolution.status };
}

/**
 * Human label for the verified pairing behind one advertised model value.
 * Returns undefined when no binding matches, so callers never invent a pairing.
 */
export function composedPairingLabel(
  catalog: ModelCatalog | undefined,
  model: string,
): string | undefined {
  if (!catalog || !model) return undefined;
  const labels = new Set<string>();
  for (const composition of catalog.compositions ?? []) {
    for (const binding of composition.bindings) {
      if (binding.model !== model) continue;
      const parts = composition.dimensions.flatMap((dimension) => {
        const value = binding.values[dimension.id];
        const choice = dimension.choices.find((entry) => entry.value === value);
        return choice ? [choice.label] : [];
      });
      if (parts.length === composition.dimensions.length) labels.add(parts.join(" / "));
    }
  }
  return labels.size === 1 ? [...labels][0] : undefined;
}

/** Starts a draft from the currently selected model's verified binding, if any. */
export function initialCompositionDraft(
  composition: ModelCatalogComposition,
  currentModel: string,
): ModelCatalogDraft {
  const binding = composition.bindings.find((entry) => entry.model === currentModel);
  return binding ? { ...binding.values } : {};
}

export function hasCompositionDimensions(
  composition: ModelCatalogComposition | undefined,
): boolean {
  return Boolean(composition && composition.dimensions.length > 0);
}

/**
 * Whether the group should render as a composed view rather than an ordinary
 * list: only a group with a composition and at least one verified binding.
 */
export function isCompositionGroup(
  catalog: ModelCatalog | undefined,
  groupId: string,
): ModelCatalogComposition | undefined {
  const composition = catalog?.compositions?.find((entry) => entry.group_id === groupId);
  if (!composition || composition.bindings.length === 0) return undefined;
  return composition;
}

export function compositionGroupLabel(composition: ModelCatalogComposition): string {
  return composition.label || composition.id;
}

export function dimensionLabel(dimension: ModelCatalogDimension): string {
  return dimension.label || dimension.id;
}

/** Whether any catalog leaf is currently selected outside a closed group. */
export function currentLeafVisible(leaves: CatalogLeafEntry[], currentModel: string): boolean {
  return leaves.some((leaf) => leaf.value === currentModel);
}

/** Ordered leaf values for a flat provider, preserved for the caller. */
export function catalogLeafValues(catalog: ModelCatalog | undefined): string[] {
  return catalog ? leafNodes(catalog.nodes).map((node) => node.value ?? "") : [];
}

function hasChoice(dimension: ModelCatalogDimension, value: string): boolean {
  return dimension.choices.some((choice) => choice.value === value);
}
