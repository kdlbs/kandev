export type CopilotPluginSelectionRow = {
  id: string;
  name: string;
  marketplace: string | null;
  version: string | null;
  nativeEnabled: boolean;
  selected: boolean;
  available: boolean;
  /** Present when the backend reported a bounded reason code. */
  reasonCode: string | null;
  /** A saved identity that a successful inventory no longer contains. */
  missing: boolean;
  /** A saved identity whose availability could not be checked on this refresh. */
  availabilityUnchecked: boolean;
};

export type CopilotPluginSelectionInput = {
  plugins: Array<{
    id: string;
    name: string;
    marketplace?: string;
    version?: string;
    native_enabled: boolean;
    availability: "available" | "unavailable";
    reason_code?: string;
  }>;
  status: "loading" | "ready" | "unavailable" | "unsupported";
  stale?: boolean;
};

/**
 * Derive the checkbox rows shown in the profile editor. Saved identities
 * always stay visible: a failed refresh reports "availability not checked",
 * while a successful inventory reports a missing plugin. Neither state clears
 * the draft.
 */
export function buildCopilotPluginSelectionRows(
  discovery: CopilotPluginSelectionInput,
  selectedPluginIds: string[],
): CopilotPluginSelectionRow[] {
  const selected = new Set(selectedPluginIds);
  const discoveredIds = new Set(discovery.plugins.map((plugin) => plugin.id));
  const fresh = discovery.status === "ready" && !discovery.stale;
  const rows: CopilotPluginSelectionRow[] = discovery.plugins.map((plugin) => ({
    id: plugin.id,
    name: plugin.name,
    marketplace: plugin.marketplace?.trim() || null,
    version: plugin.version?.trim() || null,
    nativeEnabled: plugin.native_enabled,
    selected: selected.has(plugin.id),
    available: fresh && plugin.availability === "available",
    reasonCode: fresh ? (plugin.reason_code ?? null) : null,
    missing: false,
    availabilityUnchecked: !fresh && selected.has(plugin.id),
  }));

  for (const id of selectedPluginIds) {
    if (discoveredIds.has(id)) continue;
    rows.push({
      id,
      name: pluginDisplayName(id),
      marketplace: null,
      version: null,
      nativeEnabled: false,
      selected: true,
      available: false,
      reasonCode: null,
      missing: fresh,
      availabilityUnchecked: !fresh,
    });
  }

  return rows.sort((left, right) => {
    if (left.missing !== right.missing) return left.missing ? 1 : -1;
    if (left.availabilityUnchecked !== right.availabilityUnchecked) {
      return left.availabilityUnchecked ? 1 : -1;
    }
    return left.name.localeCompare(right.name) || left.id.localeCompare(right.id);
  });
}

function pluginDisplayName(id: string): string {
  const slash = id.lastIndexOf("/");
  return slash >= 0 ? id.slice(slash + 1) : id;
}
