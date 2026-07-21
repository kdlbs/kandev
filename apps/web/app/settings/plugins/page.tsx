"use client";

import { PluginsSettings } from "@/components/settings/plugins/plugins-settings";
import { useFeature } from "@/hooks/domains/features/use-feature";

export default function PluginsSettingsPage() {
  const pluginsEnabled = useFeature("plugins");
  if (!pluginsEnabled) return null;

  return <PluginsSettings />;
}
