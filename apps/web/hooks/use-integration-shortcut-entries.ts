"use client";

import { useTranslation } from "react-i18next";
import { usePluginRegistry } from "@/lib/plugins/registry";
import { buildIntegrationShortcutEntries } from "@/lib/keyboard/integration-shortcuts";
import type { PluginRecord } from "@/lib/types/plugins";

export function useIntegrationShortcutEntries(plugins: PluginRecord[] = []) {
  const { t } = useTranslation();
  const registry = usePluginRegistry();
  return buildIntegrationShortcutEntries(registry.getNavRegistrations(), plugins, t);
}
