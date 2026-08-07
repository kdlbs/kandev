export type SavedPresetKind = "pr" | "issue";

export type SavedPreset = {
  id: string;
  kind: SavedPresetKind;
  label: string;
  customQuery: string;
  repoFilter: string;
  createdAt: string;
  isDefault: boolean;
};

export type DefaultSidebarTarget = {
  selection: {
    kind: SavedPresetKind;
    source: "preset" | "saved";
    id: string;
  };
  query: string;
  repoFilter: string;
};

export function readSavedPresets(value: unknown): SavedPreset[] | null {
  if (!Array.isArray(value)) return null;
  const defaultKinds = new Set<SavedPresetKind>();
  return value.flatMap((candidate): SavedPreset[] => {
    if (typeof candidate !== "object" || candidate === null) return [];
    const preset = candidate as Record<string, unknown>;
    const kind = preset.kind;
    if (
      typeof preset.id !== "string" ||
      (kind !== "pr" && kind !== "issue") ||
      typeof preset.label !== "string"
    ) {
      return [];
    }
    const isDefault = preset.isDefault === true && !defaultKinds.has(kind);
    if (isDefault) defaultKinds.add(kind);
    return [
      {
        ...(preset as Omit<SavedPreset, "repoFilter" | "isDefault">),
        repoFilter: typeof preset.repoFilter === "string" ? preset.repoFilter : "",
        isDefault,
      },
    ];
  });
}

export function findDefaultSavedPreset(
  presets: SavedPreset[],
  kind: SavedPresetKind,
): SavedPreset | undefined {
  return presets.find((preset) => preset.kind === kind && preset.isDefault);
}

export function setSavedPresetDefault(
  presets: SavedPreset[],
  kind: SavedPresetKind,
  id: string | null,
): SavedPreset[] {
  if (id !== null && !presets.some((preset) => preset.kind === kind && preset.id === id)) {
    return presets;
  }
  let changed = false;
  const next = presets.map((preset) => {
    if (preset.kind !== kind) return preset;
    const isDefault = id !== null && preset.id === id;
    if (preset.isDefault === isDefault) return preset;
    changed = true;
    return { ...preset, isDefault };
  });
  return changed ? next : presets;
}

export function resolveDefaultSidebarTarget(
  kind: SavedPresetKind,
  savedPresets: SavedPreset[],
  configuredPresets: ReadonlyArray<{ value: string; filter: string }>,
): DefaultSidebarTarget {
  const saved = findDefaultSavedPreset(savedPresets, kind);
  if (saved) {
    return {
      selection: { kind, source: "saved", id: saved.id },
      query: saved.customQuery,
      repoFilter: saved.repoFilter,
    };
  }
  const configured = configuredPresets[0];
  return {
    selection: { kind, source: "preset", id: configured?.value ?? "" },
    query: configured?.filter ?? "",
    repoFilter: "",
  };
}
