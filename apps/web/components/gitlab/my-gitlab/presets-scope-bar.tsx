"use client";

import { IntegrationScopeBar } from "@/components/integrations/presets-scope-bar-base";
import { MR_PRESETS, ISSUE_PRESETS, type PresetOption } from "./presets";
import type { SavedPreset } from "./use-saved-presets";
import type { SidebarSelection } from "./presets-sidebar";

type PresetsScopeBarProps = {
  className?: string;
  selected: SidebarSelection;
  onSelect: (s: SidebarSelection) => void;
  savedPresets: SavedPreset[];
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
  mrPresets?: PresetOption[];
  issuePresets?: PresetOption[];
};

const KINDS = [
  { value: "mr", label: "Merge requests" },
  { value: "issue", label: "Issues" },
] as const;

/**
 * Horizontal scope bar for the /gitlab dashboard (desktop). Thin wrapper over
 * the shared {@link IntegrationScopeBar}; mobile keeps the vertical
 * PresetsSidebar in a sheet.
 */
export function PresetsScopeBar({
  mrPresets = MR_PRESETS,
  issuePresets = ISSUE_PRESETS,
  ...props
}: PresetsScopeBarProps) {
  return (
    <IntegrationScopeBar
      {...props}
      testId="gitlab-presets-scope-bar"
      savedMenuTestId="gitlab-saved-queries-menu"
      kinds={KINDS}
      presetsByKind={(kind) => (kind === "mr" ? mrPresets : issuePresets)}
    />
  );
}
