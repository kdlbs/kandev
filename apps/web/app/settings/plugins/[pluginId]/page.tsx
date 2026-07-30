"use client";

import { PluginDetail } from "@/components/settings/plugins/plugin-detail";
import { useFeature } from "@/hooks/domains/features/use-feature";

export default function PluginDetailPage({ pluginId }: { pluginId: string }) {
  const pluginsEnabled = useFeature("plugins");
  if (!pluginsEnabled) return null;

  return <PluginDetail pluginId={pluginId} />;
}
