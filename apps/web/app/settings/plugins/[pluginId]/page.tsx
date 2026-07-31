"use client";

import { PluginDetail } from "@/components/settings/plugins/plugin-detail";

export default function PluginDetailPage({ pluginId }: { pluginId: string }) {
  return <PluginDetail pluginId={pluginId} />;
}
