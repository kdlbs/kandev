import {
  MODEL_CATALOG_LIMITS,
  MODEL_CATALOG_VERSION,
  type ModelCatalog,
  type ModelCatalogBinding,
  type ModelCatalogComposition,
  type ModelCatalogDimension,
  type ModelCatalogDraft,
  type ModelCatalogNode,
  type ModelCatalogResolution,
} from "@kandev/types/model-catalog";

/**
 * Validates provider-supplied catalog data against the same bounds the backend
 * publishes. Anything outside those bounds is discarded as a whole so callers
 * keep their validated flat choice list.
 */
export function normalizeModelCatalog(value: unknown): ModelCatalog | undefined {
  if (!isRecord(value)) return undefined;
  if (value.version !== MODEL_CATALOG_VERSION) return undefined;
  const nodes = normalizeNodes(value.nodes);
  if (!nodes) return undefined;
  const compositions = normalizeCompositions(
    value.compositions,
    advertisedValues(nodes),
    groupIds(nodes),
  );
  if (compositions === null) return undefined;
  return compositions
    ? { version: MODEL_CATALOG_VERSION, nodes, compositions }
    : { version: MODEL_CATALOG_VERSION, nodes };
}

function normalizeNodes(value: unknown): ModelCatalogNode[] | undefined {
  if (!Array.isArray(value) || value.length === 0) return undefined;
  const groups = new Set<string>();
  const counter = { count: 0 };
  const nodes = normalizeNodeList(value, 1, groups, counter);
  if (!nodes) return undefined;
  return nodes;
}

function normalizeNodeList(
  value: unknown[],
  depth: number,
  groups: Set<string>,
  counter: { count: number },
): ModelCatalogNode[] | undefined {
  if (depth > MODEL_CATALOG_LIMITS.maxDepth) return undefined;
  const nodes: ModelCatalogNode[] = [];
  for (const item of value) {
    const node = normalizeNode(item, depth, groups, counter);
    if (!node) return undefined;
    nodes.push(node);
  }
  return nodes;
}

function normalizeNode(
  value: unknown,
  depth: number,
  groups: Set<string>,
  counter: { count: number },
): ModelCatalogNode | undefined {
  if (!isRecord(value)) return undefined;
  counter.count += 1;
  if (counter.count > MODEL_CATALOG_LIMITS.maxNodes) return undefined;
  if (!validText(value.label) || !validText(value.description)) return undefined;
  const label = typeof value.label === "string" ? value.label : "";
  if (value.kind === "group") return normalizeGroupNode(value, label, depth, groups, counter);
  if (value.kind === "choice") return normalizeChoiceNode(value, label);
  return undefined;
}

function normalizeGroupNode(
  value: Record<string, unknown>,
  label: string,
  depth: number,
  groups: Set<string>,
  counter: { count: number },
): ModelCatalogNode | undefined {
  const id = typeof value.id === "string" ? value.id : "";
  if (!id || id.length > MODEL_CATALOG_LIMITS.maxIdLength || groups.has(id)) return undefined;
  if (typeof value.value === "string" && value.value !== "") return undefined;
  if (!Array.isArray(value.children) || value.children.length === 0) return undefined;
  groups.add(id);
  const children = normalizeNodeList(value.children, depth + 1, groups, counter);
  if (!children) return undefined;
  return { kind: "group", id, label, ...optionalDescription(value.description), children };
}

function normalizeChoiceNode(
  value: Record<string, unknown>,
  label: string,
): ModelCatalogNode | undefined {
  const choiceValue = typeof value.value === "string" ? value.value : "";
  if (!choiceValue || choiceValue.length > MODEL_CATALOG_LIMITS.maxIdLength) return undefined;
  if (typeof value.id === "string" && value.id !== "") return undefined;
  if (Array.isArray(value.children) && value.children.length > 0) return undefined;
  return { kind: "choice", value: choiceValue, label, ...optionalDescription(value.description) };
}

function advertisedValues(nodes: ModelCatalogNode[]): Set<string> {
  return new Set(leafNodes(nodes).map((node) => node.value ?? ""));
}

function groupIds(nodes: ModelCatalogNode[]): Set<string> {
  const ids = new Set<string>();
  const walk = (list: ModelCatalogNode[]) => {
    for (const node of list) {
      if (node.kind === "group" && node.id) ids.add(node.id);
      if (node.children) walk(node.children);
    }
  };
  walk(nodes);
  return ids;
}

function normalizeCompositions(
  value: unknown,
  advertised: Set<string>,
  groups: Set<string>,
): ModelCatalogComposition[] | undefined | null {
  if (value === undefined) return undefined;
  if (!Array.isArray(value)) return null;
  const seen = new Set<string>();
  const out: ModelCatalogComposition[] = [];
  for (const item of value) {
    const composition = normalizeComposition(item, seen, advertised, groups);
    if (!composition) return null;
    out.push(composition);
  }
  return out;
}

function normalizeComposition(
  value: unknown,
  seen: Set<string>,
  advertised: Set<string>,
  groups: Set<string>,
): ModelCatalogComposition | undefined {
  if (!isRecord(value)) return undefined;
  const id = typeof value.id === "string" ? value.id : "";
  const groupId = typeof value.group_id === "string" ? value.group_id : "";
  if (!id || id.length > MODEL_CATALOG_LIMITS.maxIdLength) return undefined;
  if (!groupId || groupId.length > MODEL_CATALOG_LIMITS.maxIdLength) return undefined;
  if (!groups.has(groupId)) return undefined;
  if (seen.has(id)) return undefined;
  if (!validText(value.label)) return undefined;
  seen.add(id);
  const dimensions = normalizeDimensions(value.dimensions);
  if (!dimensions) return undefined;
  const bindings = normalizeBindings(value.bindings, dimensions, advertised);
  if (!bindings) return undefined;
  return {
    id,
    group_id: groupId,
    ...(typeof value.label === "string" ? { label: value.label } : {}),
    dimensions,
    bindings,
  };
}

function normalizeDimensions(value: unknown): ModelCatalogDimension[] | undefined {
  if (!Array.isArray(value) || value.length === 0) return undefined;
  if (value.length > MODEL_CATALOG_LIMITS.maxDimensions) return undefined;
  const seen = new Set<string>();
  const out: ModelCatalogDimension[] = [];
  for (const item of value) {
    const dimension = normalizeDimension(item, seen);
    if (!dimension) return undefined;
    out.push(dimension);
  }
  return out;
}

function normalizeDimension(item: unknown, seen: Set<string>): ModelCatalogDimension | undefined {
  if (!isRecord(item)) return undefined;
  const id = typeof item.id === "string" ? item.id : "";
  const label = typeof item.label === "string" ? item.label : "";
  if (!id || seen.has(id) || !validText(label)) return undefined;
  seen.add(id);
  const choices = normalizeDimensionChoices(item.choices);
  if (!choices) return undefined;
  return { id, label, choices };
}

function normalizeDimensionChoices(value: unknown): ModelCatalogDimension["choices"] | undefined {
  if (!Array.isArray(value) || value.length === 0) return undefined;
  if (value.length > MODEL_CATALOG_LIMITS.maxDimensionChoices) return undefined;
  const seen = new Set<string>();
  const choices: ModelCatalogDimension["choices"] = [];
  for (const item of value) {
    if (!isRecord(item)) return undefined;
    const choiceValue = typeof item.value === "string" ? item.value : "";
    if (!choiceValue || choiceValue.length > MODEL_CATALOG_LIMITS.maxIdLength) return undefined;
    if (seen.has(choiceValue)) return undefined;
    if (!validText(item.label) || !validText(item.description)) return undefined;
    seen.add(choiceValue);
    choices.push({
      value: choiceValue,
      label: typeof item.label === "string" ? item.label : "",
      ...optionalDescription(item.description),
    });
  }
  return choices;
}

function normalizeBindings(
  value: unknown,
  dimensions: ModelCatalogDimension[],
  advertised: Set<string>,
): ModelCatalogBinding[] | undefined {
  if (!Array.isArray(value) || value.length === 0) return undefined;
  if (value.length > MODEL_CATALOG_LIMITS.maxBindings) return undefined;
  const out: ModelCatalogBinding[] = [];
  const tuples = new Set<string>();
  for (const item of value) {
    const normalized = normalizeBindingEntry(item, dimensions, advertised);
    if (!normalized || tuples.has(normalized.tuple)) return undefined;
    tuples.add(normalized.tuple);
    out.push(normalized.binding);
  }
  return out;
}

function normalizeBindingEntry(
  item: unknown,
  dimensions: ModelCatalogDimension[],
  advertised: Set<string>,
): { binding: ModelCatalogBinding; tuple: string } | undefined {
  if (!isRecord(item) || !isRecord(item.values)) return undefined;
  if (typeof item.model !== "string" || item.model === "" || !advertised.has(item.model)) {
    return undefined;
  }
  // Provider-controlled keys such as "__proto__" must stay ordinary data.
  const values: Record<string, string> = Object.create(null) as Record<string, string>;
  for (const dimension of dimensions) {
    const choice = item.values[dimension.id];
    if (typeof choice !== "string") return undefined;
    if (!dimension.choices.some((entry) => entry.value === choice)) return undefined;
    values[dimension.id] = choice;
  }
  if (Object.keys(item.values).length !== dimensions.length) return undefined;
  const tuple = dimensions
    .map((dimension) => `${dimension.id}=${values[dimension.id]}`)
    .join("\u0000");
  return { binding: { values, model: item.model }, tuple };
}

function optionalDescription(value: unknown): { description?: string } {
  return typeof value === "string" && value !== "" ? { description: value } : {};
}

function validText(value: unknown): boolean {
  return (
    value === undefined ||
    (typeof value === "string" && value.length <= MODEL_CATALOG_LIMITS.maxStringLength)
  );
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return !!value && typeof value === "object" && !Array.isArray(value);
}

/** Whether value is an exact advertised leaf of the catalog. */
export function catalogHasChoice(catalog: ModelCatalog | undefined, value: string): boolean {
  if (!catalog) return false;
  return leafNodes(catalog.nodes).some((node) => node.value === value);
}

export function leafNodes(nodes: ModelCatalogNode[]): ModelCatalogNode[] {
  const out: ModelCatalogNode[] = [];
  for (const node of nodes) {
    if (node.kind === "choice") {
      out.push(node);
    } else if (node.children) {
      out.push(...leafNodes(node.children));
    }
  }
  return out;
}

export function catalogGroup(
  catalog: ModelCatalog | undefined,
  groupId: string,
): ModelCatalogNode | undefined {
  if (!catalog) return undefined;
  return findGroup(catalog.nodes, groupId);
}

function findGroup(nodes: ModelCatalogNode[], groupId: string): ModelCatalogNode | undefined {
  for (const node of nodes) {
    if (node.kind === "group" && node.id === groupId) return node;
    if (!node.children) continue;
    const found = findGroup(node.children, groupId);
    if (found) return found;
  }
  return undefined;
}

export function catalogComposition(
  catalog: ModelCatalog | undefined,
  groupId: string,
): ModelCatalogComposition | undefined {
  return catalog?.compositions?.find((composition) => composition.group_id === groupId);
}

/**
 * Resolves one complete dimension draft to exactly one currently advertised
 * model. Incomplete, invalid, unknown, and ambiguous drafts never resolve, and
 * a model that is not an advertised leaf stays unresolved.
 */
export function resolveModelCatalogDraft(
  composition: ModelCatalogComposition,
  draft: ModelCatalogDraft,
  catalog?: ModelCatalog,
): ModelCatalogResolution {
  const dimensionIds = new Set(composition.dimensions.map((dimension) => dimension.id));
  const unknown = Object.keys(draft).filter((key) => !dimensionIds.has(key));
  if (unknown.length > 0) return { status: "invalid", missing: [], invalid: unknown };
  const missing: string[] = [];
  const invalid: string[] = [];
  for (const dimension of composition.dimensions) {
    const value = draft[dimension.id];
    if (!value) {
      missing.push(dimension.id);
      continue;
    }
    if (!dimension.choices.some((choice) => choice.value === value)) {
      invalid.push(dimension.id);
    }
  }
  if (missing.length > 0) return { status: "incomplete", missing, invalid };
  if (invalid.length > 0) return { status: "invalid", missing, invalid };
  const matches = composition.bindings.filter((binding) =>
    matchesDraft(binding, draft, composition.dimensions),
  );
  if (matches.length > 1) return { status: "ambiguous" };
  if (matches.length === 0) return { status: "invalid", missing, invalid };
  const model = matches[0].model;
  if (catalog && !catalogHasChoice(catalog, model)) {
    return { status: "invalid", missing, invalid };
  }
  return { status: "complete", model };
}

function matchesDraft(
  binding: ModelCatalogBinding,
  draft: ModelCatalogDraft,
  dimensions: ModelCatalogDimension[],
): boolean {
  return dimensions.every((dimension) => binding.values[dimension.id] === draft[dimension.id]);
}

/** Exact advertised model for a fully matched binding, or undefined. */
export function resolvedCatalogModel(
  composition: ModelCatalogComposition,
  draft: ModelCatalogDraft,
): string | undefined {
  const resolution = resolveModelCatalogDraft(composition, draft);
  return resolution.status === "complete" ? resolution.model : undefined;
}
