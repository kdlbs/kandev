/**
 * Provider-neutral model catalog shared by the HTTP DTOs, session snapshots,
 * and the selector. The shape mirrors `internal/common/modelcatalog`: bounded
 * presentation data only, never executable metadata or renderer instructions.
 */

export const MODEL_CATALOG_VERSION = 1;

export const MODEL_CATALOG_LIMITS = {
  maxDepth: 4,
  maxNodes: 4096,
  maxDimensions: 8,
  maxDimensionChoices: 256,
  maxBindings: 4096,
  maxStringLength: 512,
  maxIdLength: 256,
} as const;

export type ModelCatalogNodeKind = "group" | "choice";

export type ModelCatalogNode = {
  kind: ModelCatalogNodeKind;
  /** Stable scoped group identity. Absent on choice nodes. */
  id?: string;
  /** Exact provider value. Present only on choice nodes. */
  value?: string;
  label: string;
  description?: string;
  children?: ModelCatalogNode[];
};

export type ModelCatalogChoice = {
  value: string;
  label: string;
  description?: string;
};

export type ModelCatalogDimension = {
  id: string;
  label: string;
  choices: ModelCatalogChoice[];
};

export type ModelCatalogBinding = {
  values: Record<string, string>;
  model: string;
};

export type ModelCatalogComposition = {
  id: string;
  /** Group that owns this composition. */
  group_id: string;
  label?: string;
  dimensions: ModelCatalogDimension[];
  bindings: ModelCatalogBinding[];
};

export type ModelCatalog = {
  version: number;
  nodes: ModelCatalogNode[];
  compositions?: ModelCatalogComposition[];
};

export type ModelCatalogDraft = Record<string, string>;

export type ModelCatalogResolution =
  | { status: "complete"; model: string }
  | { status: "incomplete" | "invalid"; missing: string[]; invalid: string[] }
  | { status: "ambiguous" };
