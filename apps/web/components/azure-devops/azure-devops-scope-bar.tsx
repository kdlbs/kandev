"use client";

import {
  IntegrationScopeBar,
  type ScopeSelection,
} from "@/components/integrations/presets-scope-bar-base";
import type { AzureDevOpsSavedView } from "@/lib/types/azure-devops";
import { presetsForKind, type AzureDevOpsPresetKind } from "./azure-devops-presets";

export type AzureDevOpsScopeSelection = ScopeSelection<AzureDevOpsPresetKind>;

const KINDS = [
  { value: "work_item", label: "Work items" },
  { value: "pull_request", label: "Pull requests" },
] as const;

export function AzureDevOpsScopeBar({
  selected,
  onSelect,
  savedViews,
  onDeleteSaved,
  canSaveCurrent,
  onSaveCurrent,
}: {
  selected: AzureDevOpsScopeSelection;
  onSelect: (selection: AzureDevOpsScopeSelection) => void;
  savedViews: AzureDevOpsSavedView[];
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
}) {
  return (
    <IntegrationScopeBar
      testId="azure-devops-presets-scope-bar"
      savedMenuTestId="azure-devops-saved-views-menu"
      kinds={KINDS}
      selected={selected}
      onSelect={onSelect}
      presetsByKind={presetsForKind}
      savedPresets={savedViews}
      onDeleteSaved={onDeleteSaved}
      canSaveCurrent={canSaveCurrent}
      onSaveCurrent={onSaveCurrent}
      className="border-b"
    />
  );
}
