"use client";

import { use } from "react";
import { PluginDetail } from "@/components/settings/plugins/plugin-detail";
import { useFeature } from "@/hooks/domains/features/use-feature";

export default function PluginDetailPage({ params }: { params: Promise<{ pluginId: string }> }) {
  const { pluginId } = use(params);
  const pluginsEnabled = useFeature("plugins");
  if (!pluginsEnabled) return null;

  return <PluginDetail pluginId={pluginId} />;
}
