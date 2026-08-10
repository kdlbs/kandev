"use client";

import { useCallback, useRef } from "react";
import { useTranslation } from "react-i18next";
import { IntegrationScopeBar } from "@/components/integrations/presets-scope-bar-base";
import { PR_PRESETS, ISSUE_PRESETS, type PresetOption } from "./search-bar";
import type { SavedPreset } from "./saved-preset-model";
import type { SidebarSelection, SidebarSelectionRequest } from "./presets-sidebar";

type PresetsScopeBarProps = {
  className?: string;
  selected: SidebarSelection;
  onSelect: (request: SidebarSelectionRequest) => void;
  savedPresets: SavedPreset[];
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
  onToggleSavedDefault: (preset: SavedPreset) => void;
  defaultMutationPendingId: string | null;
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
  savedPresets,
  onSelect,
  onToggleSavedDefault,
  ...props
}: PresetsScopeBarProps) {
  const { t } = useTranslation();
  const savedPresetsRef = useRef(savedPresets);
  savedPresetsRef.current = savedPresets;
  const handleToggleSavedDefault = useCallback(
    (id: string) => {
      const preset = savedPresetsRef.current.find((candidate) => candidate.id === id);
      if (!preset) {
        if (process.env.NODE_ENV !== "production") {
          console.warn("[github:presets] default toggle target missing", { id });
        }
        return;
      }
      void onToggleSavedDefault(preset);
    },
    [onToggleSavedDefault],
  );
  const handleKindChange = useCallback(
    (kind: SidebarSelection["kind"]) => onSelect({ kind, source: "kind-switch" }),
    [onSelect],
  );
  return (
    <IntegrationScopeBar
      {...props}
      onSelect={onSelect}
      onKindChange={handleKindChange}
      savedPresets={savedPresets}
      onToggleSavedDefault={handleToggleSavedDefault}
      testId="github-presets-scope-bar"
      savedMenuTestId="github-saved-queries-menu"
      kinds={KINDS.map(({ value, labelKey }) => ({ value, label: t(labelKey) }))}
      presetsByKind={(kind) => (kind === "pr" ? prPresets : issuePresets)}
    />
  );
}
