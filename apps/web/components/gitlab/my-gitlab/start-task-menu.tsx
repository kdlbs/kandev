"use client";

import type { GitLabTaskPreset } from "./quick-task-launcher";
import { IntegrationStartTaskMenu } from "@/components/integrations/integration-start-task-menu";

export function StartTaskMenu({
  presets,
  onSelect,
}: {
  presets: GitLabTaskPreset[];
  onSelect: (preset: GitLabTaskPreset) => void;
}) {
  return <IntegrationStartTaskMenu presets={presets} onSelect={onSelect} />;
}
