import type {
  EffectiveTheme,
  ThemeBase,
  ThemeDefinition,
  ThemeInventory,
  ThemeSelection,
} from "./theme-types";
export function themeReference(packageId: string, key: string): ThemeSelection {
  return `theme:${packageId}/${key}`;
}
export function isThemeSelection(value: unknown): value is ThemeSelection {
  return (
    typeof value === "string" &&
    (["light", "dark", "system"].includes(value) ||
      /^theme:[a-z0-9][a-z0-9._-]*\/[a-z0-9][a-z0-9-]{0,63}$/.test(value))
  );
}
export function resolveEffectiveTheme(
  selection: ThemeSelection,
  system: ThemeBase,
  inventory: ThemeInventory | null,
  definitions: Record<string, ThemeDefinition>,
  lastBase?: ThemeBase,
): EffectiveTheme {
  const base = selection === "dark" || selection === "light" ? selection : (lastBase ?? system);
  if (!selection.startsWith("theme:"))
    return { reference: selection, base, digest: base, status: "ready" };
  const descriptor = inventory?.themes.find(
    (item) => themeReference(item.package_id, item.key) === selection,
  );
  if (!descriptor)
    return {
      reference: selection,
      base,
      digest: base,
      status: inventory ? "unavailable" : "loading",
    };
  const definition = definitions[descriptor.digest];
  return {
    reference: selection,
    base: descriptor.base,
    digest: definition ? descriptor.digest : descriptor.base,
    definition,
    status: definition ? "ready" : "loading",
  };
}
