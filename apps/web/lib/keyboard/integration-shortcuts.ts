import type { DestinationHref } from "@/lib/navigation/types";
import type { PluginNavRegistration } from "@/lib/plugins/registry";
import type { PluginRecord } from "@/lib/types/plugins";
import type { KeyboardShortcut } from "./constants";
import { APP_DESTINATIONS } from "@/lib/navigation/core-destinations";
import { pluginDestinationId, pluginDestinations } from "@/lib/navigation/plugin-destinations";
import { WORKSPACE_INTEGRATIONS } from "@/lib/settings-discovery/catalog/integrations";
import { UNBOUND_SHORTCUT } from "./shortcut-overrides";

export type IntegrationShortcutEntry = {
  source: "integration";
  id: string;
  label: string;
  default: KeyboardShortcut;
  href: DestinationHref;
};

export function buildIntegrationShortcutEntries(
  items: PluginNavRegistration[] = [],
  plugins: PluginRecord[] = [],
  translate: (key: string, options: { integration: string }) => string = (_key, options) =>
    options.integration,
): IntegrationShortcutEntry[] {
  const builtIn = WORKSPACE_INTEGRATIONS.map(([slug, label]): IntegrationShortcutEntry => {
    const destination = APP_DESTINATIONS.find((entry) => entry.id === slug);
    const href =
      destination?.href ?? (slug === "sentry" ? "/settings/integrations/sentry" : undefined);
    // i18n-exempt: developer invariant for catalog completeness.
    if (!href) throw new Error(`Missing integration destination: ${slug}`);
    return {
      source: "integration",
      id: `integration:${slug}`,
      label: translate("settings:shortcutOpenIntegration", { integration: label }),
      default: UNBOUND_SHORTCUT,
      href,
    };
  });
  const pluginNames = new Map(plugins.map((plugin) => [plugin.id, plugin.display_name]));
  const labels = new Map(
    items.map((item) => [
      pluginDestinationId(item.pluginId, item.id),
      `${pluginNames.get(item.pluginId) ?? item.pluginId}: ${item.label}`,
    ]),
  );
  const dynamic = pluginDestinations(items)
    .filter((entry) => entry.section === "integrations")
    .map(
      (entry): IntegrationShortcutEntry => ({
        source: "integration",
        id: `integration:${entry.id}`,
        label: translate("settings:shortcutOpenIntegration", {
          integration: labels.get(entry.id) ?? entry.id,
        }),
        default: UNBOUND_SHORTCUT,
        href: entry.href,
      }),
    );
  return [...builtIn, ...dynamic];
}

export function isValidIntegrationShortcut(value: unknown): value is KeyboardShortcut {
  if (!value || typeof value !== "object" || !("key" in value) || typeof value.key !== "string")
    return false;
  if (!("modifiers" in value) || value.modifiers === undefined) return true;
  const modifiers = value.modifiers;
  return Boolean(
    modifiers &&
    typeof modifiers === "object" &&
    !Array.isArray(modifiers) &&
    Object.entries(modifiers).every(
      ([name, enabled]) =>
        ["ctrl", "cmd", "ctrlOrCmd", "alt", "shift"].includes(name) && typeof enabled === "boolean",
    ),
  );
}
