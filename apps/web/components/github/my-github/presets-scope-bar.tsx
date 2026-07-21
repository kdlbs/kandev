"use client";

import { IntegrationScopeBar } from "@/components/integrations/presets-scope-bar-base";
import { PR_PRESETS, ISSUE_PRESETS, type PresetOption } from "./search-bar";
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
  prPresets?: PresetOption[];
  issuePresets?: PresetOption[];
};

const KINDS = [
  { value: "pr", label: "Pull requests" },
  { value: "issue", label: "Issues" },
] as const;

/**
 * Horizontal scope bar for the /github dashboard (desktop). Thin wrapper over
 * the shared {@link IntegrationScopeBar}; mobile keeps the vertical
 * PresetsSidebar in a sheet.
 */
export function PresetsScopeBar({
  prPresets = PR_PRESETS,
  issuePresets = ISSUE_PRESETS,
  ...props
}: PresetsScopeBarProps) {
  return (
    <IntegrationScopeBar
      {...props}
      testId="github-presets-scope-bar"
      savedMenuTestId="github-saved-queries-menu"
      kinds={KINDS}
      presetsByKind={(kind) => (kind === "pr" ? prPresets : issuePresets)}
    />
  );
}
