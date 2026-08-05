"use client";

import { useMemo } from "react";
import { useTranslation } from "react-i18next";
import { useSettingsDiscovery } from "@/hooks/domains/settings/use-settings-discovery";
import { useSettingsData } from "@/hooks/domains/settings/use-settings-data";
import { useRegisterCommands } from "@/hooks/use-register-commands";
import { useCommandPanelOpen } from "@/lib/commands/command-registry";
import type { CommandItem } from "@/lib/commands/types";
import { useRouter } from "@/lib/routing/client-router";
import { navigateToSettingsDiscovery } from "@/lib/settings-discovery/navigation";
import type { ResolvedSettingsDiscoveryItem } from "@/lib/settings-discovery/types";

type Push = ReturnType<typeof useRouter>["push"];
type LegacyKeywords = Partial<Record<string, string[]>>;

const LEGACY_COMMAND_IDS: Record<string, string> = {
  agents: "settings-agents",
  executors: "settings-executors",
  workspaces: "settings-workspace",
  prompts: "settings-prompts",
};

export function buildSettingsDiscoveryCommands(
  items: ResolvedSettingsDiscoveryItem[],
  push: Push,
  settingsGroupLabel: string,
  legacyKeywords: LegacyKeywords = {},
): CommandItem[] {
  return items.map((item) => ({
    id: LEGACY_COMMAND_IDS[item.id] ?? `setting:${item.id}`,
    label: item.label,
    group: settingsGroupLabel,
    context: [settingsGroupLabel, ...item.breadcrumb].join(" › "),
    keywords: unique([...item.aliases, ...item.breadcrumb, ...(legacyKeywords[item.id] ?? [])]),
    searchOnly: true,
    priority: 80,
    action: () => navigateToSettingsDiscovery(item.href, push),
  }));
}

export function SettingsDiscoveryCommands() {
  const { t } = useTranslation();
  const router = useRouter();
  const { open, mode } = useCommandPanelOpen();
  useSettingsData(open && mode === "commands");
  const items = useSettingsDiscovery();
  const settingsGroupLabel = t("common:commandGroupSettings");
  const commands = useMemo(
    () => buildSettingsDiscoveryCommands(items, router.push, settingsGroupLabel),
    [items, router.push, settingsGroupLabel],
  );

  useRegisterCommands(commands);
  return null;
}

function unique(values: string[]): string[] {
  return [...new Set(values.filter(Boolean))];
}
