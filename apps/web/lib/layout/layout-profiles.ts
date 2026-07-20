import {
  getPresetLayout,
  normalizeReusableSessionPanels,
  PANEL_REGISTRY,
  REUSABLE_PANEL_IDS,
  type BuiltInPreset,
  type LayoutGroup,
  type LayoutNode,
  type LayoutPanel,
  type LayoutState,
} from "@/lib/state/layout-manager";
import type { SavedLayout } from "@/lib/types/http";

export type BuiltInLayoutProfileId = Exclude<BuiltInPreset, "compact">;

export type BuiltInLayoutProfileDescriptor = {
  id: BuiltInLayoutProfileId;
  name: string;
  description: string;
};

export type BuiltInLayoutProfile = BuiltInLayoutProfileDescriptor & {
  layout: LayoutState;
};

export const BUILT_IN_LAYOUT_PROFILES: readonly BuiltInLayoutProfileDescriptor[] = [
  {
    id: "default",
    name: "Default",
    description: "Agent with Files, Changes, and Terminal",
  },
  {
    id: "plan",
    name: "Plan Mode",
    description: "Agent and Plan side by side",
  },
  {
    id: "preview",
    name: "Preview Mode",
    description: "Agent and Browser side by side",
  },
  {
    id: "vscode",
    name: "VS Code",
    description: "Agent and VS Code side by side",
  },
];

export type ReusableLayoutIssueCode =
  | "invalid-layout"
  | "empty-group"
  | "missing-agent"
  | "duplicate-panel"
  | "unsupported-panel"
  | "invalid-panel"
  | "invalid-active-panel";

export type ReusableLayoutIssue = {
  code: ReusableLayoutIssueCode;
  path: string;
  message: string;
};

export type ReusableLayoutValidation =
  | { valid: true; layout: LayoutState; issues: [] }
  | { valid: false; issues: ReusableLayoutIssue[] };

export type LayoutProfileCompatibility =
  | { status: "editable"; profile: SavedLayout; layout: LayoutState; issues: [] }
  | { status: "legacy"; profile: SavedLayout; issues: ReusableLayoutIssue[] };

export type EffectiveDefaultLayout =
  | { source: "built-in"; profile: BuiltInLayoutProfile; layout: LayoutState }
  | { source: "custom"; profile: SavedLayout; layout: LayoutState };

export type CreateLayoutProfileInput = {
  id: string;
  name: string;
  layout: Record<string, unknown>;
  createdAt: string;
  isDefault?: boolean;
};

export type DuplicateLayoutProfileInput = {
  id: string;
  name: string;
  createdAt: string;
};

export type UpsertBuiltInLayoutOverrideOptions = {
  createdAt: string;
  isDefault?: boolean;
};

const REUSABLE_PANEL_ID_SET = new Set<string>(REUSABLE_PANEL_IDS);
const BUILT_IN_OVERRIDE_PREFIX = "layout-override-";

export function createLayoutProfileId(): string {
  return `layout-${globalThis.crypto.randomUUID()}`;
}

export function getBuiltInLayoutProfile(id: BuiltInLayoutProfileId): BuiltInLayoutProfile {
  const descriptor = BUILT_IN_LAYOUT_PROFILES.find((profile) => profile.id === id);
  if (!descriptor) throw new Error(`Unknown built-in layout profile: ${id}`);
  return { ...descriptor, layout: getPresetLayout(id) };
}

export function getBuiltInLayoutOverrideId(id: BuiltInLayoutProfileId): string {
  return `${BUILT_IN_OVERRIDE_PREFIX}${id}`;
}

export function getBuiltInLayoutOverrideSourceId(
  profile: Pick<SavedLayout, "id">,
): BuiltInLayoutProfileId | null {
  return (
    BUILT_IN_LAYOUT_PROFILES.find(({ id }) => profile.id === getBuiltInLayoutOverrideId(id))?.id ??
    null
  );
}

export function isBuiltInLayoutOverride(profile: Pick<SavedLayout, "id">): boolean {
  return getBuiltInLayoutOverrideSourceId(profile) !== null;
}

export function getBuiltInLayoutOverride(
  profiles: SavedLayout[],
  id: BuiltInLayoutProfileId,
): SavedLayout | null {
  const overrideId = getBuiltInLayoutOverrideId(id);
  return profiles.find((profile) => profile.id === overrideId) ?? null;
}

function isRecord(value: unknown): value is Record<string, unknown> {
  return typeof value === "object" && value !== null && !Array.isArray(value);
}

function hasOptionalNumber(record: Record<string, unknown>, key: string): boolean {
  return record[key] === undefined || typeof record[key] === "number";
}

function isPanelShape(value: unknown): value is LayoutPanel {
  if (!isRecord(value)) return false;
  return (
    typeof value.id === "string" &&
    typeof value.component === "string" &&
    typeof value.title === "string" &&
    (value.tabComponent === undefined || typeof value.tabComponent === "string") &&
    (value.params === undefined || isRecord(value.params))
  );
}

function isGroupShape(value: unknown): value is LayoutGroup {
  if (!isRecord(value) || !Array.isArray(value.panels) || !value.panels.every(isPanelShape)) {
    return false;
  }
  return (
    (value.id === undefined || typeof value.id === "string") &&
    (value.activePanel === undefined || typeof value.activePanel === "string")
  );
}

function isTreeShape(value: unknown, seen: Set<unknown>): value is LayoutNode {
  if (!isRecord(value) || seen.has(value)) return false;
  seen.add(value);
  if (!hasOptionalNumber(value, "size")) return false;
  if (value.type === "leaf") return isGroupShape(value.group);
  return (
    value.type === "branch" &&
    Array.isArray(value.children) &&
    value.children.length > 0 &&
    value.children.every((child) => isTreeShape(child, seen))
  );
}

function isLayoutState(value: unknown): value is LayoutState {
  if (!isRecord(value) || !Array.isArray(value.columns) || value.columns.length === 0) return false;
  return value.columns.every((column) => {
    if (!isRecord(column) || typeof column.id !== "string" || !Array.isArray(column.groups)) {
      return false;
    }
    return (
      column.groups.length > 0 &&
      column.groups.every(isGroupShape) &&
      (column.pinned === undefined || typeof column.pinned === "boolean") &&
      hasOptionalNumber(column, "width") &&
      hasOptionalNumber(column, "maxWidth") &&
      hasOptionalNumber(column, "minWidth") &&
      (column.tree === undefined || isTreeShape(column.tree, new Set()))
    );
  });
}

function issue(code: ReusableLayoutIssueCode, path: string, message: string): ReusableLayoutIssue {
  return { code, path, message };
}

function groupsInNode(node: LayoutNode, path: string): Array<{ group: LayoutGroup; path: string }> {
  if (node.type === "leaf") return [{ group: node.group, path: `${path}.group` }];
  return node.children.flatMap((child, index) => groupsInNode(child, `${path}.children[${index}]`));
}

function groupsIn(layout: LayoutState): Array<{ group: LayoutGroup; path: string }> {
  return layout.columns.flatMap((column, columnIndex) => {
    if (column.tree) return groupsInNode(column.tree, `columns[${columnIndex}].tree`);
    return column.groups.map((group, groupIndex) => ({
      group,
      path: `columns[${columnIndex}].groups[${groupIndex}]`,
    }));
  });
}

function validateGroup(
  group: LayoutGroup,
  path: string,
  counts: Map<string, number>,
): ReusableLayoutIssue[] {
  if (group.panels.length === 0) {
    return [issue("empty-group", path, "Layout groups must contain at least one panel")];
  }

  const issues: ReusableLayoutIssue[] = [];
  if (group.activePanel && !group.panels.some((current) => current.id === group.activePanel)) {
    issues.push(
      issue("invalid-active-panel", `${path}.activePanel`, "Active panel must belong to its group"),
    );
  }
  group.panels.forEach((current, panelIndex) => {
    const panelPath = `${path}.panels[${panelIndex}]`;
    if (!REUSABLE_PANEL_ID_SET.has(current.id)) {
      issues.push(issue("unsupported-panel", panelPath, `Panel ${current.id} is not reusable`));
      return;
    }
    const expected = PANEL_REGISTRY[current.id];
    if (!expected || current.component !== expected.component) {
      issues.push(
        issue("invalid-panel", panelPath, `Panel ${current.id} has an invalid component`),
      );
      return;
    }
    counts.set(current.id, (counts.get(current.id) ?? 0) + 1);
  });
  return issues;
}

function canonicalAgentCount(layout: LayoutState): number {
  return groupsIn(layout).reduce(
    (count, { group }) => count + group.panels.filter((current) => current.id === "chat").length,
    0,
  );
}

function invalidActivePanelIssues(layout: LayoutState): ReusableLayoutIssue[] {
  return groupsIn(layout).flatMap(({ group, path }) =>
    group.activePanel && !group.panels.some((current) => current.id === group.activePanel)
      ? [
          issue(
            "invalid-active-panel",
            `${path}.activePanel`,
            "Active panel must belong to its group",
          ),
        ]
      : [],
  );
}

export function validateReusableLayout(layout: unknown): ReusableLayoutValidation {
  if (!isLayoutState(layout)) {
    return {
      valid: false,
      issues: [issue("invalid-layout", "layout", "Layout payload is not readable")],
    };
  }
  if (canonicalAgentCount(layout) > 1) {
    return {
      valid: false,
      issues: [issue("duplicate-panel", "layout", "Panel chat can only appear once")],
    };
  }

  const normalized = normalizeReusableSessionPanels(layout);
  const counts = new Map<string, number>();
  const issues = invalidActivePanelIssues(layout);
  issues.push(
    ...groupsIn(normalized).flatMap(({ group, path }) => validateGroup(group, path, counts)),
  );
  for (const [panelId, count] of counts) {
    if (count > 1) {
      issues.push(issue("duplicate-panel", "layout", `Panel ${panelId} can only appear once`));
    }
  }
  if ((counts.get("chat") ?? 0) !== 1) {
    issues.push(issue("missing-agent", "layout", "Layout must contain exactly one Agent panel"));
  }
  return issues.length === 0
    ? { valid: true, layout: normalized, issues: [] }
    : { valid: false, issues };
}

export function getLayoutProfileCompatibility(profile: SavedLayout): LayoutProfileCompatibility {
  const validation = validateReusableLayout(profile.layout);
  return validation.valid
    ? { status: "editable", profile, layout: validation.layout, issues: [] }
    : { status: "legacy", profile, issues: validation.issues };
}

export function resolveEffectiveDefaultLayout(profiles: SavedLayout[]): EffectiveDefaultLayout {
  const profile = profiles.find((candidate) => candidate.is_default);
  if (profile) {
    const layout = Array.isArray(profile.layout.columns)
      ? {
          ...profile.layout,
          columns: profile.layout.columns.filter(
            (column) => !isRecord(column) || column.id !== "sidebar",
          ),
        }
      : profile.layout;
    const validation = validateReusableLayout(layout);
    if (validation.valid) {
      return { source: "custom", profile, layout: validation.layout };
    }
  }
  const builtIn = getBuiltInLayoutProfile("default");
  return { source: "built-in", profile: builtIn, layout: builtIn.layout };
}

function requireId(id: string): string {
  const trimmed = id.trim();
  if (!trimmed) throw new Error("Layout profile ID must not be empty");
  return trimmed;
}

function requireName(name: string): string {
  const trimmed = name.trim();
  if (!trimmed) throw new Error("Layout profile name must not be empty");
  return trimmed;
}

function ensureUniqueId(profiles: SavedLayout[], id: string): void {
  if (profiles.some((profile) => profile.id === id)) {
    throw new Error("Layout profile ID must be unique");
  }
}

function cloneLayout(layout: Record<string, unknown>): Record<string, unknown> {
  return structuredClone(layout);
}

export function createLayoutProfile(
  profiles: SavedLayout[],
  input: CreateLayoutProfileInput,
): SavedLayout[] {
  const id = requireId(input.id);
  const name = requireName(input.name);
  ensureUniqueId(profiles, id);
  if (input.isDefault && !validateReusableLayout(input.layout).valid) {
    throw new Error("Only a valid reusable layout can be the default");
  }
  const existing = input.isDefault
    ? profiles.map((profile) => (profile.is_default ? { ...profile, is_default: false } : profile))
    : [...profiles];
  return [
    ...existing,
    {
      id,
      name,
      is_default: input.isDefault ?? false,
      layout: cloneLayout(input.layout),
      created_at: input.createdAt,
    },
  ];
}

export function upsertBuiltInLayoutOverride(
  profiles: SavedLayout[],
  id: BuiltInLayoutProfileId,
  layout: Record<string, unknown>,
  options: UpsertBuiltInLayoutOverrideOptions,
): SavedLayout[] {
  const validation = validateReusableLayout(layout);
  if (!validation.valid) {
    throw new Error("Only a valid reusable layout can override a built-in layout");
  }
  const normalizedLayout = validation.layout as unknown as Record<string, unknown>;
  const builtIn = getBuiltInLayoutProfile(id);
  const overrideId = getBuiltInLayoutOverrideId(id);
  const existing = getBuiltInLayoutOverride(profiles, id);
  const isDefault = options.isDefault ?? existing?.is_default ?? false;
  if (!existing) {
    return createLayoutProfile(profiles, {
      id: overrideId,
      name: builtIn.name,
      layout: normalizedLayout,
      createdAt: options.createdAt,
      isDefault,
    });
  }
  return profiles.map((profile) => {
    if (profile.id === overrideId) {
      return {
        ...profile,
        name: builtIn.name,
        layout: cloneLayout(normalizedLayout),
        is_default: isDefault,
      };
    }
    return isDefault && profile.is_default ? { ...profile, is_default: false } : profile;
  });
}

export function resetBuiltInLayoutOverride(
  profiles: SavedLayout[],
  id: BuiltInLayoutProfileId,
): SavedLayout[] {
  const overrideId = getBuiltInLayoutOverrideId(id);
  return profiles.filter((profile) => profile.id !== overrideId);
}

function findProfile(profiles: SavedLayout[], profileId: string): SavedLayout {
  const profile = profiles.find((candidate) => candidate.id === profileId);
  if (!profile) throw new Error(`Layout profile not found: ${profileId}`);
  return profile;
}

export function renameLayoutProfile(
  profiles: SavedLayout[],
  profileId: string,
  name: string,
): SavedLayout[] {
  findProfile(profiles, profileId);
  const trimmed = requireName(name);
  return profiles.map((profile) =>
    profile.id === profileId ? { ...profile, name: trimmed } : profile,
  );
}

export function duplicateLayoutProfile(
  profiles: SavedLayout[],
  source: string | Pick<SavedLayout, "layout"> | BuiltInLayoutProfile,
  input: DuplicateLayoutProfileInput,
): SavedLayout[] {
  const original = typeof source === "string" ? findProfile(profiles, source) : source;
  return createLayoutProfile(profiles, {
    ...input,
    layout: original.layout,
    isDefault: false,
  });
}

export function deleteLayoutProfile(profiles: SavedLayout[], profileId: string): SavedLayout[] {
  findProfile(profiles, profileId);
  return profiles.filter((profile) => profile.id !== profileId);
}

export function setDefaultLayoutProfile(
  profiles: SavedLayout[],
  profileId: string | null,
): SavedLayout[] {
  if (profileId) {
    const selected = findProfile(profiles, profileId);
    if (!validateReusableLayout(selected.layout).valid) {
      throw new Error("Only a valid reusable layout can be the default");
    }
  }
  return profiles.map((profile) => {
    const isDefault = profile.id === profileId;
    return profile.is_default === isDefault ? profile : { ...profile, is_default: isDefault };
  });
}
