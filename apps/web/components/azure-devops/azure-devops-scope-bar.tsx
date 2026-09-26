"use client";

import {
  IntegrationScopeBar,
  type ScopeSelection,
} from "@/components/integrations/presets-scope-bar-base";
import type { AzureDevOpsSavedView } from "@/lib/types/azure-devops";
import { useTranslation } from "react-i18next";
import {
  presetsForKind,
  type AzureDevOpsPresetKind,
  type AzureDevOpsQueryPresets,
} from "./azure-devops-presets";

export type AzureDevOpsScopeSelection = ScopeSelection<AzureDevOpsPresetKind>;

export function AzureDevOpsScopeBar({
  selected,
  onSelect,
  savedViews,
  onDeleteSaved,
  canSaveCurrent,
  onSaveCurrent,
  queryPresets,
}: {
  selected: AzureDevOpsScopeSelection;
  onSelect: (selection: AzureDevOpsScopeSelection) => void;
  savedViews: AzureDevOpsSavedView[];
  onDeleteSaved: (id: string) => void;
  canSaveCurrent: boolean;
  onSaveCurrent: () => void;
  queryPresets: AzureDevOpsQueryPresets;
}) {
  const { t } = useTranslation();
  const kinds = [
    { value: "work_item" as const, label: t("azuredevops:workItems") },
    { value: "pull_request" as const, label: t("azuredevops:pullRequests") },
  ];
  return (
    <IntegrationScopeBar
      testId="azure-devops-presets-scope-bar"
      savedMenuTestId="azure-devops-saved-views-menu"
      kinds={kinds}
      selected={selected}
      onSelect={onSelect}
      presetsByKind={(kind) => presetsForKind(kind, queryPresets, t)}
      savedPresets={savedViews}
      onDeleteSaved={onDeleteSaved}
      canSaveCurrent={canSaveCurrent}
      onSaveCurrent={onSaveCurrent}
      className="border-b"
    />
  );
}
