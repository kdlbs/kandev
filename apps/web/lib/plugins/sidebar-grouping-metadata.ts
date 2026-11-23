import type { SidebarGroupingProvider } from "@kdlbs/plugin-sdk";
import { isSidebarPluginGrouping } from "@/lib/sidebar/sidebar-plugin-grouping";

export function readSidebarGroupingProviders(
  value: unknown,
): SidebarGroupingProvider[] | undefined {
  if (!Array.isArray(value)) return undefined;
  const providers: SidebarGroupingProvider[] = [];
  const seen = new Set<string>();
  for (const entry of value) {
    const provider = readSidebarGroupingProvider(entry);
    if (!provider || seen.has(provider.reference)) continue;
    seen.add(provider.reference);
    providers.push(provider);
  }
  return providers;
}

function readSidebarGroupingProvider(value: unknown): SidebarGroupingProvider | undefined {
  if (!value || typeof value !== "object" || Array.isArray(value)) return undefined;
  const { reference, label, labelKey, generation } = value as Record<string, unknown>;
  if (
    typeof reference !== "string" ||
    !isSidebarPluginGrouping(reference) ||
    typeof label !== "string" ||
    !label.trim() ||
    label.length > 256 ||
    typeof generation !== "string" ||
    !generation ||
    generation.length > 256
  )
    return undefined;
  return {
    reference,
    label,
    generation,
    ...(typeof labelKey === "string" && labelKey.length <= 256 ? { labelKey } : {}),
  };
}
