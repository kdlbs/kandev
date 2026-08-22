"use client";

import { useTranslation } from "react-i18next";
import { IconActivity, IconGitBranch } from "@tabler/icons-react";
import { CardContent, CardHeader, CardTitle } from "@kandev/ui/card";
import { Label } from "@kandev/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@kandev/ui/select";
import { Separator } from "@kandev/ui/separator";
import { GENERAL_SETTINGS_TARGETS } from "@/lib/settings-discovery/catalog/preferences";
import type { AppearanceState } from "./appearance-settings-state";
import { AppStatusBarSettingsCard } from "./app-status-bar-settings-card";
import { SettingsCard } from "./settings-card";
import { SettingsSection } from "./settings-section";
import { SystemMetricsSettingsCard } from "./system-metrics-settings-card";

function ChangesPanelLayoutCard({
  value,
  isDirty,
  onChange,
}: {
  value: "flat" | "tree";
  isDirty: boolean;
  onChange: (value: "flat" | "tree") => void;
}) {
  const { t } = useTranslation();
  return (
    <SettingsCard
      isDirty={isDirty}
      discoveryTargetId={GENERAL_SETTINGS_TARGETS.changesPanelLayout}
      data-testid="changes-panel-layout-card"
    >
      <CardHeader>
        <CardTitle className="text-base">{t("settings:changesPanelLayout")}</CardTitle>
      </CardHeader>
      <CardContent>
        <div className="space-y-2">
          <Label htmlFor="changes-panel-layout">{t("settings:fileListView")}</Label>
          <Select value={value} onValueChange={(next) => onChange(next as "flat" | "tree")}>
            <SelectTrigger
              id="changes-panel-layout"
              data-testid="changes-panel-layout-select"
              data-settings-dirty={isDirty}
              className="cursor-pointer"
            >
              <SelectValue />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="flat">{t("settings:flatList")}</SelectItem>
              <SelectItem value="tree">{t("settings:tree")}</SelectItem>
            </SelectContent>
          </Select>
          <p className="text-xs text-muted-foreground">
            {t("settings:displayChangedFilesAsAFlat")}
          </p>
        </div>
      </CardContent>
    </SettingsCard>
  );
}

export function AppearanceAccountSections({
  draft,
  saved,
  updateDraft,
}: {
  draft: AppearanceState;
  saved: AppearanceState;
  updateDraft: (patch: Partial<AppearanceState>) => void;
}) {
  const { t } = useTranslation();
  return (
    <>
      <Separator />
      <SettingsSection
        icon={<IconActivity className="h-5 w-5" />}
        title={t("settings:statusBar")}
        description={t("settings:configureStatusBarVisibility")}
      >
        <AppStatusBarSettingsCard
          enabled={draft.appStatusBarEnabled}
          isDirty={draft.appStatusBarEnabled !== saved.appStatusBarEnabled}
          onChange={(appStatusBarEnabled) => updateDraft({ appStatusBarEnabled })}
        />
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconGitBranch className="h-5 w-5" />}
        title={t("settings:changesPanel")}
        description={t("settings:customizeHowChangedFilesAreDisplayed")}
      >
        <ChangesPanelLayoutCard
          value={draft.changesPanelLayout}
          isDirty={draft.changesPanelLayout !== saved.changesPanelLayout}
          onChange={(changesPanelLayout) => updateDraft({ changesPanelLayout })}
        />
      </SettingsSection>

      <Separator />

      <SettingsSection
        icon={<IconActivity className="h-5 w-5" />}
        title={t("settings:resourceMetrics")}
        description={t("settings:configureBackendAndExecutionResourceSampling")}
      >
        <SystemMetricsSettingsCard
          showInTopbar={draft.showMetrics}
          isShowInTopbarDirty={draft.showMetrics !== saved.showMetrics}
          onShowInTopbarChange={(showMetrics) => updateDraft({ showMetrics })}
          simplified={draft.simplifiedMetrics}
          isSimplifiedDirty={draft.simplifiedMetrics !== saved.simplifiedMetrics}
          onSimplifiedChange={(simplifiedMetrics) => updateDraft({ simplifiedMetrics })}
        />
      </SettingsSection>
    </>
  );
}
