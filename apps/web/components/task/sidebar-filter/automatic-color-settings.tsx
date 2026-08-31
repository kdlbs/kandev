"use client";

import { IconPalette, IconPlus } from "@tabler/icons-react";
import { Button } from "@kandev/ui/button";
import { Switch } from "@kandev/ui/switch";
import { useTranslation } from "react-i18next";
import { cn } from "@/lib/utils";
import { MAX_AUTOMATIC_TASK_COLOR_RULES } from "@/lib/task-color-automation-settings";
import { SidebarSettingsDisclosure } from "./sidebar-settings-disclosure";
import { AutomaticColorRuleList } from "./automatic-color-rule-list";
import { RepositoryPickerPane } from "./automatic-color-repository-picker";
import {
  useAutomaticColorSettingsController,
  type AutomaticColorSettingsController,
} from "./use-automatic-color-settings-controller";

type Props = {
  isDrawerLayout: boolean;
};

export function AutomaticColorSettings({ isDrawerLayout }: Props) {
  const { t } = useTranslation();
  const controller = useAutomaticColorSettingsController();
  const enabledRuleCount = controller.value.rules.filter((rule) => rule.enabled).length;
  const summary = controller.value.enabled
    ? t("task:automaticColorsSummaryRules", { count: enabledRuleCount })
    : t("task:automaticColorsSummaryOff");

  return (
    <SidebarSettingsDisclosure
      title={
        <span className="flex items-center gap-1.5">
          <IconPalette className="size-3.5" aria-hidden="true" />
          {t("task:automaticColors")}
        </span>
      }
      summary={summary}
      testId="automatic-color-settings"
      expanded={controller.expanded}
      onExpandedChange={controller.setExpanded}
      className={cn("border-t", isDrawerLayout && "pt-2")}
      contentClassName="space-y-2 pt-1"
    >
      <AutomaticColorSettingsBody controller={controller} isDrawerLayout={isDrawerLayout} t={t} />
    </SidebarSettingsDisclosure>
  );
}

function AutomaticColorSettingsBody({
  controller,
  isDrawerLayout,
  t,
}: {
  controller: AutomaticColorSettingsController;
  isDrawerLayout: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const activeRule = controller.activeRepositoryRule;
  const catalog = controller.repositoryCatalog;

  return (
    <>
      <div className="rounded-md border border-border/60 p-2" aria-busy={controller.saving}>
        <div className="flex min-h-11 items-center gap-2">
          <div className="min-w-0 flex-1">
            <span className="block text-xs font-medium">{t("task:automaticColorsEnabled")}</span>
            <span className="block text-[11px] text-muted-foreground">
              {t("task:automaticColorsPersonalGlobal")}
            </span>
          </div>
          <label className="flex size-11 shrink-0 cursor-pointer items-center justify-center">
            <Switch
              checked={controller.value.enabled}
              onCheckedChange={(enabled) => controller.updateSettings(undefined, enabled)}
              aria-label={t("task:automaticColorsEnabled")}
              data-testid="automatic-color-enabled"
            />
          </label>
        </div>
        <p className="px-1 pt-1 text-[11px] text-muted-foreground">
          {t("task:automaticColorsPrecedence")}
        </p>
      </div>

      {controller.error && (
        <p
          role="alert"
          className="px-1 text-xs text-destructive"
          data-testid="automatic-color-error"
        >
          {controller.error}
        </p>
      )}

      {isDrawerLayout && activeRule ? (
        <RepositoryPickerPane
          options={catalog.options}
          query={controller.repositoryQuery}
          loading={catalog.loading}
          error={catalog.error}
          onQueryChange={controller.setRepositorySearch}
          onRefresh={catalog.refresh}
          onBack={controller.closeRepository}
          onSelect={(option) => {
            controller.updateRule(activeRule.id, {
              ...activeRule,
              condition: {
                ...activeRule.condition,
                value: option.target,
                label: option.label,
              },
            });
            controller.closeRepository();
          }}
        />
      ) : (
        <AutomaticColorRules controller={controller} isDrawerLayout={isDrawerLayout} t={t} />
      )}
    </>
  );
}

function AutomaticColorRules({
  controller,
  isDrawerLayout,
  t,
}: {
  controller: AutomaticColorSettingsController;
  isDrawerLayout: boolean;
  t: (key: string, options?: Record<string, unknown>) => string;
}) {
  const catalog = controller.repositoryCatalog;
  return (
    <>
      {controller.value.rules.length > 0 && (
        <AutomaticColorRuleList
          rules={controller.value.rules}
          scalarOptions={controller.scalarOptions}
          repositoryOptions={catalog.options}
          repositoryQuery={controller.repositoryQuery}
          repositoryLoading={catalog.loading}
          repositoryError={catalog.error}
          onRepositoryQueryChange={controller.setRepositorySearch}
          onRefreshRepositories={catalog.refresh}
          isDrawerLayout={isDrawerLayout}
          onChange={controller.updateRule}
          onRemove={controller.removeRule}
          onOpenRepository={controller.openRepository}
          onReorder={controller.reorderRules}
          t={t}
        />
      )}
      <div className="flex items-center justify-between gap-2">
        <span className="px-1 text-[11px] text-muted-foreground">
          {t("task:automaticColorsRuleCount", { count: controller.value.rules.length })}
        </span>
        <Button
          type="button"
          variant="outline"
          size="sm"
          className="min-h-11 cursor-pointer text-xs md:min-h-0 md:h-7"
          onClick={controller.addRule}
          disabled={
            controller.value.rules.length >= MAX_AUTOMATIC_TASK_COLOR_RULES || controller.saving
          }
          data-testid="automatic-color-add-rule"
        >
          <IconPlus className="mr-1 size-3.5" aria-hidden="true" />
          {t("task:automaticColorsAddRule")}
        </Button>
      </div>
      {controller.value.rules.length >= MAX_AUTOMATIC_TASK_COLOR_RULES && (
        <p className="px-1 text-[11px] text-muted-foreground">
          {t("task:automaticColorsRuleLimit")}
        </p>
      )}
    </>
  );
}
