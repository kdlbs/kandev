"use client";

import { IntegrationScopeBar } from "@/components/integrations/presets-scope-bar-base";
import { PR_PRESETS, ISSUE_PRESETS, type PresetOption } from "./search-bar";
import type { SavedPreset } from "./use-saved-presets";
import type { SidebarSelection } from "./presets-sidebar";
import { useTranslation } from "react-i18next";

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

/** `value` is the persisted kind; only the catalog key is copy. */
const KINDS = [
  { value: "pr", labelKey: "github:titlePullRequests" },
  { value: "issue", labelKey: "github:titleIssues" },
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
  const { t } = useTranslation();
  return (
    <IntegrationScopeBar
      {...props}
      testId="github-presets-scope-bar"
      savedMenuTestId="github-saved-queries-menu"
      kinds={KINDS.map(({ value, labelKey }) => ({ value, label: t(labelKey) }))}
      presetsByKind={(kind) => (kind === "pr" ? prPresets : issuePresets)}
    />
  );
}
